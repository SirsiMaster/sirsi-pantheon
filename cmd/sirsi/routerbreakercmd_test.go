package main

import (
	"errors"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

// TestBreakerResetUnblocksSend is the negative control for the 2026-08-07
// outage: sender:claude-home tripped, every send returned ErrBreakerOpen, and
// no verb existed to clear it. The test drives the real store through that
// exact sequence — trip, verify blocked, reset, verify sending again.
//
// The trip is driven through a GENUINE delivery fault (items from claude-home
// dead-lettering), not a quota flood: quota backpressure is a throttle, not a
// breaker failure (Stack Lab convention), so only real faults trip the breaker.
//
// Delete ResetBreaker's wiring and the final send still returns
// ErrBreakerOpen, so this fails by name.
func TestBreakerResetUnblocksSend(t *testing.T) {
	store, err := routerstore.OpenPath(t.TempDir() + "/router.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	oldT := routerstore.BreakerThreshold
	routerstore.BreakerThreshold = 3
	t.Cleanup(func() { routerstore.BreakerThreshold = oldT })
	oldR := routerstore.MaxRetriesPerItem
	routerstore.MaxRetriesPerItem = 1 // each Fail dead-letters immediately
	t.Cleanup(func() { routerstore.MaxRetriesPerItem = oldR })

	send := func(title string) (string, bool, error) {
		return store.SendGuarded(routerstore.SendReq{
			From: "claude-home", To: "codex-home", Type: "decision",
			Title: title, Instructions: "body",
		})
	}

	// Trip sender:claude-home through real dead-letters: send work FROM
	// claude-home to a worker, then claim and fail each until it dead-letters,
	// which records a sender:claude-home breaker failure (lease.go). At
	// BreakerThreshold dead-letters the sender breaker trips.
	for i := 0; i < routerstore.BreakerThreshold; i++ {
		id, _, sErr := store.SendGuarded(routerstore.SendReq{
			From: "claude-home", To: "w", Type: "decision",
			Title: "job-" + itoa(i), Instructions: "body",
		})
		if sErr != nil {
			t.Fatalf("seed send %d: %v", i, sErr)
		}
		lease, cErr := store.ClaimNext("w", time.Minute)
		if cErr != nil {
			t.Fatalf("claim %d (%s): %v", i, id, cErr)
		}
		if fErr := store.Fail(lease.ItemID, lease.Token, "delivery fault", "downstream"); fErr != nil {
			t.Fatalf("fail %d: %v", i, fErr)
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

	// Only three jobs were sent (well under quota), so after the reset a fresh
	// send must SUCCEED outright — not merely escape the breaker. A returned id
	// with no error is the proof the reset re-opened the path.
	id, _, sErr := send("after-reset")
	if sErr != nil {
		t.Fatalf("send after reset must succeed, got %v", sErr)
	}
	if id == "" {
		t.Fatal("send after reset returned no id — the path is not truly re-opened")
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
