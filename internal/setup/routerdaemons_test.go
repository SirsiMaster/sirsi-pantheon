package setup

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
)

// TestRouterDaemonPlist_ProgramFoundByNodeStatus proves the generated plist's
// ProgramArguments round-trips through router.LaunchAgentProgram — the exact
// parser node-status uses for program_found. The older hand-installed plists
// wrote ProgramArguments inline, which that parser cannot read, so an installed
// daemon read program_found=false forever. This guards that regression.
func TestRouterDaemonPlist_ProgramFoundByNodeStatus(t *testing.T) {
	dir := t.TempDir()
	spec := routerDaemonSpec{Label: "com.sirsi.idea-router-sweep", Trigger: "interval", IntervalS: 3600, ScriptRel: "sweep.sh"}
	scriptPath := filepath.Join(dir, "sweep.sh")
	plist := routerDaemonPlist(spec, scriptPath, dir, filepath.Join(dir, "logs"), "/bin")
	plistPath := filepath.Join(dir, spec.Label+".plist")
	if err := os.WriteFile(plistPath, []byte(plist), 0o644); err != nil {
		t.Fatal(err)
	}
	prog, err := router.LaunchAgentProgram(plistPath)
	if err != nil {
		t.Fatalf("LaunchAgentProgram could not resolve the program: %v", err)
	}
	if prog != scriptPath {
		t.Errorf("resolved program = %q, want %q", prog, scriptPath)
	}
}

// TestRouterDaemonPlist_WatchShape asserts the FSEvents dispatch daemon renders a
// WatchPaths-driven plist with a throttle and no StartInterval.
func TestRouterDaemonPlist_WatchShape(t *testing.T) {
	routerRoot := "/repo/.agents/idea-router"
	specs := routerDaemonSpecs(routerRoot)
	var watch routerDaemonSpec
	for _, s := range specs {
		if s.Label == "com.sirsi.idea-router" {
			watch = s
		}
	}
	if watch.Label == "" {
		t.Fatal("idea-router spec not found")
	}
	plist := routerDaemonPlist(watch, filepath.Join(routerRoot, watch.ScriptRel), "/repo", filepath.Join(routerRoot, "logs"), "/bin")

	for _, want := range []string{
		"<key>Label</key><string>com.sirsi.idea-router</string>",
		"<key>WatchPaths</key>",
		filepath.Join(routerRoot, "state.json"),
		filepath.Join(routerRoot, "items"),
		"<key>ThrottleInterval</key><integer>10</integer>",
		"run-on-event.sh",
		"<key>PATH</key>",
	} {
		if !strings.Contains(plist, want) {
			t.Errorf("watch plist missing %q:\n%s", want, plist)
		}
	}
	if strings.Contains(plist, "StartInterval") {
		t.Error("watch daemon must NOT use StartInterval (it is FSEvents-driven)")
	}
}

// TestRouterDaemonPlist_IntervalShape asserts the sweep/police daemons render
// StartInterval-driven plists with no WatchPaths.
func TestRouterDaemonPlist_IntervalShape(t *testing.T) {
	routerRoot := "/repo/.agents/idea-router"
	specs := routerDaemonSpecs(routerRoot)
	for _, label := range []string{"com.sirsi.idea-router-sweep", "ai.sirsi.registry-police"} {
		var spec routerDaemonSpec
		for _, s := range specs {
			if s.Label == label {
				spec = s
			}
		}
		if spec.Label == "" {
			t.Fatalf("%s spec not found", label)
		}
		plist := routerDaemonPlist(spec, filepath.Join(routerRoot, spec.ScriptRel), "/repo", filepath.Join(routerRoot, "logs"), "/bin")
		if !strings.Contains(plist, "<key>StartInterval</key>") {
			t.Errorf("%s must use StartInterval:\n%s", label, plist)
		}
		if strings.Contains(plist, "WatchPaths") {
			t.Errorf("%s must NOT use WatchPaths (it is interval-driven)", label)
		}
	}
}

// TestRouterDaemonPlist_XMLEscapesPaths proves a path with XML-hostile characters
// cannot break (or inject into) the plist.
func TestRouterDaemonPlist_XMLEscapesPaths(t *testing.T) {
	spec := routerDaemonSpec{Label: "com.sirsi.idea-router-sweep", Trigger: "interval", IntervalS: 3600, ScriptRel: "sweep.sh"}
	plist := routerDaemonPlist(spec, "/re&po/<x>/sweep.sh", "/re&po/<x>", "/re&po/<x>/logs", "/bin")
	if strings.Contains(plist, "/re&po/<x>") {
		t.Error("raw & or < leaked into plist — must be XML-escaped")
	}
	if !strings.Contains(plist, "/re&amp;po/&lt;x&gt;/sweep.sh") {
		t.Errorf("expected escaped script path in plist:\n%s", plist)
	}
}

