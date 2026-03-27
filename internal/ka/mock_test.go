package ka

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// MockDirEntry implements os.DirEntry for testing.
type MockDirEntry struct {
	name  string
	isDir bool
}

func (m MockDirEntry) Name() string               { return m.name }
func (m MockDirEntry) IsDir() bool                { return m.isDir }
func (m MockDirEntry) Type() os.FileMode          { return 0 }
func (m MockDirEntry) Info() (os.FileInfo, error) { return nil, nil }

func TestScanner_BuildInstalledAppIndex_Mocked(t *testing.T) {
	s := NewScanner()
	s.appDirs = []string{"/MockApps"}
	s.SkipBrew = true

	s.DirReader = func(path string) ([]os.DirEntry, error) {
		if path == "/MockApps" {
			return []os.DirEntry{
				MockDirEntry{name: "TestApp.app", isDir: true},
				MockDirEntry{name: "NotAnApp.txt", isDir: false},
			}, nil
		}
		return nil, nil
	}

	s.ReadBundleIDFn = func(path string) (string, error) {
		if strings.Contains(path, "TestApp.app") {
			return "com.mock.testapp", nil
		}
		return "", nil
	}

	err := s.buildInstalledAppIndex()
	if err != nil {
		t.Fatalf("buildInstalledAppIndex failed: %v", err)
	}

	if !s.installedApps["com.mock.testapp"] {
		t.Error("expected com.mock.testapp in installedApps")
	}
	if !s.installedNames["testapp"] {
		t.Error("expected 'testapp' in installedNames")
	}
}

func TestScanner_ScanForOrphans_Mocked(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, "home")
	prefsDir := filepath.Join(homeDir, "Library/Preferences")
	os.MkdirAll(prefsDir, 0755)

	// Create real files for Lstat to find
	ghostFile := filepath.Join(prefsDir, "com.ghost.app.plist")
	os.WriteFile(ghostFile, []byte("ghost data"), 0644)
	liveFile := filepath.Join(prefsDir, "com.installed.app.plist")
	os.WriteFile(liveFile, []byte("live data"), 0644)

	s := NewScanner()
	s.homeDir = homeDir
	s.installedApps["com.installed.app"] = true

	// Mock locations to point to our temp prefs dir
	originalLocations := userResidualLocations
	userResidualLocations = []residualLocation{
		{Type: ResidualPreferences, Dir: "~/Library/Preferences", RequiresSudo: false},
	}
	defer func() { userResidualLocations = originalLocations }()

	// We still use DirReader to return ONLY our two files
	s.DirReader = func(path string) ([]os.DirEntry, error) {
		if strings.Contains(path, "Preferences") {
			return []os.DirEntry{
				MockDirEntry{name: "com.ghost.app.plist", isDir: false},
				MockDirEntry{name: "com.installed.app.plist", isDir: false},
			}, nil
		}
		return nil, nil
	}

	orphans := s.scanForOrphans(false)

	if len(orphans) != 1 {
		t.Errorf("expected 1 orphan, got %d", len(orphans))
	}
	if _, ok := orphans["com.ghost.app"]; !ok {
		t.Error("expected com.ghost.app to be an orphan")
	}
}

func TestScanner_Scan_Mocked(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, "home")
	appDir := filepath.Join(tmpDir, "Applications")
	prefsDir := filepath.Join(homeDir, "Library/Preferences")
	os.MkdirAll(appDir, 0755)
	os.MkdirAll(prefsDir, 0755)

	// Create real file for orphan scan
	ghostFile := filepath.Join(prefsDir, "com.ghost.app.plist")
	os.WriteFile(ghostFile, []byte("ghost data"), 0644)

	s := NewScanner()
	s.homeDir = homeDir
	s.appDirs = []string{appDir}
	s.SkipLaunchServices = true
	s.SkipBrew = true

	// Mock locations to point to our temp prefs dir
	originalLocations := userResidualLocations
	userResidualLocations = []residualLocation{
		{Type: ResidualPreferences, Dir: "~/Library/Preferences", RequiresSudo: false},
	}
	defer func() { userResidualLocations = originalLocations }()

	s.DirReader = func(path string) ([]os.DirEntry, error) {
		if path == appDir {
			return []os.DirEntry{MockDirEntry{name: "Live.app", isDir: true}}, nil
		}
		if strings.Contains(path, "Preferences") {
			return []os.DirEntry{MockDirEntry{name: "com.ghost.app.plist", isDir: false}}, nil
		}
		return nil, nil
	}
	s.ReadBundleIDFn = func(path string) (string, error) { return "com.live.app", nil }

	ghosts, err := s.Scan(false)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	if len(ghosts) != 1 {
		t.Fatalf("expected 1 ghost, got %d", len(ghosts))
	}
	if ghosts[0].BundleID != "com.ghost.app" {
		t.Errorf("ghost = %q, want com.ghost.app", ghosts[0].BundleID)
	}
}

func TestScanner_Clean_Mocked_RealFiles(t *testing.T) {
	tmpDir := t.TempDir()
	ghostPath1 := filepath.Join(tmpDir, "ghost1.plist")
	ghostPath2 := filepath.Join(tmpDir, "ghost2.plist")
	os.WriteFile(ghostPath1, make([]byte, 100), 0644)
	os.WriteFile(ghostPath2, make([]byte, 100), 0644)

	s := NewScanner()
	ghost := Ghost{
		BundleID: "com.ghost",
		Residuals: []Residual{
			{Path: ghostPath1, SizeBytes: 100},
			{Path: ghostPath2, SizeBytes: 100},
		},
	}

	freed, cleaned, err := s.Clean(ghost, true, false)
	if err != nil {
		t.Fatalf("Clean: %v", err)
	}
	if freed < 200 {
		t.Errorf("freed = %d, want >= 200", freed)
	}
	if cleaned != 2 {
		t.Errorf("cleaned = %d, want 2", cleaned)
	}
}

