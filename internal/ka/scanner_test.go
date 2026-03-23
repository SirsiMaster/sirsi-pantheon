package ka

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestExtractBundleID(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"plist file", "com.parallels.desktop.console.plist", "com.parallels.desktop.console"},
		{"directory name", "com.parallels.desktop.console", "com.parallels.desktop.console"},
		{"saved state", "com.apple.Safari.savedState", "com.apple.Safari"},
		{"group prefix", "group.com.docker", "com.docker"},
		{"org prefix", "org.mozilla.firefox", "org.mozilla.firefox"},
		{"io prefix", "io.github.nickvision.money", "io.github.nickvision.money"},
		{"net prefix", "net.sourceforge.skim-app.skim", "net.sourceforge.skim-app.skim"},
		{"dev prefix", "dev.zed.Zed", "dev.zed.Zed"},
		{"no dots - not bundle ID", "SomeAppName", ""},
		{"single dot - not bundle ID", "com.app", "com.app"},
		{"unknown TLD prefix", "xyz.something.app", ""},
		{"empty string", "", ""},
		{"de prefix", "de.appmaker.myapp", "de.appmaker.myapp"},
		// additional edge cases
		{"me prefix", "me.developer.myapp", "me.developer.myapp"},
		{"co prefix", "co.company.tool", "co.company.tool"},
		{"app prefix", "app.custom.widget", "app.custom.widget"},
		{"uk prefix", "uk.co.company.app", "uk.co.company.app"},
		{"fr prefix", "fr.company.product", "fr.company.product"},
		{"jp prefix", "jp.co.company.app", "jp.co.company.app"},
		{"double group prefix", "group.group.com.app", ""},
		{"plist + savedState", "com.test.app.savedState.plist", "com.test.app"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractBundleID(tt.input)
			if got != tt.expected {
				t.Errorf("extractBundleID(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestGuessAppName(t *testing.T) {
	tests := []struct {
		name     string
		bundleID string
		expected string
	}{
		{"standard 3-part", "com.parallels.desktop", "Desktop"},
		{"4-part bundle", "com.parallels.desktop.console", "Desktop"},
		{"hyphenated name", "com.apple.dt.xcode-build", "Dt"},
		{"capitalization", "com.google.chrome", "Chrome"},
		{"2-part returns raw", "com.app", "com.app"},
		{"1-part returns raw", "something", "something"},
		{"org prefix", "org.mozilla.firefox", "Firefox"},
		// additional
		{"hyphen to space", "com.company.my-cool-app", "My cool app"},
		{"empty parts", "com..empty", "Empty"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := guessAppName(tt.bundleID)
			if got != tt.expected {
				t.Errorf("guessAppName(%q) = %q, want %q", tt.bundleID, got, tt.expected)
			}
		})
	}
}

func TestIsSystemBundleID(t *testing.T) {
	tests := []struct {
		name     string
		bundleID string
		expected bool
	}{
		{"apple system", "com.apple.Safari", true},
		{"apple subsystem", "com.apple.dt.Xcode", true},
		{"google drivefs", "com.google.drivefs.helper", true},
		{"google keystone", "com.google.keystone.agent", true},
		{"microsoft onedrive", "com.microsoft.OneDrive.FinderSync", true},
		{"microsoft autoupdate", "com.microsoft.autoupdate2", true},
		{"third party app", "com.parallels.desktop", false},
		{"user app", "com.mycompany.myapp", false},
		{"mozilla", "org.mozilla.firefox", false},
		{"docker", "com.docker.docker", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isSystemBundleID(tt.bundleID)
			if got != tt.expected {
				t.Errorf("isSystemBundleID(%q) = %v, want %v", tt.bundleID, got, tt.expected)
			}
		})
	}
}

func TestExpandPath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		homeDir string
		want    string
	}{
		{"tilde expansion", "~/Library", "/Users/test", "/Users/test/Library"},
		{"no tilde", "/usr/local", "/Users/test", "/usr/local"},
		{"tilde only prefix", "~foo/bar", "/Users/test", "~foo/bar"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := expandPath(tt.path, tt.homeDir)
			if got != tt.want {
				t.Errorf("expandPath(%q, %q) = %q, want %q", tt.path, tt.homeDir, got, tt.want)
			}
		})
	}
}

func TestNewScanner(t *testing.T) {
	s := NewScanner()
	if s == nil {
		t.Fatal("NewScanner() returned nil")
	}
	if s.installedApps == nil {
		t.Error("installedApps map not initialized")
	}
	if s.installedNames == nil {
		t.Error("installedNames map not initialized")
	}
	if s.knownBundleIDs == nil {
		t.Error("knownBundleIDs map not initialized")
	}
	if s.readBundleID == nil {
		t.Error("readBundleID function not initialized")
	}
}

