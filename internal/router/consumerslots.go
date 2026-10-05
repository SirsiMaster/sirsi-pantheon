package router

import (
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// Host-wide consumer concurrency cap. Each wake loop is nearly free when idle (a
// 2-minute probe cost 0.05 CPU-seconds); the CPU on a busy Mac is the headless
// consumers it spawns, each a full agent session. Lane gates are per-lane, so six
// lanes could each run one. This caps how many run at once on the host: a lane
// whose turn it is simply waits for the next tick (the wake is level-triggered, so
// nothing is lost, only paced).

const HoldSlots = "slots" // the host is already running its maximum number of consumers

// maxConcurrentConsumers is SIRSI_MAX_CONSUMERS if set, else one per five cores
// (at least one): 2 on a 10-core Mac, 3 on an 18-core one.
func maxConcurrentConsumers() int {
	if v, err := strconv.Atoi(strings.TrimSpace(os.Getenv("SIRSI_MAX_CONSUMERS"))); err == nil && v > 0 {
		return v
	}
	if n := runtime.NumCPU() / 5; n > 1 {
		return n
	}
	return 1
}

// runningConsumersExcept counts consumers alive on this host (from the per-lane pid
// markers in the router root), not counting agentID's own marker.
func runningConsumersExcept(routerRoot, agentID string) int {
	matches, _ := filepath.Glob(filepath.Join(routerRoot, "wake-consumer-*.pid"))
	n := 0
	own := filepath.Base(consumerPIDFilePath(routerRoot, agentID))
	for _, m := range matches {
		if filepath.Base(m) == own {
			continue
		}
		data, err := os.ReadFile(m)
		if err != nil {
			continue
		}
		lines := strings.SplitN(strings.TrimSpace(string(data)), "\n", 2)
		pid, perr := strconv.Atoi(strings.TrimSpace(lines[0]))
		if perr != nil || pid <= 0 {
			continue
		}
		started := ""
		if len(lines) > 1 {
			started = strings.TrimSpace(lines[1])
		}
		if PIDStateOf(pid, started) == PIDAlive {
			n++
		}
	}
	return n
}

// consumerSlotsFull reports whether starting another consumer would exceed the host
// cap. Quiet: the caller logs.
func consumerSlotsFull(routerRoot, agentID string) bool {
	return runningConsumersExcept(routerRoot, agentID) >= maxConcurrentConsumers()
}

func hostConsumerSlotsFull(routerRoot, agentID string, depth int) bool {
	if !consumerSlotsFull(routerRoot, agentID) {
		return false
	}
	log.Printf("wake-loop %s: dispatch held — %d consumer(s) already running on this host (max %d); will retry (inbox depth %d)",
		agentID, runningConsumersExcept(routerRoot, agentID), maxConcurrentConsumers(), depth)
	return true
}
