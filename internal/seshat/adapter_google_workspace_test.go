package seshat

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGoogleWorkspaceDriveQueryUsesStandardsEncoding(t *testing.T) {
	var observedQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		observedQuery = r.URL.Query().Get("q")
		if got := r.URL.Query().Get("orderBy"); got != "modifiedTime desc" {
			t.Fatalf("orderBy=%q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"files":[]}`))
	}))
	defer server.Close()

	adapter := &GoogleWorkspaceAdapter{DriveBaseURL: server.URL}
	since := time.Date(2026, 8, 22, 1, 2, 3, 0, time.UTC)
	_, err := adapter.listDriveFiles(&googleToken{AccessToken: "test-token"}, "application/vnd.google-apps.document", since)
	if err != nil {
		t.Fatal(err)
	}
	want := "mimeType='application/vnd.google-apps.document' and modifiedTime>'2026-08-22T01:02:03Z' and trashed=false"
	if observedQuery != want {
		t.Fatalf("q=%q, want %q", observedQuery, want)
	}
}

func TestGoogleWorkspaceDriveErrorsAreNotEmptySuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "malformed", http.StatusBadRequest)
	}))
	defer server.Close()

	adapter := &GoogleWorkspaceAdapter{DriveBaseURL: server.URL}
	token := &googleToken{AccessToken: "test-token"}
	_, docsErr := adapter.listDriveFiles(token, "application/vnd.google-apps.document", time.Now())
	_, sheetsErr := adapter.listDriveFiles(token, "application/vnd.google-apps.spreadsheet", time.Now())
	if docsErr == nil || sheetsErr == nil {
		t.Fatalf("expected both list failures, docs=%v sheets=%v", docsErr, sheetsErr)
	}
}
