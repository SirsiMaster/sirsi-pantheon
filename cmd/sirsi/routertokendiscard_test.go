package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

// discardFixture is a same-host token replacement in flight: an old token and a fresh one, each with a live
// session minted through the real service handler (so token_id is stamped from the authenticated bearer), plus an
// unattributed legacy session on the same host. Synthetic values only.
type discardFixture struct {
	store              *routerstore.SQLiteStore
	oldTok, newTok     routerstore.HostToken
	oldSess, newSess   string
	legacySess         string
	secretResource     string
	failingPublish     func(context.Context, string, string) (string, error)
	discard, revokeAll func(string) error
}

func newDiscardFixture(t *testing.T, scoped bool) *discardFixture {
	t.Helper()
	s, err := routerstore.OpenPath(filepath.Join(t.TempDir(), "router.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	h, err := routerstore.Handler(s, routerstore.ServerOptions{Token: "synthetic-bootstrap"})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	f := &discardFixture{store: s, secretResource: "projects/210890802638/secrets/router-m1",
		failingPublish: func(context.Context, string, string) (string, error) { return "", errors.New("publish refused") },
		discard:        s.DiscardMintedToken, revokeAll: s.RevokeHostToken}
	var oldPlain, newPlain string
	if oldPlain, f.oldTok, err = s.MintHostToken("m1", "old"); err != nil {
		t.Fatal(err)
	}
	if scoped {
		newPlain, f.newTok, err = s.MintScopedHostToken("m1", "new", []string{"lane-new"})
	} else {
		newPlain, f.newTok, err = s.MintHostToken("m1", "new")
	}
	if err != nil {
		t.Fatal(err)
	}
	mint := func(token, agent string) string {
		body, _ := json.Marshal(map[string]any{"args": []any{"m1", agent, "rt"}})
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/call/MintSession", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		var out struct {
			Result []routerstore.Session `json:"result"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || resp.StatusCode != 200 || len(out.Result) != 1 {
			t.Fatalf("mint for %s: status %d err %v", agent, resp.StatusCode, err)
		}
		return out.Result[0].ID
	}
	f.oldSess = mint(oldPlain, "lane-old")
	f.newSess = mint(newPlain, "lane-new")
	legacy, err := s.MintSession("m1", "lane-legacy", "rt")
	if err != nil {
		t.Fatal(err)
	}
	f.legacySess = legacy.ID
	return f
}

func (f *discardFixture) live(id string) bool { _, err := f.store.GetSession(id); return err == nil }

func TestFailedPublishDiscardKeepsOldTokenSessionsAndKillsNewOnes(t *testing.T) {
	for _, scoped := range []bool{false, true} {
		name := "host-wide"
		if scoped {
			name = "scoped"
		}
		t.Run(name, func(t *testing.T) {
			f := newDiscardFixture(t, scoped)
			var out bytes.Buffer
			var err error
			if scoped {
				err = publishMintedRouterToken(context.Background(), &out, "sentinel-secret", f.newTok, f.secretResource, f.failingPublish, f.discard)
			} else {
				err = publishMintedHostWideToken(context.Background(), &out, "sentinel-secret", f.newTok, f.secretResource, f.failingPublish, f.discard)
			}
			if err == nil || !strings.Contains(err.Error(), "fresh token revoked") || strings.Contains(err.Error()+out.String(), "sentinel-secret") {
				t.Fatalf("failed publish must report the revoked fresh token without the credential: %v", err)
			}
			if !f.live(f.oldSess) || !f.live(f.legacySess) {
				t.Fatal("old-token and legacy sessions must survive a failed replacement")
			}
			if f.live(f.newSess) {
				t.Fatal("the fresh token's session must be revoked")
			}
		})
	}
}

// Negative control: the previous callback (RevokeHostToken) on the same failed publish signs out the old token's
// session. If this stops failing the way it asserts, the test above proves nothing.
func TestFailedPublishWithOldRevokeCallbackKillsOldTokenSessions(t *testing.T) {
	f := newDiscardFixture(t, false)
	var out bytes.Buffer
	if err := publishMintedHostWideToken(context.Background(), &out, "sentinel-secret", f.newTok, f.secretResource, f.failingPublish, f.revokeAll); err == nil {
		t.Fatal("expected the publish failure to be reported")
	}
	if f.live(f.oldSess) || f.live(f.legacySess) {
		t.Fatal("negative control no longer reproduces the defect: the old callback should have revoked every host session")
	}
}

// A cleanup that itself fails is reported as CLEANUP_DEBT naming the token id, never swallowed.
func TestFailedCleanupIsReportedAsDebt(t *testing.T) {
	f := newDiscardFixture(t, false)
	rec := routerstore.HostToken{ID: "no-such-token", Host: "m1"}
	var out bytes.Buffer
	err := publishMintedHostWideToken(context.Background(), &out, "sentinel-secret", rec, f.secretResource, f.failingPublish, f.discard)
	if err == nil || !strings.Contains(err.Error(), "CLEANUP_DEBT") || !strings.Contains(err.Error(), "no-such-token") || strings.Contains(err.Error(), "sentinel-secret") {
		t.Fatalf("cleanup failure must be reported as debt: %v", err)
	}
	if !f.live(f.oldSess) || !f.live(f.newSess) {
		t.Fatal("a failed cleanup of an unknown id must change nothing")
	}
}

// The mint command must hand DiscardMintedToken, not RevokeHostToken, to both publish paths. (The command itself
// needs the cloud metadata server to reach the publish step, so the wiring is pinned at the source.)
func TestMintCommandWiresDiscardMintedTokenAsPublishCleanup(t *testing.T) {
	src, err := os.ReadFile("routerservecmd.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	if n := strings.Count(text, "publishScopedRouterSecret, store.DiscardMintedToken)"); n != 2 {
		t.Fatalf("expected both publish paths to use store.DiscardMintedToken, found %d", n)
	}
	if strings.Contains(text, "publishScopedRouterSecret, store.RevokeHostToken") {
		t.Fatal("a publish path still uses RevokeHostToken as its cleanup")
	}
}