func TestScanner_BuildInstalledAppIndex(t *testing.T) {
	tmp := t.TempDir()
	appDir := filepath.Join(tmp, "Applications")
	os.MkdirAll(appDir, 0755)

	// Create a fake .app
	os.MkdirAll(filepath.Join(appDir, "Test.app"), 0755)

	s := NewScanner()
	s.appDirs = []string{appDir}
	s.skipBrew = true // Don't try to call brew
	s.readBundleID = func(path string) string {
		if strings.HasSuffix(path, "Test.app") {
			return "com.test.app"
		}
		return ""
	}

	err := s.buildInstalledAppIndex()
	if err != nil {
		t.Fatalf("buildInstalledAppIndex() error: %v", err)
	}

	if !s.installedApps["com.test.app"] {
		t.Error("expected com.test.app to be indexed")
	}
	if !s.installedNames["test"] {
		t.Error("expected 'test' name to be indexed")
	}
}

func TestScanner_IndexHomebrewCasks_Skip(t *testing.T) {
	s := NewScanner()
	s.appDirs = []string{}
	s.skipBrew = true

	err := s.buildInstalledAppIndex() // Should skip brew list
	if err != nil {
		t.Fatalf("buildInstalledAppIndex with skipBrew=true error: %v", err)
	}
	if len(s.installedNames) != 0 {
		t.Errorf("expected 0 installed names, got %d", len(s.installedNames))
	}
}

// ═══════════════════════════════════════════
// isInstalled
// ═══════════════════════════════════════════

func TestIsInstalled_ByBundleID(t *testing.T) {
	s := &Scanner{
		installedApps:  map[string]bool{"com.example.app": true},
		installedNames: map[string]bool{},
		knownBundleIDs: map[string]string{},
	}

	if !s.isInstalled("com.example.app", "anything") {
		t.Error("should be installed by bundle ID")
	}
}

func TestIsInstalled_ByName(t *testing.T) {
	s := &Scanner{
		installedApps:  map[string]bool{},
		installedNames: map[string]bool{"firefox": true},
		knownBundleIDs: map[string]string{},
	}

	if !s.isInstalled("org.mozilla.firefox", "com.mozilla.firefox.plist") {
		t.Error("should match by name substring")
	}
}

func TestIsInstalled_NoMatch(t *testing.T) {
	s := &Scanner{
		installedApps:  map[string]bool{},
		installedNames: map[string]bool{"safari": true},
		knownBundleIDs: map[string]string{},
	}

	if s.isInstalled("com.parallels.desktop", "com.parallels.desktop.plist") {
		t.Error("should NOT match — parallels is not installed")
	}
}

func TestScanner_ScanForOrphans(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	prefsDir := filepath.Join(home, "Library/Preferences")
	os.MkdirAll(prefsDir, 0755)

	// Create a residual for an app that is NOT installed
	residualFile := filepath.Join(prefsDir, "com.dead.app.plist")
	os.WriteFile(residualFile, []byte("junk"), 0644)

	// Create a residual for an app that IS installed
	installedFile := filepath.Join(prefsDir, "com.live.app.plist")
	os.WriteFile(installedFile, []byte("live"), 0644)

	s := NewScanner()
	s.homeDir = home
	s.installedApps["com.live.app"] = true

	// Mock locations to only look in our temp dir
	// Save current locations to restore later
	originalLocations := userResidualLocations
	userResidualLocations = []residualLocation{
		{ResidualPreferences, "~/Library/Preferences", false},
	}
	defer func() { userResidualLocations = originalLocations }()

	orphans := s.scanForOrphans(false)

	if len(orphans) != 1 {
		t.Fatalf("expected 1 orphan bundle ID, got %d", len(orphans))
	}
	if _, ok := orphans["com.dead.app"]; !ok {
		t.Error("expected com.dead.app to be an orphan")
	}
	if _, ok := orphans["com.live.app"]; ok {
		t.Error("com.live.app should NOT be an orphan")
	}
}

// ═══════════════════════════════════════════
// dirSizeAndCount
// ═══════════════════════════════════════════

func TestDirSizeAndCount_Nested(t *testing.T) {
	tmp := t.TempDir()
	sub := filepath.Join(tmp, "sub")
	os.MkdirAll(sub, 0755)
	os.WriteFile(filepath.Join(tmp, "a.txt"), []byte("a"), 0644)
	os.WriteFile(filepath.Join(sub, "b.txt"), []byte("b"), 0644)
	os.WriteFile(filepath.Join(sub, "c.txt"), []byte("c"), 0644)

	size, count := dirSizeAndCount(tmp)
	if count != 3 {
		t.Errorf("dirSizeAndCount count = %d, want 3", count)
	}
	if size != 3 {
		t.Errorf("dirSizeAndCount size = %d, want 3", size)
	}
}

