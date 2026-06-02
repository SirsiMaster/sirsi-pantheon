package router

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestDaemonDryRunDispatchesPendingInbox(t *testing.T) {
	r, tmp := setupTestRouter(t)
	id, err := r.SubmitAddressed(DocReview, "claude", "Needs Codex", "# Review: Needs Codex\n\nreviewer: claude", "codex")
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	ctx, cancel := context.WithCancel(context.Background())
	d := NewDaemon(r, DaemonOptions{
		RepoRoot: tmp,
		DryRun:   true,
		Interval: time.Hour,
		Debounce: time.Millisecond,
		Out:      &buf,
	})
	cancel()
	if err := d.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), id) {
		t.Fatalf("daemon dry-run output missing pending id: %s", buf.String())
	}
}

func TestDaemonFSNotifyDispatchesStateChange(t *testing.T) {
	r, tmp := setupTestRouter(t)

	calls := make(chan string, 2)
	notify := func(target, docType, docID, repoRoot string) error {
		calls <- docID
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	daemonDone := make(chan struct{})
	t.Cleanup(func() { cancel(); <-daemonDone })

	d := NewDaemon(r, DaemonOptions{
		RepoRoot:    tmp,
		Interval:    time.Hour,
		Debounce:    10 * time.Millisecond,
		Out:         &bytes.Buffer{},
		Notify:      notify,
		LedgerPath:  "",
		UseFSNotify: true,
	})
	go func() {
		defer close(daemonDone)
		_ = d.Run(ctx)
	}()

	// Give fsnotify time to register the watcher before writing state.json
	time.Sleep(200 * time.Millisecond)

	id, err := r.SubmitAddressed(DocReview, "claude", "Needs Codex", "# Review: Needs Codex\n\nreviewer: claude", "codex")
	if err != nil {
		t.Fatal(err)
	}

	select {
	case got := <-calls:
		if got != id {
			t.Fatalf("dispatched %s, want %s", got, id)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("daemon did not dispatch after router state change")
	}
}

func TestDaemonDebounceCollapsesRepeatedWrites(t *testing.T) {
	r, tmp := setupTestRouter(t)

	// Use a channel so notify calls are race-free.
	calls := make(chan string, 10)
	notify := func(target, docType, docID, repoRoot string) error {
		calls <- docID
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	daemonDone := make(chan struct{})
	t.Cleanup(func() { cancel(); <-daemonDone })

	d := NewDaemon(r, DaemonOptions{
		RepoRoot:    tmp,
		Interval:    time.Hour, // disable polling
		Debounce:    200 * time.Millisecond,
		Out:         &bytes.Buffer{},
		Notify:      notify,
		UseFSNotify: true,
	})
	go func() {
		defer close(daemonDone)
		_ = d.Run(ctx)
	}()

	// Let daemon start and finish its initial Tick (empty inbox).
	time.Sleep(100 * time.Millisecond)

	// Rapid-fire three submissions with unique titles within the debounce window.
	// Different titles produce different doc IDs (the slug is part of the ID).
	titles := []string{"Burst Alpha", "Burst Beta", "Burst Gamma"}
	for _, title := range titles {
		r.SubmitAddressed(DocProposal, "claude", title, "# Proposal: "+title+"\n\ncontent", "codex")
		time.Sleep(20 * time.Millisecond)
	}

	// Wait for debounce to fire (200ms after last write) + margin.
	time.Sleep(500 * time.Millisecond)

	// Drain the channel and count. The debounce should have collapsed the
	// fsnotify events into one Tick pass that dispatched all 3 items.
	count := 0
	for {
		select {
		case <-calls:
			count++
		default:
			goto done
		}
	}
done:
	if count != 3 {
		t.Fatalf("notify calls = %d, want 3 (all dispatched in collapsed debounce pass)", count)
	}
}

func TestDaemonPollingFallbackWhenFSNotifyDisabled(t *testing.T) {
	r, tmp := setupTestRouter(t)
	r.SubmitAddressed(DocReview, "claude", "For Codex", "content", "codex")

	// Use a synchronized buffer to avoid races between daemon writes and test reads.
	var mu sync.Mutex
	var buf bytes.Buffer
	ctx, cancel := context.WithCancel(context.Background())
	daemonDone := make(chan struct{})

	d := NewDaemon(r, DaemonOptions{
		RepoRoot:    tmp,
		Interval:    50 * time.Millisecond,
		DryRun:      true,
		Out:         &syncWriter{mu: &mu, w: &buf},
		UseFSNotify: false,
	})
	go func() {
		defer close(daemonDone)
		_ = d.Run(ctx)
	}()
	time.Sleep(200 * time.Millisecond)
	cancel()
	<-daemonDone // wait for the daemon goroutine to fully exit before the test returns + t.TempDir cleanup runs

	mu.Lock()
	output := buf.String()
	mu.Unlock()

	if !strings.Contains(output, "dry-run") {
		t.Fatalf("polling fallback did not dispatch: %s", output)
	}
}

// syncWriter wraps an io.Writer with a mutex for race-free concurrent writes.
type syncWriter struct {
	mu *sync.Mutex
	w  *bytes.Buffer
}

func (s *syncWriter) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w.Write(p)
}
