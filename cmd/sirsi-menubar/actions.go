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
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"fyne.io/systray"
	"github.com/SirsiMaster/sirsi-pantheon/internal/jackal"
	"github.com/SirsiMaster/sirsi-pantheon/internal/notify"
)

// openFullDiskAccessPane opens System Settings to the Full Disk Access pane and
// records which binary to add. macOS cannot self-grant FDA (TCC security model) —
// this makes the one required user action a single click + an explicit path.
func openFullDiskAccessPane(store *notify.Store) {
	_ = exec.Command("open", "x-apple.systempreferences:com.apple.preference.security?Privacy_AllFiles").Start()
	self, _ := os.Executable()
	recordNotify(store, "Full Disk Access", "grant", notify.SeverityWarning,
		"Add this to Full Disk Access, then relaunch: "+self,
		"System Settings → Privacy & Security → Full Disk Access → + → add the menubar binary above (and the sirsi CLI). Required for Sirsi to see and clean protected folders.")
}

// actionTimeout bounds an in-place action so a hung command can never wedge the
// menu item in a running state.
const actionTimeout = 90 * time.Second

// confirmArmWindow is how long the "✓ Confirm Clean" item stays armed after a
// preview. The macOS menu closes the instant "Clean Waste…" is clicked, so the
// user must REOPEN the menu to see (and click) the armed Confirm item — 25s was
// far too short for that round-trip and made clean feel like a dead click. A
// banner toast points the user back to the menu; this window must outlive that
// human round-trip while still never leaving a standing one-click delete forever.
const confirmArmWindow = 2 * time.Minute

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)

var reclaimRE = regexp.MustCompile(`\(([0-9.]+\s*[KMGTP]?B)\)`)

// runCleanPreview runs the SAFE dry-run (`anubis clean`, --dry-run defaults true)
// and, when there is something to reclaim, ARMS the confirm item with the amount
// (auto-disarming after a window). This is the first half of the in-app clean
// flow — preview only, nothing is ever deleted here (Rule A1: dry-run available).
func runCleanPreview(judge, confirm *systray.MenuItem, judgeTitle, sirsiBin string, store *notify.Store, rr *resultRow) {
	if judge == nil || confirm == nil || sirsiBin == "" {
		return
	}
	judge.SetTitle("⏳ " + judgeTitle)
	judge.Disable()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), actionTimeout)
		defer cancel()
		// SAFE-ONLY (no --include-caution): a one-click menubar surface must only
		// ever touch clearly-regenerable, trash-first items (node_modules, caches,
		// build artifacts). Caution-tier items (app remnants, judgment calls) are
		// deliberately excluded — they need explicit review, not a blind one-click,
		// and stay reachable via `sirsi anubis clean --include-caution` in the CLI.
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
			icon := "•"
			if err != nil {
				sev, icon = notify.SeverityError, "✗"
			}
			recordNotify(store, "Clean Waste", "anubis clean (preview)", sev, s, text)
			if rr != nil {
				rr.set("Clean Waste (preview)", icon, s, text)
			}
			// Always give the click VISIBLE feedback — the menu has already
			// closed, so without a banner the user sees nothing at all.
			if err != nil {
				notify.Toast("Sirsi — Clean Waste", "Couldn't preview: "+s)
			} else {
				notify.Toast("Sirsi — Clean Waste", "Nothing to clean right now.")
			}
			confirm.Hide()
			return
		}
		recordNotify(store, "Clean Waste", "anubis clean (preview)", notify.SeverityInfo, summary, text)
		if rr != nil {
			rr.set("Clean Waste (preview)", "•", summary, text) // visible, clickable detail
		}
		amount := "items"
		if m := reclaimRE.FindStringSubmatch(summary); len(m) == 2 {
			amount = m[1]
		}
		confirm.SetTitle("  ✓ Confirm Clean — " + amount + " → Trash")
		confirm.Show()
		// Tell the user what to do next — the menu closed on their click, so the
		// armed Confirm item is invisible until they reopen. Without this banner,
		// the preview's only effect is an unseen menu item = "nothing happened".
		// Point them at the full itemized manifest first — consent needs visibility.
		notify.Toast("Sirsi — "+amount+" safe to clean",
			"Reopen → Anubis → 'Review what will be cleaned' to see the list, then ✓ Confirm Clean.")
		// Auto-disarm: never leave a standing one-click delete in the menu.
		time.AfterFunc(confirmArmWindow, func() { confirm.Hide() })
	}()
}

