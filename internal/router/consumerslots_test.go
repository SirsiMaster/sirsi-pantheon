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
