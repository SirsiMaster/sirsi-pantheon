package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// fleetCheckWarning runs scripts/fleet-check.sh (relay, router ping,
// outbox/parked, Stack Lab key — ra-fleet-check-into-repo-20261022) and
// returns its output when it reports RED. Best-effort: a missing script or
// missing bash is reported, never a doctor crash, since this host's own
// fleet health is informational, not load-bearing for the rest of the report.
func fleetCheckWarning(repoRoot string) string {
	script := filepath.Join(repoRoot, "scripts", "fleet-check.sh")
	if _, err := os.Stat(script); err != nil {
		return fmt.Sprintf("fleet-check script unavailable; health UNKNOWN: %v", err)
	}
	// The script makes two real network calls (a router round trip and a Stack Lab
	// read), each bounded at 20-25 s inside it; this bound only has to outlast both.
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", script, os.Getenv("SIRSI_AGENT_ID"))
	out, err := cmd.CombinedOutput()
	if err == nil {
		return ""
	}
	if ctx.Err() != nil {
		// A timeout is unknown, not a verdict: say so instead of printing a partial RED.
		return "fleet-check did not finish within 60 s, so this host's router/Stack Lab health is UNKNOWN, not green"
	}
	if text := strings.TrimSpace(string(out)); text != "" {
		return text
	}
	return fmt.Sprintf("fleet-check execution unavailable; health UNKNOWN: %v", err)
}