func TestDirSizeAndCount_EmptyDir(t *testing.T) {
	tmp := t.TempDir()
	size, count := dirSizeAndCount(tmp)
	if count != 0 {
		t.Errorf("dirSizeAndCount(empty) count = %d, want 0", count)
	}
	if size != 0 {
		t.Errorf("dirSizeAndCount(empty) size = %d, want 0", size)
	}
}

func TestDirSizeAndCount_NonExistent(t *testing.T) {
	size, count := dirSizeAndCount("/nonexistent/path/12345")
	if count != 0 {
		t.Errorf("dirSizeAndCount(nonexistent) count = %d, want 0", count)
	}
	if size != 0 {
		t.Errorf("dirSizeAndCount(nonexistent) size = %d, want 0", size)
	}
}

// ═══════════════════════════════════════════
// mergeOrphans
// ═══════════════════════════════════════════

func TestMergeOrphans_FilesystemOnly(t *testing.T) {
	s := NewScanner()
	orphans := map[string][]Residual{
		"com.test.app": {
			{Path: "/tmp/a", Type: ResidualCaches, SizeBytes: 100, FileCount: 1},
			{Path: "/tmp/b", Type: ResidualPreferences, SizeBytes: 50, FileCount: 1},
		},
	}
	lsGhosts := map[string]bool{}

	ghosts := s.mergeOrphans(orphans, lsGhosts)
	if len(ghosts) != 1 {
		t.Fatalf("expected 1 ghost, got %d", len(ghosts))
	}
	if ghosts[0].TotalSize != 150 {
		t.Errorf("TotalSize = %d, want 150", ghosts[0].TotalSize)
	}
	if ghosts[0].TotalFiles != 2 {
		t.Errorf("TotalFiles = %d, want 2", ghosts[0].TotalFiles)
	}
	if ghosts[0].DetectionMethod != "filesystem" {
		t.Errorf("DetectionMethod = %q, want filesystem", ghosts[0].DetectionMethod)
	}
	if ghosts[0].InLaunchServices {
		t.Error("InLaunchServices should be false")
	}
}

func TestMergeOrphans_LaunchServicesOnly(t *testing.T) {
	s := NewScanner()
	orphans := map[string][]Residual{}
	lsGhosts := map[string]bool{"com.dead.app": true}

	ghosts := s.mergeOrphans(orphans, lsGhosts)
	if len(ghosts) != 1 {
		t.Fatalf("expected 1 ghost, got %d", len(ghosts))
	}
	if ghosts[0].DetectionMethod != "launch_services" {
		t.Errorf("DetectionMethod = %q, want launch_services", ghosts[0].DetectionMethod)
	}
	if !ghosts[0].InLaunchServices {
		t.Error("InLaunchServices should be true")
	}
}

func TestMergeOrphans_Combined(t *testing.T) {
	s := NewScanner()

	orphans := map[string][]Residual{
		"com.found.both": {
			{Path: "/tmp/cache", Type: ResidualCaches, SizeBytes: 200, FileCount: 3},
		},
	}
	lsGhosts := map[string]bool{
		"com.found.both": true,
		"com.only.ls":    true,
	}

	ghosts := s.mergeOrphans(orphans, lsGhosts)
	if len(ghosts) != 2 {
		t.Fatalf("expected 2 ghosts, got %d", len(ghosts))
	}

	// Find the one detected by both
	var foundBoth, foundLS bool
	for _, g := range ghosts {
		if g.BundleID == "com.found.both" {
			foundBoth = true
			if g.DetectionMethod != "filesystem" {
				t.Errorf("combined ghost should have detection method 'filesystem', got %q", g.DetectionMethod)
			}
			if !g.InLaunchServices {
				t.Error("combined ghost should have InLaunchServices=true")
			}
		}
		if g.BundleID == "com.only.ls" {
			foundLS = true
			if g.DetectionMethod != "launch_services" {
				t.Errorf("LS-only ghost should have detection method 'launch_services', got %q", g.DetectionMethod)
			}
		}
	}
	if !foundBoth {
		t.Error("missing ghost com.found.both")
	}
	if !foundLS {
		t.Error("missing ghost com.only.ls")
	}
}

