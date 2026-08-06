package router

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestWakeLoopReadsThroughCutoverEntryPoint pins that RunWakeLoop's inbox read
// goes through the cutover-aware entry point rather than internal/work directly.
//
// This is a source-level assertion on purpose. The defect it guards is not a
// behavior you can provoke cheaply — it needs a live launchd loop, a store
// cutover, and frozen legacy files diverging from the store. What it IS, is a
// one-line drift: #315 routed ctr / conduit-tick / router plan / the work board
// through OpenItems and claimed "no observer can drift back onto the files",
// while this call site in the same file kept calling work.ListInbox. The
// heartbeat status was therefore derived from files nothing writes post-cutover.
//
// A grep-style test is the honest shape for a drift of that kind.
func TestWakeLoopReadsThroughCutoverEntryPoint(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("wake.go"))
	if err != nil {
		t.Fatalf("read wake.go: %v", err)
	}
	body := string(src)

	start := strings.Index(body, "func RunWakeLoop(")
	if start < 0 {
		t.Fatal("RunWakeLoop not found in wake.go")
	}
	// Bound the scan to this function: the guarded pre-cutover fallback elsewhere
	// in this file legitimately calls work.ListInbox and must NOT trip this.
	end := strings.Index(body[start+1:], "\nfunc ")
	if end < 0 {
		end = len(body) - start - 1
	}
	fn := body[start : start+1+end]

	// Strip comment lines before scanning. The fix's own comment explains why
	// work.ListInbox is wrong here, and a guard that trips on its own rationale
	// is a guard people delete. (Second time today I wrote this bug — the
	// menubar font guard had the identical flaw.)
	var code []string
	for _, line := range strings.Split(fn, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "//") {
			continue
		}
		code = append(code, line)
	}
	fn = strings.Join(code, "\n")

	if strings.Contains(fn, "work.ListInbox") {
		t.Error("RunWakeLoop calls work.ListInbox directly — post-cutover the legacy " +
			"files are frozen, so the heartbeat status would be derived from stale data. " +
			"Use OpenItems (the cutover-aware entry point).")
	}
	if !strings.Contains(fn, "OpenItems(") {
		t.Error("RunWakeLoop no longer reads the inbox through OpenItems")
	}
}

// An inbox read failure must never be published as idle/empty. That false-green
// let Horus report a healthy fabric while codex-home had dozens of open items:
// the wake loop logged the error, discarded it, and heartbeated idle at depth 0.
// The durable thread record is the supervisory source of truth, so it must carry
// both a blocked status and the concrete read failure until a successful read
// clears it.
func TestWakeLoopReadFailurePublishesDurableBlocker(t *testing.T) {
	status, depth, lastError := wakeLoopInboxState(0, errors.New("store unavailable"))
	if status != ThreadStatusBlocked {
		t.Fatalf("status = %q, want blocked", status)
	}
	if depth != -1 {
		t.Errorf("depth = %d, want -1 (unknown, not falsely empty)", depth)
	}
	if !strings.Contains(lastError, "store unavailable") {
		t.Errorf("last_error = %q, want concrete read failure", lastError)
	}

	status, depth, lastError = wakeLoopInboxState(0, nil)
	if status != ThreadStatusIdle || depth != 0 || lastError != "" {
		t.Errorf("successful empty read = (%q, %d, %q), want (idle, 0, empty error)",
			status, depth, lastError)
	}

	status, depth, lastError = wakeLoopInboxState(41, nil)
	if status != ThreadStatusActive || depth != 41 || lastError != "" {
		t.Errorf("successful nonempty read = (%q, %d, %q), want (active, 41, empty error)",
			status, depth, lastError)
	}
}

// A running loop must be distinguishable from a wedged one in its own log.
//
// The first version logged only on inbox-depth CHANGE. At a constant depth that
// means one start line, one transition, then silence forever — so a healthy
// loop at depth 32 and a loop wedged immediately after observing depth 32 leave
// IDENTICAL forensic records (codex review of #327). The heartbeat interval is
// the bound on that ambiguity, and it must be short enough to be useful and
// long enough not to become a tick log.
func TestWakeLoopHeartbeatBoundsSilence(t *testing.T) {
	if wakeLoopHeartbeatLog <= 0 {
		t.Fatal("no heartbeat interval — a constant-depth loop would be silent forever")
	}
	if wakeLoopHeartbeatLog > 30*time.Minute {
		t.Errorf("heartbeat every %s is too sparse to distinguish quiet from dead", wakeLoopHeartbeatLog)
	}
	if wakeLoopHeartbeatLog < time.Minute {
		t.Errorf("heartbeat every %s turns the log into a tick log", wakeLoopHeartbeatLog)
	}

	// And the loop body must actually emit an unchanged-depth line.
	src, err := os.ReadFile("wake.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), "unchanged") {
		t.Error("no unchanged-depth liveness line — silence still means both quiet and dead")
	}
}
