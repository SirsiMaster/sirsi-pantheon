package routerstore

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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
		ErrSessionUnknown, ErrSessionRevoked, ErrNotOwner, ErrTokenUnknown, ErrTokenRevoked, ErrHostMismatch, ErrServiceUnavailable, ErrUnregistered, ErrThreadUnknown, ErrThreadAuthority}
	if len(all) != len(sentinelErrors) {
		t.Fatalf("sentinel table has %d entries, package exports %d", len(sentinelErrors), len(all))
	}
	for _, e := range all {
		if sentinelName(e) == "" {
			t.Fatalf("sentinel %v missing from the wire table", e)
		}
	}
}

// The Rule of Ra CLI identity hook (ADR-062 20b.2): when the environment
// carries no agent/thread, NewRemoteStore fills them from IdentityHook; the
// environment always wins; each field falls back independently.
func TestIdentityHookFillsAgentAndThreadWhenEnvUnset(t *testing.T) {
	prev := IdentityHook
	t.Cleanup(func() { IdentityHook = prev })
	IdentityHook = func() (string, string) { return "ra", "thr-abc" }

	// env unset → hook supplies both.
	t.Setenv("SIRSI_AGENT_ID", "")
	t.Setenv("SIRSI_THREAD_ID", "")
	rs := NewRemoteStore("https://x", "t")
	if rs.agent != "ra" || rs.threadID != "thr-abc" {
		t.Fatalf("hook must fill both when env is empty: agent=%q thread=%q", rs.agent, rs.threadID)
	}

	// env wins for both.
	t.Setenv("SIRSI_AGENT_ID", "claude-io")
	t.Setenv("SIRSI_THREAD_ID", "thr-env")
	rs = NewRemoteStore("https://x", "t")
	if rs.agent != "claude-io" || rs.threadID != "thr-env" {
		t.Fatalf("env must win: agent=%q thread=%q", rs.agent, rs.threadID)
	}

	// coherence (SSA #731): a FOREIGN env agent must NOT inherit the marker's
	// thread (which names a different agent) — that would be an incoherent
	// (claude-io, ra's thread) pair. Left threadless until it supplies its own.
	t.Setenv("SIRSI_AGENT_ID", "claude-io")
	t.Setenv("SIRSI_THREAD_ID", "")
	rs = NewRemoteStore("https://x", "t")
	if rs.agent != "claude-io" || rs.threadID != "" {
		t.Fatalf("foreign env agent must stay threadless, not borrow the marker thread: agent=%q thread=%q", rs.agent, rs.threadID)
	}

	// but when the env agent AGREES with the marker agent, the thread is adopted.
	IdentityHook = func() (string, string) { return "claude-io", "thr-io" }
	t.Setenv("SIRSI_AGENT_ID", "claude-io")
	t.Setenv("SIRSI_THREAD_ID", "")
	rs = NewRemoteStore("https://x", "t")
	if rs.agent != "claude-io" || rs.threadID != "thr-io" {
		t.Fatalf("matching agent must adopt the marker thread: agent=%q thread=%q", rs.agent, rs.threadID)
	}
	// an explicit env thread is always honored, even with the agent from the marker.
	IdentityHook = func() (string, string) { return "ra", "thr-abc" }
	t.Setenv("SIRSI_AGENT_ID", "")
	t.Setenv("SIRSI_THREAD_ID", "thr-explicit")
	rs = NewRemoteStore("https://x", "t")
	if rs.agent != "ra" || rs.threadID != "thr-explicit" {
		t.Fatalf("explicit env thread must win with marker agent: agent=%q thread=%q", rs.agent, rs.threadID)
	}

	// no hook installed → env-only, host fallback for agent, empty thread.
	IdentityHook = nil
	t.Setenv("SIRSI_AGENT_ID", "")
	t.Setenv("SIRSI_THREAD_ID", "")
	rs = NewRemoteStore("https://x", "t")
	host, _ := os.Hostname()
	if rs.agent != host || rs.threadID != "" {
		t.Fatalf("without a hook, env-only: agent=%q (want host %q) thread=%q", rs.agent, host, rs.threadID)
	}
}

// The CLIENT per-call context must exceed the 30s spool wait, or it cancels a
// spool round-trip before the relay can answer; the old 5s also canceled a warm
// ~3-4s full-ledger read outright (SSA 2026-09-11). This is a client-side
// budget. ListAll's server-side QueryContext (rs-26) is a separate, independent
// bound enforced by serve.go's own per-request deadline — see
// TestServerListAllDeadlineEndsAnInFlightRead below.
func TestPerCallTimeoutExceedsSpoolWait(t *testing.T) {
	rs := NewRemoteStore("https://x", "t")
	st := newSpoolTransport(t.TempDir(), "a")
	if rs.perCall <= st.wait {
		t.Fatalf("client perCall %v must exceed the spool wait %v", rs.perCall, st.wait)
	}
	// And a comfortable ceiling for a slow warm read (was 5s, which canceled a 4s read).
	if rs.perCall < 20*time.Second {
		t.Fatalf("client perCall %v is too tight for a large ledger read", rs.perCall)
	}
}

