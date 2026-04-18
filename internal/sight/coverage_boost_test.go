package sight

import (
	"errors"
	"strings"
	"testing"
)

// ── parseLSRegisterDump — thorough block parsing ────────────────────────

func TestParseLSRegisterDump_GhostApp(t *testing.T) {
	t.Parallel()
	// App registered but not on disk — "test -d" fails (mockRunnerFail)
	dump := `--------------------------------------------------------------------------------
	bundle id: com.example.ghost
	path: /Applications/Ghost.app
	name: Ghost App
--------------------------------------------------------------------------------`
	ghosts := parseLSRegisterDump(dump, mockRunnerFail())
	if len(ghosts) != 1 {
		t.Fatalf("expected 1 ghost, got %d", len(ghosts))
	}
	if ghosts[0].BundleID != "com.example.ghost" {
		t.Errorf("BundleID = %q", ghosts[0].BundleID)
	}
	if ghosts[0].Path != "/Applications/Ghost.app" {
		t.Errorf("Path = %q", ghosts[0].Path)
	}
	if ghosts[0].Name != "Ghost App" {
		t.Errorf("Name = %q", ghosts[0].Name)
	}
}

func TestParseLSRegisterDump_InstalledApp(t *testing.T) {
	t.Parallel()
	// App on disk — "test -d" succeeds (mockRunnerSuccess)
	dump := `--------------------------------------------------------------------------------
	bundle id: com.example.alive
	path: /Applications/Alive.app
	name: Alive
--------------------------------------------------------------------------------`
	ghosts := parseLSRegisterDump(dump, mockRunnerSuccess())
	if len(ghosts) != 0 {
		t.Errorf("installed app should not be a ghost, got %d ghosts", len(ghosts))
	}
}

func TestParseLSRegisterDump_MissingPath(t *testing.T) {
	t.Parallel()
	dump := `--------------------------------------------------------------------------------
	bundle id: com.example.nopath
	name: NoPath
--------------------------------------------------------------------------------`
	ghosts := parseLSRegisterDump(dump, mockRunnerFail())
	if len(ghosts) != 0 {
		t.Errorf("no path should mean no ghost, got %d", len(ghosts))
	}
}

func TestParseLSRegisterDump_NoName(t *testing.T) {
	t.Parallel()
	// If name is missing, should use bundleID as name
	dump := `--------------------------------------------------------------------------------
	bundle id: com.example.noname
	path: /Applications/NoName.app
--------------------------------------------------------------------------------`
	ghosts := parseLSRegisterDump(dump, mockRunnerFail())
	if len(ghosts) != 1 {
		t.Fatalf("expected 1 ghost, got %d", len(ghosts))
	}
	if ghosts[0].Name != "com.example.noname" {
		t.Errorf("Name should fallback to bundleID, got %q", ghosts[0].Name)
	}
}

func TestParseLSRegisterDump_SubPath(t *testing.T) {
	t.Parallel()
	// Path that contains .app but goes deeper
	dump := `--------------------------------------------------------------------------------
	bundle id: com.example.deep
	path: /Applications/Deep.app/Contents/MacOS/helper
	name: Deep Helper
--------------------------------------------------------------------------------`
	ghosts := parseLSRegisterDump(dump, mockRunnerFail())
	if len(ghosts) != 1 {
		t.Fatalf("expected 1 ghost, got %d", len(ghosts))
	}
	if ghosts[0].Path != "/Applications/Deep.app" {
		t.Errorf("Path should be truncated to .app, got %q", ghosts[0].Path)
	}
}

func TestParseLSRegisterDump_MultipleBlocks(t *testing.T) {
	t.Parallel()
	dump := `--------------------------------------------------------------------------------
	bundle id: com.example.ghost1
	path: /Applications/Ghost1.app
	name: Ghost1
--------------------------------------------------------------------------------
	bundle id: com.example.ghost2
	path: /Applications/Ghost2.app
	name: Ghost2
--------------------------------------------------------------------------------`
	ghosts := parseLSRegisterDump(dump, mockRunnerFail())
	if len(ghosts) != 2 {
		t.Errorf("expected 2 ghosts, got %d", len(ghosts))
	}
}

// ── ScanWith ─────────────────────────────────────────────────────────────

