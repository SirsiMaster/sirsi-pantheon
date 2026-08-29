package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSNEOwnershipDetectsCanonicalAndLegacyDrift(t *testing.T) {
	originalHome := sneOwnershipHome
	originalExtract := sneOwnershipExtract
	originalLoaded := sneOwnershipLoaded
	t.Cleanup(func() {
		sneOwnershipHome = originalHome
		sneOwnershipExtract = originalExtract
		sneOwnershipLoaded = originalLoaded
	})

	home := t.TempDir()
	agents := filepath.Join(home, "Library", "LaunchAgents")
	if err := os.MkdirAll(agents, 0o755); err != nil {
		t.Fatal(err)
	}
	canonical := filepath.Join(agents, canonicalSNESupervisorLabel+".plist")
	legacy := filepath.Join(agents, "ai.sirsi.pantheon-sne-e2b.plist")
	for _, path := range []string{canonical, legacy} {
		if err := os.WriteFile(path, []byte("fixture"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	sneOwnershipHome = func() (string, error) { return home, nil }
	sneOwnershipExtract = func(path, key string) (string, error) {
		label := filepath.Base(path[:len(path)-len(filepath.Ext(path))])
		if key == "Label" {
			return label, nil
		}
		return "/signed/" + label, nil
	}
	sneOwnershipLoaded = func(label string) bool { return label == canonicalSNESupervisorLabel }

	report, err := inspectSNEOwnership()
	if err != nil {
		t.Fatal(err)
	}
	if report.State != "ownership-drift" || report.CanonicalCount != 1 || report.LegacyCount != 1 || report.LoadedCount != 1 || len(report.Records) != 2 || report.Recovery == "" {
		t.Fatalf("unexpected ownership report: %+v", report)
	}
}

func TestSNEOwnershipRepairRetiresLegacyWithReceipt(t *testing.T) {
	originalHome := sneOwnershipHome
	originalExtract := sneOwnershipExtract
	originalLoaded := sneOwnershipLoaded
	originalLaunchctl := sneOwnershipLaunchctl
	originalNow := sneOwnershipNow
	t.Cleanup(func() {
		sneOwnershipHome = originalHome
		sneOwnershipExtract = originalExtract
		sneOwnershipLoaded = originalLoaded
		sneOwnershipLaunchctl = originalLaunchctl
		sneOwnershipNow = originalNow
	})
	home := t.TempDir()
	agents := filepath.Join(home, "Library", "LaunchAgents")
	if err := os.MkdirAll(agents, 0o755); err != nil {
		t.Fatal(err)
	}
	canonical := filepath.Join(agents, canonicalSNESupervisorLabel+".plist")
	legacyLabel := "ai.sirsi.pantheon-sne-e2b"
	legacy := filepath.Join(agents, legacyLabel+".plist")
	for _, path := range []string{canonical, legacy} {
		if err := os.WriteFile(path, []byte(path), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	sneOwnershipHome = func() (string, error) { return home, nil }
	sneOwnershipExtract = func(path, key string) (string, error) {
		label := strings.TrimSuffix(filepath.Base(path), ".plist")
		if key == "Label" {
			return label, nil
		}
		return "/signed/" + label, nil
	}
	sneOwnershipLoaded = func(label string) bool { return label == canonicalSNESupervisorLabel }
	var calls [][]string
	sneOwnershipLaunchctl = func(args ...string) error {
		calls = append(calls, append([]string(nil), args...))
		return nil
	}
	sneOwnershipNow = func() time.Time { return time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC) }

	receipt, err := repairLegacySNEOwnership()
	if err != nil {
		t.Fatal(err)
	}
	if receipt.State != "accepted" || len(receipt.Retired) != 1 || receipt.Retired[0] != legacyLabel || receipt.PlistSHA256[legacyLabel] == "" {
		t.Fatalf("unexpected repair receipt: %+v", receipt)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatalf("legacy plist still exists: %v", err)
	}
	for _, name := range []string{legacyLabel + ".plist.backup", legacyLabel + ".plist.retired", "receipt.staged.json", "receipt.accepted.json"} {
		if _, err := os.Stat(filepath.Join(receipt.ReceiptDir, name)); err != nil {
			t.Fatalf("missing recovery artifact %s: %v", name, err)
		}
	}
	if len(calls) != 2 || calls[0][0] != "bootout" || calls[1][0] != "disable" {
		t.Fatalf("unexpected launchctl sequence: %#v", calls)
	}
}

func TestSNEOwnershipRepairRollsBackOnDisableFailure(t *testing.T) {
	originalHome := sneOwnershipHome
	originalExtract := sneOwnershipExtract
	originalLoaded := sneOwnershipLoaded
	originalLaunchctl := sneOwnershipLaunchctl
	t.Cleanup(func() {
		sneOwnershipHome = originalHome
		sneOwnershipExtract = originalExtract
		sneOwnershipLoaded = originalLoaded
		sneOwnershipLaunchctl = originalLaunchctl
	})
	home := t.TempDir()
	agents := filepath.Join(home, "Library", "LaunchAgents")
	if err := os.MkdirAll(agents, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, label := range []string{canonicalSNESupervisorLabel, "ai.sirsi.pantheon-sne-e2b"} {
		if err := os.WriteFile(filepath.Join(agents, label+".plist"), []byte(label), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	sneOwnershipHome = func() (string, error) { return home, nil }
	sneOwnershipExtract = func(path, key string) (string, error) {
		label := strings.TrimSuffix(filepath.Base(path), ".plist")
		if key == "Label" {
			return label, nil
		}
		return "/signed/" + label, nil
	}
	sneOwnershipLoaded = func(label string) bool { return label == canonicalSNESupervisorLabel }
	sneOwnershipLaunchctl = func(args ...string) error {
		if len(args) > 0 && args[0] == "disable" {
			return errors.New("denied")
		}
		return nil
	}

	if _, err := repairLegacySNEOwnership(); err == nil {
		t.Fatal("expected disable failure")
	}
	legacy := filepath.Join(agents, "ai.sirsi.pantheon-sne-e2b.plist")
	if _, err := os.Stat(legacy); err != nil {
		t.Fatalf("legacy plist was not preserved after failure: %v", err)
	}
}

func TestSNEOwnershipRepairRequiresConfirmation(t *testing.T) {
	command := newSNEOwnershipRepairCommand()
	if err := command.Execute(); err == nil {
		t.Fatal("expected unconfirmed repair to fail closed")
	}
}

func TestSNEOwnershipAcceptsOneCanonicalOwner(t *testing.T) {
	originalHome := sneOwnershipHome
	originalExtract := sneOwnershipExtract
	originalLoaded := sneOwnershipLoaded
	t.Cleanup(func() {
		sneOwnershipHome = originalHome
		sneOwnershipExtract = originalExtract
		sneOwnershipLoaded = originalLoaded
	})

	home := t.TempDir()
	agents := filepath.Join(home, "Library", "LaunchAgents")
	if err := os.MkdirAll(agents, 0o755); err != nil {
		t.Fatal(err)
	}
	plist := filepath.Join(agents, canonicalSNESupervisorLabel+".plist")
	if err := os.WriteFile(plist, []byte("fixture"), 0o644); err != nil {
		t.Fatal(err)
	}
	sneOwnershipHome = func() (string, error) { return home, nil }
	sneOwnershipExtract = func(_ string, key string) (string, error) {
		if key == "Label" {
			return canonicalSNESupervisorLabel, nil
		}
		return "/Applications/Pantheon.app/Contents/MacOS/sirsi-sne-supervisor", nil
	}
	sneOwnershipLoaded = func(string) bool { return true }

	report, err := inspectSNEOwnership()
	if err != nil {
		t.Fatal(err)
	}
	if report.State != "canonical" || report.CanonicalCount != 1 || report.LegacyCount != 0 || report.LoadedCount != 1 {
		t.Fatalf("unexpected canonical report: %+v", report)
	}
}

func TestSNEOwnershipReportsNotInstalled(t *testing.T) {
	originalHome := sneOwnershipHome
	t.Cleanup(func() { sneOwnershipHome = originalHome })
	sneOwnershipHome = func() (string, error) { return t.TempDir(), nil }

	report, err := inspectSNEOwnership()
	if err != nil {
		t.Fatal(err)
	}
	if report.State != "not-installed" || report.Recovery == "" || len(report.Records) != 0 {
		t.Fatalf("unexpected empty report: %+v", report)
	}
}
