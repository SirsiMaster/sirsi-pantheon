package router

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeMarker(t *testing.T, root, agent string, pid int) {
	t.Helper()
	if err := os.WriteFile(consumerPIDFilePath(root, agent), []byte(fmt.Sprintf("%d\n", pid)), 0o644); err != nil {
		t.Fatal(err)
	}
}

// With the cap reached by OTHER lanes' live consumers, a lane is held; below the
// cap, or when the only live consumer is its own, it is not (both directions).
func TestConsumerSlotsCapHostConcurrency(t *testing.T) {
	root := t.TempDir()
	t.Setenv("SIRSI_MAX_CONSUMERS", "2")
	self := os.Getpid() // a live pid with a valid marker
	writeMarker(t, root, "lane-a", self)
	if consumerSlotsFull(root, "lane-z") {
		t.Fatal("1 running < cap 2: must not be full")
	}
	writeMarker(t, root, "lane-b", self)
	if !consumerSlotsFull(root, "lane-z") {
		t.Fatal("2 running = cap 2: must be full for a third lane")
	}
	if consumerSlotsFull(root, "lane-a") {
		t.Fatal("lane-a's own marker must not count against itself (1 other < 2)")
	}
	// a stale marker (dead pid) is not a running consumer
	writeMarker(t, root, "lane-b", 2147480000)
	if consumerSlotsFull(root, "lane-z") {
		t.Fatal("a dead pid must not hold a slot")
	}
	if h, _ := currentHold(root, "lane-z", 0, time.Time{}); h == HoldSlots {
		t.Fatal("slots not full: hold must not be slots")
	}
	_ = filepath.Join
}

func TestMaxConcurrentConsumersDefaultsFromCores(t *testing.T) {
	t.Setenv("SIRSI_MAX_CONSUMERS", "")
	if maxConcurrentConsumers() < 1 {
		t.Fatal("cap must be at least 1")
	}
	t.Setenv("SIRSI_MAX_CONSUMERS", "7")
	if maxConcurrentConsumers() != 7 {
		t.Fatal("env override ignored")
	}
}

// A lane flagged reserved_slot may start one consumer beyond the cap; an
// unflagged lane may not; and two flagged lanes still cannot exceed cap + 1.
func TestReservedSlotLetsAReviewerStartBeyondTheCapByOne(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".agents", "idea-router"), 0o755); err != nil {
		t.Fatal(err)
	}
	routerRoot := filepath.Join(root, ".agents", "idea-router")
	reg := `{"agents":{
	  "rev-a":{"id":"rev-a","type":"codex","command":["codex"],"consumer":{"command":["codex","exec"],"prompt":"You are {{agent}}","reserved_slot":true}},
	  "rev-b":{"id":"rev-b","type":"codex","command":["codex"],"consumer":{"command":["codex","exec"],"prompt":"You are {{agent}}","reserved_slot":true}},
	  "plain":{"id":"plain","type":"claude","command":["claude"],"consumer":{"command":["claude","--print"],"prompt":"You are {{agent}}"}}}}`
	if err := os.WriteFile(filepath.Join(routerRoot, "agents.json"), []byte(reg), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SIRSI_MAX_CONSUMERS", "2")
	self := os.Getpid()
	writeMarker(t, routerRoot, "x1", self)
	writeMarker(t, routerRoot, "x2", self) // cap (2) reached by other lanes
	if !consumerSlotsFull(routerRoot, "plain") {
		t.Fatal("an unflagged lane must be held at the cap")
	}
	if consumerSlotsFull(routerRoot, "rev-a") {
		t.Fatal("a reserved_slot lane must be allowed one beyond the cap")
	}
	writeMarker(t, routerRoot, "rev-a", self) // the reserved slot is now used
	if !consumerSlotsFull(routerRoot, "rev-b") {
		t.Fatal("a second reserved lane must NOT exceed cap + 1")
	}
	if !consumerSlotsFull(routerRoot, "plain") {
		t.Fatal("the reserved consumer still counts against everyone else")
	}
}
