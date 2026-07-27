package router

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
