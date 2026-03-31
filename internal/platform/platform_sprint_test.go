package platform

import (
	"runtime"
	"testing"
)

// ── detectFor Tests ──────────────────────────────────────────────────────

func TestDetectFor_Darwin(t *testing.T) {
	t.Parallel()
	p := detectFor("darwin")
	if p.Name() != "darwin" {
		t.Errorf("detectFor('darwin').Name() = %q", p.Name())
	}
}

func TestDetectFor_Linux(t *testing.T) {
	t.Parallel()
	p := detectFor("linux")
	if p.Name() != "linux" {
		t.Errorf("detectFor('linux').Name() = %q", p.Name())
	}
}

func TestDetectFor_Windows(t *testing.T) {
	t.Parallel()
	// Should fall back to Linux
	p := detectFor("windows")
	if p.Name() != "linux" {
		t.Errorf("detectFor('windows').Name() = %q, want 'linux' (fallback)", p.Name())
	}
}

func TestDetectFor_Unknown(t *testing.T) {
	t.Parallel()
	p := detectFor("freebsd")
	if p.Name() != "linux" {
		t.Errorf("detectFor('freebsd').Name() = %q, want 'linux' (fallback)", p.Name())
	}
}

// ── Current/Set/Reset Tests ──────────────────────────────────────────────

func TestCurrentSetReset(t *testing.T) {
	original := Current()
	if original == nil {
		t.Fatal("Current() should not be nil")
	}

	mock := &Mock{NameStr: "test-platform"}
	Set(mock)
	if Current().Name() != "test-platform" {
		t.Errorf("after Set(), Current().Name() = %q", Current().Name())
	}

	Reset()
	// Reset should restore to detected platform.
	// On macOS this is "darwin", on Linux "linux".
	// Verifying the call doesn't panic; value correctness is platform-dependent.
	_ = Current().Name()
}

// ── Darwin Tests ─────────────────────────────────────────────────────────

func TestDarwin_Name_Sprint2(t *testing.T) {
	t.Parallel()
	d := &Darwin{}
	if d.Name() != "darwin" {
		t.Errorf("Name() = %q", d.Name())
	}
}

func TestDarwin_SupportsTrash_Sprint2(t *testing.T) {
	t.Parallel()
	d := &Darwin{}
	if !d.SupportsTrash() {
		t.Error("Darwin should support Trash")
	}
}

func TestDarwin_ProtectedPrefixes_Sprint2(t *testing.T) {
	t.Parallel()
	d := &Darwin{}
	prefixes := d.ProtectedPrefixes()
	if len(prefixes) == 0 {
		t.Error("should have protected prefixes")
	}
	found := false
	for _, p := range prefixes {
		if p == "/System/" {
			found = true
		}
	}
	if !found {
		t.Error("should include /System/")
	}
}

func TestDarwin_Getenv(t *testing.T) {
	t.Parallel()
	d := &Darwin{}
	// HOME should always be set on macOS
	if runtime.GOOS == "darwin" {
		home := d.Getenv("HOME")
		if home == "" {
			t.Error("HOME should not be empty on darwin")
		}
	}
}

func TestDarwin_UserHomeDir(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "darwin" {
		t.Skip("Darwin-only test")
	}
	d := &Darwin{}
	home, err := d.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir error: %v", err)
	}
	if home == "" {
		t.Error("home should not be empty")
	}
}

func TestDarwin_Getwd(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "darwin" {
		t.Skip("Darwin-only test")
	}
	d := &Darwin{}
	wd, err := d.Getwd()
	if err != nil {
		t.Fatalf("Getwd error: %v", err)
	}
	if wd == "" {
		t.Error("working dir should not be empty")
	}
}

