// apps_seam_test.go exercises the app-enumeration pipeline through the
// injectable seams (PANTHEON_RULES A16/A21). All system calls are swapped
// for canned data; the filesystem is confined to t.TempDir().
//
// NOTE: these tests swap package-level function pointers — they must NOT
// use t.Parallel() (see PRs #129/#131).
package ka

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// swapSeam replaces a package-level seam for the duration of a test.
func swapSeam[T any](t *testing.T, target *T, replacement T) {
	t.Helper()
	old := *target
	*target = replacement
	t.Cleanup(func() { *target = old })
}

// ── enumerateSystemProfiler ─────────────────────────────────────────────

func TestEnumerateSystemProfiler_Canned(t *testing.T) {
	swapSeam(t, &systemProfilerJSON, func(ctx context.Context) ([]byte, error) {
		return []byte(`[{"_items": [
			{"_name": "Zzqx Store App", "version": "2.1", "path": "/Applications/Zzqx Store App.app", "info": "com.zzqx.store", "obtained_from": "mac_app_store"},
			{"_name": "Zzqx Plain", "version": "1.0", "path": "/Applications/Zzqx Plain.app", "obtained_from": "identified_developer"}
		]}]`), nil
	})

	apps, err := enumerateSystemProfiler(context.Background())
	if err != nil {
		t.Fatalf("enumerateSystemProfiler: %v", err)
	}
	if len(apps) != 2 {
		t.Fatalf("got %d apps, want 2", len(apps))
	}
	if apps[0].Source != "appstore" {
		t.Errorf("store app source = %q, want appstore", apps[0].Source)
	}
	if apps[0].BundleID != "com.zzqx.store" {
		t.Errorf("store app bundle = %q, want com.zzqx.store", apps[0].BundleID)
	}
	if apps[1].Source != "applications" {
		t.Errorf("plain app source = %q, want applications", apps[1].Source)
	}
	if apps[1].BundleID != "" {
		t.Errorf("plain app bundle = %q, want empty", apps[1].BundleID)
	}
}

func TestEnumerateSystemProfiler_Errors(t *testing.T) {
	swapSeam(t, &systemProfilerJSON, func(ctx context.Context) ([]byte, error) {
		return nil, fmt.Errorf("profiler exploded")
	})
	if _, err := enumerateSystemProfiler(context.Background()); err == nil {
		t.Error("want error when system_profiler fails")
	}

	swapSeam(t, &systemProfilerJSON, func(ctx context.Context) ([]byte, error) {
		return []byte("not json"), nil
	})
	if _, err := enumerateSystemProfiler(context.Background()); err == nil {
		t.Error("want error on invalid JSON")
	}
}

// ── enumerateHomebrew ───────────────────────────────────────────────────

func TestEnumerateHomebrew_Canned(t *testing.T) {
	home := t.TempDir()
	// Cask name chosen so /Applications/<name>.app can never exist on a
	// real machine; the ~/Applications fallback is created in the temp home.
	appDir := filepath.Join(home, "Applications")
	if err := os.MkdirAll(filepath.Join(appDir, "Zzqx Test Cask.app"), 0o755); err != nil {
		t.Fatal(err)
	}

	swapSeam(t, &brewListCasks, func(ctx context.Context) ([]byte, error) {
		return []byte("zzqx-test-cask\n\nzzqx-missing-cask\n"), nil
	})
	swapSeam(t, &brewCaskInfoJSON, func(ctx context.Context, cask string) ([]byte, error) {
		if cask == "zzqx-test-cask" {
			return []byte(`{"casks": [{"version": "3.7.1"}]}`), nil
		}
		return nil, fmt.Errorf("no such cask")
	})
	swapSeam(t, &readBundleIDFile, func(ctx context.Context, path string) (string, error) {
		return "com.zzqx.cask", nil
	})

	apps, err := enumerateHomebrew(context.Background(), home)
	if err != nil {
		t.Fatalf("enumerateHomebrew: %v", err)
	}
	if len(apps) != 2 {
		t.Fatalf("got %d apps, want 2", len(apps))
	}

	found := apps[0]
	if found.Name != "Zzqx Test Cask" {
		t.Errorf("name = %q, want 'Zzqx Test Cask'", found.Name)
	}
	if found.Path != filepath.Join(appDir, "Zzqx Test Cask.app") {
		t.Errorf("path = %q, want the ~/Applications bundle", found.Path)
	}
	if found.Version != "3.7.1" {
		t.Errorf("version = %q, want 3.7.1", found.Version)
	}
	if found.BundleID != "com.zzqx.cask" {
		t.Errorf("bundle = %q, want com.zzqx.cask", found.BundleID)
	}

	missing := apps[1]
	if missing.Path != "" {
		t.Errorf("missing cask path = %q, want empty", missing.Path)
	}
	if missing.Version != "" {
		t.Errorf("missing cask version = %q, want empty", missing.Version)
	}
}

