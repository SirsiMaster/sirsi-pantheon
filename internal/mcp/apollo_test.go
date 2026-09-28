package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/apollo"
)

func TestReadApolloSessionTelemetryReturnsAwaitingWithoutSample(t *testing.T) {
	result, err := readApolloSessionTelemetry(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError || len(result.Content) != 1 {
		t.Fatalf("result = %+v", result)
	}
	var got apollo.TelemetryRead
	if err := json.Unmarshal([]byte(result.Content[0].Text), &got); err != nil {
		t.Fatal(err)
	}
	if got.State != "awaiting_session" || got.Telemetry != nil {
		t.Fatalf("telemetry = %+v", got)
	}
}

func TestReadApolloSessionTelemetryProjectsMeasuredSample(t *testing.T) {
	home := t.TempDir()
	path := apollo.TelemetryPath(home)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	sample := `{"schema_version":"apollo-session-telemetry/v1","session_id":"session-1","engine_id":"apollo-local-sne","emitted_at":"2026-09-28T00:00:00Z","tokens_per_second":42.5,"chip_estates":[{"id":"gpu","residency_percent":70.0}]}`
	if err := os.WriteFile(path, []byte(sample), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := readApolloSessionTelemetry(home)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError || len(result.Content) != 1 || result.Content[0].MimeType != "application/json" {
		t.Fatalf("result = %+v", result)
	}
	var got apollo.TelemetryRead
	if err := json.Unmarshal([]byte(result.Content[0].Text), &got); err != nil {
		t.Fatal(err)
	}
	if got.State != "active" || got.Telemetry == nil || got.Telemetry.TokensPerSec == nil || *got.Telemetry.TokensPerSec != 42.5 {
		t.Fatalf("telemetry = %+v", got)
	}
}
