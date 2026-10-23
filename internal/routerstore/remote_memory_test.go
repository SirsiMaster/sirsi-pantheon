package routerstore

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestMemoryOnlyOperatorApprovalNeverUsesDiskSessions(t *testing.T) {
	for _, existing := range []bool{false, true} {
		name := "absent-cache"
		if existing {
			name = "poisoned-existing-cache"
		}
		t.Run(name, func(t *testing.T) {
			s, manifest, _, _ := noticeFixture(t)
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("SIRSI_AGENT_ID", "sirsi-software-admin")
			t.Setenv("SIRSI_THREAD_ID", "real-fixture-thread")
			t.Setenv(EnvUseMachineIDHost, "")
			t.Setenv(EnvCloudHost, "")
			host, err := os.Hostname()
			if err != nil {
				t.Fatal(err)
			}
			if err := s.UpsertThreads([]ThreadRecord{{ThreadID: "real-fixture-thread", Agent: "sirsi-software-admin", Host: host, Status: "active", LastSeenAt: s.clock().Format(time.RFC3339Nano), Payload: json.RawMessage(`{"surface":"codex"}`)}}); err != nil {
				t.Fatal(err)
			}
			handler, err := Handler(s, ServerOptions{Token: "test-only-bootstrap", RuleOfRa: "off", RecipientRegistry: deliveryRegistry(t)})
			if err != nil {
				t.Fatal(err)
			}
			var minted, poisoned atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.Contains(r.URL.Path, "MintSession") {
					minted.Add(1)
				}
				if r.Header.Get("X-Sirsi-Session") == "must-never-load" {
					poisoned.Add(1)
				}
				handler.ServeHTTP(w, r)
			}))
			defer server.Close()
			ordinary := NewRemoteStore(server.URL, "test-only-bootstrap")
			cacheDir, cachePath := ordinary.sessionDir, ordinary.sessionPaths()[0]
			oldPath := filepath.Join(cacheDir, "retained-old.json")
			var original []byte
			if existing {
				if err := os.MkdirAll(cacheDir, 0700); err != nil {
					t.Fatal(err)
				}
				original, err = json.Marshal(Session{ID: "must-never-load", Secret: "not-a-real-secret", RuntimeHash: ordinary.runtime, ThreadID: ordinary.threadID})
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(cachePath, original, 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(oldPath, []byte(`{"runtime_hash":"keep-history"}`), 0600); err != nil {
					t.Fatal(err)
				}
				old := time.Now().Add(-30 * 24 * time.Hour)
				if err := os.Chtimes(oldPath, old, old); err != nil {
					t.Fatal(err)
				}
			}
			client := NewMemoryOnlyRemoteStore(server.URL, "test-only-bootstrap")
			defer func() { _ = client.Close() }()
			result, err := client.ReconcileMailboxNotice(context.Background(), MailboxReconcileRequest{Action: "approve", Manifest: &manifest})
			if err != nil || result.ManifestID != manifest.ID {
				t.Fatalf("real SDK approval failed: %+v %v", result, err)
			}
			if minted.Load() != 1 || poisoned.Load() != 0 {
				t.Fatalf("cache read or mint mismatch: minted=%d poisoned=%d", minted.Load(), poisoned.Load())
			}
			client.dropSession()
			if !existing {
				if _, err := os.Stat(cacheDir); !os.IsNotExist(err) {
					t.Fatalf("session directory created: %v", err)
				}
			} else {
				retained, err := os.ReadFile(cachePath)
				if err != nil || string(retained) != string(original) {
					t.Fatal("existing session was changed/deleted")
				}
				if _, err := os.Stat(oldPath); err != nil {
					t.Fatal("existing cache pruned", err)
				}
				entries, err := os.ReadDir(cacheDir)
				if err != nil || len(entries) != 2 {
					t.Fatal("new cache file written", err)
				}
			}
		})
	}
}