func TestEnumerateHomebrew_ListError(t *testing.T) {
	swapSeam(t, &brewListCasks, func(ctx context.Context) ([]byte, error) {
		return nil, fmt.Errorf("brew not installed")
	})
	if _, err := enumerateHomebrew(context.Background(), t.TempDir()); err == nil {
		t.Error("want error when brew list fails")
	}
}

// ── brewCaskVersion ─────────────────────────────────────────────────────

func TestBrewCaskVersion_Canned(t *testing.T) {
	tests := []struct {
		name string
		out  []byte
		err  error
		want string
	}{
		{"valid", []byte(`{"casks": [{"version": "1.2.3"}]}`), nil, "1.2.3"},
		{"empty casks", []byte(`{"casks": []}`), nil, ""},
		{"invalid json", []byte("garbage"), nil, ""},
		{"exec error", nil, fmt.Errorf("boom"), ""},
	}
	for _, tt := range tests {
		swapSeam(t, &brewCaskInfoJSON, func(ctx context.Context, cask string) ([]byte, error) {
			return tt.out, tt.err
		})
		if got := brewCaskVersion(context.Background(), "any"); got != tt.want {
			t.Errorf("%s: brewCaskVersion = %q, want %q", tt.name, got, tt.want)
		}
	}
}

// ── enumerateAppDirs / walkAppsUnder ────────────────────────────────────

func TestEnumerateAppDirs_TempDirs(t *testing.T) {
	root := t.TempDir()
	sys := filepath.Join(root, "SysApps")
	usr := filepath.Join(root, "UserApps")
	// Top-level app, nested app one level down, non-app noise, missing dir.
	mustMkdir(t, filepath.Join(sys, "Alpha.app"))
	mustMkdir(t, filepath.Join(sys, "Bundle.localized", "Beta.app"))
	if err := os.WriteFile(filepath.Join(sys, "README.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	swapSeam(t, &appDirSources, func(homeDir string) []appDirSource {
		return []appDirSource{
			{sys, "applications"},
			{usr, "user-applications"}, // does not exist — must be skipped
		}
	})
	swapSeam(t, &readBundleIDFile, func(ctx context.Context, path string) (string, error) {
		switch filepath.Base(path) {
		case "Alpha.app":
			return "com.zzqx.alpha", nil
		case "Beta.app":
			return "com.zzqx.beta", nil
		}
		return "", fmt.Errorf("no bundle")
	})

	apps := enumerateAppDirs(context.Background(), root)
	if len(apps) != 2 {
		t.Fatalf("got %d apps, want 2: %+v", len(apps), apps)
	}
	byName := map[string]InstalledApp{}
	for _, a := range apps {
		byName[a.Name] = a
	}
	if byName["Alpha"].BundleID != "com.zzqx.alpha" {
		t.Errorf("Alpha bundle = %q", byName["Alpha"].BundleID)
	}
	if byName["Beta"].BundleID != "com.zzqx.beta" {
		t.Errorf("Beta bundle = %q (nested app must be found)", byName["Beta"].BundleID)
	}
	if byName["Alpha"].Source != "applications" {
		t.Errorf("Alpha source = %q", byName["Alpha"].Source)
	}
}