func TestMergeOrphans_Empty(t *testing.T) {
	s := NewScanner()
	ghosts := s.mergeOrphans(map[string][]Residual{}, map[string]bool{})
	if len(ghosts) != 0 {
		t.Errorf("expected 0 ghosts, got %d", len(ghosts))
	}
}

// ═══════════════════════════════════════════
// Clean — dry-run safety
// ═══════════════════════════════════════════

func TestClean_DryRun(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "cache.data")
	os.WriteFile(path, make([]byte, 512), 0644)

	s := NewScanner()
	ghost := Ghost{
		BundleID: "com.test.app",
		Residuals: []Residual{
			{Path: path, Type: ResidualCaches, SizeBytes: 512, FileCount: 1},
		},
	}

	freed, cleaned, err := s.Clean(ghost, true, false)
	if err != nil {
		t.Fatalf("Clean(dry-run) error: %v", err)
	}
	if freed < 512 {
		t.Errorf("freed = %d, want >= 512", freed)
	}
	if cleaned != 1 {
		t.Errorf("cleaned = %d, want 1", cleaned)
	}

	// File should still exist
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("file should still exist after dry-run")
	}
}

func TestClean_ProtectedPathSkipped(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("protected path test requires macOS")
	}
	s := NewScanner()
	ghost := Ghost{
		BundleID: "com.test.protected",
		Residuals: []Residual{
			{Path: "/System/Library/something", Type: ResidualPreferences, SizeBytes: 100},
		},
	}

	freed, cleaned, err := s.Clean(ghost, false, false)
	if err != nil {
		t.Fatalf("Clean() error: %v", err)
	}
	if freed != 0 {
		t.Errorf("freed = %d, want 0 (protected path should be skipped)", freed)
	}
	if cleaned != 0 {
		t.Errorf("cleaned = %d, want 0 (protected path should be skipped)", cleaned)
	}
}

func TestClean_EmptyResiduals(t *testing.T) {
	s := NewScanner()
	ghost := Ghost{BundleID: "com.test.empty"}

	freed, cleaned, err := s.Clean(ghost, false, false)
	if err != nil {
		t.Fatalf("Clean() error: %v", err)
	}
	if freed != 0 || cleaned != 0 {
		t.Errorf("freed=%d, cleaned=%d, want 0, 0", freed, cleaned)
	}
}

// ═══════════════════════════════════════════
// Ghost / Residual struct defaults
// ═══════════════════════════════════════════

func TestGhost_Defaults(t *testing.T) {
	g := Ghost{}
	if g.AppName != "" {
		t.Error("default AppName should be empty")
	}
	if g.InLaunchServices {
		t.Error("default InLaunchServices should be false")
	}
	if g.TotalSize != 0 {
		t.Error("default TotalSize should be 0")
	}
}

func TestResidual_Defaults(t *testing.T) {
	r := Residual{}
	if r.RequiresSudo {
		t.Error("default RequiresSudo should be false")
	}
	if r.Type != "" {
		t.Error("default Type should be empty")
	}
}

// ═══════════════════════════════════════════
// Residual location constants
// ═══════════════════════════════════════════

func TestUserResidualLocations_Complete(t *testing.T) {
	if len(userResidualLocations) < 12 {
		t.Errorf("expected at least 12 user residual locations, got %d", len(userResidualLocations))
	}

	// All user locations should NOT require sudo
	for _, loc := range userResidualLocations {
		if loc.RequiresSudo {
			t.Errorf("user location %q should not require sudo", loc.Dir)
		}
		if loc.Dir == "" {
			t.Error("location Dir should not be empty")
		}
	}
}

func TestSystemResidualLocations_Complete(t *testing.T) {
	if len(systemResidualLocations) < 5 {
		t.Errorf("expected at least 5 system residual locations, got %d", len(systemResidualLocations))
	}

	// All system locations SHOULD require sudo
	for _, loc := range systemResidualLocations {
		if !loc.RequiresSudo {
			t.Errorf("system location %q should require sudo", loc.Dir)
		}
	}
}

func TestResidualTypes_UniqueValues(t *testing.T) {
	types := []ResidualType{
		ResidualPreferences, ResidualAppSupport, ResidualCaches,
		ResidualContainers, ResidualGroupContainers, ResidualSavedState,
		ResidualHTTPStorages, ResidualWebKit, ResidualCookies,
		ResidualAppScripts, ResidualLogs, ResidualLaunchAgent,
		ResidualLaunchDaemon, ResidualReceipts, ResidualLoginItems,
		ResidualCrashReports, ResidualGhostApp,
	}

	seen := make(map[ResidualType]bool)
	for _, rt := range types {
		if seen[rt] {
			t.Errorf("duplicate ResidualType: %q", rt)
		}
		seen[rt] = true
	}
}
