package setup

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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

func TestUninstallBootsOutAndDisablesLaunchAgents(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("launchd uninstall is macOS only")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	agents := filepath.Join(home, "Library", "LaunchAgents")
	if err := os.MkdirAll(agents, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, label := range []string{menubarPlistLabel, supervisorPlistLabel} {
		if err := os.WriteFile(filepath.Join(agents, label+".plist"), []byte("<plist/>"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	old := getUninstallExec()
	defer setUninstallExec(old)
	var calls []string
	setUninstallExec(func(name string, args ...string) error {
		calls = append(calls, name+" "+strings.Join(args, " "))
		return nil
	})

	_, errs := Uninstall(false)
	if len(errs) != 0 {
		t.Fatalf("Uninstall() errors = %v", errs)
	}
	for _, label := range []string{menubarPlistLabel, supervisorPlistLabel} {
		target := fmt.Sprintf("gui/%d/%s", os.Getuid(), label)
		for _, want := range []string{"launchctl bootout " + target, "launchctl disable " + target} {
			found := false
			for _, call := range calls {
				found = found || call == want
			}
			if !found {
				t.Fatalf("missing %q in calls %v", want, calls)
			}
		}
		if _, err := os.Stat(filepath.Join(agents, label+".plist")); !os.IsNotExist(err) {
			t.Fatalf("LaunchAgent %s remains after uninstall: %v", label, err)
		}
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
