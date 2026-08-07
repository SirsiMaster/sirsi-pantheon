package routerstore

import (
	"errors"
	"os"
	"strings"
	"testing"
)

// ClaimNext is the only transition into executable ownership. Before this fix it
// called beginImmediate() with no retry, so a lane that lost a WAL race did not
// merely retry later — it silently failed to pick up work at all. Contention
// arrives as SQLITE_READONLY(8), never SQLITE_BUSY(5), so the DSN's
// busy_timeout(5000) never engages (see writeretry.go).
//
// NEGATIVE CONTROL: unwrap ClaimNext (call claimNextOnce directly) and
// TestClaimNextRetriesReadonlyContention fails — the first READONLY error
// propagates instead of being retried.
func TestClaimNextRetriesReadonlyContention(t *testing.T) {
	calls := 0
	// Two READONLY failures then success is the shape of a real WAL race: the
	// competing writer commits and the lock frees.
	err := (&Store{}).retryWrite(func() error {
		calls++
		if calls < 3 {
			return errors.New("attempt to write a readonly database (8)")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("retryWrite returned %v, want nil after the contention cleared", err)
	}
	if calls != 3 {
		t.Errorf("op ran %d times, want 3 — the whole transaction must re-run, "+
			"not just the failing statement", calls)
	}
}

// A budget or breaker rejection is a DECISION, not contention. Retrying it burns
// the retry budget on an answer that cannot change, and delays the caller by the
// full window before returning the same refusal.
func TestRetryWriteDoesNotRetryNonContention(t *testing.T) {
	calls := 0
	sentinel := errors.New("routerstore: budget exceeded")
	err := (&Store{}).retryWrite(func() error {
		calls++
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want the original refusal unwrapped", err)
	}
	if calls != 1 {
		t.Errorf("op ran %d times, want 1 — a non-contention error must not be retried", calls)
	}
}

// The exhaustion message has to name the real cause. Operators who saw
// "readonly" reasonably read it as a permissions fault and went looking at file
// modes; the store is writable and the fault is lock contention under WAL.
func TestRetryWriteExhaustionNamesContentionNotPermissions(t *testing.T) {
	err := (&Store{}).retryWrite(func() error {
		return errors.New("attempt to write a readonly database (8)")
	})
	if err == nil {
		t.Fatal("want an error after the budget is spent")
	}
	msg := err.Error()
	for _, want := range []string{"another process", "not a permissions fault"} {
		if !strings.Contains(msg, want) {
			t.Errorf("exhaustion message %q is missing %q — it must not read as a chmod problem", msg, want)
		}
	}
}

// Guard against the wrapper being unwound. ClaimNext must route through
// retryWrite; claimNextOnce is the single-attempt body and callers must never
// reach it directly.
func TestClaimNextGoesThroughRetryWrite(t *testing.T) {
	src := mustReadSource(t, "lease.go")
	claim := sliceFunc(src, "func (s *Store) ClaimNext(")
	if !strings.Contains(claim, "s.retryWrite(") {
		t.Error("ClaimNext no longer wraps its transaction in retryWrite — a lane that " +
			"loses a WAL race will silently fail to claim work (READONLY is not BUSY)")
	}
	if !strings.Contains(claim, "claimNextOnce(") {
		t.Error("ClaimNext no longer delegates to claimNextOnce")
	}
}

func mustReadSource(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(b)
}

// sliceFunc returns the body of the function whose signature starts at sig,
// bounded by the next top-level func so a later function cannot satisfy the
// assertion on its behalf.
func sliceFunc(src, sig string) string {
	i := strings.Index(src, sig)
	if i < 0 {
		return ""
	}
	rest := src[i+len(sig):]
	if j := strings.Index(rest, "\nfunc "); j >= 0 {
		return rest[:j]
	}
	return rest
}
