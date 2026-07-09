package router

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// psLine renders one ps -axo pid,ppid,lstart,args row.
func psLine(pid, ppid int, started time.Time, args string) string {
	return fmt.Sprintf("%d %d %s %s", pid, ppid, started.Format(psLstartLayout), args)
}

const cliPath = "/Applications/Claude.app/Contents/MacOS/claude"

// TestReapSupersededSessions_IncidentShape reproduces the 2026-07-08 leak:
// many parent/child claude groups resuming the SAME session uuid. Everything
// but the newest group is reaped; a different session and a no-resume
// interactive session are untouched.
func TestReapSupersededSessions_IncidentShape(t *testing.T) {
	base := time.Date(2026, 7, 8, 6, 0, 0, 0, time.UTC)
	uuid := "40d9d244-aaaa-bbbb-cccc-121212121212"
	var lines []string
	// 5 leaked generations, each a parent+child pair, 25 minutes apart.
	for i := 0; i < 5; i++ {
		root := 1000 + i*10
		lines = append(lines,
			psLine(root, 1, base.Add(time.Duration(i)*25*time.Minute), cliPath+" --resume "+uuid),
			psLine(root+1, root, base.Add(time.Duration(i)*25*time.Minute), cliPath+" --resume "+uuid),
		)
	}
	// A different, single-group session and a no-resume interactive session.
	lines = append(lines,
		psLine(3000, 1, base, cliPath+" --resume beefbeef-1111-2222-3333-444444444444"),
		psLine(4000, 1, base, cliPath),
	)

	var killed []int
	report, err := ReapSupersededSessionsWith(99999, 99998, strings.Join(lines, "\n"), func(pid int) error {
		killed = append(killed, pid)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// 4 old generations × 2 procs each reaped; newest (root 1040) kept.
	if len(killed) != 8 {
		t.Fatalf("expected 8 reaps (4 superseded pairs), got %d: %v", len(killed), killed)
	}
	if got := report.KeptNewest[uuid]; got != 1040 {
		t.Fatalf("newest group root must be kept, got %d", got)
	}
	for _, pid := range killed {
		if pid >= 1040 && pid <= 1041 {
			t.Fatalf("reaped the NEWEST group (pid %d) — that is the live session", pid)
		}
		if pid == 3000 || pid == 4000 {
			t.Fatalf("reaped an innocent session (pid %d)", pid)
		}
	}
}

// TestReapNeverTouchesOwnGroup: a superseded-looking group containing the
// caller's own pid/parent is exempt — the reaper must not saw off its branch.
func TestReapNeverTouchesOwnGroup(t *testing.T) {
	base := time.Date(2026, 7, 8, 6, 0, 0, 0, time.UTC)
	uuid := "0d0d0d0d-1111-2222-3333-444444444444"
	lines := []string{
		psLine(500, 1, base, cliPath+" --resume "+uuid),                     // old group = SELF
		psLine(600, 1, base.Add(30*time.Minute), cliPath+" --resume "+uuid), // newer group
	}
	var killed []int
	_, err := ReapSupersededSessionsWith(500, 1, strings.Join(lines, "\n"), func(pid int) error {
		killed = append(killed, pid)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(killed) != 0 {
		t.Fatalf("own group must never be reaped, got kills: %v", killed)
	}
}

// TestReapOrphanedWatcherAndLegacySidecar: watch-router with a dead parent is
// reaped; one with a live claude parent survives; legacy sidecars are reaped.
func TestReapOrphanedWatcherAndLegacySidecar(t *testing.T) {
	base := time.Date(2026, 7, 8, 6, 0, 0, 0, time.UTC)
	lines := []string{
		psLine(700, 1, base, cliPath+" --resume aaaabbbb-1111-2222-3333-444444444444"),
		psLine(701, 700, base, "sirsi thread watch-router --thread thr-live"),   // healthy: claude parent
		psLine(800, 999, base, "sirsi thread watch-router --thread thr-orphan"), // parent 999 not in table
		psLine(900, 1, base, "bash -c while true; do cat /tmp/watcher-thr-x; done"),
	}
	var killed []int
	_, err := ReapSupersededSessionsWith(99999, 99998, strings.Join(lines, "\n"), func(pid int) error {
		killed = append(killed, pid)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := map[int]bool{800: true, 900: true}
	if len(killed) != 2 || !want[killed[0]] || !want[killed[1]] {
		t.Fatalf("expected exactly the orphan (800) and sidecar (900) reaped, got %v", killed)
	}
}

// TestGoDutyRegisteredWithCadence: the duty is in the registry as native Go
// on a bounded cadence — a script-missing skip must not apply to it.
func TestGoDutyRegisteredWithCadence(t *testing.T) {
	for _, d := range supervisorDuties() {
		if d.Name == "session-reaper" {
			if d.GoRun == nil {
				t.Fatal("session-reaper must be a native Go duty")
			}
			if d.Cadence != 10*time.Minute {
				t.Fatalf("cadence = %v, want 10m", d.Cadence)
			}
			return
		}
	}
	t.Fatal("session-reaper duty not registered")
}
