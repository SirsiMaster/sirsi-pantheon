package routerstore

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// These fixtures use owned HTTP handlers and synthetic files only. A forward
// crossing cancellation must retain custody until its normal outcome settles;
// cancellation must neither replay it nor erase the late pending request.
func TestRelayDrainWaitsForSettledForward(t *testing.T) {
	for _, heldOutbox := range []bool{false, true} {
		name := "fresh"
		if heldOutbox {
			name = "outbox"
		}
		t.Run(name, func(t *testing.T) {
			rl, spool, entered, release := newHeldDrainRelay(t, heldOutbox)
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan error, 1)
			var controlDuties sync.WaitGroup
			go func() {
				defer close(done)
				if os.Getenv("SIRSI_TEST_RELAY_FAST_CANCEL_CONTROL") == "1" {
					// Use the exact fast-return serving path, retaining a fixture-only
					// join for teardown. That join is NOT used by the assertion.
					done <- rl.serve(ctx, &controlDuties)
					return
				}
				done <- rl.ServeDrained(ctx, time.Second)
			}()
			t.Cleanup(cancel)
			t.Cleanup(func() {
				cancel()
				release()
				select {
				case <-done:
				case <-time.After(2 * time.Second):
					t.Error("owned fixture did not exit")
				}
				joined := make(chan struct{})
				go func() { controlDuties.Wait(); close(joined) }()
				select {
				case <-joined:
				case <-time.After(2 * time.Second):
					t.Error("causal control retained active fixture duties")
				}
			})
			awaitDrainEntry(t, entered)
			cancel()
			publishDrainRequest(t, spool, "req", "late", "late")
			select {
			case err := <-done:
				t.Fatalf("acknowledged drain while the forward was held: %v", err)
			case <-time.After(40 * time.Millisecond):
			}
			release()
			select {
			case err := <-done:
				if err != nil {
					t.Fatalf("settled drain: %v", err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("settled forward did not finish drain")
			}
			assertDrainRetainedPending(t, spool)
			if heldOutbox {
				if _, err := os.Stat(filepath.Join(spool, "lane", "outbox", "held.json")); !os.IsNotExist(err) {
					t.Fatalf("positively delivered held item was not settled: %v", err)
				}
				if _, err := os.Stat(filepath.Join(spool, "lane", "outbox", "tail.json")); err != nil {
					t.Fatalf("shutdown consumed the next held request: %v", err)
				}
			} else if _, err := os.Stat(filepath.Join(spool, "lane", "res", "held.json")); err != nil {
				t.Fatalf("fresh response not retained before acknowledgment: %v", err)
			}
			if files, _ := filepath.Glob(filepath.Join(spool, "lane", "inflight", "*.json")); len(files) != 0 {
				t.Fatalf("inflight custody remains after successful drain: %v", files)
			}
		})
	}
}

func TestRelayDrainTimeoutIsNotSuccessOrCancellationOfForward(t *testing.T) {
	rl, spool, entered, release := newHeldDrainRelay(t, false)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	done := make(chan error, 1)
	go func() { done <- rl.ServeDrained(ctx, 30*time.Millisecond) }()
	awaitDrainEntry(t, entered)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, ErrRelayDrainIncomplete) {
			t.Fatalf("held duty must refuse drain, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("drain deadline was not bounded")
	}
	if _, err := os.Stat(filepath.Join(spool, "lane", "inflight", "held.json")); err != nil {
		t.Fatalf("timeout lost the original in-flight request: %v", err)
	}
	release()
	deadline := time.Now().Add(time.Second)
	for {
		_, responseErr := os.Stat(filepath.Join(spool, "lane", "res", "held.json"))
		_, inflightErr := os.Stat(filepath.Join(spool, "lane", "inflight", "held.json"))
		if responseErr == nil && os.IsNotExist(inflightErr) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("owned forward did not settle after the timeout")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestRelayFastCancellationDoesNotProveDrain(t *testing.T) {
	rl, spool, entered, release := newHeldDrainRelay(t, false)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	done := make(chan error, 1)
	go func() { done <- rl.Serve(ctx) }()
	awaitDrainEntry(t, entered)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(300 * time.Millisecond):
		t.Fatal("legacy fast cancellation contract changed")
	}
	if _, err := os.Stat(filepath.Join(spool, "lane", "inflight", "held.json")); err != nil {
		t.Fatalf("control did not retain the outstanding duty: %v", err)
	}
	release()
	deadline := time.Now().Add(time.Second)
	for {
		if _, err := os.Stat(filepath.Join(spool, "lane", "inflight", "held.json")); os.IsNotExist(err) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("control's released duty did not settle")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestRelayDrainRefusesInvalidDeadlineAndReusedInstance(t *testing.T) {
	rl := &Relay{}
	for _, timeout := range []time.Duration{0, -time.Second, 3 * time.Minute} {
		if err := rl.ServeDrained(context.Background(), timeout); err == nil {
			t.Fatal("invalid drain deadline accepted")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	rl.Spool, rl.Base, rl.Token = t.TempDir(), "http://127.0.0.1", "fixture"
	if err := rl.ServeDrained(ctx, time.Second); err != nil {
		t.Fatal(err)
	}
	if err := rl.ServeDrained(ctx, time.Second); err == nil {
		t.Fatal("reuse of an instance with retired lane channels accepted")
	}
}

func newHeldDrainRelay(t *testing.T, outbox bool) (*Relay, string, <-chan struct{}, func()) {
	t.Helper()
	spool := t.TempDir()
	entered, release := make(chan struct{}), make(chan struct{})
	var enteredOnce, releaseOnce sync.Once
	free := func() { releaseOnce.Do(func() { close(release) }) }
	svc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Sirsi-Session") == "held" {
			enteredOnce.Do(func() { close(entered) })
			<-release
		}
		_, _ = w.Write([]byte(`{"result":[]}`))
	}))
	t.Cleanup(svc.Close)
	t.Cleanup(free) // release handlers before closing the fixture server
	queue := "req"
	if outbox {
		queue = "outbox"
		publishDrainRequest(t, spool, "req", "trigger", "trigger")
		publishDrainRequest(t, spool, "outbox", "tail", "tail")
	}
	publishDrainRequest(t, spool, queue, "held", "held")
	rl := &Relay{Spool: spool, Base: svc.URL, Token: "fixture", Client: svc.Client(), Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	return rl, spool, entered, free
}

func publishDrainRequest(t *testing.T, spool, queue, id, session string) {
	t.Helper()
	dir := filepath.Join(spool, "lane", queue)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := writeAtomic(filepath.Join(dir, id+".json"), spoolRequest{Method: "ListAll", Headers: map[string]string{"X-Sirsi-Session": session}, Body: []byte(`{"args":[]}`)}, 0o600); err != nil {
		t.Fatal(err)
	}
}

func awaitDrainEntry(t *testing.T, entered <-chan struct{}) {
	t.Helper()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("owned forward never entered its handler")
	}
}

func assertDrainRetainedPending(t *testing.T, spool string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(spool, "lane", "req", "late.json")); err != nil {
		t.Fatalf("late pending request was consumed or lost: %v", err)
	}
	if _, err := os.Stat(filepath.Join(spool, "lane", "res", "late.json")); !os.IsNotExist(err) {
		t.Fatalf("late request was answered after cancellation: %v", err)
	}
}