func TestWalkAppsUnder_DepthAndAppBoundaries(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "L1.app"))
	mustMkdir(t, filepath.Join(root, "a", "b", "L3.app"))
	mustMkdir(t, filepath.Join(root, "a", "b", "c", "TooDeep.app"))
	// An .app nested INSIDE an .app must not be descended into.
	mustMkdir(t, filepath.Join(root, "L1.app", "Contents", "Inner.app"))

	swapSeam(t, &readBundleIDFile, func(ctx context.Context, path string) (string, error) {
		return "com.zzqx." + strings.ToLower(strings.TrimSuffix(filepath.Base(path), ".app")), nil
	})

	apps := walkAppsUnder(context.Background(), root, "applications", 3)
	names := map[string]bool{}
	for _, a := range apps {
		names[a.Name] = true
	}
	if !names["L1"] || !names["L3"] {
		t.Errorf("expected L1 and L3, got %v", names)
	}
	if names["TooDeep"] {
		t.Error("TooDeep.app is beyond maxDepth and must not be enumerated")
	}
	if names["Inner"] {
		t.Error("apps inside .app bundles must not be enumerated")
	}

	if got := walkAppsUnder(context.Background(), root, "applications", 0); got != nil {
		t.Errorf("maxDepth 0 must return nil, got %v", got)
	}
	if got := walkAppsUnder(context.Background(), filepath.Join(root, "nope"), "applications", 2); got != nil {
		t.Errorf("unreadable dir must return nil, got %v", got)
	}
}

// ── ScanLaunchServicesGhosts ────────────────────────────────────────────

func TestScanLaunchServicesGhosts_CannedDump(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Launch Services scan is macOS-only")
	}
	tmp := t.TempDir()
	liveApp := filepath.Join(tmp, "LiveApp.app")
	mustMkdir(t, liveApp)
	ghostPath := filepath.Join(tmp, "GhostApp.app")
	finalPath := filepath.Join(tmp, "Final.app")

	sep := strings.Repeat("-", 80)
	dump := strings.Join([]string{
		"bundle id: com.dead.ghostapp",
		"name: Ghost App",
		"path: " + ghostPath,
		sep,
		"bundle id: com.live.app",
		"path: " + liveApp + " (0x1234)", // suffix junk → truncated at .app
		sep,
		"bundle id: com.apple.Safari", // system — skipped
		"path: /Applications/Safari.app",
		sep,
		"bundle id: com.dead.ghostapp", // duplicate — deduped
		"path: " + ghostPath,
		sep,
		"bundle id: com.no.appfile", // no .app in path — skipped
		"path: /usr/local/bin/thing",
		sep,
		"bundle id: com.dead.final", // final block without trailing separator
		"path: " + finalPath,
	}, "\n")

	swapSeam(t, &lsRegisterStream, func(ctx context.Context) (io.Reader, func(), error) {
		return strings.NewReader(dump), func() {}, nil
	})

	ghosts, err := ScanLaunchServicesGhosts(context.Background())
	if err != nil {
		t.Fatalf("ScanLaunchServicesGhosts: %v", err)
	}
	if len(ghosts) != 2 {
		t.Fatalf("got %d ghosts, want 2: %+v", len(ghosts), ghosts)
	}
	if ghosts[0].BundleID != "com.dead.ghostapp" || ghosts[0].Name != "Ghost App" {
		t.Errorf("ghost[0] = %+v, want com.dead.ghostapp / Ghost App", ghosts[0])
	}
	if ghosts[1].BundleID != "com.dead.final" || ghosts[1].Name != "com.dead.final" {
		t.Errorf("ghost[1] = %+v, want com.dead.final with bundle-id fallback name", ghosts[1])
	}
}

