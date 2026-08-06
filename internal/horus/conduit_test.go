package horus

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenConduit_CreatesIdentity(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	c, err := OpenConduit()
	if err != nil {
		t.Fatalf("OpenConduit: %v", err)
	}

	id := c.Identity()
	if id.Hostname == "" {
		t.Error("hostname should be non-empty")
	}
	if id.SchemaVer != conduitSchemaVersion {
		t.Errorf("schema version = %q, want %q", id.SchemaVer, conduitSchemaVersion)
	}
	if id.TelemetryOn {
		t.Error("telemetry should be off by default")
	}

	// Identity file must exist on disk.
	identityFile := filepath.Join(dir, ".config", "sirsi", "horus", "conduit.json")
	if _, err := os.Stat(identityFile); err != nil {
		t.Errorf("conduit.json not written: %v", err)
	}
}

func TestOpenConduit_Idempotent(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	c1, err := OpenConduit()
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	c2, err := OpenConduit()
	if err != nil {
		t.Fatalf("second open: %v", err)
	}

	if c1.Identity().NodeID != c2.Identity().NodeID {
		t.Errorf("node IDs differ: %q vs %q", c1.Identity().NodeID, c2.Identity().NodeID)
	}
}

func TestSetTelemetry(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	c, err := OpenConduit()
	if err != nil {
		t.Fatalf("OpenConduit: %v", err)
	}

	if err := c.SetTelemetry(true); err != nil {
		t.Fatalf("SetTelemetry: %v", err)
	}
	if !c.Identity().TelemetryOn {
		t.Error("telemetry should be on after SetTelemetry(true)")
	}

	// Re-open: telemetry persisted to disk.
	c2, _ := OpenConduit()
	if !c2.Identity().TelemetryOn {
		t.Error("telemetry not persisted across open")
	}
}

func TestBuildReport(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	c, _ := OpenConduit()
	report := c.BuildReport(3, nil)

	if report.RouterItems != 3 {
		t.Errorf("router items = %d, want 3", report.RouterItems)
	}
	if report.Identity.NodeID == "" {
		t.Error("report identity node ID should be set")
	}
	if report.ReportedAt.IsZero() {
		t.Error("reported_at should be set")
	}
}