// runCleanApply executes the real clean feeding "y" to the interactive [y/N]
// prompt — the user's click on the (armed, amount-labeled) confirm item IS that
// yes. Trash-first (recoverable), protected paths enforced in the engine. Second
// half of the two-click flow.
//
// Uses `anubis clean --dry-run=false` (NOT `--confirm`): `--confirm` does double
// duty in anubis.go — it both applies AND widens the target from safe to
// safe+caution, so confirming would move MORE than the safe-only preview showed.
// `--dry-run=false` applies the SAME safe-only set the preview displayed — the
// amount shown is exactly the amount trashed (Rule A1: preview == apply).
func runCleanApply(confirm *systray.MenuItem, sirsiBin string, store *notify.Store, rr *resultRow) {
	if confirm == nil || sirsiBin == "" {
		return
	}
	confirm.SetTitle("  ⏳ Cleaning…")
	confirm.Disable()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		// SAFE-ONLY apply — NO --include-caution, matching the safe-only preview
		// so the set trashed is exactly the set shown (Rule A1: preview == apply).
		// Trash-first (recoverable); protected system paths enforced in
		// internal/cleaner/safety.go. Caution-tier items are never one-click
		// deleted here — they require deliberate review (CLI --include-caution).
		cmd := exec.CommandContext(ctx, sirsiBin, "anubis", "clean", "--dry-run=false")
		cmd.Stdin = strings.NewReader("y\n") // the confirm-click is the [y/N] yes
		out, err := cmd.CombinedOutput()
		text := stripANSI(string(out))
		sev, icon, summary := notify.SeveritySuccess, "✓", firstMeaningfulLine(text)
		if err != nil {
			sev, icon = notify.SeverityError, "✗"
			if summary == "" {
				summary = err.Error()
			}
		}
		if summary == "" {
			summary = "cleaned — items moved to Trash"
		}
		recordNotify(store, "Clean Waste", "anubis clean --dry-run=false", sev, summary, text)
		if rr != nil {
			rr.set("Clean Waste", icon, summary, text)
		}
		// Visible completion feedback — close the loop the user started.
		if err != nil {
			notify.Toast("Sirsi — Clean failed", summary)
		} else {
			notify.Toast("Sirsi — Clean complete", summary)
		}
		confirm.Enable()
		confirm.Hide()
		// Refresh the waste title immediately so the notice reflects the clean
		// (no waiting for the 4h tick) — this is what makes "8.4 GB → Clean"
		// happen live in the demo.
		rescanWaste(context.Background())
	}()
}

// reviewCleanList opens the COMPLETE itemized manifest of a one-click clean —
// every SAFE item that WILL be moved to Trash (full path + size) and every
// CAUTION item that will NOT be touched — read from the same persisted scan the
// cleaner uses, so the list is exactly what Confirm Clean will move. This is the
// visibility half of consent: a total with no manifest is not consent (Rule A1).
func reviewCleanList() {
	ps, err := jackal.LoadLatest()
	if err != nil || ps == nil || len(ps.Findings) == 0 {
		notify.Toast("Sirsi — Review", "No scan results yet. Run Anubis → Scan for Waste first.")
		return
	}
	var safe, caution []jackal.PersistedFinding
	var safeBytes, cautionBytes int64
	for _, f := range ps.Findings {
		switch f.Severity {
		case jackal.SeveritySafe:
			safe = append(safe, f)
			safeBytes += f.SizeBytes
		case jackal.SeverityCaution:
			caution = append(caution, f)
			cautionBytes += f.SizeBytes
		}
	}
	sort.Slice(safe, func(i, j int) bool { return safe[i].SizeBytes > safe[j].SizeBytes })
	sort.Slice(caution, func(i, j int) bool { return caution[i].SizeBytes > caution[j].SizeBytes })

	var b strings.Builder
	fmt.Fprintf(&b, "Based on the scan from %s.\n\n", ps.Timestamp.Format("Mon 3:04 PM"))
	fmt.Fprintf(&b, "■ WILL BE MOVED TO TRASH — what 'Confirm Clean' does (one click)\n")
	fmt.Fprintf(&b, "  %d safe items · %s · regenerable (caches, node_modules, build artifacts)\n", len(safe), jackal.FormatSize(safeBytes))
	fmt.Fprintf(&b, "  Recoverable from Trash. Protected system paths are never touched.\n\n")
	for _, f := range safe {
		fmt.Fprintf(&b, "  %10s   %s\n", jackal.FormatSize(f.SizeBytes), f.Path)
	}
	if len(safe) == 0 {
		fmt.Fprintf(&b, "  (nothing safe to clean right now)\n")
	}
	fmt.Fprintf(&b, "\n\n□ EXCLUDED — NOT cleaned by the menubar (needs your review)\n")
	fmt.Fprintf(&b, "  %d caution items · %s · app remnants & judgment calls\n", len(caution), jackal.FormatSize(cautionBytes))
	fmt.Fprintf(&b, "  Clean these deliberately in a terminal:  sirsi anubis clean --include-caution --confirm\n\n")
	for _, f := range caution {
		fmt.Fprintf(&b, "  %10s   %s\n", jackal.FormatSize(f.SizeBytes), f.Path)
	}
	openDetail("What Clean Waste will do", b.String())
}

// ── Empty Trash (permanent delete) ─────────────────────────────────────────

