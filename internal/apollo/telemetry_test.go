package apollo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadTelemetryReturnsAwaitingWhenNoSessionExists(t *testing.T) {
	read, err := ReadTelemetry(t.TempDir())
	if err != nil {
		t.Fatalf("ReadTelemetry() error = %v", err)
	}
	if read.State != "awaiting_session" || read.Telemetry != nil {
		t.Fatalf("read = %+v", read)
	}
}

func TestReadTelemetryRejectsUnknownFieldsAndInvalidPercentage(t *testing.T) {
	home := t.TempDir()
	path := TelemetryPath(home)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	bad := `{"schema_version":"apollo-session-telemetry/v1","session_id":"s","engine_id":"apollo-local-sne","emitted_at":"2026-09-28T00:00:00Z","cpu_residency_percent":101}`
	if err := os.WriteFile(path, []byte(bad), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadTelemetry(home); err == nil {
		t.Fatal("ReadTelemetry() accepted invalid CPU residency")
	}
	bad = `{"schema_version":"apollo-session-telemetry/v1","session_id":"s","engine_id":"apollo-local-sne","emitted_at":"2026-09-28T00:00:00Z","invented":true}`
	if err := os.WriteFile(path, []byte(bad), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadTelemetry(home); err == nil {
		t.Fatal("ReadTelemetry() accepted an unknown field")
	}
}
