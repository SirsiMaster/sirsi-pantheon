package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/ledger"
)

// TestRenderLedgerHeaderSplitsItemsAndTasks is the regression guard for the
// silent revert introduced in PR #668: that PR was built by copying this
// file's WHOLE CONTENTS from a checkout that predated PR #663's fix, which
// clobbered #663's header format back to the pre-fix single-count line
// without a single failing test to catch it — #668 had its own passing tests
// (for the unrelated reset-attempts verb it actually added) and none of them
// touched renderLedger, so a stale-file overwrite of a completely different
// function shipped to main and was deployed to production undetected.
//
// #663 exists because an item-only header ("0 open · blocked 0 ·
// unblocked/unpicked 0") renders directly above a full non-empty task
// registry and reads as "no work to do" (A35: scope the check to the claim).
// This test pins the OUTPUT shape directly — not the Agent struct's fields,
// which #668's accidental revert left completely correct and untouched, only
// the print statement consuming them regressed. A future whole-file copy
// from a stale source will fail this test by name instead of shipping silent.
func TestRenderLedgerHeaderSplitsItemsAndTasks(t *testing.T) {
	snapshot := ledger.Snapshot{
		Agents: []ledger.Agent{
			{
				AgentID:           "claude-home",
				Items:             nil,
				OldestAgeSeconds:  0,
				BlockedCount:      0,
				UnblockedUnpicked: 0,
				OpenTasks:         45,
				BlockedTasks:      1,
			},
		},
	}

	out := captureStdout(t, func() { renderLedger(snapshot) })

	if !strings.Contains(out, "items: 0 open") {
		t.Fatalf("header must label the item count as items:, got: %q", out)
	}
	if !strings.Contains(out, "tasks: 45 open") || !strings.Contains(out, "1 blocked") {
		t.Fatalf("header must report the task registry (45 open, 1 blocked) separately from items, got: %q", out)
	}
}

// TestRenderLedgerHeaderZeroTasksStillLabelsScope pins the OTHER direction:
// a lane with genuinely nothing in its task registry must still say so
// under the "tasks:" label, not collapse back to the ambiguous unlabeled
// count that started this whole class of defect.
func TestRenderLedgerHeaderZeroTasksStillLabelsScope(t *testing.T) {
	snapshot := ledger.Snapshot{
		Agents: []ledger.Agent{
			{AgentID: "claude-finalwishes", OpenTasks: 0, BlockedTasks: 0},
		},
	}
	out := captureStdout(t, func() { renderLedger(snapshot) })
	if !strings.Contains(out, "tasks: 0 open · 0 blocked") {
		t.Fatalf("zero-task lane must still print the labeled tasks: scope, got: %q", out)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = old }()

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("close pipe writer: %v", err)
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("read pipe: %v", err)
	}
	return buf.String()
}
