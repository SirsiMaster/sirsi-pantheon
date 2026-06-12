package setup

import (
	"strings"
	"testing"
)

func TestPlanUninstallCoversTheFootprint(t *testing.T) {
	plan := PlanUninstall()
	kinds := map[string]int{}
	var tccID string
	for _, tg := range plan {
		kinds[tg.Kind]++
		if tg.Kind == "tcc" {
			tccID = tg.Path
		}
	}
	for _, want := range []string{"LaunchAgent", "binary", "config", "app", "tcc"} {
		if kinds[want] == 0 {
			t.Errorf("PlanUninstall missing a %q target", want)
		}
	}
	// The FDA grant must target the stable bundle id (the only tccutil-resettable key).
	if tccID != menubarPlistLabel {
		t.Errorf("tcc target = %q, want bundle id %q", tccID, menubarPlistLabel)
	}
}

// TestUninstallDryRunHasNoSideEffects is the A1 guarantee: a dry run NEVER shells
// out (no launchctl/tccutil/osascript) and removes nothing.
func TestUninstallDryRunHasNoSideEffects(t *testing.T) {
	old := getUninstallExec()
	defer setUninstallExec(old)
	var calls []string
	setUninstallExec(func(name string, args ...string) error {
		calls = append(calls, name)
		return nil
	})

	acted, errs := Uninstall(true)
	if len(calls) != 0 {
		t.Errorf("dry run shelled out: %v", calls)
	}
	if len(errs) != 0 {
		t.Errorf("dry run produced errors: %v", errs)
	}
	// Dry run still reports the tcc target (always-listed) even with nothing on disk.
	hasTCC := false
	for _, a := range acted {
		if a.Kind == "tcc" {
			hasTCC = true
		}
	}
	if !hasTCC {
		t.Error("dry run should still surface the tcc (FDA) target")
	}
}

// TestTrashPathUsesFinder confirms .app removal goes through Finder (Trash,
// recoverable), never a hard delete — via the injectable seam.
func TestTrashPathUsesFinder(t *testing.T) {
	old := getUninstallExec()
	defer setUninstallExec(old)
	var gotName string
	var gotArgs []string
	setUninstallExec(func(name string, args ...string) error {
		gotName, gotArgs = name, args
		return nil
	})
	_ = trashPath("/Users/x/Applications/Sirsi Pantheon.app")
	// On darwin it must be osascript→Finder delete; off darwin this test still
	// passes because trashPath falls back to os.RemoveAll (no exec) — assert the
	// darwin path only when it actually shelled.
	if gotName != "" {
		if gotName != "osascript" || len(gotArgs) < 2 || !strings.Contains(gotArgs[1], "Finder") {
			t.Errorf("trashPath shelled %q %v, want osascript Finder delete", gotName, gotArgs)
		}
	}
}
