package router

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func writeAgentPlist(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("<plist/>"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestKickstartDeadLabels(t *testing.T) {
	dir := t.TempDir()
	writeAgentPlist(t, dir, "ai.sirsi.gemma.plist")            // dead → revive
	writeAgentPlist(t, dir, "ai.sirsi.pantheon.plist")         // loaded → untouched
	writeAgentPlist(t, dir, "actions.runner.X.m5-sirsi.plist") // dead runner → revive
	writeAgentPlist(t, dir, "ai.sirsi.gemma.plist.bak-1")      // backup → skipped
	writeAgentPlist(t, dir, "ai.sirsi.worker.plist.quarantined")
	writeAgentPlist(t, dir, "com.other.thing.plist") // foreign → skipped

	var bootstrapped []string
	deps := launchdDeps{
		listLabels: func() (map[string]bool, error) {
			return map[string]bool{"ai.sirsi.pantheon": true}, nil
		},
		bootstrapPlist: func(p string) error {
			bootstrapped = append(bootstrapped, filepath.Base(p))
			return nil
		},
		uid: func() int { return 501 },
	}
	revived, err := KickstartDeadLabels(dir, deps)
	if err != nil {
		t.Fatal(err)
	}
	if len(revived) != 2 {
		t.Fatalf("want 2 revived, got %v", revived)
	}
	want := map[string]bool{"ai.sirsi.gemma": true, "actions.runner.X.m5-sirsi": true}
	for _, label := range revived {
		if !want[label] {
			t.Errorf("unexpected revival %q", label)
		}
	}
	if len(bootstrapped) != 2 {
		t.Errorf("bootstrap called %d times: %v", len(bootstrapped), bootstrapped)
	}
}

func TestKickstartBootstrapFailureIsIsolatedAndReported(t *testing.T) {
	dir := t.TempDir()
	writeAgentPlist(t, dir, "ai.sirsi.a.plist")
	writeAgentPlist(t, dir, "ai.sirsi.b.plist")
	deps := launchdDeps{
		listLabels: func() (map[string]bool, error) { return map[string]bool{}, nil },
		bootstrapPlist: func(p string) error {
			if filepath.Base(p) == "ai.sirsi.a.plist" {
				return errors.New("boom")
			}
			return nil
		},
		uid: func() int { return 501 },
	}
	revived, err := KickstartDeadLabels(dir, deps)
	if err == nil {
		t.Error("first failure must be reported")
	}
	if len(revived) != 1 || revived[0] != "ai.sirsi.b" {
		t.Errorf("failure must not stop the sweep: got %v", revived)
	}
}

func TestKickstartMissingDirIsQuiet(t *testing.T) {
	revived, err := KickstartDeadLabels(filepath.Join(t.TempDir(), "nope"), launchdDeps{
		listLabels: func() (map[string]bool, error) { return nil, nil },
	})
	if err != nil || revived != nil {
		t.Fatalf("missing dir: %v %v", revived, err)
	}
}

// TestKickstartDisabledPlusUnloaded verifies the 2026-07-31 fabric-loss class:
// a label disabled in the override DB AND absent from launchctl list requires
// enable before bootstrap — bootstrap alone silently fails on a disabled label.
func TestKickstartDisabledPlusUnloaded(t *testing.T) {
	dir := t.TempDir()
	writeAgentPlist(t, dir, "ai.sirsi.disabled-label.plist") // disabled+unloaded → enable then bootstrap
	writeAgentPlist(t, dir, "ai.sirsi.normal-label.plist")   // not disabled → bootstrap only

	var enabledLabels, bootstrapped []string
	deps := launchdDeps{
		listLabels: func() (map[string]bool, error) {
			return map[string]bool{}, nil // both missing from launchd
		},
		disabledLabels: func() (map[string]bool, error) {
			return map[string]bool{"ai.sirsi.disabled-label": true}, nil
		},
		enableLabel: func(domain, label string) error {
			enabledLabels = append(enabledLabels, label)
			return nil
		},
		bootstrapPlist: func(p string) error {
			bootstrapped = append(bootstrapped, filepath.Base(p))
			return nil
		},
		uid: func() int { return 501 },
	}
	revived, err := KickstartDeadLabels(dir, deps)
	if err != nil {
		t.Fatal(err)
	}
	if len(revived) != 2 {
		t.Fatalf("want 2 revived, got %v", revived)
	}
	// Enable must be called for the disabled label, not the normal one.
	if len(enabledLabels) != 1 || enabledLabels[0] != "ai.sirsi.disabled-label" {
		t.Errorf("enable called on wrong labels: %v", enabledLabels)
	}
	if len(bootstrapped) != 2 {
		t.Errorf("bootstrap called %d times: %v", len(bootstrapped), bootstrapped)
	}
}

// TestKickstartHonorsGemmaQuarantine verifies the codex-inference-flagged gap
// against PR #611: RunGemmaLivenessDuty checking the quarantine marker was not
// enough, because this duty can still revive the broker's plist directly from
// disk. A quarantined broker's labels must never be silently rebootstrapped;
// unrelated labels must still revive normally.
func TestKickstartHonorsGemmaQuarantine(t *testing.T) {
	dir := t.TempDir()
	writeAgentPlist(t, dir, "ai.sirsi.gemma-broker.plist") // quarantined → must NOT revive
	writeAgentPlist(t, dir, "ai.sirsi.gemma-worker.plist") // quarantined → must NOT revive
	writeAgentPlist(t, dir, "ai.sirsi.pantheon.plist")     // unrelated → must still revive

	var bootstrapped []string
	deps := launchdDeps{
		listLabels: func() (map[string]bool, error) { return map[string]bool{}, nil },
		bootstrapPlist: func(p string) error {
			bootstrapped = append(bootstrapped, filepath.Base(p))
			return nil
		},
		uid:           func() int { return 501 },
		isQuarantined: func() bool { return true },
	}
	revived, err := KickstartDeadLabels(dir, deps)
	if err != nil {
		t.Fatal(err)
	}
	if len(revived) != 1 || revived[0] != "ai.sirsi.pantheon" {
		t.Fatalf("quarantine must block only the gemma labels: got %v", revived)
	}
	for _, b := range bootstrapped {
		if b == "ai.sirsi.gemma-broker.plist" || b == "ai.sirsi.gemma-worker.plist" {
			t.Fatalf("bootstrapped a quarantined label: %v", bootstrapped)
		}
	}
}

// TestKickstartRevivesGemmaWhenNotQuarantined is the other direction: with no
// quarantine in effect, the broker's labels revive exactly like any other dead
// label — the guard must not become a standing block.
func TestKickstartRevivesGemmaWhenNotQuarantined(t *testing.T) {
	dir := t.TempDir()
	writeAgentPlist(t, dir, "ai.sirsi.gemma-broker.plist")

	revived, err := KickstartDeadLabels(dir, launchdDeps{
		listLabels:     func() (map[string]bool, error) { return map[string]bool{}, nil },
		bootstrapPlist: func(p string) error { return nil },
		uid:            func() int { return 501 },
		isQuarantined:  func() bool { return false },
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(revived) != 1 || revived[0] != "ai.sirsi.gemma-broker" {
		t.Fatalf("must revive gemma-broker when not quarantined: got %v", revived)
	}
}

func TestHealCollectorDrains(t *testing.T) {
	drainHeals() // clean slate
	RecordHeal("one")
	RecordHeal("two")
	got := drainHeals()
	if len(got) != 2 || got[0] != "one" {
		t.Fatalf("drain: %v", got)
	}
	if again := drainHeals(); len(again) != 0 {
		t.Fatalf("second drain must be empty: %v", again)
	}
}