func TestScanLaunchServicesGhosts_StreamError(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Launch Services scan is macOS-only")
	}
	swapSeam(t, &lsRegisterStream, func(ctx context.Context) (io.Reader, func(), error) {
		return nil, nil, fmt.Errorf("lsregister unavailable")
	})
	if _, err := ScanLaunchServicesGhosts(context.Background()); err == nil {
		t.Error("want error when lsregister cannot start")
	}
}

// ── EnumerateApps end-to-end through seams ──────────────────────────────

func TestEnumerateApps_SeamMocked(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("EnumerateApps is macOS-only")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)

	dirRoot := t.TempDir()
	mustMkdir(t, filepath.Join(dirRoot, "DirOnly.app"))

	// Homebrew cask installed under the fake ~/Applications so its path
	// resolves; it deliberately collides with a profiler entry to exercise
	// the source-override merge branch.
	brewApp := filepath.Join(home, "Applications", "Zzqx Brewed.app")
	mustMkdir(t, brewApp)

	profilerJSON := fmt.Sprintf(`[{"_items": [
		{"_name": "Zzqx Profiler", "version": "9.0", "path": "/Applications/Zzqx Profiler.app", "info": "com.zzqx.profiler", "obtained_from": "identified_developer"},
		{"_name": "Zzqx Brewed", "version": "1.0", "path": %q, "info": "com.zzqx.brewed", "obtained_from": "identified_developer"}
	]}]`, brewApp)

	swapSeam(t, &systemProfilerJSON, func(ctx context.Context) ([]byte, error) {
		return []byte(profilerJSON), nil
	})
	swapSeam(t, &brewListCasks, func(ctx context.Context) ([]byte, error) {
		return []byte("zzqx-brewed\n"), nil
	})
	swapSeam(t, &brewCaskInfoJSON, func(ctx context.Context, cask string) ([]byte, error) {
		return []byte(`{"casks": [{"version": "1.0"}]}`), nil
	})
	swapSeam(t, &readBundleIDFile, func(ctx context.Context, path string) (string, error) {
		switch filepath.Base(path) {
		case "Zzqx Brewed.app":
			return "com.zzqx.brewed", nil
		case "DirOnly.app":
			return "com.zzqx.dironly", nil
		}
		return "", fmt.Errorf("no bundle")
	})
	swapSeam(t, &appDirSources, func(homeDir string) []appDirSource {
		return []appDirSource{{dirRoot, "applications"}}
	})
	swapSeam(t, &ghostScanFn, func(ctx context.Context) ([]Ghost, error) {
		return []Ghost{
			// Residual of an installed app family — must be filtered out.
			{AppName: "Profiler Helper", BundleID: "com.zzqx.profiler.helper", TotalFiles: 3, TotalSize: 300},
			// True ghost — must be appended as a pure-ghost entry.
			{AppName: "DeadZq", BundleID: "com.deadzq.gone", TotalFiles: 7, TotalSize: 700},
		}, nil
	})
	// enrichApp shells out (pgrep/du/mdls) per app — no-op it; its internals
	// are covered separately.
	swapSeam(t, &enrichAppFn, func(ctx context.Context, app *InstalledApp, _, _ map[string]Ghost) {})

	apps, err := EnumerateApps(context.Background())
	if err != nil {
		t.Fatalf("EnumerateApps: %v", err)
	}

	byName := map[string]InstalledApp{}
	for _, a := range apps {
		byName[a.Name] = a
	}

	if _, ok := byName["Zzqx Profiler"]; !ok {
		t.Error("profiler app missing from inventory")
	}
	if _, ok := byName["DirOnly"]; !ok {
		t.Error("direct-scan app missing from inventory")
	}
	if got := byName["Zzqx Brewed"].Source; got != "homebrew" {
		t.Errorf("brew-managed app source = %q, want homebrew (merge override)", got)
	}

	ghost, ok := byName["DeadZq"]
	if !ok {
		t.Fatal("pure ghost DeadZq missing from inventory")
	}
	if ghost.Source != "ghost" || !ghost.HasGhosts || ghost.GhostCount != 7 || ghost.GhostSize != 700 {
		t.Errorf("ghost entry = %+v, want source=ghost with residual totals", ghost)
	}

	if _, ok := byName["Profiler Helper"]; ok {
		t.Error("residual of installed app family must not surface as a ghost app")
	}

	// Inventory must be sorted by name (case-insensitive).
	for i := 1; i < len(apps); i++ {
		if strings.ToLower(apps[i-1].Name) > strings.ToLower(apps[i].Name) {
			t.Errorf("apps not sorted: %q > %q", apps[i-1].Name, apps[i].Name)
		}
	}
}