// trashInfo returns the number of items and total byte count in ~/.Trash.
// Returns 0,0 on any error (permission denied, no home dir, etc.).
func trashInfo() (items int, size int64) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	trashDir := filepath.Join(home, ".Trash")
	entries, err := os.ReadDir(trashDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.Name() == ".DS_Store" {
			continue
		}
		items++
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.IsDir() {
			size += dirSize(trashDir, e.Name())
		} else {
			size += info.Size()
		}
	}
	return
}

// dirSize returns the recursive byte count of dir/name, ignoring errors.
func dirSize(parent, name string) int64 {
	var total int64
	_ = filepath.Walk(filepath.Join(parent, name), func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	return total
}

// runEmptyTrashPreview checks what is in ~/.Trash and, if non-empty, arms the
// permanent-delete confirm item. This is the first half of the two-click
// empty-trash flow (Rule A1: always preview before permanent deletion).
func runEmptyTrashPreview(preview, confirm *systray.MenuItem, store *notify.Store, rr *resultRow) {
	if preview == nil || confirm == nil {
		return
	}
	preview.SetTitle("⏳ Empty Trash…")
	preview.Disable()
	go func() {
		n, sz := trashInfo()
		preview.SetTitle("Empty Trash…")
		preview.Enable()

		if n == 0 {
			recordNotify(store, "Empty Trash", "preview", notify.SeverityInfo, "Trash is already empty", "")
			if rr != nil {
				rr.set("Empty Trash", "•", "Trash is already empty", "")
			}
			notify.Toast("Sirsi — Empty Trash", "Trash is already empty.")
			confirm.Hide()
			return
		}

		label := fmt.Sprintf("%d item", n)
		if n != 1 {
			label += "s"
		}
		label += " · " + formatTrashSize(sz)
		recordNotify(store, "Empty Trash", "preview", notify.SeverityWarning,
			"Trash contains "+label+" (permanent delete pending confirmation)", "")
		if rr != nil {
			rr.set("Empty Trash (preview)", "⚠", label+" in Trash — confirm to delete permanently", "")
		}
		notify.Toast("Sirsi — Trash contains "+label,
			"Reopen → Anubis → '⚠ Delete permanently' to permanently delete. This cannot be undone.")
		confirm.SetTitle("  ⚠ Delete permanently — " + label)
		confirm.Show()
		// Auto-disarm: never leave a standing one-click permanent delete in the menu.
		// ponytail: shorter than confirmArmWindow so a permanent-delete confirm ages out faster.
		time.AfterFunc(90*time.Second, func() { confirm.Hide() })
	}()
}

// runEmptyTrashApply permanently deletes everything in ~/.Trash. This is the
// second half of the two-click flow — the user's click on the armed confirm
// item is their explicit consent (Rule A1: confirm before permanent deletion).
func runEmptyTrashApply(confirm *systray.MenuItem, store *notify.Store, rr *resultRow) {
	if confirm == nil {
		return
	}
	confirm.SetTitle("  ⏳ Deleting permanently…")
	confirm.Disable()
	go func() {
		home, err := os.UserHomeDir()
		if err != nil {
			confirm.SetTitle("  ⚠ Delete permanently — error")
			confirm.Enable()
			return
		}
		trashDir := filepath.Join(home, ".Trash")
		entries, err := os.ReadDir(trashDir)
		if err != nil {
			confirm.Enable()
			confirm.Hide()
			return
		}

		var freed int64
		var errs []string
		for _, e := range entries {
			if e.Name() == ".DS_Store" {
				continue
			}
			p := filepath.Join(trashDir, e.Name())
			info, _ := e.Info()
			var itemSize int64
			if info != nil {
				if info.IsDir() {
					itemSize = dirSize(trashDir, e.Name())
				} else {
					itemSize = info.Size()
				}
			}
			var rmErr error
			if info != nil && info.IsDir() {
				rmErr = os.RemoveAll(p)
			} else {
				rmErr = os.Remove(p)
			}
			if rmErr != nil {
				errs = append(errs, e.Name()+": "+rmErr.Error())
			} else {
				freed += itemSize
			}
		}

		sev, icon, summary := notify.SeveritySuccess, "✓", "Trash emptied — "+formatTrashSize(freed)+" deleted permanently"
		if len(errs) > 0 {
			sev, icon = notify.SeverityError, "✗"
			summary = fmt.Sprintf("%d error(s): %s", len(errs), strings.Join(errs[:min(3, len(errs))], "; "))
		}
		recordNotify(store, "Empty Trash", "delete permanently", sev, summary, strings.Join(errs, "\n"))
		if rr != nil {
			rr.set("Empty Trash", icon, summary, strings.Join(errs, "\n"))
		}
		if len(errs) > 0 {
			notify.Toast("Sirsi — Empty Trash error", summary)
		} else {
			notify.Toast("Sirsi — Trash emptied", summary)
		}
		confirm.Enable()
		confirm.Hide()
	}()
}

// formatTrashSize formats bytes as a human-readable size string.
func formatTrashSize(b int64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%d KB", b>>10)
	default:
		return fmt.Sprintf("%d B", b)
	}
}

// min returns the smaller of two ints.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
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
