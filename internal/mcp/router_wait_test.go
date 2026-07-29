package mcp

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/dispatch"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

func TestHandleRouterWait_MissingAgent(t *testing.T) {
	res, err := handleRouterWait(map[string]interface{}{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil || !res.IsError {
		t.Fatalf("expected an error result when 'agent' is missing, got %+v", res)
	}
}

// newTestFacade builds the Phase-3 dispatch facade over a fresh temp root, so
// the wait path can be exercised without the real repo or home store.
func newTestFacade(t *testing.T) *dispatch.Facade {
	t.Helper()
	store, err := routerstore.Open(filepath.Join(t.TempDir(), "router.db"))
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "idea-router")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "agents.json"), []byte(`{"agents":{"claude-pantheon":{},"codex-pantheon":{}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	f := dispatch.New(root, store)
	t.Cleanup(func() { _ = f.Close() })
	return f
}

func TestWait_TimesOutCleanly(t *testing.T) {
	f := newTestFacade(t)
	start := time.Now()
	items, err := f.Wait(context.Background(), "claude", 200*time.Millisecond)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected empty inbox, got %d items", len(items))
	}
	// Must return near the deadline, never hang.
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("Wait blocked too long on an empty inbox: %v", elapsed)
	}
}

func TestWait_FacadeSendWakesWaiter(t *testing.T) {
	f := newTestFacade(t)
	woke := make(chan int, 1)
	go func() {
		items, err := f.Wait(context.Background(), "claude-pantheon", 5*time.Second)
		if err != nil {
			t.Errorf("Wait: %v", err)
		}
		woke <- len(items)
	}()
	time.Sleep(50 * time.Millisecond) // let the waiter block
	if _, err := f.Send("codex-pantheon", "claude-pantheon", "wake up", "", "x"); err != nil {
		t.Fatal(err)
	}
	select {
	case n := <-woke:
		if n != 1 {
			t.Fatalf("expected 1 item on wake, got %d", n)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("facade send never woke the waiter")
	}
}
