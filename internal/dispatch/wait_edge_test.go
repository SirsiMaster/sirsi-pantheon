package dispatch

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

func edgeSend(t *testing.T, f *Facade, title string) {
	t.Helper()
	if _, _, err := f.store.SendGuarded(routerstore.SendReq{From: "b", To: "victim", Title: title, Instructions: "x"}); err != nil {
		t.Fatalf("send: %v", err)
	}
}

// Router items 20260729-225311 and 20260731-182937 (independent reports):
// `router wait` was LEVEL-triggered — any non-empty inbox returned instantly —
// and the documented /loop arming instruction calls it in a shell loop, so one
// stuck-open item turned the loop into a full-speed spin. These tests pin the
// edge semantics: each inbox STATE is delivered exactly once.

func TestWaitDeliversExistingWorkOnFirstCall(t *testing.T) {
	f := testFacade(t)
	edgeSend(t, f, "existing work")

	items, err := f.Wait(context.Background(), "victim", 3*time.Second)
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("a consumer arriving to existing work must see it (anti-stranding), got %d items", len(items))
	}
}

func TestWaitDoesNotRedeliverAnUnchangedInbox(t *testing.T) {
	f := testFacade(t)
	edgeSend(t, f, "stuck item")

	first, err := f.Wait(context.Background(), "victim", 3*time.Second)
	if err != nil || len(first) != 1 {
		t.Fatalf("first wait must deliver, got %d err=%v", len(first), err)
	}

	// The spin: an immediate second wait on the SAME inbox. Level-triggered
	// code returned instantly here, thousands of times per minute.
	start := time.Now()
	second, err := f.Wait(context.Background(), "victim", 2*time.Second)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if len(second) != 0 {
		t.Fatalf("an unchanged inbox must NOT be redelivered — this is the fork-storm, got %d items", len(second))
	}
	if elapsed < 1500*time.Millisecond {
		t.Fatalf("the un-delivering wait must PARK for its timeout, not return in %s — an instant empty return is still a spin", elapsed)
	}
}

func TestWaitWakesOnANewArrival(t *testing.T) {
	f := testFacade(t)
	edgeSend(t, f, "first")
	if _, err := f.Wait(context.Background(), "victim", 3*time.Second); err != nil {
		t.Fatalf("prime cursor: %v", err)
	}

	go func() {
		time.Sleep(400 * time.Millisecond)
		_, _, _ = f.store.SendGuarded(routerstore.SendReq{From: "b", To: "victim", Title: "second — a CHANGE", Instructions: "x"})
	}()
	start := time.Now()
	items, err := f.Wait(context.Background(), "victim", 10*time.Second)
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("a changed inbox must be delivered (both items), got %d", len(items))
	}
	if time.Since(start) > 8*time.Second {
		t.Fatal("wake on change should be prompt, not a full-timeout park")
	}
}

func TestWaitRedeliversAfterCursorExpiry(t *testing.T) {
	f := testFacade(t)
	edgeSend(t, f, "stuck item")
	if _, err := f.Wait(context.Background(), "victim", 3*time.Second); err != nil {
		t.Fatalf("prime: %v", err)
	}

	// A consumer that died after delivery must not strand the inbox forever:
	// age the cursor past waitRedeliverAfter and the same state redelivers.
	old := time.Now().Add(-waitRedeliverAfter - time.Minute)
	if err := os.Chtimes(f.waitCursorPath("victim"), old, old); err != nil {
		t.Fatalf("age cursor: %v", err)
	}
	items, err := f.Wait(context.Background(), "victim", 3*time.Second)
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("an expired cursor must redeliver the unchanged inbox, got %d", len(items))
	}
}
