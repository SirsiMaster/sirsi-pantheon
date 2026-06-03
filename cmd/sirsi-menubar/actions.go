package main

// actions.go — in-place menubar actions. The menubar was a launcher: every
// click osascript-opened a Terminal running `sirsi <cmd>`. This makes SAFE
// (non-destructive) actions actually EXECUTE in-process and surface the result,
// so the menubar acts instead of just informing (user's #1 complaint, 2026-06-03).
//
// Destructive actions (anubis clean, ra kill/deploy, guard watch) keep the
// Terminal/confirm path — Rule A1 forbids one-click destruction without a
// dry-run/confirm boundary.

import (
	"context"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"fyne.io/systray"
	"github.com/SirsiMaster/sirsi-pantheon/internal/notify"
)

// actionTimeout bounds an in-place action so a hung command can never wedge the
// menu item in a running state.
const actionTimeout = 90 * time.Second

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)

// runActionInPlace executes `sirsi <command>` off the UI thread, writes the
// result to the notify store (so it appears in Recent Activity), and gives the
// clicked item transient ⏳/✓/✗ feedback before restoring its title. Non-blocking:
// returns immediately; the menu stays responsive while the action runs.
func runActionInPlace(item *systray.MenuItem, origTitle, sirsiBin, command string, store *notify.Store) {
	if item == nil || sirsiBin == "" {
		return
	}
	item.SetTitle("⏳ " + origTitle)
	item.Disable()
	go func() {
		start := time.Now()
		ctx, cancel := context.WithTimeout(context.Background(), actionTimeout)
		defer cancel()
		out, err := exec.CommandContext(ctx, sirsiBin, strings.Fields(command)...).CombinedOutput()
		dur := time.Since(start)

		sev, icon := notify.SeveritySuccess, "✓"
		summary := firstMeaningfulLine(string(out))
		if ctx.Err() == context.DeadlineExceeded {
			sev, icon, summary = notify.SeverityWarning, "⏱", "timed out after "+actionTimeout.String()
		} else if err != nil {
			sev, icon = notify.SeverityError, "✗"
			if summary == "" {
				summary = err.Error()
			}
		}
		if summary == "" {
			summary = "done"
		}

		if store != nil {
			_ = store.Record(notify.Notification{
				Source:     origTitle,
				Action:     command,
				Severity:   sev,
				Summary:    truncate(summary, 140),
				Details:    truncate(stripANSI(string(out)), 4000),
				DurationMs: dur.Milliseconds(),
			})
		}
		item.SetTitle(icon + " " + origTitle)
		item.Enable()
		// Restore the original title after a beat so the menu reads normally again.
		time.AfterFunc(5*time.Second, func() { item.SetTitle(origTitle) })
	}()
}

// firstMeaningfulLine returns the first non-empty, ANSI-stripped line of output —
// the at-a-glance result for the notification summary.
func firstMeaningfulLine(s string) string {
	for _, line := range strings.Split(stripANSI(s), "\n") {
		if t := strings.TrimSpace(line); t != "" {
			return t
		}
	}
	return ""
}

func stripANSI(s string) string { return ansiRE.ReplaceAllString(s, "") }

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
