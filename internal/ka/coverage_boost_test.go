package ka

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// ── classifySource ───────────────────────────────────────────────────────

func TestClassifySource(t *testing.T) {
	t.Parallel()
	homeDir, _ := os.UserHomeDir()

	tests := []struct {
		name         string
		appPath      string
		obtainedFrom string
		want         string
	}{
		{"app store", "/Applications/Pages.app", "mac_app_store", "appstore"},
		{"user applications", filepath.Join(homeDir, "Applications", "MyApp.app"), "identified_developer", "user-applications"},
		{"system applications", "/Applications/Safari.app", "apple", "applications"},
		{"unknown source", "/Applications/Other.app", "", "applications"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifySource(tt.appPath, tt.obtainedFrom)
			if got != tt.want {
				t.Errorf("classifySource(%q, %q) = %q, want %q", tt.appPath, tt.obtainedFrom, got, tt.want)
			}
		})
	}
}

// ── caskToAppName ────────────────────────────────────────────────────────

func TestCaskToAppName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		cask string
		want string
	}{
		{"visual-studio-code", "Visual Studio Code"},
		{"firefox", "Firefox"},
		{"google-chrome", "Google Chrome"},
		{"1password", "1password"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.cask, func(t *testing.T) {
			got := caskToAppName(tt.cask)
			if got != tt.want {
				t.Errorf("caskToAppName(%q) = %q, want %q", tt.cask, got, tt.want)
			}
		})
	}
}

// ── bundleIDPrefix ───────────────────────────────────────────────────────

