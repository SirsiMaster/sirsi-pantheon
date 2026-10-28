package mailops

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

func fixtureGmailClient(t *testing.T, server *httptest.Server) *gmailClient {
	t.Helper()
	t.Setenv("GOOGLE_API_USE_CLIENT_CERTIFICATE", "false")
	svc, err := gmail.NewService(context.Background(), option.WithHTTPClient(server.Client()), option.WithEndpoint(server.URL+"/"))
	if err != nil {
		t.Fatal(err)
	}
	return &gmailClient{svc: svc}
}

func TestGmailClientListsPagesAndPropagatesHTTPFailure(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/gmail/v1/users/me/messages" || r.URL.Query().Get("q") != "in:inbox" {
			t.Errorf("unexpected list request: %s", r.URL.String())
		}
		if requests == 1 {
			if r.URL.Query().Get("pageToken") != "" {
				t.Errorf("first page unexpectedly had token: %s", r.URL.String())
			}
			_, _ = fmt.Fprint(w, `{"messages":[{"id":"a"}],"nextPageToken":"p2"}`)
			return
		}
		if requests == 2 {
			if r.URL.Query().Get("pageToken") != "p2" {
				t.Errorf("second page token missing: %s", r.URL.String())
			}
			_, _ = fmt.Fprint(w, `{"messages":[{"id":"b"}]}`)
			return
		}
		http.Error(w, "fixture unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	c := fixtureGmailClient(t, server)
	ids, err := c.ListInboxIDs("in:inbox")
	if err != nil || !reflect.DeepEqual(ids, []string{"a", "b"}) {
		t.Fatalf("paged IDs = %v, %v", ids, err)
	}
	if _, err := c.ListInboxIDs("in:inbox"); err == nil || !strings.Contains(err.Error(), "list messages") {
		t.Fatalf("HTTP failure = %v, want wrapped list error", err)
	}
}

func TestGmailClientMetadataAndSenderAuthority(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/gmail/v1/users/me/messages/headers" && r.URL.Query().Get("format") == "metadata":
			if got := strings.Join(r.URL.Query()["metadataHeaders"], ","); got != "From,Subject,Date" {
				t.Errorf("metadata authority headers = %q", got)
			}
			_, _ = fmt.Fprint(w, `{"payload":{"headers":[{"name":"From","value":"A <a@example.test>"},{"name":"Subject","value":"fixture"},{"name":"Date","value":"today"}]}}`)
		case r.URL.Path == "/gmail/v1/users/me/messages/empty" && r.URL.Query().Get("format") == "full":
			_, _ = fmt.Fprint(w, `{"payload":{"mimeType":"multipart/mixed","parts":[{"mimeType":"text/plain","body":{"size":0}}]}}`)
		case r.URL.Path == "/gmail/v1/users/me/messages/no-payload":
			_, _ = fmt.Fprint(w, `{}`)
		case r.URL.Path == "/gmail/v1/users/me/messages":
			if r.URL.Query().Get("q") != "in:inbox newer_than:365d" {
				t.Errorf("sender query = %q", r.URL.Query().Get("q"))
			}
			_, _ = fmt.Fprint(w, `{"messages":[{"id":"sender1"},{"id":"sender2"},{"id":"unknown"}]}`)
		case strings.HasPrefix(r.URL.Path, "/gmail/v1/users/me/messages/"):
			if r.URL.Query().Get("format") != "metadata" {
				t.Errorf("sender metadata format = %q", r.URL.Query().Get("format"))
			}
			switch strings.TrimPrefix(r.URL.Path, "/gmail/v1/users/me/messages/") {
			case "sender1":
				_, _ = fmt.Fprint(w, `{"payload":{"headers":[{"name":"From","value":"News <news@example.test>"},{"name":"List-Unsubscribe","value":"<mailto:leave@example.test>"}]}}`)
			case "sender2":
				_, _ = fmt.Fprint(w, `{"payload":{"headers":[{"name":"From","value":"news@example.test"}]}}`)
			default:
				_, _ = fmt.Fprint(w, `{}`)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	c := fixtureGmailClient(t, server)
	poison, err := c.HasEmptyTextPart([]string{"empty", "no-payload"})
	if err != nil || !reflect.DeepEqual(poison, []string{"empty"}) {
		t.Fatalf("empty text IDs = %v, %v", poison, err)
	}
	headers, err := c.Headers([]string{"headers"})
	if err != nil || headers["headers"] != (Message{ID: "headers", From: "A <a@example.test>", Subject: "fixture", Date: "today"}) {
		t.Fatalf("headers = %+v, %v", headers, err)
	}
	counts, unsub, err := c.SendersLastYear()
	if err != nil || counts["news@example.test"] != 2 || counts["?"] != 1 || !unsub["news@example.test"] {
		t.Fatalf("sender census = %v %v %v", counts, unsub, err)
	}
}

func TestGmailClientDiscardsPartialScanOnReadError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/first") {
			_, _ = fmt.Fprint(w, `{"payload":{"mimeType":"multipart/mixed","parts":[{"mimeType":"text/plain","body":{"size":0}}]}}`)
			return
		}
		http.Error(w, "fixture read failure", http.StatusBadGateway)
	}))
	defer server.Close()
	c := fixtureGmailClient(t, server)
	if ids, err := c.HasEmptyTextPart([]string{"first", "second"}); err == nil || ids != nil {
		t.Fatalf("partial poison scan leaked results: %v, %v", ids, err)
	}
}