func TestDarwin_ReadDir(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "darwin" {
		t.Skip("Darwin-only test")
	}
	d := &Darwin{}
	entries, err := d.ReadDir(t.TempDir())
	if err != nil {
		t.Fatalf("ReadDir error: %v", err)
	}
	// Empty temp dir = 0 entries
	if len(entries) != 0 {
		t.Errorf("expected 0 entries in temp dir, got %d", len(entries))
	}
}

// ── Linux Tests ──────────────────────────────────────────────────────────

func TestLinux_Name_Sprint2(t *testing.T) {
	t.Parallel()
	l := &Linux{}
	if l.Name() != "linux" {
		t.Errorf("Name() = %q", l.Name())
	}
}

func TestLinux_SupportsTrash_Sprint2(t *testing.T) {
	t.Parallel()
	l := &Linux{}
	if l.SupportsTrash() {
		t.Error("Linux SupportsTrash should be false (TODO)")
	}
}

func TestLinux_ProtectedPrefixes_Sprint2(t *testing.T) {
	t.Parallel()
	l := &Linux{}
	prefixes := l.ProtectedPrefixes()
	if len(prefixes) == 0 {
		t.Error("should have protected prefixes")
	}
	found := false
	for _, p := range prefixes {
		if p == "/boot/" {
			found = true
		}
	}
	if !found {
		t.Error("should include /boot/")
	}
}

// ── TryLock Tests ────────────────────────────────────────────────────────

func TestTryLock_AcquireAndRelease(t *testing.T) {
	cleanup, err := TryLock("test-sprint-lock")
	if err != nil {
		t.Fatalf("TryLock error: %v", err)
	}
	if cleanup == nil {
		t.Fatal("cleanup should not be nil")
	}
	cleanup()
}

func TestTryLock_DoubleAcquireFails(t *testing.T) {
	cleanup, err := TryLock("test-double-lock")
	if err != nil {
		t.Fatalf("first TryLock error: %v", err)
	}
	defer cleanup()

	_, err2 := TryLock("test-double-lock")
	if err2 == nil {
		t.Error("second TryLock should fail (already locked)")
	}
}

// ── Mock Tests ───────────────────────────────────────────────────────────

func TestMock_Accessors(t *testing.T) {
	t.Parallel()
	m := &Mock{
		NameStr: "mock-test",
		HomeDir: "/mock/home",
		WorkDir: "/mock/work",
	}
	if m.Name() != "mock-test" {
		t.Errorf("Name() = %q", m.Name())
	}
	home, _ := m.UserHomeDir()
	if home != "/mock/home" {
		t.Errorf("UserHomeDir() = %q", home)
	}
	wd, _ := m.Getwd()
	if wd != "/mock/work" {
		t.Errorf("Getwd() = %q", wd)
	}
}

func TestMock_PickFolder_Sprint2(t *testing.T) {
	t.Parallel()
	m := &Mock{PickFolderPath: "/picked"}
	path, err := m.PickFolder()
	if err != nil {
		t.Fatalf("PickFolder error: %v", err)
	}
	if path != "/picked" {
		t.Errorf("path = %q", path)
	}
}

func TestMock_OpenBrowser_Sprint2(t *testing.T) {
	t.Parallel()
	m := &Mock{}
	err := m.OpenBrowser("http://localhost:8080")
	if err != nil {
		t.Fatalf("OpenBrowser error: %v", err)
	}
	if m.OpenBrowserURL != "http://localhost:8080" {
		t.Errorf("OpenBrowserURL = %q", m.OpenBrowserURL)
	}
}

func TestMock_SupportsTrash_Sprint2(t *testing.T) {
	t.Parallel()
	m := &Mock{}
	// Mock defaults to true as hardcoded in mock.go
	if !m.SupportsTrash() {
		t.Error("Mock SupportsTrash should be true by default")
	}
}

func TestMock_ProtectedPrefixes_Sprint2(t *testing.T) {
	t.Parallel()
	m := &Mock{}
	prefixes := m.ProtectedPrefixes()
	if prefixes == nil {
		t.Error("should return non-nil (empty) slice")
	}
}