// TestInstallRouterDaemons_NonDarwinSkips guards the platform gate.
func TestInstallRouterDaemons_NonDarwinSkips(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("darwin path exercised elsewhere")
	}
	res := InstallRouterDaemons()
	if res.Status != StatusSkipped {
		t.Errorf("InstallRouterDaemons on %s = %v, want StatusSkipped", runtime.GOOS, res.Status)
	}
	if res.Surface != SurfaceRouterDaemons {
		t.Errorf("Surface = %q, want %q", res.Surface, SurfaceRouterDaemons)
	}
}

// TestInstallRouterDaemons_InstallsAndIdempotent exercises the real installer on
// darwin against a scaffolded clone with a stubbed launchctl and a temp HOME.
// First run installs all three; second run reports them unchanged (idempotent).
func TestInstallRouterDaemons_InstallsAndIdempotent(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("router daemons are macOS only")
	}

	// Scaffold a clone with the router scripts present and chdir into it so
	// FindRepoRoot's cwd walk-up resolves here.
	repo := t.TempDir()
	routerRoot := filepath.Join(repo, ".agents", "idea-router", "police")
	if err := os.MkdirAll(routerRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	rr := filepath.Join(repo, ".agents", "idea-router")
	for _, rel := range []string{"run-on-event.sh", "sweep.sh", filepath.Join("police", "registry-police.sh")} {
		if err := os.WriteFile(filepath.Join(rr, rel), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	// Isolate HOME so plists land in a temp LaunchAgents dir. Drive the
	// root-parameterized core directly so this never touches the real checkout.
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Stub launchctl so no real load happens.
	var loaded []string
	orig := launchctlExecFn
	launchctlExecFn = func(args ...string) error {
		if len(args) > 0 && args[0] == "load" {
			loaded = append(loaded, args[len(args)-1])
		}
		return nil
	}
	t.Cleanup(func() { launchctlExecFn = orig })

	res := installRouterDaemonsAt(repo)
	if res.Status != StatusOK {
		t.Fatalf("first install status = %v (%s), want StatusOK", res.Status, res.Message)
	}
	// All three plists written under the temp HOME.
	for _, label := range []string{"com.sirsi.idea-router", "com.sirsi.idea-router-sweep", "ai.sirsi.registry-police"} {
		p := filepath.Join(home, "Library", "LaunchAgents", label+".plist")
		if !fileExists(p) {
			t.Errorf("expected %s to be written", p)
		}
	}
	if len(loaded) != 3 {
		t.Errorf("expected 3 launchctl loads on first install, got %d", len(loaded))
	}

	// Second run: identical content → unchanged, no reload.
	loaded = nil
	res2 := installRouterDaemonsAt(repo)
	if res2.Status != StatusOK {
		t.Fatalf("second install status = %v (%s), want StatusOK", res2.Status, res2.Message)
	}
	if !strings.Contains(res2.Message, "no change") && !strings.Contains(res2.Message, "unchanged") {
		t.Errorf("second install should report no change, got %q", res2.Message)
	}
	if len(loaded) != 0 {
		t.Errorf("idempotent re-install must not reload, got %d loads", len(loaded))
	}
}

// TestInstallRouterDaemons_SkipsMissingScript proves a daemon whose backing
// script is absent is skipped, never installed broken.
func TestInstallRouterDaemons_SkipsMissingScript(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("router daemons are macOS only")
	}
	repo := t.TempDir()
	rr := filepath.Join(repo, ".agents", "idea-router")
	if err := os.MkdirAll(filepath.Join(rr, "police"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Only the sweep script exists; the other two are missing → skipped.
	if err := os.WriteFile(filepath.Join(rr, "sweep.sh"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	orig := launchctlExecFn
	launchctlExecFn = func(args ...string) error { return nil }
	t.Cleanup(func() { launchctlExecFn = orig })

	res := installRouterDaemonsAt(repo)
	// One installed (sweep) → partial success is StatusOK, message names the skips.
	if res.Status != StatusOK {
		t.Fatalf("status = %v (%s), want StatusOK (sweep installs, others skip)", res.Status, res.Message)
	}
	if !strings.Contains(res.Message, "skipped") {
		t.Errorf("message should note skipped daemons, got %q", res.Message)
	}
	// The missing-script daemons must NOT have been written.
	for _, label := range []string{"com.sirsi.idea-router", "ai.sirsi.registry-police"} {
		p := filepath.Join(home, "Library", "LaunchAgents", label+".plist")
		if fileExists(p) {
			t.Errorf("%s should NOT be installed (script missing)", label)
		}
	}
}
