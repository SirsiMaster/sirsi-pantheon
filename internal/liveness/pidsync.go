package liveness

// pidsync.go — keeps gemma-server.pid pointed at the broker launchd is
// actually running (A32/ADR-040).
//
// launchd forks/respawns the broker directly; nothing else in the serving path
// ever rewrites this file after the initial launch. A stale entry doesn't just
// go unrecognized — a dead PID gets reused by an unrelated process, so
// guard.LoadBearingPIDs() ends up protecting the wrong process while the real
// broker is left killable by routine reclaim. Found via a false liveness alert
// (router item 20260809-060146): the broker was healthy the whole time, but
// the pidfile named a 6-hour-stale PID that macOS had already recycled.

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// gemmaBrokerLaunchdLabel mirrors setup.GemmaBrokerLabel. Duplicated as a
// literal rather than imported: internal/setup imports internal/router, and
// internal/router imports this package, so importing setup here would close
// an import cycle.
const gemmaBrokerLaunchdLabel = "ai.sirsi.gemma-broker"

// SyncGemmaPidFile writes launchd's live PID for the broker into
// ~/.sirsi/gemma-server.pid. Best-effort: any failure (launchctl unavailable,
// label not loaded, unparseable output) is silently skipped rather than
// surfaced, since callers use this as a side-effect on an already-successful
// health check, never as the source of truth for whether the broker is up.
func SyncGemmaPidFile(home string) {
	label := "gui/" + strconv.Itoa(os.Getuid()) + "/" + gemmaBrokerLaunchdLabel
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "launchctl", "print", label).Output()
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(out), "\n") {
		rest, ok := strings.CutPrefix(strings.TrimSpace(line), "pid = ")
		if !ok {
			continue
		}
		if pid, err := strconv.Atoi(strings.TrimSpace(rest)); err == nil && pid > 0 {
			_ = os.WriteFile(filepath.Join(home, ".sirsi", "gemma-server.pid"), []byte(strconv.Itoa(pid)), 0o644)
		}
		return
	}
}
