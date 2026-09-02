package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

// TestBreakerResetUnblocksSend is the negative control for the 2026-08-07
// outage: sender:claude-home tripped, every send returned ErrBreakerOpen, and
// no verb existed to clear it. The test drives the real store through that
// exact sequence — trip, verify blocked, reset, verify sending again.
//
// Delete ResetBreaker's wiring and the final send still returns
// ErrBreakerOpen, so this fails by name.
func TestBreakerResetUnblocksSend(t *testing.T) {
	store, err := routerstore.OpenPath(t.TempDir() + "/router.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	send := func(title string) (string, bool, error) {
		return store.SendGuarded(routerstore.SendReq{
			From: "claude-home", To: "codex-home", Type: "decision",
			Title: title, Instructions: "body",
		})
	}

	// Trip sender:claude-home by blowing the per-window send quota. Each
	// over-quota rejection records a breaker failure (facade.go), so the
	// breaker trips at BreakerThreshold rejections.
	for i := 0; i < routerstore.MaxSendsPerSenderPerWindow+routerstore.BreakerThreshold+1; i++ {
		if _, _, sErr := send(strings.Repeat("x", 3) + string(rune('a'+i%26)) + itoa(i)); sErr != nil {
			if errors.Is(sErr, routerstore.ErrBreakerOpen) {
				break
			}
			if !errors.Is(sErr, routerstore.ErrOverQuota) {
				t.Fatalf("unexpected send error at %d: %v", i, sErr)
			}
		}
	}

	if _, _, sErr := send("blocked-probe"); !errors.Is(sErr, routerstore.ErrBreakerOpen) {
		t.Fatalf("expected the breaker to be tripped and gating sends, got %v", sErr)
	}

	breakers, err := store.Breakers()
	if err != nil {
		t.Fatalf("Breakers: %v", err)
	}
	var tripped bool
	for _, b := range breakers {
		if b.Domain == "sender:claude-home" && b.TrippedAt != "" {
			tripped = true
		}
	}
	if !tripped {
		t.Fatal("Breakers() did not report sender:claude-home as tripped")
	}

	if err := store.ResetBreaker("sender:claude-home"); err != nil {
		t.Fatalf("ResetBreaker: %v", err)
	}

	// The quota window is still spent, so the send may be refused for quota —
	// but it MUST NOT be refused by the breaker any more. That is the fix.
	if _, _, sErr := send("after-reset"); errors.Is(sErr, routerstore.ErrBreakerOpen) {
		t.Fatal("send still gated by the breaker after reset — the reset did nothing")
	}
}

// TestResetBreakerUnknownDomain pins the not-found contract the command
// surfaces to the operator, so a typo reports a miss instead of succeeding.
func TestResetBreakerUnknownDomain(t *testing.T) {
	store, err := routerstore.OpenPath(t.TempDir() + "/router.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	if err := store.ResetBreaker("sender:nobody"); err == nil {
		t.Fatal("expected an error resetting a domain with no recorded failures")
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}
