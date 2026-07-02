package setup

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// NOTE: none of the tests in this file use t.Parallel — they swap the
// uninstallExecFn seam and redirect $HOME/$PATH via t.Setenv, so they must
// run sequentially (repo law from PRs #129/#131).

// writeExecutable drops an executable script into dir and returns its path.
func writeExecutable(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

// restrictPATH pins PATH to the given dirs plus the system dirs so LookPath
// results are deterministic regardless of what the host has installed.
func restrictPATH(t *testing.T, extra ...string) {
	t.Helper()
	t.Setenv("PATH", strings.Join(append(extra, "/usr/bin", "/bin"), ":"))
}

// ── setup.go ────────────────────────────────────────────────────────────────

func TestDependencyInstall(t *testing.T) {
	t.Run("no install command", func(t *testing.T) {
		d := Dependency{Name: "widget", InstallCmd: ""}
		if err := d.Install(os.Stdout, os.Stderr); err == nil ||
			!strings.Contains(err.Error(), "no install command") {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("install command fails", func(t *testing.T) {
		d := Dependency{Name: "ls", InstallCmd: "false"}
		if err := d.Install(os.Stdout, os.Stderr); err == nil {
			t.Fatal("expected error from failing install command")
		}
	})

	t.Run("installed but not on PATH", func(t *testing.T) {
		d := Dependency{Name: "definitely-not-installed-xyz-123", InstallCmd: "true"}
		if err := d.Install(os.Stdout, os.Stderr); err == nil ||
			!strings.Contains(err.Error(), "not on PATH") {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("success", func(t *testing.T) {
		d := Dependency{Name: "ls", InstallCmd: "true"}
		if err := d.Install(os.Stdout, os.Stderr); err != nil {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestInstalledVariants(t *testing.T) {
	t.Run("missing tool", func(t *testing.T) {
		ok, ver := Dependency{Name: "definitely-not-installed-xyz-123"}.Installed()
		if ok || ver != "" {
			t.Errorf("Installed() = (%v, %q)", ok, ver)
		}
	})

	t.Run("version probe fails", func(t *testing.T) {
		// `false --version` exits nonzero → installed, no version string.
		ok, ver := Dependency{Name: "false"}.Installed()
		if !ok || ver != "" {
			t.Errorf("Installed() = (%v, %q)", ok, ver)
		}
	})

	t.Run("overlong version suppressed", func(t *testing.T) {
		bin := t.TempDir()
		writeExecutable(t, bin, "chatty-tool", "#!/bin/sh\necho "+strings.Repeat("v", 100)+"\n")
		restrictPATH(t, bin)
		ok, ver := Dependency{Name: "chatty-tool"}.Installed()
		if !ok || ver != "" {
			t.Errorf("Installed() = (%v, %q)", ok, ver)
		}
	})

	t.Run("short version reported", func(t *testing.T) {
		bin := t.TempDir()
		writeExecutable(t, bin, "terse-tool", "#!/bin/sh\necho terse-tool 1.2.3\n")
		restrictPATH(t, bin)
		ok, ver := Dependency{Name: "terse-tool"}.Installed()
		if !ok || ver != "terse-tool 1.2.3" {
			t.Errorf("Installed() = (%v, %q)", ok, ver)
		}
	})
}

// ── surface.go: titles, details, plist, binary resolution ──────────────────

func TestSurfaceTitleDetailAll(t *testing.T) {
	all := []Surface{SurfaceCLI, SurfaceTUI, SurfaceIDE, SurfaceMenubar, SurfaceGUI, SurfaceSupervisor, SurfaceMaatGate}
	for _, s := range all {
		if s.Title() == "" {
			t.Errorf("Title(%s) empty", s)
		}
		if s.Detail() == "" {
			t.Errorf("Detail(%s) empty", s)
		}
	}
	if got := Surface("weird").Title(); got != "weird" {
		t.Errorf("unknown Title = %q", got)
	}
	if got := Surface("weird").Detail(); got != "" {
		t.Errorf("unknown Detail = %q", got)
	}
}

func TestMenubarPlistContent(t *testing.T) {
	out := menubarPlistContent("/opt/space dir/sirsi-menubar")
	if !strings.Contains(out, "ai.sirsi.pantheon") {
		t.Error("plist missing label")
	}
	if !strings.Contains(out, `exec "/opt/space dir/sirsi-menubar"`) {
		t.Errorf("plist must exec the quoted binary path:\n%s", out)
	}
}

func TestMenubarBinaryPath(t *testing.T) {
	t.Run("found on PATH", func(t *testing.T) {
		bin := t.TempDir()
		want := writeExecutable(t, bin, "sirsi-menubar", "#!/bin/sh\n")
		restrictPATH(t, bin)
		if got := MenubarBinaryPath(); got != want {
			t.Errorf("MenubarBinaryPath() = %q, want %q", got, want)
		}
	})

	t.Run("not installed", func(t *testing.T) {
		restrictPATH(t)
		if got := MenubarBinaryPath(); got != "" {
			t.Errorf("MenubarBinaryPath() = %q, want empty", got)
		}
	})
}

func TestGUIBinaryPath(t *testing.T) {
	t.Run("found on PATH", func(t *testing.T) {
		bin := t.TempDir()
		want := writeExecutable(t, bin, "sirsi-gui", "#!/bin/sh\n")
		restrictPATH(t, bin)
		if got := GUIBinaryPath(); got != want {
			t.Errorf("GUIBinaryPath() = %q, want %q", got, want)
		}
	})

	t.Run("not installed", func(t *testing.T) {
		restrictPATH(t)
		if got := GUIBinaryPath(); got != "" {
			t.Errorf("GUIBinaryPath() = %q, want empty", got)
		}
	})
}

func TestSirsiBinaryPath(t *testing.T) {
	t.Run("env override wins", func(t *testing.T) {
		t.Setenv("SIRSI_BINARY", "/opt/custom/sirsi")
		if got := SirsiBinaryPath(); got != "/opt/custom/sirsi" {
			t.Errorf("SirsiBinaryPath() = %q", got)
		}
	})

	t.Run("found via PATH", func(t *testing.T) {
		t.Setenv("SIRSI_BINARY", "")
		t.Setenv("HOME", t.TempDir())
		bin := t.TempDir()
		want := writeExecutable(t, bin, "sirsi", "#!/bin/sh\n")
		restrictPATH(t, bin)
		if got := SirsiBinaryPath(); got != want {
			t.Errorf("SirsiBinaryPath() = %q, want %q", got, want)
		}
	})

	t.Run("well-known home dir fallback", func(t *testing.T) {
		t.Setenv("SIRSI_BINARY", "")
		home := t.TempDir()
		t.Setenv("HOME", home)
		restrictPATH(t) // no sirsi on PATH
		localBin := filepath.Join(home, ".local", "bin")
		os.MkdirAll(localBin, 0o755)
		want := writeExecutable(t, localBin, "sirsi", "#!/bin/sh\n")
		if got := SirsiBinaryPath(); got != want {
			t.Errorf("SirsiBinaryPath() = %q, want %q", got, want)
		}
	})
}

func TestFileExists(t *testing.T) {
	tmp := t.TempDir()
	p := filepath.Join(tmp, "f")
	if fileExists(p) {
		t.Error("fileExists(true) for missing file")
	}
	os.WriteFile(p, []byte("x"), 0o644)
	if !fileExists(p) {
		t.Error("fileExists(false) for existing file")
	}
}

// ── surface.go: menubar + supervisor install state ──────────────────────────

func TestMenubarAndSupervisorInstalled(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if MenubarInstalled() {
		t.Error("MenubarInstalled() = true with empty home")
	}
	if SupervisorInstalled() {
		t.Error("SupervisorInstalled() = true with empty home")
	}
	if runtime.GOOS != "darwin" {
		return // installed-state is always false off darwin
	}

	la := filepath.Join(home, "Library", "LaunchAgents")
	os.MkdirAll(la, 0o755)
	os.WriteFile(filepath.Join(la, "ai.sirsi.pantheon.plist"), []byte("<plist/>"), 0o644)
	os.WriteFile(filepath.Join(la, "ai.sirsi.horus.agent-router.plist"), []byte("<plist/>"), 0o644)

	if !MenubarInstalled() {
		t.Error("MenubarInstalled() = false with plist present")
	}
	if !SupervisorInstalled() {
		t.Error("SupervisorInstalled() = false with plist present")
	}
}

func TestInstallMenubar_NoBinary(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	restrictPATH(t) // no sirsi-menubar anywhere

	res := InstallMenubar()
	if runtime.GOOS != "darwin" {
		if res.Status != StatusSkipped {
			t.Fatalf("Status = %v, want Skipped off darwin", res.Status)
		}
		return
	}
	if res.Status != StatusFailed || !strings.Contains(res.Message, "not found") {
		t.Fatalf("res = %+v, want binary-not-found failure", res)
	}
}

func TestInstallMenubar_LaunchAgentDirBlocked(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("menubar install is macOS only")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)

	bin := t.TempDir()
	writeExecutable(t, bin, "sirsi-menubar", "#!/bin/sh\n")
	restrictPATH(t, bin)

	// ~/Library is a file → MkdirAll for LaunchAgents fails before launchctl.
	os.WriteFile(filepath.Join(home, "Library"), []byte("x"), 0o644)

	res := InstallMenubar()
	if res.Status != StatusFailed {
		t.Fatalf("res = %+v, want mkdir failure", res)
	}
	// The bundle scaffold still happened under the temp home.
	bundleExec := filepath.Join(home, "Applications", "Sirsi Menubar.app", "Contents", "MacOS", "sirsi-menubar")
	if !fileExists(bundleExec) {
		t.Error("expected app bundle scaffold under temp ~/Applications")
	}
}

func TestInstallMenubar_BundleFallbackThenWriteFail(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("menubar install is macOS only")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)

	bin := t.TempDir()
	writeExecutable(t, bin, "sirsi-menubar", "#!/bin/sh\n")
	restrictPATH(t, bin)

	// ~/Applications is a file → bundle creation fails → bare-binary fallback.
	os.WriteFile(filepath.Join(home, "Applications"), []byte("x"), 0o644)
	// LaunchAgents exists but is read-only → WriteFile fails before launchctl.
	la := filepath.Join(home, "Library", "LaunchAgents")
	os.MkdirAll(la, 0o755)
	os.Chmod(la, 0o555)
	t.Cleanup(func() { os.Chmod(la, 0o755) })

	res := InstallMenubar()
	if res.Status != StatusFailed {
		t.Fatalf("res = %+v, want plist write failure", res)
	}
}

func TestInstallMenubarAppBundle(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	t.Run("missing source", func(t *testing.T) {
		if _, err := installMenubarAppBundle(filepath.Join(home, "missing-bin")); err == nil {
			t.Error("expected error for missing source binary")
		}
	})

	t.Run("scaffolds bundle", func(t *testing.T) {
		src := writeExecutable(t, t.TempDir(), "sirsi-menubar", "#!/bin/sh\necho hi\n")
		execPath, err := installMenubarAppBundle(src)
		if err != nil {
			t.Fatalf("installMenubarAppBundle() error = %v", err)
		}
		want := filepath.Join(home, "Applications", "Sirsi Menubar.app", "Contents", "MacOS", "sirsi-menubar")
		if execPath != want {
			t.Errorf("execPath = %q, want %q", execPath, want)
		}
		if !fileExists(execPath) {
			t.Error("bundled executable not written")
		}
	})
}

// ── surface.go: IDE (MCP) via a fake claude CLI ─────────────────────────────

func TestIDESurface_NoClaudeCLI(t *testing.T) {
	restrictPATH(t) // no claude

	if claudeCLIAvailable() {
		t.Fatal("claudeCLIAvailable() = true with restricted PATH")
	}
	if IDERegistered() {
		t.Fatal("IDERegistered() = true without claude CLI")
	}
	res := RegisterIDE()
	if res.Status != StatusMissing || !strings.Contains(res.Message, "claude") {
		t.Fatalf("res = %+v, want missing-CLI guidance", res)
	}
}

func TestIDESurface_AlreadyRegistered(t *testing.T) {
	bin := t.TempDir()
	writeExecutable(t, bin, "claude",
		"#!/bin/sh\nif [ \"$1\" = mcp ] && [ \"$2\" = list ]; then echo 'sirsi: sirsi mcp'; fi\nexit 0\n")
	restrictPATH(t, bin)

	if !IDERegistered() {
		t.Fatal("IDERegistered() = false when list reports sirsi")
	}
	res := RegisterIDE()
	if res.Status != StatusOK || !strings.Contains(res.Message, "already") {
		t.Fatalf("res = %+v, want already-registered OK", res)
	}
}

func TestIDESurface_RegisterSucceeds(t *testing.T) {
	bin := t.TempDir()
	writeExecutable(t, bin, "claude", "#!/bin/sh\nexit 0\n") // list empty, add ok
	restrictPATH(t, bin)

	res := RegisterIDE()
	if res.Status != StatusOK || !strings.Contains(res.Message, "registered") {
		t.Fatalf("res = %+v, want fresh registration OK", res)
	}
}

func TestIDESurface_RegisterFails(t *testing.T) {
	bin := t.TempDir()
	writeExecutable(t, bin, "claude",
		"#!/bin/sh\nif [ \"$2\" = add ]; then echo boom; exit 1; fi\nexit 0\n")
	restrictPATH(t, bin)

	res := RegisterIDE()
	if res.Status != StatusFailed || !strings.Contains(res.Message, "boom") {
		t.Fatalf("res = %+v, want add failure surfaced", res)
	}
}

func TestIDESurface_ListFails(t *testing.T) {
	bin := t.TempDir()
	writeExecutable(t, bin, "claude", "#!/bin/sh\nexit 1\n")
	restrictPATH(t, bin)

	if IDERegistered() {
		t.Error("IDERegistered() = true when `claude mcp list` fails")
	}
}

// ── surface.go: Install dispatch + LaunchSurface ────────────────────────────

func TestInstallDispatch(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	restrictPATH(t) // no claude, no sirsi-menubar

	if res := Install(SurfaceCLI); res.Status != StatusOK {
		t.Errorf("Install(cli) = %+v", res)
	}
	if res := Install(SurfaceTUI); res.Status != StatusOK {
		t.Errorf("Install(tui) = %+v", res)
	}
	if res := Install(SurfaceIDE); res.Status != StatusMissing {
		t.Errorf("Install(ide) = %+v", res)
	}
	res := Install(SurfaceMenubar)
	if runtime.GOOS == "darwin" {
		if res.Status != StatusFailed {
			t.Errorf("Install(menubar) = %+v, want failed (no binary)", res)
		}
	} else if res.Status != StatusSkipped {
		t.Errorf("Install(menubar) = %+v, want skipped off darwin", res)
	}
	if res := Install(Surface("bogus")); res.Status != StatusSkipped {
		t.Errorf("Install(bogus) = %+v", res)
	}
}

func TestLaunchSurface_CLIAndTUI(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	msg, err := LaunchSurface(SurfaceCLI)
	if err != nil || !strings.Contains(msg, "CLI") {
		t.Fatalf("LaunchSurface(cli) = (%q, %v)", msg, err)
	}
	if got := ActiveSurface(); got != SurfaceCLI {
		t.Errorf("ActiveSurface() = %q after launching cli", got)
	}

	msg, err = LaunchSurface(SurfaceTUI)
	if err != nil || !strings.Contains(msg, "TUI") {
		t.Fatalf("LaunchSurface(tui) = (%q, %v)", msg, err)
	}
}

func TestLaunchSurface_IDE(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	t.Run("no CLI → registration guidance", func(t *testing.T) {
		restrictPATH(t)
		msg, err := LaunchSurface(SurfaceIDE)
		if err != nil || !strings.Contains(msg, "claude") {
			t.Fatalf("LaunchSurface(ide) = (%q, %v)", msg, err)
		}
	})

	t.Run("registered → active", func(t *testing.T) {
		bin := t.TempDir()
		writeExecutable(t, bin, "claude",
			"#!/bin/sh\nif [ \"$2\" = list ]; then echo sirsi; fi\nexit 0\n")
		restrictPATH(t, bin)
		msg, err := LaunchSurface(SurfaceIDE)
		if err != nil || !strings.Contains(msg, "IDE surface active") {
			t.Fatalf("LaunchSurface(ide) = (%q, %v)", msg, err)
		}
	})
}

func TestLaunchSurface_GUI(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Setenv("HOME", t.TempDir())
		if _, err := LaunchSurface(SurfaceGUI); err == nil {
			t.Fatal("expected macOS-only error off darwin")
		}
		return
	}
	t.Setenv("HOME", t.TempDir())
	bin := t.TempDir()
	writeExecutable(t, bin, "sirsi-gui", "#!/bin/sh\nexit 0\n")
	restrictPATH(t, bin)

	msg, err := LaunchSurface(SurfaceGUI)
	if err != nil || !strings.Contains(msg, "GUI surface launched") {
		t.Fatalf("LaunchSurface(gui) = (%q, %v)", msg, err)
	}
}

func TestLaunchSurface_MenubarInstallFails(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("menubar surface is macOS only")
	}
	t.Setenv("HOME", t.TempDir()) // not installed
	restrictPATH(t)               // and no binary to install

	if _, err := LaunchSurface(SurfaceMenubar); err == nil {
		t.Fatal("expected install failure to surface as error")
	}
}

func TestLaunchSurface_UnknownAndSaveError(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if _, err := LaunchSurface(Surface("bogus")); err == nil ||
		!strings.Contains(err.Error(), "unknown surface") {
		t.Fatalf("err = %v", err)
	}

	home := t.TempDir()
	t.Setenv("HOME", home)
	// ~/.config is a file → SaveActiveSurface fails before any launch.
	os.WriteFile(filepath.Join(home, ".config"), []byte("x"), 0o644)
	if _, err := LaunchSurface(SurfaceCLI); err == nil {
		t.Fatal("expected SaveActiveSurface error")
	}
}

func TestSaveActiveSurface_MkdirFails(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	os.WriteFile(filepath.Join(home, ".config"), []byte("x"), 0o644)
	if err := SaveActiveSurface(SurfaceTUI); err == nil {
		t.Fatal("expected mkdir error")
	}
}

// ── uninstall.go: execute the plan hermetically ─────────────────────────────

func TestUninstallExecutesPlan(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	old := getUninstallExec()
	defer setUninstallExec(old)
	var execCalls []string
	setUninstallExec(func(name string, args ...string) error {
		execCalls = append(execCalls, name)
		return nil
	})

	// Lay down the runtime footprint inside the temp home.
	la := filepath.Join(home, "Library", "LaunchAgents")
	os.MkdirAll(la, 0o755)
	menubarPlist := filepath.Join(la, "ai.sirsi.pantheon.plist")
	supervisorPlist := filepath.Join(la, "ai.sirsi.horus.agent-router.plist")
	os.WriteFile(menubarPlist, []byte("<plist/>"), 0o644)
	os.WriteFile(supervisorPlist, []byte("<plist/>"), 0o644)

	localBin := filepath.Join(home, ".local", "bin")
	os.MkdirAll(localBin, 0o755)
	sirsiBin := filepath.Join(localBin, "sirsi")
	os.WriteFile(sirsiBin, []byte("bin"), 0o755)

	cfg := filepath.Join(home, ".config", "sirsi")
	os.MkdirAll(cfg, 0o755)
	os.WriteFile(filepath.Join(cfg, "surface"), []byte("cli"), 0o644)

	app := filepath.Join(home, "Applications", "Sirsi Menubar.app")
	os.MkdirAll(app, 0o755)

	acted, errs := Uninstall(false)
	if len(errs) != 0 {
		t.Fatalf("errs = %v", errs)
	}
	// 2 plists + 1 binary + 1 config + 1 app + the always-listed tcc entry.
	if len(acted) != 6 {
		t.Fatalf("acted = %d targets (%+v), want 6", len(acted), acted)
	}
	for _, gone := range []string{menubarPlist, supervisorPlist, sirsiBin, cfg} {
		if targetExists(gone) {
			t.Errorf("%s still exists after uninstall", gone)
		}
	}
	if runtime.GOOS == "darwin" {
		joined := strings.Join(execCalls, ",")
		for _, want := range []string{"launchctl", "osascript", "tccutil"} {
			if !strings.Contains(joined, want) {
				t.Errorf("expected %s call, got %v", want, execCalls)
			}
		}
	} else if !targetExists(app) {
		// Off darwin trash falls back to RemoveAll.
		_ = app // removed as expected
	}
}

func TestUninstallSurfacesTrashErrors(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Finder trash path is macOS only")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)

	old := getUninstallExec()
	defer setUninstallExec(old)
	setUninstallExec(func(name string, args ...string) error {
		if name == "osascript" {
			return fmt.Errorf("finder said no")
		}
		return nil
	})

	app := filepath.Join(home, "Applications", "Sirsi Menubar.app")
	os.MkdirAll(app, 0o755)

	_, errs := Uninstall(false)
	if len(errs) == 0 || !strings.Contains(strings.Join(errs, ";"), "finder said no") {
		t.Fatalf("errs = %v, want trash failure surfaced", errs)
	}
}