func TestScanWith_MockDump(t *testing.T) {
	skipIfNotDarwin(t)
	t.Parallel()

	runner := &mockRunner{
		OutputFunc: func(name string, args ...string) ([]byte, error) {
			dump := `--------------------------------------------------------------------------------
	bundle id: com.test.dead
	path: /Applications/Dead.app
	name: Dead App
--------------------------------------------------------------------------------`
			return []byte(dump), nil
		},
		RunFunc: func(name string, args ...string) error {
			// "test -d" should fail for ghost apps
			if name == "test" {
				return errors.New("directory not found")
			}
			return nil
		},
	}

	result, err := ScanWith(runner)
	if err != nil {
		t.Fatalf("ScanWith: %v", err)
	}
	if result == nil {
		t.Fatal("result should not be nil")
	}
	if result.TotalGhosts != 1 {
		t.Errorf("TotalGhosts = %d, want 1", result.TotalGhosts)
	}
	if !result.CanFix {
		t.Error("CanFix should be true")
	}
}

func TestScanWith_DumpFails(t *testing.T) {
	skipIfNotDarwin(t)
	t.Parallel()

	_, err := ScanWith(mockRunnerFail())
	if err == nil {
		t.Error("ScanWith should error when lsregister fails")
	}
}

func TestScanWith_EmptyDump(t *testing.T) {
	skipIfNotDarwin(t)
	t.Parallel()

	runner := &mockRunner{
		OutputFunc: func(name string, args ...string) ([]byte, error) {
			return []byte(""), nil
		},
	}

	result, err := ScanWith(runner)
	if err != nil {
		t.Fatalf("ScanWith: %v", err)
	}
	if result.TotalGhosts != 0 {
		t.Errorf("TotalGhosts = %d, want 0", result.TotalGhosts)
	}
}

// ── FixWith comprehensive ────────────────────────────────────────────────

func TestFixWith_CallsLsregisterAndFinder(t *testing.T) {
	skipIfNotDarwin(t)
	t.Parallel()

	runner := &mockRunner{}
	err := FixWith(false, runner)
	if err != nil {
		t.Fatalf("FixWith: %v", err)
	}
	if len(runner.Calls) != 2 {
		t.Errorf("expected 2 calls (lsregister + killall), got %d: %v", len(runner.Calls), runner.Calls)
	}
	// First call should be lsregister
	if !strings.Contains(runner.Calls[0], "lsregister") {
		t.Errorf("first call should be lsregister, got %q", runner.Calls[0])
	}
	// Second call should be killall
	if runner.Calls[1] != "killall" {
		t.Errorf("second call should be killall, got %q", runner.Calls[1])
	}
}

// ── ReindexSpotlightWith ────────────────────────────────────────────────

func TestReindexSpotlightWith_CallsMdutil(t *testing.T) {
	skipIfNotDarwin(t)
	t.Parallel()

	runner := &mockRunner{}
	err := ReindexSpotlightWith(false, runner)
	if err != nil {
		t.Fatalf("ReindexSpotlightWith: %v", err)
	}
	if len(runner.Calls) != 1 {
		t.Errorf("expected 1 call (mdutil), got %d", len(runner.Calls))
	}
	if runner.Calls[0] != "mdutil" {
		t.Errorf("call should be mdutil, got %q", runner.Calls[0])
	}
}

// ── defaultRunner Output ─────────────────────────────────────────────────

func TestDefaultRunner_Output(t *testing.T) {
	t.Parallel()
	runner := defaultRunner{}
	out, err := runner.Output("echo", "hello")
	if err != nil {
		t.Fatalf("Output: %v", err)
	}
	if !strings.Contains(string(out), "hello") {
		t.Errorf("Output = %q, want 'hello'", string(out))
	}
}

func TestDefaultRunner_Output_BadCommand(t *testing.T) {
	t.Parallel()
	runner := defaultRunner{}
	_, err := runner.Output("nonexistent-command-xyz-123")
	if err == nil {
		t.Error("Output should error for bad command")
	}
}

// ── StreamingRunner ──────────────────────────────────────────────────────