// TestRemoteListAllSessionMintRacesCallerContext (SSA review, PR #746): a
// caller's ctx must bound session acquisition, not just the eventual
// ListAll RPC. Before this fix, ensureSession/callUnsigned built their own
// context.Background(), so an already-canceled caller ctx still let
// MintSession run to completion against a slow/unresponsive server instead of
// failing fast. A plain fast local httptest server can't distinguish "ctx
// honored" from "ctx ignored" — both return near-instantly — so the mint
// route is deliberately delayed past both rs.perCall (short, for this test)
// and the assertion threshold: only a canceled ctx that is ACTUALLY wired into
// the mint's HTTP request aborts before that delay elapses.
func TestRemoteListAllSessionMintRacesCallerContext(t *testing.T) {
	backend := openBackendStore(t, filepath.Join(t.TempDir(), "router.db"))
	t.Cleanup(func() { _ = backend.Close() })
	backend.notifyDir = t.TempDir()
	real, err := Handler(backend, ServerOptions{Token: "t0k", MaxWait: time.Second})
	if err != nil {
		t.Fatalf("Handler: %v", err)
	}
	const mintDelay = 1 * time.Second
	slowMint := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "MintSession") {
			time.Sleep(mintDelay)
		}
		real.ServeHTTP(w, r)
	})
	srv := httptest.NewServer(slowMint)
	t.Cleanup(srv.Close)
	rs := NewRemoteStore(srv.URL, "t0k")
	rs.sessionDir = ""           // no cached session — ListAll must mint
	rs.perCall = 5 * time.Second // longer than mintDelay: a ctx-ignoring mint would ride this out, not its own short budget

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	_, err = rs.ListAll(ctx)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("ListAll(canceled ctx) with no cached session = nil error, want a context error from the mint path")
	}
	if elapsed > 200*time.Millisecond {
		t.Fatalf("ListAll(canceled ctx) took %v against a %v-delayed mint endpoint — session mint is not bound to the caller's ctx (SSA PR #746 finding)", elapsed, mintDelay)
	}
}

// TestServerListAllDeadlineEndsAnInFlightRead (SSA review, PR #746): proves
// the SERVER actually terminates an in-flight ListAll read at its own
// deadline — not merely that QueryContext receives an already-expired ctx
// before starting (TestListAllHonorsContext covers that narrower claim).
// scanRowHook forces determinism: it sleeps well past the server's CallTimeout
// after the first row is scanned, so the query has demonstrably started
// returning rows before the deadline fires, then the read must still be cut
// off — proven by the whole request completing near CallTimeout, not near the
// sum of the hook's sleeps.
func TestServerListAllDeadlineEndsAnInFlightRead(t *testing.T) {
	backend := openBackendStore(t, filepath.Join(t.TempDir(), "router.db"))
	t.Cleanup(func() { _ = backend.Close() })
	backend.notifyDir = t.TempDir()
	for i := 0; i < 3; i++ {
		if _, err := backend.Send("a", "b", "row", "review", "x"); err != nil {
			t.Fatalf("seed row %d: %v", i, err)
		}
	}
	const callTimeout = 40 * time.Millisecond
	const hookSleep = 150 * time.Millisecond // >> callTimeout, << t.Deadline
	h, err := Handler(backend, ServerOptions{Token: "t0k", CallTimeout: callTimeout, MaxWait: time.Second})
	if err != nil {
		t.Fatalf("Handler: %v", err)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	rs := NewRemoteStore(srv.URL, "t0k")
	rs.sessionDir = ""
	rs.perCall = 5 * time.Second // client budget stays generous; the SERVER deadline is what's under test

	scanRowHook = func() { time.Sleep(hookSleep) }
	t.Cleanup(func() { scanRowHook = nil })

	start := time.Now()
	_, err = rs.ListAll(context.Background())
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("ListAll over a server read that outlives CallTimeout = nil error, want the server deadline to end it")
	}
	// If the server did NOT cancel the in-flight read, scanning all 3 rows
	// would take ~3*hookSleep (450ms) before the response is even written. A
	// server-side cutoff at CallTimeout returns close to that instead.
	if elapsed > 300*time.Millisecond {
		t.Fatalf("ListAll took %v — the server did not cut off the in-flight read at its %v deadline (ran closer to the full %v of hook sleeps)", elapsed, callTimeout, 3*hookSleep)
	}
}
