package routerstore

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestMaintenanceDirectCallerDoesNotUseHeldSpoolOrSessionCache(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SIRSI_AGENT_ID", "ambient")
	t.Setenv("SIRSI_THREAD_ID", "ambient-thread")
	spool := t.TempDir()
	t.Setenv("SIRSI_ROUTER_URL", "spool://"+spool)
	// A pending transaction is intentionally not bypassed; this separately
	// configured selected-service HTTP client must never enter the spool.
	marker := filepath.Join(spool, ".sirsi-root-maintenance.json")
	if err := os.WriteFile(marker, []byte("owned pending marker"), 0600); err != nil {
		t.Fatal(err)
	}
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); w.WriteHeader(http.StatusUnauthorized) }))
	defer server.Close()
	client, err := NewMemoryOnlyRemoteStoreForCaller(server.URL, "owned-token", "selected-actor", "selected-thread", "")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if client.agent != "selected-actor" || client.threadID != "selected-thread" || client.sessionDir != "" {
		t.Fatal("caller or cache binding lost")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := client.GetThreadRecordContext(ctx, "selected-thread"); err == nil {
		t.Fatal("unauthenticated direct authority accepted")
	}
	if requests.Load() != 1 {
		t.Fatalf("direct authorization requests%d want1", requests.Load())
	}
	if entries, err := os.ReadDir(spool); err != nil || len(entries) != 1 {
		t.Fatal("held spool was accessed/published", err)
	}
	if _, err := os.Stat(filepath.Join(os.Getenv("HOME"), ".sirsi", "sessions")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("derived credential cache touched", err)
	}
	before := requests.Load()
	cancel()
	if _, err := client.GetThreadRecordContext(ctx, "selected-thread"); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation was not retained", err)
	}
	if requests.Load() != before {
		t.Fatal("canceled authorization reached service")
	}
}

func TestMaintenanceDirectOwnThreadRequiresRealServiceSession(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SIRSI_ROUTER_URL", "spool:///owned/not-entered")
	store, err := OpenPath(filepath.Join(t.TempDir(), "owned-authority.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	handler, err := Handler(store, ServerOptions{Token: "owned-master-token", RuleOfRa: "enforce"})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	client, err := NewMemoryOnlyRemoteStoreForCaller(server.URL, "owned-master-token", "owned-maintainer", "owned-runtime", "")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	record := ThreadRecord{ThreadID: "owned-runtime", Agent: "owned-maintainer", Host: client.host, Status: "active", LastSeenAt: time.Now().UTC().Format(time.RFC3339Nano), Payload: []byte("{}")}
	if err := store.UpsertThreads([]ThreadRecord{record}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	got, err := client.GetThreadRecordContext(ctx, record.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ThreadID != record.ThreadID || got.Agent != record.Agent || client.session.ID == "" || client.session.ThreadID != record.ThreadID || client.session.Agent != record.Agent {
		t.Fatal("real own-thread session/audience not preserved")
	}
	if _, err := client.GetThreadRecordContext(ctx, "missing-runtime"); !errors.Is(err, ErrThreadUnknown) {
		t.Fatal("unknown thread semantics changed", err)
	}
	if client.sessionDir != "" {
		t.Fatal("derived session persisted")
	}
}