func TestBundleIDPrefix(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  string
	}{
		{"com.adobe.Acrobat", "com.adobe"},
		{"net.whatsapp.WhatsApp", "net.whatsapp"},
		{"com.app", ""},
		{"single", ""},
		{"", ""},
		{"com.example.sub.deep", "com.example"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := bundleIDPrefix(tt.input)
			if got != tt.want {
				t.Errorf("bundleIDPrefix(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ── normalizeAppName ─────────────────────────────────────────────────────

func TestNormalizeAppName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  string
	}{
		{"CleanMyMac4", "cleanmymac"},
		{"WhatsAppSMB", "whatsappsmb"},
		{"Acrobat_webcapture", "acrobatwebcapture"},
		{"Firefox", "firefox"},
		{"app123", "app"},
		{"", ""},
		{"123", ""},
		{"My-Cool-App", "mycoolapp"},
		{"App 2023", "app"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeAppName(tt.input)
			if got != tt.want {
				t.Errorf("normalizeAppName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ── ghostBelongsToInstalledApp ───────────────────────────────────────────

func TestGhostBelongsToInstalledApp(t *testing.T) {
	t.Parallel()

	bundles := map[string]bool{"com.adobe.Acrobat": true}
	bundlePrefixes := map[string]bool{"com.adobe": true}
	names := map[string]bool{"firefox": true, "whatsapp": true}
	nameNorms := map[string]bool{"firefox": true, "whatsapp": true}

	tests := []struct {
		name    string
		ghost   Ghost
		belongs bool
	}{
		{
			"exact bundle match",
			Ghost{BundleID: "com.adobe.Acrobat"},
			true,
		},
		{
			"bundle prefix match",
			Ghost{BundleID: "com.adobe.CRDaemon"},
			true,
		},
		{
			"exact name match",
			Ghost{AppName: "Firefox"},
			true,
		},
		{
			"normalized name substring",
			Ghost{AppName: "WhatsAppSMB"},
			true,
		},
		{
			"no match",
			Ghost{BundleID: "com.parallels.desktop", AppName: "Parallels"},
			false,
		},
		{
			"empty ghost",
			Ghost{},
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ghostBelongsToInstalledApp(tt.ghost, bundles, bundlePrefixes, names, nameNorms)
			if got != tt.belongs {
				t.Errorf("ghostBelongsToInstalledApp() = %v, want %v", got, tt.belongs)
			}
		})
	}
}

// ── buildResidualPaths ───────────────────────────────────────────────────

func TestBuildResidualPaths_BundleID(t *testing.T) {
	t.Parallel()
	paths := buildResidualPaths("/Users/test", "com.example.app", "")
	if len(paths) == 0 {
		t.Fatal("expected residual paths for bundle ID")
	}
	// Should contain Preferences, Caches, Containers, etc.
	foundPrefs := false
	foundCaches := false
	for _, p := range paths {
		if filepath.Base(p) == "com.example.app.plist" {
			foundPrefs = true
		}
		if filepath.Base(p) == "com.example.app" {
			foundCaches = true
		}
	}
	if !foundPrefs {
		t.Error("expected preferences plist path")
	}
	if !foundCaches {
		t.Error("expected caches path")
	}
}

func TestBuildResidualPaths_AppName(t *testing.T) {
	t.Parallel()
	paths := buildResidualPaths("/Users/test", "", "MyApp")
	if len(paths) == 0 {
		t.Fatal("expected residual paths for app name")
	}
}

func TestBuildResidualPaths_Both(t *testing.T) {
	t.Parallel()
	paths := buildResidualPaths("/Users/test", "com.example.app", "MyApp")
	if len(paths) < 10 {
		t.Errorf("expected at least 10 paths with both bundleID and appName, got %d", len(paths))
	}
}

func TestBuildResidualPaths_Empty(t *testing.T) {
	t.Parallel()
	paths := buildResidualPaths("/Users/test", "", "")
	if len(paths) != 0 {
		t.Errorf("expected 0 paths with empty args, got %d", len(paths))
	}
}

func TestBuildResidualPaths_CaseInsensitive(t *testing.T) {
	t.Parallel()
	paths := buildResidualPaths("/Users/test", "", "MyApp")
	// Should generate lowercase variant paths too
	foundLower := false
	for _, p := range paths {
		if filepath.Base(p) == "myapp" {
			foundLower = true
		}
	}
	if !foundLower {
		t.Error("expected lowercase variant path")
	}
}

// ── Uninstall ────────────────────────────────────────────────────────────

func TestUninstall_NoArgs(t *testing.T) {
	t.Parallel()
	_, err := Uninstall(UninstallOptions{})
	if err == nil {
		t.Error("Uninstall with no args should error")
	}
}

func TestUninstall_DryRun_AppPath(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	appDir := filepath.Join(tmp, "Test.app")
	os.MkdirAll(appDir, 0755)
	os.WriteFile(filepath.Join(appDir, "dummy"), make([]byte, 100), 0644)

	result, err := Uninstall(UninstallOptions{
		AppPath: appDir,
		DryRun:  true,
	})
	if err != nil {
		t.Fatalf("Uninstall(DryRun): %v", err)
	}
	if result == nil {
		t.Fatal("result should not be nil")
	}
	if !result.AppRemoved {
		t.Error("AppRemoved should be true for dry run")
	}
}

func TestUninstall_DryRun_Complete(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	// Create fake residual
	prefsDir := filepath.Join(tmp, "Library", "Preferences")
	os.MkdirAll(prefsDir, 0755)
	os.WriteFile(filepath.Join(prefsDir, "com.test.app.plist"), []byte("pref"), 0644)

	result, err := Uninstall(UninstallOptions{
		BundleID: "com.test.app",
		AppName:  "TestApp",
		Complete: true,
		DryRun:   true,
	})
	if err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	if result == nil {
		t.Fatal("result should not be nil")
	}
}

func TestUninstall_MissingAppPath(t *testing.T) {
	t.Parallel()
	result, err := Uninstall(UninstallOptions{
		AppPath: "/nonexistent/Test.app",
		DryRun:  true,
	})
	if err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	if result.AppRemoved {
		t.Error("should not be marked removed when app doesn't exist")
	}
}

// ── UninstallResult / UninstallOptions structs ───────────────────────────

func TestUninstallResult_Defaults(t *testing.T) {
	t.Parallel()
	r := UninstallResult{}
	if r.AppRemoved {
		t.Error("default AppRemoved should be false")
	}
	if r.FilesRemoved != 0 {
		t.Error("default FilesRemoved should be 0")
	}
}

func TestUninstallOptions_Defaults(t *testing.T) {
	t.Parallel()
	o := UninstallOptions{}
	if o.Complete {
		t.Error("default Complete should be false")
	}
	if o.DryRun {
		t.Error("default DryRun should be false")
	}
	if o.UseTrash {
		t.Error("default UseTrash should be false")
	}
}

// ── InstalledApp struct ──────────────────────────────────────────────────

func TestInstalledApp_Defaults(t *testing.T) {
	t.Parallel()
	app := InstalledApp{}
	if app.Name != "" {
		t.Error("default Name should be empty")
	}
	if app.IsRunning {
		t.Error("default IsRunning should be false")
	}
	if app.HasGhosts {
		t.Error("default HasGhosts should be false")
	}
}

// ── LinuxProvider BuildInstalledIndex ─────────────────────────────────────

func TestLinuxProvider_BuildInstalledIndex(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	desktopDir := filepath.Join(tmp, "applications")
	os.MkdirAll(desktopDir, 0755)

	// Create a fake .desktop file
	desktopContent := "[Desktop Entry]\nName=Test App\nExec=/usr/bin/testapp\n"
	os.WriteFile(filepath.Join(desktopDir, "testapp.desktop"), []byte(desktopContent), 0644)

	s := NewScanner()
	s.homeDir = tmp
	// Override DirReader to include our test dir
	s.DirReader = func(path string) ([]os.DirEntry, error) {
		if path == desktopDir || path == "/usr/share/applications" {
			return os.ReadDir(desktopDir)
		}
		return nil, os.ErrNotExist
	}
	s.SkipBrew = true

	p := &LinuxProvider{}
	err := p.BuildInstalledIndex(context.Background(), s)
	if err != nil {
		t.Fatalf("BuildInstalledIndex: %v", err)
	}
	if !s.installedNames["testapp"] {
		t.Error("expected 'testapp' in installed names")
	}
}

func TestLinuxProvider_ParseDesktopName(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	desktopContent := "[Desktop Entry]\nName=Firefox Web Browser\nExec=/usr/bin/firefox\nType=Application\n"
	path := filepath.Join(tmp, "firefox.desktop")
	os.WriteFile(path, []byte(desktopContent), 0644)

	p := &LinuxProvider{}
	name := p.parseDesktopName(path)
	if name != "Firefox Web Browser" {
		t.Errorf("parseDesktopName = %q, want 'Firefox Web Browser'", name)
	}
}

func TestLinuxProvider_ParseDesktopName_Missing(t *testing.T) {
	t.Parallel()
	p := &LinuxProvider{}
	name := p.parseDesktopName("/nonexistent/file.desktop")
	if name != "" {
		t.Errorf("missing file should return empty, got %q", name)
	}
}

func TestLinuxProvider_ParseDesktopExec(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	desktopContent := "[Desktop Entry]\nExec=/usr/bin/firefox %u\nName=Firefox\n"
	path := filepath.Join(tmp, "firefox.desktop")
	os.WriteFile(path, []byte(desktopContent), 0644)

	p := &LinuxProvider{}
	execPath := p.parseDesktopExec(path)
	if execPath != "/usr/bin/firefox" {
		t.Errorf("parseDesktopExec = %q, want '/usr/bin/firefox'", execPath)
	}
}

func TestLinuxProvider_ParseDesktopExec_Missing(t *testing.T) {
	t.Parallel()
	p := &LinuxProvider{}
	execPath := p.parseDesktopExec("/nonexistent/file.desktop")
	if execPath != "" {
		t.Errorf("missing file should return empty, got %q", execPath)
	}
}

func TestLinuxProvider_ExtractAppID_Extensions(t *testing.T) {
	t.Parallel()
	p := &LinuxProvider{}
	tests := []struct {
		input string
		want  string
	}{
		{"firefox.conf", "firefox"},
		{"myapp.cfg", "myapp"},
		{"vlc", "vlc"},
	}
	for _, tt := range tests {
		got := p.ExtractAppID(tt.input)
		if got != tt.want {
			t.Errorf("ExtractAppID(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// ── WindowsProvider ExtractAppID ─────────────────────────────────────────

func TestWindowsProvider_ExtractAppID_Extensions(t *testing.T) {
	t.Parallel()
	p := &WindowsProvider{}
	tests := []struct {
		input string
		want  string
	}{
		{"MyApp.cfg", "myapp"},
		{"Program.ini", "program"},
		{"ShortName", "shortname"},
	}
	for _, tt := range tests {
		got := p.ExtractAppID(tt.input)
		if got != tt.want {
			t.Errorf("ExtractAppID(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// ── GhostRegistration (from apps.go) ─────────────────────────────────────

func TestGhostRegistration_Struct(t *testing.T) {
	t.Parallel()
	gr := GhostRegistration{
		BundleID: "com.test.dead",
		Path:     "/Applications/Dead.app",
	}
	if gr.BundleID != "com.test.dead" {
		t.Errorf("BundleID = %q", gr.BundleID)
	}
	if gr.Path != "/Applications/Dead.app" {
		t.Errorf("Path = %q", gr.Path)
	}
}

// ── LinuxProvider ScanRegistry ────────────────────────────────────────────

func TestLinuxProvider_ScanRegistry(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	// Use the home-based path that ScanRegistry constructs
	homeDesktopDir := filepath.Join(tmp, ".local", "share", "applications")
	os.MkdirAll(homeDesktopDir, 0755)

	// Create a .desktop file pointing to a non-existent exec
	desktopContent := "[Desktop Entry]\nName=Dead App\nExec=/nonexistent/deadapp\nType=Application\n"
	os.WriteFile(filepath.Join(homeDesktopDir, "deadapp.desktop"), []byte(desktopContent), 0644)

	// Create a .desktop file pointing to an existing exec
	liveContent := "[Desktop Entry]\nName=Echo\nExec=/bin/echo\nType=Application\n"
	os.WriteFile(filepath.Join(homeDesktopDir, "echo.desktop"), []byte(liveContent), 0644)

	s := NewScanner()
	s.homeDir = tmp
	// Use real DirReader so file paths match
	s.DirReader = os.ReadDir

	p := &LinuxProvider{}
	ghosts := p.ScanRegistry(context.Background(), s)
	if !ghosts["deadapp"] {
		t.Errorf("expected deadapp to be a ghost, got ghosts: %v", ghosts)
	}
	if ghosts["echo"] {
		t.Error("echo should not be a ghost (binary exists)")
	}
}

// ── LinuxProvider indexDpkg ───────────────────────────────────────────────

func TestLinuxProvider_IndexDpkg_Mocked(t *testing.T) {
	t.Parallel()
	s := NewScanner()
	s.ExecCommand = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		if name == "dpkg" {
			return exec.CommandContext(ctx, "printf", "firefox:amd64\tinstall\nvlc\tinstall\nremoved-pkg\tdeinstall\n")
		}
		return exec.CommandContext(ctx, "true")
	}

	p := &LinuxProvider{}
	p.indexDpkg(context.Background(), s)

	if !s.installedNames["firefox"] {
		t.Error("expected firefox (with arch stripped) in installed names")
	}
	if !s.installedNames["vlc"] {
		t.Error("expected vlc in installed names")
	}
	if s.installedNames["removed-pkg"] {
		t.Error("deinstalled packages should not be in installed names")
	}
}

// ── enrichApp ────────────────────────────────────────────────────────────

func TestEnrichApp_NoPath(t *testing.T) {
	t.Parallel()
	app := InstalledApp{Name: "NoPath"}
	ghostByBundleID := map[string]Ghost{}
	ghostByName := map[string]Ghost{}
	// Should not panic
	enrichApp(context.Background(), &app, ghostByBundleID, ghostByName)
	// No path means no enrichment
}

func TestEnrichApp_WithGhostByName(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping enrichApp test in short mode")
	}
	t.Parallel()
	tmp := t.TempDir()
	appDir := filepath.Join(tmp, "Test.app")
	os.MkdirAll(filepath.Join(appDir, "Contents"), 0755)

	app := InstalledApp{Name: "Test", Path: appDir}
	ghostByBundleID := map[string]Ghost{}
	ghostByName := map[string]Ghost{
		"test": {AppName: "Test", TotalFiles: 5, TotalSize: 1024},
	}
	enrichApp(context.Background(), &app, ghostByBundleID, ghostByName)
	if !app.HasGhosts {
		t.Error("should detect ghosts by name")
	}
	if app.GhostCount != 5 {
		t.Errorf("GhostCount = %d, want 5", app.GhostCount)
	}
}

// ── Uninstall Complete with real residuals ────────────────────────────────

func TestUninstall_Complete_WithResiduals(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	// Create a fake home with Library structure
	homeDir := filepath.Join(tmp, "home")
	prefsDir := filepath.Join(homeDir, "Library", "Preferences")
	cachesDir := filepath.Join(homeDir, "Library", "Caches")
	os.MkdirAll(prefsDir, 0755)
	os.MkdirAll(cachesDir, 0755)

	// Create residuals
	os.WriteFile(filepath.Join(prefsDir, "com.test.uninstall.plist"), []byte("pref"), 0644)
	os.MkdirAll(filepath.Join(cachesDir, "com.test.uninstall"), 0755)
	os.WriteFile(filepath.Join(cachesDir, "com.test.uninstall", "cache.db"), make([]byte, 256), 0644)

	// Can't easily test with real home dir, but we can test the paths are built
	paths := buildResidualPaths(homeDir, "com.test.uninstall", "TestUninstall")
	if len(paths) == 0 {
		t.Fatal("expected residual paths")
	}

	// Check various path types exist in the output
	foundLaunchAgent := false
	foundAppScripts := false
	for _, p := range paths {
		if filepath.Dir(p) == filepath.Join(homeDir, "Library", "LaunchAgents") {
			foundLaunchAgent = true
		}
		if filepath.Dir(p) == filepath.Join(homeDir, "Library", "Application Scripts") {
			foundAppScripts = true
		}
	}
	if !foundLaunchAgent {
		t.Error("expected LaunchAgents path")
	}
	if !foundAppScripts {
		t.Error("expected Application Scripts path")
	}
}

// ── LinuxProvider more edge cases ────────────────────────────────────────

func TestLinuxProvider_IsSystemID_Prefixes(t *testing.T) {
	t.Parallel()
	p := &LinuxProvider{}
	tests := []struct {
		id     string
		system bool
	}{
		{"networkmanager", true},
		{"network-manager", true},
		{"gtk-3.0", true},
		{"gtk-4.0", true},
		{"fontconfig", true},
		{"apt", true},
		{"dpkg", true},
		{"flatpak", true},
		{"snap", true},
		{"bash", true},
		{"zsh", true},
		{"fish", true},
		{"myapp", false},
		{"custom-tool", false},
	}
	for _, tt := range tests {
		got := p.IsSystemID(tt.id)
		if got != tt.system {
			t.Errorf("IsSystemID(%q) = %v, want %v", tt.id, got, tt.system)
		}
	}
}

// ── WindowsProvider IsSystemID edge cases ────────────────────────────────

func TestWindowsProvider_IsSystemID_EdgeCases(t *testing.T) {
	t.Parallel()
	p := &WindowsProvider{}
	tests := []struct {
		id     string
		system bool
	}{
		{"WindowsApps", true},
		{"Packages", true},
		{"ConnectedDevicesPlatform", true},
		{"D3DSCache", true},
		{"Comms", true},
		{"CustomApp", false},
		{"MyTool", false},
	}
	for _, tt := range tests {
		got := p.IsSystemID(tt.id)
		if got != tt.system {
			t.Errorf("IsSystemID(%q) = %v, want %v", tt.id, got, tt.system)
		}
	}
}
