//go:build !windows

package apollo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadTelemetryRejectsSymlinkedLeaf(t *testing.T) {
	home := t.TempDir()
	path := TelemetryPath(home)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(home, "outside.json")
	if err := os.WriteFile(target, []byte(`{"schema_version":"apollo-session-telemetry/v1","session_id":"s","engine_id":"apollo-local-sne","emitted_at":"2026-09-28T00:00:00Z","chip_estates":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadTelemetry(home); err == nil {
		t.Fatal("ReadTelemetry accepted a symlinked session sample")
	}
}