func TestScanner_Clean_SkipsProtected(t *testing.T) {
	// Protected path that will trigger ValidatePath error
	protectedPath := "/System/Library/protected_mock"

	s := NewScanner()
	ghost := Ghost{
		BundleID: "com.ghost",
		Residuals: []Residual{
			{Path: protectedPath, SizeBytes: 100},
		},
	}

	freed, cleaned, err := s.Clean(ghost, false, false)
	if err != nil {
		t.Fatalf("Clean should not return error on protected path skip: %v", err)
	}
	if freed != 0 || cleaned != 0 {
		t.Errorf("expected 0 freed/cleaned for protected path, got %d/%d", freed, cleaned)
	}
}

func TestScanner_MergeOrphans_Isolation(t *testing.T) {
	s := NewScanner()
	orphans := map[string][]Residual{
		"com.a": {{Path: "/a", SizeBytes: 100}},
	}
	ls := map[string]bool{"com.b": true, "com.a": true}

	ghosts := s.mergeOrphans(orphans, ls)
	if len(ghosts) != 2 {
		t.Fatalf("expected 2 ghosts, got %d", len(ghosts))
	}

	var foundA, foundB bool
	for _, g := range ghosts {
		if g.BundleID == "com.a" {
			foundA = true
			if !g.InLaunchServices {
				t.Error("com.a should be in Launch Services")
			}
		}
		if g.BundleID == "com.b" {
			foundB = true
			if !g.InLaunchServices {
				t.Error("com.b should be in Launch Services")
			}
			if g.DetectionMethod != "launch_services" {
				t.Errorf("com.b method = %q, want launch_services", g.DetectionMethod)
			}
		}
	}
	if !foundA || !foundB {
		t.Error("missing ghosts")
	}
}

func TestScanner_ScanLaunchServices_Mocked(t *testing.T) {
	s := NewScanner()
	// Mock lsregister -dump output
	s.ExecCommand = func(name string, arg ...string) *exec.Cmd {
		// Verify it's calling lsregister
		if strings.Contains(name, "lsregister") {
			return exec.Command("printf", "bundle id: com.ghost.id\npath: /Applications/Ghost.app\n")
		}
		return exec.Command("true")
	}

	ghosts := s.scanLaunchServices()
	if !ghosts["com.ghost.id"] {
		t.Error("expected com.ghost.id to be found in LS")
	}
}

func TestScanner_IndexHomebrewCasks_Mocked(t *testing.T) {
	s := NewScanner()
	s.ExecCommand = func(name string, arg ...string) *exec.Cmd {
		if name == "brew" {
			return exec.Command("printf", "firefox\ngoogle-chrome\n")
		}
		return exec.Command("true")
	}

	s.indexHomebrewCasks()
	if !s.installedNames["firefox"] {
		t.Error("expected firefox to be indexed from brew")
	}
	if !s.installedNames["google-chrome"] {
		t.Error("expected google-chrome to be indexed from brew")
	}
}

func TestScanner_Scan_Sudo_Mocked(t *testing.T) {
	s := NewScanner()
	s.SkipLaunchServices = true
	s.SkipBrew = true
	s.DirReader = func(path string) ([]os.DirEntry, error) {
		return nil, nil // Just testing branch coverage for sudo
	}
	s.Scan(true) // Should append system residual locations
}

func TestScanner_Scan_ErrorPaths_Mocked(t *testing.T) {
	s := NewScanner()
	s.SkipLaunchServices = true
	s.SkipBrew = true
	s.appDirs = []string{"/bad/dir"}
	s.DirReader = func(path string) ([]os.DirEntry, error) {
		return nil, os.ErrPermission
	}
	_, err := s.Scan(false)
	if err != nil {
		t.Errorf("Scan should handle dir read errors gracefully: %v", err)
	}
}

// MockManifest implements the Horus manifest interface for testing.
type MockManifest struct {
	Size  int64
	Count int
}

func (m MockManifest) DirSizeAndCount(dir string) (int64, int) {
	return m.Size, m.Count
}

func (m MockManifest) Exists(path string) bool {
	return true
}

func TestScanner_ScanForOrphans_ManifestMocked(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, "home")
	prefsDir := filepath.Join(homeDir, "Library/Preferences")
	os.MkdirAll(prefsDir, 0755)

	// Create real directory for Lstat to find as a dir
	ghostDir := filepath.Join(prefsDir, "com.horus.app.plist")
	os.MkdirAll(ghostDir, 0755)

	s := NewScanner()
	s.homeDir = homeDir
	s.Manifest = &MockManifest{Size: 1000, Count: 10}

	// Mock locations
	originalLocations := userResidualLocations
	userResidualLocations = []residualLocation{
		{Type: ResidualPreferences, Dir: "~/Library/Preferences", RequiresSudo: false},
	}
	defer func() { userResidualLocations = originalLocations }()

	s.DirReader = func(path string) ([]os.DirEntry, error) {
		if strings.Contains(path, "Preferences") {
			return []os.DirEntry{MockDirEntry{name: "com.horus.app.plist", isDir: true}}, nil
		}
		return nil, nil
	}

	orphans := s.scanForOrphans(false)
	if len(orphans) != 1 {
		t.Fatalf("expected 1 orphan, got %d", len(orphans))
	}
	o := orphans["com.horus.app"][0]
	if o.SizeBytes != 1000 || o.FileCount != 10 {
		t.Errorf("expected size/count from manifest (1000/10), got %d/%d", o.SizeBytes, o.FileCount)
	}
}
