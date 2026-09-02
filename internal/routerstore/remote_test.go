package routerstore

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

// remoteHarness serves a real backend store over httptest and returns a
// RemoteStore pointed at it. Whatever backend the shared helpers pick
// (SQLite, or Postgres under SIRSI_TEST_PG_DSN) is what the wire wraps.
func remoteHarness(t *testing.T) (Store, *RemoteStore) {
	t.Helper()
	backend := openBackendStore(t, filepath.Join(t.TempDir(), "router.db"))
	t.Cleanup(func() { _ = backend.Close() })
	backend.notifyDir = t.TempDir()
	h, err := Handler(backend, ServerOptions{Token: "t0k", MaxWait: 3 * time.Second})
	if err != nil {
		t.Fatalf("Handler: %v", err)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	rs := NewRemoteStore(srv.URL, "t0k")
	rs.sessionDir = "" // never touch ~/.sirsi/sessions from a test
	return backend, rs
}

func TestRemoteRoundTripsAClaimLifecycle(t *testing.T) {
	backend, rs := remoteHarness(t)

	id, err := rs.Send("claude-a", "claude-b", "hello", "proposal", "do the thing")
	if err != nil {
		t.Fatalf("Send over the wire: %v", err)
	}
	items, err := rs.Inbox("claude-b")
	if err != nil || len(items) != 1 || items[0].ID != id {
		t.Fatalf("Inbox over the wire: items=%v err=%v", items, err)
	}
	lease, err := rs.ClaimNext("claude-b", time.Minute)
	if err != nil || lease == nil || lease.ItemID != id {
		t.Fatalf("ClaimNext over the wire: lease=%v err=%v", lease, err)
	}
	if cerr := rs.Complete(id, lease.Token, "done"); cerr != nil {
		t.Fatalf("Complete over the wire: %v", cerr)
	}
	// The backend, not a mirror, holds the truth.
	got, err := backend.Get(id)
	if err != nil || got.Status == "open" {
		t.Fatalf("backend state after remote Complete: %+v err=%v", got, err)
	}
}

func TestRemoteSentinelErrorsSurviveTheWire(t *testing.T) {
	_, rs := remoteHarness(t)
	if _, err := rs.ClaimNext("nobody", time.Minute); !errors.Is(err, ErrNoWork) {
		t.Fatalf("want errors.Is(ErrNoWork) across HTTP, got %v", err)
	}
	if _, err := rs.Get("missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want errors.Is(ErrNotFound) across HTTP, got %v", err)
	}
}

func TestRemoteWaitIsALongPollThatWakesOnSend(t *testing.T) {
	_, rs := remoteHarness(t)
	done := make(chan bool, 1)
	go func() {
		woke, err := rs.Wait(t.Context(), "claude-w", 2*time.Second)
		done <- woke && err == nil
	}()
	time.Sleep(150 * time.Millisecond)
	// SendGuarded is the production send (dispatch.Facade); it is the path that
	// notifies in-process waiters. Plain Send is the legacy import path.
	if _, _, err := rs.SendGuarded(SendReq{From: "x", To: "claude-w", Title: "wake", Type: "proposal", Instructions: "..."}); err != nil {
		t.Fatal(err)
	}
	select {
	case ok := <-done:
		if !ok {
			t.Fatal("Wait returned without waking")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Wait did not return after a Send")
	}
}

func TestRemoteRejectsBadTokenAndUnknownMethod(t *testing.T) {
	backend := openBackendStore(t, filepath.Join(t.TempDir(), "router.db"))
	t.Cleanup(func() { _ = backend.Close() })
	h, err := Handler(backend, ServerOptions{Token: "right"})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	wrong := NewRemoteStore(srv.URL, "wrong")
	wrong.sessionDir = ""
	if _, ierr := wrong.Inbox("a"); ierr == nil {
		t.Fatal("wrong token must be refused")
	}
	resp, err := http.Post(srv.URL+"/v1/call/NoSuchMethod", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated unknown method: want 401 before lookup, got %d", resp.StatusCode)
	}
	if _, err := Handler(backend, ServerOptions{}); err == nil {
		t.Fatal("Handler must refuse to serve without a token")
	}
}

// Every sentinel the package exports must be in the wire table, or a client
// loses errors.Is on it silently. Pins the closed set in remote.go.
func TestSentinelsRoundTrip(t *testing.T) {
	all := []error{ErrNotComplete, ErrBreakerOpen, ErrOverQuota, ErrIdentifierTaken, ErrNoWork,
		ErrNoClaimableTask, ErrLeaseInvalid, ErrTerminal, ErrBudgetExceeded, ErrReasonRequired,
		ErrIncompleteEvidence, ErrNotFound, ErrAlreadyClosed, ErrConcurrentTaskUpdate, ErrTaskExists,
		ErrSessionUnknown, ErrSessionRevoked, ErrNotOwner, ErrTokenUnknown, ErrTokenRevoked, ErrHostMismatch}
	if len(all) != len(sentinelErrors) {
		t.Fatalf("sentinel table has %d entries, package exports %d", len(sentinelErrors), len(all))
	}
	for _, e := range all {
		if sentinelName(e) == "" {
			t.Fatalf("sentinel %v missing from the wire table", e)
		}
	}
}
