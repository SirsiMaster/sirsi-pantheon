package guard

// 𓁵 Sekhmet's runaway-executor check (ADR-035).
//
// The 2026-07-03/04 incident: a build-worker LaunchAgent spawned a full
// headless agentic session for EVERY open router item and never closed any —
// 19,195 sessions, 0 completed. Its timeout-killed `go test` runs orphaned
// 7,689 go-build* + 7,681 sirsi-integration-* trees (~1.3 TB in 36h), filling
// the disk to 100%. Nothing on the host ALARMED while it happened; the disease
// ran silent until ENOSPC. This check is the host-level backstop that makes
// that impossible: it watches the two live signatures of the disease and, per
// ADR-033, carries a real lever (`sirsi router quarantine-worker`).
//
// Surfaces canon: the finding alarms ONLY on a current, fixable condition —
// quarantining the worker stops the spawner NOW, and the artifact backlog
// drains via the hourly sweep reaper (PR #161).

import (
	"fmt"
	"strings"

	"github.com/SirsiMaster/sirsi-pantheon/internal/platform"
)

// Thresholds sit far above healthy peaks and unmistakably below incident
// scale. Healthy: the owner's ONE interactive session plus a handful of
// short-lived headless calls; a parallel `go test ./...` makes tens of build
// trees. The incident: dozens of concurrent sessions, thousands of trees.
const (
	runawaySessionsWarn     = 6
	runawaySessionsCritical = 12
	runawayTreesWarn        = 300
	runawayTreesCritical    = 1500
)

// checkRunawayExecutor emits the "Runaway Executor" finding.
func checkRunawayExecutor(p platform.Platform, report *DoctorReport) {
	if p.Name() != "darwin" && p.Name() != "mock" {
		return
	}
	sessions := countHeadlessAgentSessions(p)
	trees := countFreshBuildTrees(p)

	f := DiagnosticFinding{Check: "Runaway Executor"}
	switch {
	case sessions >= runawaySessionsCritical || trees >= runawayTreesCritical:
		f.Severity = SeverityCritical
	case sessions >= runawaySessionsWarn || trees >= runawayTreesWarn:
		f.Severity = SeverityWarn
	default:
		f.Severity = SeverityOK
		f.Message = fmt.Sprintf("No runaway executor — %d headless agent session(s), %d fresh build tree(s)", sessions, trees)
	}
	if f.Severity >= SeverityWarn {
		f.Message = fmt.Sprintf("Possible runaway executor — %d headless agent sessions and %d build trees under 24h old", sessions, trees)
		f.Detail = "An automation is spawning agent sessions or build runs faster than it finishes them " +
			"(the 2026-07-04 disease: 19,195 sessions, 1.3 TB of build trees, disk full). " +
			"The fix stops only the claude build-worker LaunchAgents; wake-loop watchers and the supervisor are never touched."
	}
	report.Findings = append(report.Findings, f)
}

// countHeadlessAgentSessions counts live headless claude sessions (`claude -p`
// / `claude --print`). The owner's interactive session has no print flag and
// is never counted.
func countHeadlessAgentSessions(p platform.Platform) int {
	out, err := p.Command("ps", "-axo", "args")
	if err != nil {
		return 0
	}
	count := 0
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		base := fields[0]
		if idx := strings.LastIndex(base, "/"); idx >= 0 {
			base = base[idx+1:]
		}
		if base != "claude" {
			continue
		}
		for _, arg := range fields[1:] {
			if arg == "-p" || arg == "--print" {
				count++
				break
			}
		}
	}
	return count
}

// countFreshBuildTrees counts go-build*/sirsi-integration-* dirs younger than
// 24h in the darwin per-user temp dir — the hourly reaper's deliberate blind
// window (it only reaps >24h so live builds are safe). Hundreds of YOUNG trees
// means something is churning builds right now.
func countFreshBuildTrees(p platform.Platform) int {
	tmpOut, err := p.Command("getconf", "DARWIN_USER_TEMP_DIR")
	if err != nil {
		return 0
	}
	tmp := strings.TrimSpace(string(tmpOut))
	if tmp == "" {
		return 0
	}
	out, err := p.Command("find", tmp, "-maxdepth", "1",
		"(", "-name", "go-build*", "-o", "-name", "sirsi-integration-*", ")",
		"-mmin", "-1440")
	if err != nil {
		return 0
	}
	count := 0
	for _, line := range strings.Split(string(out), "\n") {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count
}
