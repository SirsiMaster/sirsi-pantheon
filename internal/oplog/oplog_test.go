package oplog

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func resetState() {
	mu.Lock()
	if logFile != nil {
		logFile.Close()
	}
	logFile = nil
	mu.Unlock()
}

func TestLog_WritesEntry(t *testing.T) {
	resetState()

	tmp := t.TempDir()
	// Point home to temp so the log goes to a predictable location
	t.Setenv("HOME", tmp)

	Log("purge", "/tmp/test/node_modules", 1024)
	Close()

	logPath := filepath.Join(tmp, "Library", "Logs", "sirsi", "operations.log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("log file not created: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "purge") {
		t.Errorf("log missing action 'purge': %s", content)
	}
	if !strings.Contains(content, "/tmp/test/node_modules") {
		t.Errorf("log missing path: %s", content)
	}
	if !strings.Contains(content, "(1024 bytes)") {
		t.Errorf("log missing byte count: %s", content)
	}
}

func TestLog_ZeroBytesOmitsSize(t *testing.T) {
	resetState()

	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	Log("clean", "/tmp/empty", 0)
	Close()

	logPath := filepath.Join(tmp, "Library", "Logs", "sirsi", "operations.log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("log file not created: %v", err)
	}

	content := string(data)
	if strings.Contains(content, "bytes") {
		t.Errorf("zero-byte entry should not contain size: %s", content)
	}
}

func TestLog_NoOplog(t *testing.T) {
	resetState()

	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("SIRSI_NO_OPLOG", "1")

	Log("purge", "/tmp/test", 500)

	logPath := filepath.Join(tmp, "Library", "Logs", "sirsi", "operations.log")
	if _, err := os.Stat(logPath); !os.IsNotExist(err) {
		t.Error("log file should not be created when SIRSI_NO_OPLOG=1")
	}
}

func TestLog_Concurrent(t *testing.T) {
	resetState()

	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			Log("clean", "/tmp/concurrent", int64(n*100))
		}(i)
	}
	wg.Wait()
	Close()

	logPath := filepath.Join(tmp, "Library", "Logs", "sirsi", "operations.log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("log file not created: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 20 {
		t.Errorf("expected 20 log lines, got %d", len(lines))
	}
}

func TestClose_NilFile(t *testing.T) {
	resetState()
	// Close without any Log calls should not panic
	Close()
}

func TestClose_DoubleClose(t *testing.T) {
	resetState()

	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	Log("test", "/tmp/double", 1)
	Close()
	Close() // should not panic
}

// TestParseLine_RoundTrip pins the reader to the writer's exact format: every
// shape Log emits parses back to the values it was given. This is the contract
// that lets `sirsi activity --json` be the ONE parser of the free-text ledger.
func TestParseLine_RoundTrip(t *testing.T) {
	cases := []struct {
		name string
		line string
		want Entry
	}{
		{
			name: "with size suffix",
			line: "2026-07-01T10:30:00  clean  /Users/x/Library/Caches/foo (2048 bytes)",
			want: Entry{Time: "2026-07-01T10:30:00", Action: "clean", Target: "/Users/x/Library/Caches/foo", Bytes: 2048, Source: "oplog"},
		},
		{
			name: "zero bytes omits suffix",
			line: "2026-07-01T10:31:05  purge  /Users/x/dev/app/node_modules",
			want: Entry{Time: "2026-07-01T10:31:05", Action: "purge", Target: "/Users/x/dev/app/node_modules", Source: "oplog"},
		},
		{
			name: "path containing single spaces",
			line: "2026-07-01T10:32:00  clean  /Users/x/Library/Application Support/Old App (512 bytes)",
			want: Entry{Time: "2026-07-01T10:32:00", Action: "clean", Target: "/Users/x/Library/Application Support/Old App", Bytes: 512, Source: "oplog"},
		},
		{
			name: "path ending in parenthesized words is not a size",
			line: "2026-07-01T10:33:00  clean  /tmp/backup (old bytes)",
			want: Entry{Time: "2026-07-01T10:33:00", Action: "clean", Target: "/tmp/backup (old bytes)", Source: "oplog"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ParseLine(tc.line)
			if !ok {
				t.Fatalf("ParseLine(%q) not ok", tc.line)
			}
			if got != tc.want {
				t.Errorf("ParseLine(%q) = %+v, want %+v", tc.line, got, tc.want)
			}
		})
	}
}

func TestParseLine_Malformed(t *testing.T) {
	for _, line := range []string{"", "   ", "no-separators-here", "ts  action-only"} {
		if _, ok := ParseLine(line); ok {
			t.Errorf("ParseLine(%q) should not parse", line)
		}
	}
}

// TestReadLast_NewestFirstAndLimit verifies ordering (newest first), the limit,
// and that ReadLast reads what Log wrote (writer↔reader agreement via Path).
func TestReadLast_NewestFirstAndLimit(t *testing.T) {
	resetState()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	Log("clean", "/tmp/first", 100)
	Log("clean", "/tmp/second", 200)
	Log("purge", "/tmp/third", 300)
	Close()

	entries, err := ReadLast(Path(), 2)
	if err != nil {
		t.Fatalf("ReadLast: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("ReadLast limit 2 returned %d entries", len(entries))
	}
	if entries[0].Target != "/tmp/third" || entries[0].Action != "purge" || entries[0].Bytes != 300 {
		t.Errorf("entries[0] should be the NEWEST line, got %+v", entries[0])
	}
	if entries[1].Target != "/tmp/second" {
		t.Errorf("entries[1] = %+v, want /tmp/second", entries[1])
	}

	// n <= 0 returns everything.
	all, err := ReadLast(Path(), 0)
	if err != nil {
		t.Fatalf("ReadLast all: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("ReadLast(0) returned %d entries, want 3", len(all))
	}
}

// TestReadLast_MissingFile confirms an absent ledger is a normal empty state,
// not an error (a machine that never cleaned anything has no log yet).
func TestReadLast_MissingFile(t *testing.T) {
	entries, err := ReadLast(filepath.Join(t.TempDir(), "nope", "operations.log"), 10)
	if err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("missing file should yield 0 entries, got %d", len(entries))
	}
}