func TestDefaultRunner_StreamLines(t *testing.T) {
	t.Parallel()
	runner := defaultRunner{}
	next, cleanup, err := runner.StreamLines("echo", "line1\nline2")
	if err != nil {
		t.Fatalf("StreamLines: %v", err)
	}
	defer cleanup()

	var lines []string
	for {
		line, ok := next()
		if !ok {
			break
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		t.Error("expected at least one line")
	}
}

func TestDefaultRunner_StreamLines_BadCommand(t *testing.T) {
	t.Parallel()
	runner := defaultRunner{}
	_, _, err := runner.StreamLines("nonexistent-command-xyz-123")
	if err == nil {
		t.Error("StreamLines should error for bad command")
	}
}

// ── parseLSRegisterStream ────────────────────────────────────────────────

type mockStreamRunner struct {
	mockRunner
	lines []string
}

func (m *mockStreamRunner) StreamLines(name string, args ...string) (func() (string, bool), func() error, error) {
	idx := 0
	next := func() (string, bool) {
		if idx >= len(m.lines) {
			return "", false
		}
		line := m.lines[idx]
		idx++
		return line, true
	}
	cleanup := func() error { return nil }
	return next, cleanup, nil
}

func TestParseLSRegisterStream_Ghost(t *testing.T) {
	skipIfNotDarwin(t)
	t.Parallel()

	sr := &mockStreamRunner{
		mockRunner: mockRunner{
			RunFunc: func(name string, args ...string) error {
				if name == "test" {
					return errors.New("not found")
				}
				return nil
			},
		},
		lines: []string{
			"	bundle id: com.example.streamed",
			"	path: /Applications/Streamed.app",
			"	name: Streamed App",
			"--------------------------------------------------------------------------------",
		},
	}

	ghosts, err := parseLSRegisterStream(sr, "lsregister", sr)
	if err != nil {
		t.Fatalf("parseLSRegisterStream: %v", err)
	}
	if len(ghosts) != 1 {
		t.Fatalf("expected 1 ghost, got %d", len(ghosts))
	}
	if ghosts[0].BundleID != "com.example.streamed" {
		t.Errorf("BundleID = %q", ghosts[0].BundleID)
	}
}

func TestParseLSRegisterStream_Installed(t *testing.T) {
	skipIfNotDarwin(t)
	t.Parallel()

	sr := &mockStreamRunner{
		mockRunner: mockRunner{},
		lines: []string{
			"	bundle id: com.example.alive",
			"	path: /Applications/Alive.app",
			"	name: Alive",
			"--------------------------------------------------------------------------------",
		},
	}

	ghosts, err := parseLSRegisterStream(sr, "lsregister", &sr.mockRunner)
	if err != nil {
		t.Fatalf("parseLSRegisterStream: %v", err)
	}
	if len(ghosts) != 0 {
		t.Errorf("installed app should not be ghost, got %d", len(ghosts))
	}
}

func TestParseLSRegisterStream_FinalBlock(t *testing.T) {
	skipIfNotDarwin(t)
	t.Parallel()

	// No trailing separator — tests the final block processing
	sr := &mockStreamRunner{
		mockRunner: mockRunner{
			RunFunc: func(name string, args ...string) error {
				if name == "test" {
					return errors.New("not found")
				}
				return nil
			},
		},
		lines: []string{
			"	bundle id: com.example.final",
			"	path: /Applications/Final.app",
			"	name: Final",
		},
	}

	ghosts, err := parseLSRegisterStream(sr, "lsregister", sr)
	if err != nil {
		t.Fatalf("parseLSRegisterStream: %v", err)
	}
	if len(ghosts) != 1 {
		t.Errorf("expected 1 ghost from final block, got %d", len(ghosts))
	}
}

func TestParseLSRegisterStream_SkipsApple(t *testing.T) {
	skipIfNotDarwin(t)
	t.Parallel()

	sr := &mockStreamRunner{
		mockRunner: mockRunner{
			RunFunc: func(name string, args ...string) error {
				return errors.New("not found")
			},
		},
		lines: []string{
			"	bundle id: com.apple.Safari",
			"	path: /Applications/Safari.app",
			"	name: Safari",
			"--------------------------------------------------------------------------------",
		},
	}

	ghosts, err := parseLSRegisterStream(sr, "lsregister", sr)
	if err != nil {
		t.Fatalf("parseLSRegisterStream: %v", err)
	}
	if len(ghosts) != 0 {
		t.Errorf("Apple apps should be skipped, got %d", len(ghosts))
	}
}

func TestParseLSRegisterStream_Deduplication(t *testing.T) {
	skipIfNotDarwin(t)
	t.Parallel()

	sr := &mockStreamRunner{
		mockRunner: mockRunner{
			RunFunc: func(name string, args ...string) error {
				if name == "test" {
					return errors.New("not found")
				}
				return nil
			},
		},
		lines: []string{
			"	bundle id: com.example.dup",
			"	path: /Applications/Dup.app",
			"	name: Dup",
			"--------------------------------------------------------------------------------",
			"	bundle id: com.example.dup",
			"	path: /Applications/Dup.app/Contents/helper",
			"	name: Dup Helper",
			"--------------------------------------------------------------------------------",
		},
	}

	ghosts, err := parseLSRegisterStream(sr, "lsregister", sr)
	if err != nil {
		t.Fatalf("parseLSRegisterStream: %v", err)
	}
	if len(ghosts) != 1 {
		t.Errorf("duplicates should be deduped, got %d", len(ghosts))
	}
}