// ── hasBundlePrefix ─────────────────────────────────────────────────────

func TestHasBundlePrefix(t *testing.T) {
	tests := []struct {
		child, parent string
		want          bool
	}{
		{"com.foo.bar", "com.foo", true},
		{"com.foo", "com.foo", true},
		{"com.foobar", "com.foo", false}, // not dot-aligned
		{"com.foo", "com.foo.bar", false},
		{"", "com.foo", false},
		{"com.foo", "", false},
	}
	for _, tt := range tests {
		if got := hasBundlePrefix(tt.child, tt.parent); got != tt.want {
			t.Errorf("hasBundlePrefix(%q, %q) = %v, want %v", tt.child, tt.parent, got, tt.want)
		}
	}
}

// ── Uninstall ───────────────────────────────────────────────────────────

func TestUninstall_RequiresIdentity(t *testing.T) {
	if _, err := Uninstall(UninstallOptions{}); err == nil {
		t.Error("want error when no AppPath/BundleID/AppName given")
	}
}

func TestUninstall_DryRunComplete(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	appPath := filepath.Join(home, "ZzqxUninstall.app")
	mustMkdir(t, appPath)
	if err := os.WriteFile(filepath.Join(appPath, "binary"), []byte("0123456789"), 0o644); err != nil {
		t.Fatal(err)
	}

	prefs := filepath.Join(home, "Library", "Preferences")
	mustMkdir(t, prefs)
	plist := filepath.Join(prefs, "com.zzqx.uninstall.plist")
	if err := os.WriteFile(plist, []byte("prefs"), 0o644); err != nil {
		t.Fatal(err)
	}
	appSupport := filepath.Join(home, "Library", "Application Support", "ZzqxUninstall")
	mustMkdir(t, appSupport)
	if err := os.WriteFile(filepath.Join(appSupport, "state.db"), []byte("db"), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := Uninstall(UninstallOptions{
		AppPath:  appPath,
		BundleID: "com.zzqx.uninstall",
		AppName:  "ZzqxUninstall",
		Complete: true,
		DryRun:   true,
		UseTrash: false,
	})
	if err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	if !res.AppRemoved {
		t.Error("AppRemoved = false, want true (dry-run still reports)")
	}
	if res.FilesRemoved < 3 {
		t.Errorf("FilesRemoved = %d, want >= 3 (app + prefs + app support)", res.FilesRemoved)
	}
	if res.BytesReclaimed <= 0 {
		t.Errorf("BytesReclaimed = %d, want > 0", res.BytesReclaimed)
	}
	// Dry-run must not delete anything.
	for _, p := range []string{appPath, plist, appSupport} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("dry-run deleted %s: %v", p, err)
		}
	}
}

func TestUninstall_MissingAppBundle(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	res, err := Uninstall(UninstallOptions{
		AppPath: filepath.Join(home, "Nope.app"),
		DryRun:  true,
	})
	if err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	if res.AppRemoved {
		t.Error("AppRemoved = true for a bundle that does not exist")
	}
	if res.FilesRemoved != 0 {
		t.Errorf("FilesRemoved = %d, want 0", res.FilesRemoved)
	}
}

// ── helpers ─────────────────────────────────────────────────────────────

func mustMkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
}
