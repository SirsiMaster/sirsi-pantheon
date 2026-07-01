package guard

import (
	"fmt"
	"os/exec"
	"strings"
)

// localSnapshotsFn lists local Time Machine (APFS) snapshot identifiers on a
// volume. Injectable (Rule A16) so the doctor check is testable without a real
// tmutil. Only `com.apple.TimeMachine.*` entries are reclaimable snapshots —
// `com.apple.os.update-*` snapshots are non-purgeable and are excluded.
var localSnapshotsFn = defaultLocalSnapshots

func defaultLocalSnapshots(vol string) []string {
	out, err := exec.Command("/usr/bin/tmutil", "listlocalsnapshots", vol).CombinedOutput()
	if err != nil {
		return nil
	}
	var ids []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "com.apple.TimeMachine.") {
			ids = append(ids, line)
		}
	}
	return ids
}

// LocalSnapshots is the exported accessor the reclaim-snapshots command shares
// with the doctor check, so both agree on what's reclaimable.
func LocalSnapshots(vol string) []string { return localSnapshotsFn(vol) }

// checkLocalSnapshots surfaces reclaimable local Time Machine snapshots as
// actionable INFO — never an alarm (snapshots are useful, not a problem; the
// surfaces law). It carries the reclaim action so the user has a one-click lever
// when they want the space, without the finding ever reading red/yellow.
func checkLocalSnapshots(report *DoctorReport) {
	snaps := localSnapshotsFn("/")
	if len(snaps) == 0 {
		return // nothing reclaimable — no finding (honest: don't invent a row)
	}
	noun := "snapshots"
	if len(snaps) == 1 {
		noun = "snapshot"
	}
	report.Findings = append(report.Findings, DiagnosticFinding{
		Check:    "Local Snapshots",
		Severity: SeverityInfo,
		Message:  fmt.Sprintf("%d local Time Machine %s on this disk — reclaimable if you want the space", len(snaps), noun),
		Detail:   strings.Join(snaps, "\n"),
	})
}
