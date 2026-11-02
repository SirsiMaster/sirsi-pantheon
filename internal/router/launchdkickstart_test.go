package router

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func writeAgentPlist(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("<plist/>"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func emptyDisabledLabels() (map[string]bool, error) { return map[string]bool{}, nil }

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
		disabledLabels: emptyDisabledLabels,
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
		disabledLabels: emptyDisabledLabels,
		listLabels:     func() (map[string]bool, error) { return map[string]bool{}, nil },
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

// Passive supervision must preserve disabled intent while recovering enabled jobs.
func TestKickstartDisabledPlusUnloaded(t *testing.T) {
	dir := t.TempDir()
	writeAgentPlist(t, dir, "ai.sirsi.disabled-label.plist") // disabled+unloaded → untouched
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
	if !slices.Equal(revived, []string{"ai.sirsi.normal-label"}) {
		t.Fatalf("only enabled job may revive, got %v", revived)
	}
	if len(enabledLabels) != 0 {
		t.Errorf("passive sweep enabled labels: %v", enabledLabels)
	}
	if !slices.Equal(bootstrapped, []string{"ai.sirsi.normal-label.plist"}) {
		t.Errorf("bootstrap called %d times: %v", len(bootstrapped), bootstrapped)
	}
}

// A disabled override remains operator intent even when the job is still loaded.
func TestKickstartPreservesDisabledLoadedLabelWithoutMutation(t *testing.T) {
	dir := t.TempDir()
	writeAgentPlist(t, dir, "ai.sirsi.loaded-disabled.plist")

	var enabled, bootstrapped []string
	revived, err := KickstartDeadLabels(dir, launchdDeps{
		listLabels: func() (map[string]bool, error) {
			return map[string]bool{"ai.sirsi.loaded-disabled": true}, nil
		},
		disabledLabels: func() (map[string]bool, error) {
			return map[string]bool{"ai.sirsi.loaded-disabled": true}, nil
		},
		enableLabel: func(_ string, label string) error {
			enabled = append(enabled, label)
			return nil
		},
		bootstrapPlist: func(path string) error {
			bootstrapped = append(bootstrapped, filepath.Base(path))
			return nil
		},
		uid: func() int { return 501 },
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(revived) != 0 {
		t.Fatalf("loaded label must not be reported as bootstrapped: %v", revived)
	}
	if len(enabled) != 0 {
		t.Fatalf("enabled = %v", enabled)
	}
	if len(bootstrapped) != 0 {
		t.Fatalf("loaded label must not be bootstrapped: %v", bootstrapped)
	}
}

// The explicit UI/CLI recovery is deliberately narrower than the resident
// supervisor duty. A confirmation to repair a disabled override must not
// become authority to bootstrap every unrelated managed plist on disk.
func TestRecoverManagedLaunchdTargetsOnlyDisabledLabels(t *testing.T) {
	dir := t.TempDir()
	writeAgentPlist(t, dir, "ai.sirsi.disabled.plist")
	writeAgentPlist(t, dir, "ai.sirsi.unrelated.plist")
	writeAgentPlist(t, dir, "ai.sirsi.gemma-broker.plist")

	var enabled, bootstrapped []string
	recovery, err := recoverManagedLaunchd(dir, launchdDeps{
		listLabels: func() (map[string]bool, error) { return map[string]bool{}, nil },
		disabledLabels: func() (map[string]bool, error) {
			return map[string]bool{"ai.sirsi.disabled": true}, nil
		},
		enableLabel: func(_ string, label string) error {
			enabled = append(enabled, label)
			return nil
		},
		bootstrapPlist: func(path string) error {
			bootstrapped = append(bootstrapped, filepath.Base(path))
			return nil
		},
		uid: func() int { return 501 },
	}, map[string]bool{"ai.sirsi.disabled": true})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := enabled, []string{"ai.sirsi.disabled"}; !slices.Equal(got, want) {
		t.Fatalf("enabled = %v, want %v", got, want)
	}
	if got, want := bootstrapped, []string{"ai.sirsi.disabled.plist"}; !slices.Equal(got, want) {
		t.Fatalf("bootstrapped = %v, want %v", got, want)
	}
	if got, want := recovery.Bootstrapped, []string{"ai.sirsi.disabled"}; !slices.Equal(got, want) {
		t.Fatalf("recovery bootstrapped = %v, want %v", got, want)
	}
}

func TestKickstartRejectsManagedSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.plist")
	if err := os.WriteFile(target, []byte("<plist/>"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "ai.sirsi.symlinked.plist")); err != nil {
		t.Fatal(err)
	}
	var bootstrapped []string
	revived, err := KickstartDeadLabels(dir, launchdDeps{
		disabledLabels: emptyDisabledLabels,
		listLabels:     func() (map[string]bool, error) { return map[string]bool{}, nil },
		bootstrapPlist: func(path string) error {
			bootstrapped = append(bootstrapped, path)
			return nil
		},
	})
	if err == nil {
		t.Fatal("expected symlink refusal")
	}
	if len(revived) != 0 || len(bootstrapped) != 0 {
		t.Fatalf("symlink must not be revived or bootstrapped: revived=%v bootstrapped=%v", revived, bootstrapped)
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
		disabledLabels: emptyDisabledLabels,
		listLabels:     func() (map[string]bool, error) { return map[string]bool{}, nil },
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
		disabledLabels: emptyDisabledLabels,
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

// TestKickstartHonorsFabricQuarantine is the R7/G6 generalization of
// TestKickstartHonorsGemmaQuarantine: with the fabric-wide marker set, NOT
// EVEN unrelated labels may be revived — unlike isQuarantined (gemma labels
// only), isFabricQuarantined blocks the whole duty.
func TestKickstartHonorsFabricQuarantine(t *testing.T) {
	dir := t.TempDir()
	writeAgentPlist(t, dir, "ai.sirsi.pantheon.plist")
	writeAgentPlist(t, dir, "ai.sirsi.gemma.plist")

	var bootstrapped []string
	deps := launchdDeps{
		listLabels: func() (map[string]bool, error) { return map[string]bool{}, nil },
		bootstrapPlist: func(p string) error {
			bootstrapped = append(bootstrapped, filepath.Base(p))
			return nil
		},
		uid:                 func() int { return 501 },
		isFabricQuarantined: func() bool { return true },
	}
	revived, err := KickstartDeadLabels(dir, deps)
	if err != nil {
		t.Fatal(err)
	}
	if len(revived) != 0 {
		t.Fatalf("fabric quarantine must block every revival: got %v", revived)
	}
	if len(bootstrapped) != 0 {
		t.Fatalf("fabric quarantine must never bootstrap: got %v", bootstrapped)
	}
}

func TestManagedLaunchdPreviewAndApplyUseOnlyReviewedEligibleTargets(t *testing.T) {
	dir := t.TempDir()
	writeAgentPlist(t, dir, "ai.sirsi.loaded.plist")
	writeAgentPlist(t, dir, "ai.sirsi.unloaded.plist")
	writeAgentPlist(t, dir, "ai.sirsi.unrelated.plist")
	writeAgentPlist(t, dir, "com.other.foreign.plist")
	writeAgentPlist(t, dir, "ai.sirsi.loaded.plist.bak-1")

	enabled, bootstrapped := []string{}, []string{}
	listCalls := 0
	deps := launchdDeps{
		listLabels: func() (map[string]bool, error) {
			listCalls++
			return map[string]bool{"ai.sirsi.loaded": true}, nil
		},
		disabledLabels: func() (map[string]bool, error) {
			return map[string]bool{"ai.sirsi.loaded": true, "ai.sirsi.unloaded": true, "ai.sirsi.unrelated": false, "ai.sirsi.gemma-broker": true, "com.other.foreign": true}, nil
		},
		isQuarantined:  func() bool { return true },
		enableLabel:    func(_, label string) error { enabled = append(enabled, label); return nil },
		bootstrapPlist: func(path string) error { bootstrapped = append(bootstrapped, filepath.Base(path)); return nil },
		uid:            func() int { return 501 },
	}
	preview, err := previewManagedLaunchd(dir, deps)
	if err != nil {
		t.Fatal(err)
	}
	if err := preview.Validate(); err != nil {
		t.Fatalf("preview validation: %v", err)
	}
	if got := []string{preview.Targets[0].Label, preview.Targets[1].Label}; !slices.Equal(got, []string{"ai.sirsi.loaded", "ai.sirsi.unloaded"}) {
		t.Fatalf("preview targets = %v", got)
	}
	if len(enabled)+len(bootstrapped) != 0 {
		t.Fatalf("preview actuated launchd: enabled=%v bootstrapped=%v", enabled, bootstrapped)
	}

	recovery, err := restoreReviewedManagedLaunchd(dir, deps, preview.Binding)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(enabled, []string{"ai.sirsi.loaded", "ai.sirsi.unloaded"}) {
		t.Fatalf("enabled labels = %v", enabled)
	}
	if !slices.Equal(bootstrapped, []string{"ai.sirsi.unloaded.plist"}) {
		t.Fatalf("bootstrapped plists = %v", bootstrapped)
	}
	if !slices.Equal(recovery.Enabled, enabled) || !slices.Equal(recovery.Bootstrapped, []string{"ai.sirsi.unloaded"}) {
		t.Fatalf("recovery receipt = %+v", recovery)
	}
	if listCalls != 2 {
		t.Fatalf("launchd list calls = %d, want initial preview and binding recheck only", listCalls)
	}
}

func TestManagedLaunchdChangedPreviewRefusesBeforeAnyActuation(t *testing.T) {
	dir := t.TempDir()
	writeAgentPlist(t, dir, "ai.sirsi.target.plist")
	enabled, bootstrapped := 0, 0
	deps := launchdDeps{
		listLabels:     func() (map[string]bool, error) { return map[string]bool{}, nil },
		disabledLabels: func() (map[string]bool, error) { return map[string]bool{"ai.sirsi.target": true}, nil },
		enableLabel:    func(string, string) error { enabled++; return nil },
		bootstrapPlist: func(string) error { bootstrapped++; return nil },
		uid:            func() int { return 501 },
	}
	preview, err := previewManagedLaunchd(dir, deps)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ai.sirsi.target.plist"), []byte("<changed/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := restoreReviewedManagedLaunchd(dir, deps, preview.Binding); err == nil || !strings.Contains(err.Error(), "preview changed") {
		t.Fatalf("changed preview error = %v", err)
	}
	if enabled != 0 || bootstrapped != 0 {
		t.Fatalf("stale preview actuated launchd: enabled=%d bootstrapped=%d", enabled, bootstrapped)
	}
}

func TestManagedLaunchdOversizedSecondTargetRefusesBeforeAnyActuation(t *testing.T) {
	dir := t.TempDir()
	writeAgentPlist(t, dir, "ai.sirsi.a-first.plist")
	writeAgentPlist(t, dir, "ai.sirsi.z-second.plist")
	enabled, bootstrapped := 0, 0
	deps := launchdDeps{
		listLabels: func() (map[string]bool, error) { return map[string]bool{}, nil },
		disabledLabels: func() (map[string]bool, error) {
			return map[string]bool{"ai.sirsi.a-first": true, "ai.sirsi.z-second": true}, nil
		},
		enableLabel:    func(string, string) error { enabled++; return nil },
		bootstrapPlist: func(string) error { bootstrapped++; return nil },
	}
	preview, err := previewManagedLaunchd(dir, deps)
	if err != nil {
		t.Fatal(err)
	}
	oversized := bytes.Repeat([]byte("x"), managedLaunchdPreviewMaxPlistBytes+1)
	if err := os.WriteFile(filepath.Join(dir, "ai.sirsi.z-second.plist"), oversized, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := restoreReviewedManagedLaunchd(dir, deps, preview.Binding); err == nil {
		t.Fatal("oversized changed target was accepted")
	}
	if enabled != 0 || bootstrapped != 0 {
		t.Fatalf("later malformed target was detected after actuation: enabled=%d bootstrapped=%d", enabled, bootstrapped)
	}
}

func TestManagedLaunchdPlistReaderCapsGrowthBeyondReviewLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "growing.plist")
	if err := os.WriteFile(path, bytes.Repeat([]byte("x"), managedLaunchdPreviewMaxPlistBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	contents, err := readManagedLaunchdPlist(path)
	if err == nil || contents != nil {
		t.Fatalf("oversized plist read = %d bytes, error %v", len(contents), err)
	}
}

func TestManagedLaunchdPreviewRejectsMissingAndMalformedTargets(t *testing.T) {
	for _, tc := range []struct {
		name   string
		setup  func(*testing.T, string)
		labels map[string]bool
	}{
		{name: "missing", setup: func(*testing.T, string) {}, labels: map[string]bool{"ai.sirsi.missing": true}},
		{name: "symlink", setup: func(t *testing.T, dir string) {
			if err := os.Symlink("absent", filepath.Join(dir, "ai.sirsi.link.plist")); err != nil {
				t.Fatal(err)
			}
		}, labels: map[string]bool{"ai.sirsi.link": true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			tc.setup(t, dir)
			_, err := previewManagedLaunchd(dir, launchdDeps{
				listLabels:     func() (map[string]bool, error) { return map[string]bool{}, nil },
				disabledLabels: func() (map[string]bool, error) { return tc.labels, nil },
			})
			if err == nil {
				t.Fatal("preview accepted a missing or malformed target")
			}
		})
	}
}

// TestKickstartRevivesWhenFabricNotQuarantined is the other direction: with no
// fabric quarantine in effect, dead labels revive normally — the guard must
// not become a standing block (A35).
func TestKickstartRevivesWhenFabricNotQuarantined(t *testing.T) {
	dir := t.TempDir()
	writeAgentPlist(t, dir, "ai.sirsi.pantheon.plist")

	revived, err := KickstartDeadLabels(dir, launchdDeps{
		disabledLabels:      emptyDisabledLabels,
		listLabels:          func() (map[string]bool, error) { return map[string]bool{}, nil },
		bootstrapPlist:      func(p string) error { return nil },
		uid:                 func() int { return 501 },
		isFabricQuarantined: func() bool { return false },
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(revived) != 1 || revived[0] != "ai.sirsi.pantheon" {
		t.Fatalf("must revive when fabric not quarantined: got %v", revived)
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

func TestKickstartUnknownDisabledIntentRefusesMutation(t *testing.T) {
	probeErr := errors.New("override database unavailable")
	for _, tc := range []struct {
		name  string
		probe func() (map[string]bool, error)
	}{
		{"missing probe", nil},
		{"probe error", func() (map[string]bool, error) { return nil, probeErr }},
		{"unknown map", func() (map[string]bool, error) { return nil, nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeAgentPlist(t, dir, "ai.sirsi.fixture.plist")
			mutations := 0
			revived, err := KickstartDeadLabels(dir, launchdDeps{
				listLabels: emptyDisabledLabels, disabledLabels: tc.probe,
				enableLabel:    func(string, string) error { mutations++; return nil },
				bootstrapPlist: func(string) error { mutations++; return nil },
			})
			if err == nil || mutations != 0 || len(revived) != 0 {
				t.Fatalf("unknown intent: revived=%v mutations=%d err=%v", revived, mutations, err)
			}
			if tc.name == "probe error" && !errors.Is(err, probeErr) {
				t.Fatalf("lost probe error: %v", err)
			}
		})
	}
}

func TestRecoverManagedLaunchdConfirmedLoadedTarget(t *testing.T) {
	dir := t.TempDir()
	writeAgentPlist(t, dir, "ai.sirsi.fixture.plist")
	var enabled []string
	recovery, err := recoverManagedLaunchd(dir, launchdDeps{
		listLabels:     func() (map[string]bool, error) { return map[string]bool{"ai.sirsi.fixture": true}, nil },
		disabledLabels: func() (map[string]bool, error) { return map[string]bool{"ai.sirsi.fixture": true}, nil },
		uid:            func() int { return 501 },
		enableLabel: func(domain, label string) error {
			if domain != "gui/501" {
				t.Fatalf("unexpected domain %q", domain)
			}
			enabled = append(enabled, label)
			return nil
		},
		bootstrapPlist: func(string) error { t.Fatal("loaded target must not bootstrap"); return nil },
	}, map[string]bool{"ai.sirsi.fixture": true})
	if err != nil || !slices.Equal(enabled, []string{"ai.sirsi.fixture"}) || !slices.Equal(recovery.Enabled, enabled) || len(recovery.Bootstrapped) != 0 {
		t.Fatalf("confirmed repair: enabled=%v recovery=%+v err=%v", enabled, recovery, err)
	}
}

func TestParseManagedDisabledLabels(t *testing.T) {
	for _, tc := range []struct {
		name, output string
		want         []string
		bad          bool
	}{
		{"empty known", "disabled services = {\n}", nil, false},
		{"word values", "disabled services = {\n\"ai.sirsi.a\" => disabled\n\"ai.sirsi.b\" => enabled\n}", []string{"ai.sirsi.a"}, false},
		{"boolean joined values", "disabled services = {\n\"ai.sirsi.a ai.sirsi.b\" => true\n\"ai.sirsi.c\" => false\n}", []string{"ai.sirsi.a", "ai.sirsi.b"}, false},
		{"missing output", "", nil, true},
		{"unframed", "\"ai.sirsi.a\" => disabled", nil, true},
		{"truncated", "disabled services = {\n\"ai.sirsi.a\" => disabled", nil, true},
		{"unknown value", "disabled services = {\n\"ai.sirsi.a\" => unknown\n}", nil, true},
		{"malformed entry", "disabled services = {\nnot an entry\n}", nil, true},
		{"contradiction", "disabled services = {\n\"ai.sirsi.a\" => true\n\"ai.sirsi.a\" => false\n}", nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseManagedDisabledLabels(tc.output)
			if (err != nil) != tc.bad {
				t.Fatalf("got %v, err=%v", got, err)
			}
			if tc.bad {
				return
			}
			if len(got) != len(tc.want) {
				t.Fatalf("disabled=%v want=%v", got, tc.want)
			}
			for _, label := range tc.want {
				if !got[label] {
					t.Fatalf("missing disabled label %s", label)
				}
			}
		})
	}
}
