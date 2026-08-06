package router

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// WorkerLabelPrefix identifies claude build-worker LaunchAgents — the executor
// tier that ran away on 2026-07-03/04 (19,195 agentic sessions spawned, 0
// closed; an 11,564-item escalation flood; 1.3 TB of orphaned build trees in
// 36h). Quarantine targets ONLY this prefix: the wake-loop watchers
// (ai.sirsi.router.wake.*) and the router supervisor (ai.sirsi.horus.agent-router)
// are the fabric's heartbeat and are never touched — the incident's watchers
// were healthy, it was the executor that needed a kill switch (ADR-035).
const WorkerLabelPrefix = "ai.sirsi.claude-worker."

// quarantineSuffix is appended to a worker plist's filename so launchd (which
// loads only *.plist) can never resurrect it at login, while the owner can
// rename it back by hand once the Dispatch Contract acceptance bar passes
// (PRD ROUTER_V2_DURABLE_DISPATCH §2b).
const quarantineSuffix = ".quarantined"

// QuarantineResult reports exactly what QuarantineWorkers did (or, in dry-run,
// would have done). Zero-length fields mean nothing matched — a clean host.
type QuarantineResult struct {
	BootedOut   []string // launchd labels booted out of the gui domain
	Quarantined []string // plist paths renamed *.plist → *.plist.quarantined
	DryRun      bool
}

// LaunchctlRunner abstracts launchctl for testability (Rule A16). nil means
// "shell the real launchctl".
type LaunchctlRunner func(args ...string) (string, error)

func defaultLaunchctlRunner(args ...string) (string, error) {
	// Bounded for the same reason as runLaunchctl: a kill switch that blocks on
	// launchd is not a kill switch.
	ctx, cancel := context.WithTimeout(context.Background(), launchctlTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "launchctl", args...).CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return string(out), fmt.Errorf("launchctl %s: no response in %s",
			strings.Join(args, " "), launchctlTimeout)
	}
	return string(out), err
}

// QuarantineWorkers is 𓁵 Sekhmet's kill switch for the runaway-executor class
// (ADR-035): it stops every claude build-worker LaunchAgent NOW (bootout) and
// keeps it stopped across logins (plist rename). Idempotent — a second run
// finds nothing and reports a clean host. It is the remediation behind the
// doctor's "Runaway Executor" finding, and the OFF state it produces is the
// one the Dispatch Contract requires until the Phase-2 acceptance bar passes.
func QuarantineWorkers(dryRun bool, run LaunchctlRunner) (*QuarantineResult, error) {
	if run == nil {
		run = defaultLaunchctlRunner
	}
	res := &QuarantineResult{DryRun: dryRun}

	// 1. Live jobs: `launchctl list` emits "PID\tStatus\tLabel" per line; the
	// label is the last field. Bootout stops the job (and its KeepAlive) now.
	if out, err := run("list"); err == nil {
		for _, line := range strings.Split(out, "\n") {
			fields := strings.Fields(line)
			if len(fields) < 3 {
				continue
			}
			label := fields[len(fields)-1]
			if !strings.HasPrefix(label, WorkerLabelPrefix) {
				continue
			}
			if !dryRun {
				// Best-effort: the job may race away between list and bootout.
				_, _ = run("bootout", fmt.Sprintf("gui/%d/%s", os.Getuid(), label))
			}
			res.BootedOut = append(res.BootedOut, label)
		}
	}

	// 2. Durability: rename the plists so login/RunAtLoad cannot resurrect the
	// worker. Rename (not delete) keeps the definition recoverable by hand.
	matches, err := filepath.Glob(filepath.Join(launchAgentsDir(), WorkerLabelPrefix+"*.plist"))
	if err != nil {
		return res, fmt.Errorf("scan LaunchAgents: %w", err)
	}
	for _, p := range matches {
		if !dryRun {
			if err := os.Rename(p, p+quarantineSuffix); err != nil {
				return res, fmt.Errorf("quarantine %s: %w", p, err)
			}
		}
		res.Quarantined = append(res.Quarantined, p)
	}
	return res, nil
}
