package router

import (
	"errors"
	"fmt"
	"testing"
)

// A lost fence is contention, not corruption: the pass must be re-run. The
// pre-fix behavior was a single attempt whose error reached the caller as
// "OS-truth sweep incomplete", which is byte-identical to "nothing to reap".
func TestRetryOnLostFenceRedoesUntilItWins(t *testing.T) {
	calls := 0
	out, err := retryOnLostFence(func() ([]ReapedThread, error) {
		calls++
		if calls < 3 {
			return nil, fmt.Errorf("thread %q mutation %w", "thr-x", ErrLostLifecycleFence)
		}
		return []ReapedThread{{ThreadID: "thr-x"}}, nil
	})
	if err != nil {
		t.Fatalf("want success after contention clears, got %v", err)
	}
	if calls != 3 {
		t.Fatalf("want 3 attempts, got %d", calls)
	}
	if len(out) != 1 {
		t.Fatalf("want the winning pass's result, got %v", out)
	}
}

// The retry is scoped to the fence signal only. Any other failure is returned
// on the first attempt — retrying a real error just delays the report.
func TestRetryOnLostFenceDoesNotRetryOtherErrors(t *testing.T) {
	calls := 0
	boom := errors.New("store is unreadable")
	_, err := retryOnLostFence(func() ([]ReapedThread, error) {
		calls++
		return nil, boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("want the original error, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("want 1 attempt for a non-fence error, got %d", calls)
	}
}

// Contention that never clears must still surface — a retry budget that
// silently swallowed the last failure would reinstate the exact false
// "nothing to reap" reading this change exists to remove.
func TestRetryOnLostFenceSurfacesPersistentContention(t *testing.T) {
	calls := 0
	_, err := retryOnLostFence(func() ([]ReapedThread, error) {
		calls++
		return nil, fmt.Errorf("thread %q mutation %w", "thr-y", ErrLostLifecycleFence)
	})
	if !errors.Is(err, ErrLostLifecycleFence) {
		t.Fatalf("want the fence error surfaced after the budget, got %v", err)
	}
	if calls != 3 {
		t.Fatalf("want the full budget of 3 attempts, got %d", calls)
	}
}

// retryOnLostFenceErr is retryOnLostFence's counterpart for a plain-error
// pass (lifecycle-fence-lost: setThreadConsumerCapable had NO retry at all,
// unlike the two reap passes above). Same three behaviors, same budget.

func TestRetryOnLostFenceErrRedoesUntilItWins(t *testing.T) {
	calls := 0
	err := retryOnLostFenceErr(func() error {
		calls++
		if calls < 3 {
			return fmt.Errorf("thread %q mutation %w", "thr-x", ErrLostLifecycleFence)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("want success after contention clears, got %v", err)
	}
	if calls != 3 {
		t.Fatalf("want 3 attempts, got %d", calls)
	}
}

func TestRetryOnLostFenceErrDoesNotRetryOtherErrors(t *testing.T) {
	calls := 0
	boom := errors.New("store is unreadable")
	err := retryOnLostFenceErr(func() error {
		calls++
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("want the original error, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("want 1 attempt for a non-fence error, got %d", calls)
	}
}

// Negative control for the fix itself: delete retryOnLostFenceErr's wiring
// out of setThreadConsumerCapable (revert to a single LoadThreadRegistry +
// mutate + SaveThreadRegistry call) and this test still passes unchanged,
// because it exercises the helper directly — so it alone does not prove the
// production call site uses it. TestRetryOnLostFenceErrSurfacesPersistentContention
// pins the budget; the wiring itself is verified by reading
// setThreadConsumerCapable's body, which now has no bare load-mutate-save
// path left to regress to silently.
func TestRetryOnLostFenceErrSurfacesPersistentContention(t *testing.T) {
	calls := 0
	err := retryOnLostFenceErr(func() error {
		calls++
		return fmt.Errorf("thread %q mutation %w", "thr-y", ErrLostLifecycleFence)
	})
	if !errors.Is(err, ErrLostLifecycleFence) {
		t.Fatalf("want the fence error surfaced after the budget, got %v", err)
	}
	if calls != 3 {
		t.Fatalf("want the full budget of 3 attempts, got %d", calls)
	}
}
