package router

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Loading a wake LaunchAgent, and then CHECKING that launchd agrees.
//
// `wake-install` used to write the plist and print "Load it: launchctl load -w
// <path>" — leaving the load to a human who, in a headless or scripted run, was
// never there. On the idempotent path it printed "already installed (no change)"
// and did not even print that hint.
//
// The result: a file on disk was reported as an armed watcher. Measured
// 2026-08-07 — 18 lanes reported installed, launchd had loaded NONE of them, and
// the live count sat at 5/26 while every surface said the lanes were armed. That
// is the whole "NO WATCHER" saga: the board was right and the tool was wrong.
//
// So "installed" must mean LOADED, and must be verified by asking launchd rather
// than by having written a file.

// wakeLabel is the launchd label for an agent's wake LaunchAgent.
func wakeLabel(agentID string) string { return "ai.sirsi.router.wake." + agentID }

// WakeAgentLoaded asks launchd whether the wake job for agentID exists in the
// user's GUI domain. This is the ONLY honest source for "is it armed" — the
// presence of a plist says nothing about whether launchd ever read it.
func WakeAgentLoaded(agentID string) bool {
	out, err := exec.Command("launchctl", "print",
		fmt.Sprintf("gui/%d/%s", os.Getuid(), wakeLabel(agentID))).CombinedOutput()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "state = ")
}

// LoadWakeAgent bootstraps the plist into the GUI domain and then verifies the
// job is present. Returns (loaded, detail).
//
// An "already bootstrapped" error is NOT a failure — it means the job is there,
// which is the state the caller wants. Everything else is reported with the
// launchctl text, because the caller's next move depends on which failure it hit
// (a disabled label needs `launchctl enable`; an I/O error usually needs a
// bootout first). Verification runs regardless of what bootstrap said, so a
// bootstrap that silently no-ops cannot be mistaken for success.
func LoadWakeAgent(agentID, plistPath string) (bool, string) {
	out, err := exec.Command("launchctl", "bootstrap",
		fmt.Sprintf("gui/%d", os.Getuid()), plistPath).CombinedOutput()
	detail := strings.TrimSpace(string(out))
	if WakeAgentLoaded(agentID) {
		return true, ""
	}
	if err == nil {
		return false, "launchctl bootstrap reported success but the job is absent from the domain"
	}
	if detail == "" {
		detail = err.Error()
	}
	return false, detail
}
