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

var reclaimRE = regexp.MustCompile(`\(([0-9.]+\s*[KMGTP]?B)\)`)

// runCleanPreview runs the SAFE dry-run (`anubis clean`, --dry-run defaults true)
// and, when there is something to reclaim, ARMS the confirm item with the amount
// (auto-disarming after a window). This is the first half of the in-app clean
// flow — preview only, nothing is ever deleted here (Rule A1: dry-run available).
func runCleanPreview(judge, confirm *systray.MenuItem, judgeTitle, sirsiBin string, store *notify.Store) {
	if judge == nil || confirm == nil || sirsiBin == "" {
		return
	}
	judge.SetTitle("⏳ " + judgeTitle)
	judge.Disable()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), actionTimeout)
		defer cancel()
		out, err := exec.CommandContext(ctx, sirsiBin, "anubis", "clean").CombinedOutput()
		text := stripANSI(string(out))
		judge.SetTitle(judgeTitle)
		judge.Enable()

		summary := firstLineContaining(text, "would be cleaned")
		if err != nil || summary == "" {
			// Nothing to clean, or the preview failed — record and stay disarmed.
			s := firstMeaningfulLine(text)
			if s == "" {
				s = "nothing to clean"
			}
			sev := notify.SeverityInfo
			if err != nil {
				sev = notify.SeverityError
			}
			recordNotify(store, "Clean Waste", "anubis clean (preview)", sev, s, text)
			confirm.Hide()
			return
		}
		recordNotify(store, "Clean Waste", "anubis clean (preview)", notify.SeverityInfo, summary, text)
		amount := "items"
		if m := reclaimRE.FindStringSubmatch(summary); len(m) == 2 {
			amount = m[1]
		}
		confirm.SetTitle("  ✓ Confirm Clean — " + amount + " → Trash")
		confirm.Show()
		// Auto-disarm: never leave a standing one-click delete in the menu.
		time.AfterFunc(25*time.Second, func() { confirm.Hide() })
	}()
}

// runCleanApply executes the real clean (`anubis clean --confirm`), feeding "y"
// to the interactive [y/N] prompt — the user's click on the (armed, amount-
// labeled) confirm item IS that yes. Trash-first (recoverable), protected paths
// enforced in the engine. Second half of the two-click flow.
func runCleanApply(confirm *systray.MenuItem, sirsiBin string, store *notify.Store) {
	if confirm == nil || sirsiBin == "" {
		return
	}
	confirm.SetTitle("  ⏳ Cleaning…")
	confirm.Disable()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, sirsiBin, "anubis", "clean", "--confirm")
		cmd.Stdin = strings.NewReader("y\n") // the confirm-click is the [y/N] yes
		out, err := cmd.CombinedOutput()
		text := stripANSI(string(out))
		sev, summary := notify.SeveritySuccess, firstMeaningfulLine(text)
		if err != nil {
			sev = notify.SeverityError
			if summary == "" {
				summary = err.Error()
			}
		}
		if summary == "" {
			summary = "cleaned — items moved to Trash"
		}
		recordNotify(store, "Clean Waste", "anubis clean --confirm", sev, summary, text)
		confirm.Enable()
		confirm.Hide()
	}()
}

// recordNotify writes a result to the notify store (→ Recent Activity), if any.
func recordNotify(store *notify.Store, source, action, severity, summary, details string) {
	if store == nil {
		return
	}
	_ = store.Record(notify.Notification{
		Source:   source,
		Action:   action,
		Severity: severity,
		Summary:  truncate(summary, 140),
		Details:  truncate(details, 4000),
	})
}

// firstLineContaining returns the first ANSI-stripped line containing substr.
func firstLineContaining(s, substr string) string {
	for _, line := range strings.Split(stripANSI(s), "\n") {
		if strings.Contains(line, substr) {
			return strings.TrimSpace(line)
		}
	}
	return ""
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
