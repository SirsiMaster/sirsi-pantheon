package routerstore

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// readonlyErr is the error as it actually reaches callers. NOTE ON SCOPE: the
// driver's *sqlite.Error has unexported fields and no exported constructor, so a
// typed driver error cannot be built from outside the driver package. These tests
// therefore exercise the STRING path of isReadonlyContention, not the typed
// errors.As path. Stated plainly rather than implied — the typed branch is
// covered by the primary-code mask being a two-line expression, and by the live
// reproduction recorded in writeretry.go's header, not by this file.
func readonlyErr() error {
	return errors.New("attempt to write a readonly database (8)")
}

func TestIsReadonlyContention(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"readonly", readonlyErr(), true},
		{"readonly wrapped across a boundary", fmt.Errorf("store mirror failed: %w", readonlyErr()), true},
		{"busy is NOT readonly", errors.New("database is locked (5)"), false},
		{"unrelated", errors.New("UNIQUE constraint failed"), false},
	}
	for _, c := range cases {
		if got := isReadonlyContention(c.err); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

// TestRetryWrite_SucceedsAfterContentionClears is the behavior that fixes the
// 2026-08-06 outage: a writer that loses the lock a few times must go on to
// succeed, instead of failing outright with what looks like a permissions error.
func TestRetryWrite_SucceedsAfterContentionClears(t *testing.T) {
	s := &Store{}
	calls := 0
	err := s.retryWrite(func() error {
		calls++
		if calls < 4 {
			return readonlyErr()
		}
		return nil
	})
	if err != nil {
		t.Fatalf("write did not recover once contention cleared: %v", err)
	}
	if calls != 4 {
		t.Errorf("op called %d times, want 4 (3 contended + 1 success)", calls)
	}
}

// TestRetryWrite_PassesThroughNonContention guards against the retry swallowing
// or delaying real errors. A constraint violation must surface immediately.
func TestRetryWrite_PassesThroughNonContention(t *testing.T) {
	s := &Store{}
	sentinel := errors.New("UNIQUE constraint failed")
	start := time.Now()
	calls := 0
	err := s.retryWrite(func() error { calls++; return sentinel })
	if !errors.Is(err, sentinel) {
		t.Fatalf("real error not passed through: %v", err)
	}
	if calls != 1 {
		t.Errorf("real error retried %d times, want 1", calls)
	}
	if time.Since(start) > time.Second {
		t.Error("real error was delayed by backoff")
	}
}

// TestRetryWrite_GivesUpWithADiagnosticError proves a genuinely unwritable store
// still fails — bounded, wrapping the driver error, and naming contention so the
// next operator does not lose an hour to chmod and rebuilds.
func TestRetryWrite_GivesUpWithADiagnosticError(t *testing.T) {
	s := &Store{}
	underlying := readonlyErr()

	start := time.Now()
	err := s.retryWrite(func() error { return underlying })
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("permanent readonly did not fail")
	}
	if !errors.Is(err, underlying) {
		t.Error("underlying driver error was not wrapped — operators lose the real code")
	}
	for _, want := range []string{"another process holding the store", "not a permissions fault"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("give-up error missing %q; got: %v", want, err)
		}
	}
	if elapsed < writeRetryBudget {
		t.Errorf("gave up after %s, before the %s budget", elapsed, writeRetryBudget)
	}
	if elapsed > writeRetryBudget+2*time.Second {
		t.Errorf("overshot the budget badly: %s", elapsed)
	}
}