func TestGmailClientReturnsNoPartialScanAndUsesOnlyArchiveLabelRemoval(t *testing.T) {
	ids := make([]string, 2001)
	for i := range ids {
		ids[i] = fmt.Sprintf("id-%d", i)
	}
	var archiveCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/gmail/v1/users/me/messages/") && strings.HasSuffix(r.URL.Path, "/batchModify"):
			archiveCount++
			var body gmail.BatchModifyMessagesRequest
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			start := (archiveCount - 1) * 1000
			end := start + 1000
			if end > len(ids) {
				end = len(ids)
			}
			if !reflect.DeepEqual(body.Ids, ids[start:end]) || !reflect.DeepEqual(body.RemoveLabelIds, []string{"INBOX"}) || len(body.AddLabelIds) != 0 {
				t.Errorf("archive request %d exceeded its exact ID/label authority: %+v", archiveCount, body)
			}
			if archiveCount == 2 {
				http.Error(w, "fixture rejected", http.StatusConflict)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	c := fixtureGmailClient(t, server)
	if err := c.Archive(ids); err == nil || !strings.Contains(err.Error(), "archive batch") {
		t.Fatalf("second-batch archive failure = %v", err)
	}
	if archiveCount != 2 {
		t.Fatalf("archive calls = %d, want stop after failing second batch", archiveCount)
	}
}

func TestAuthorizeWritesRestrictedTokenAndKeepsFailureInert(t *testing.T) {
	dir := t.TempDir()
	secret := filepath.Join(dir, "client.json")
	if err := os.WriteFile(secret, []byte(`{"installed":{"client_id":"fixture-id","client_secret":"fixture-secret","auth_uri":"https://auth.invalid","token_uri":"https://token.invalid","redirect_uris":["http://localhost"]}}`), 0600); err != nil {
		t.Fatal(err)
	}
	tokenPath := filepath.Join(dir, "token.json")
	called := false
	err := Authorize(context.Background(), secret, tokenPath, func(cfg *oauth2.Config) (*oauth2.Token, error) {
		called = true
		if cfg.ClientID != "fixture-id" || !reflect.DeepEqual(cfg.Scopes, []string{gmail.GmailModifyScope, gmail.GmailSettingsBasicScope}) || strings.Contains(strings.Join(cfg.Scopes, " "), "send") {
			t.Fatalf("unexpected OAuth authority: %+v", cfg)
		}
		return &oauth2.Token{AccessToken: "access-fixture", RefreshToken: "refresh-fixture", Expiry: time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)}, nil
	})
	if err != nil || !called {
		t.Fatalf("Authorize = %v, called=%v", err, called)
	}
	info, err := os.Stat(tokenPath)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("token file mode = %v, %v", info, err)
	}
	var stored storedToken
	data, _ := os.ReadFile(tokenPath)
	if err := json.Unmarshal(data, &stored); err != nil || stored.RefreshToken != "refresh-fixture" || stored.ClientSecret != "fixture-secret" || !reflect.DeepEqual(stored.Scopes, []string{gmail.GmailModifyScope, gmail.GmailSettingsBasicScope}) {
		t.Fatalf("stored token = %+v, %v", stored, err)
	}
	failedPath := filepath.Join(dir, "not-written.json")
	if err := Authorize(context.Background(), secret, failedPath, func(*oauth2.Config) (*oauth2.Token, error) { return nil, errors.New("fixture consent denied") }); err == nil {
		t.Fatal("consent failure was swallowed")
	}
	if _, err := os.Stat(failedPath); !os.IsNotExist(err) {
		t.Fatalf("failed authorization left token artifact: %v", err)
	}
	if err := Authorize(context.Background(), filepath.Join(dir, "missing.json"), failedPath, nil); err == nil {
		t.Fatal("missing client secret accepted")
	}
}

func TestReceiptLogRecordsAndReportsFilesystemFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "receipts.jsonl")
	log := &ReceiptLog{path: path}
	if err := log.RecordArchive("poison", []string{"message-fixture"}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("receipt mode = %v, %v", info, err)
	}
	data, err := os.ReadFile(path)
	var got Receipt
	if err != nil || json.Unmarshal(data, &got) != nil || got.Reason != "poison" || !reflect.DeepEqual(got.IDs, []string{"message-fixture"}) || got.Timestamp.IsZero() {
		t.Fatalf("receipt = %+v, read error=%v", got, err)
	}
	if err := (&ReceiptLog{path: t.TempDir()}).RecordArchive("poison", []string{"x"}); err == nil {
		t.Fatal("directory open failure was swallowed")
	}
}

func TestAccountsLookupAndConfigurationFailures(t *testing.T) {
	if _, err := LoadAccounts(filepath.Join(t.TempDir(), "absent.yaml")); err == nil {
		t.Fatal("missing account registry accepted")
	}
	bad := filepath.Join(t.TempDir(), "bad.yaml")
	if err := os.WriteFile(bad, []byte("accounts: ["), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadAccounts(bad); err == nil {
		t.Fatal("malformed account registry accepted")
	}
	if account, err := Find([]Account{{Label: "known"}}, "known"); err != nil || account.Label != "known" {
		t.Fatalf("known account = %+v, %v", account, err)
	}
	if _, err := Find([]Account{{Label: "known"}}, "unknown"); err == nil {
		t.Fatal("unregistered account accepted")
	}
}
