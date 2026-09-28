package apollo

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const maxTelemetryBytes = 64 * 1024

// Telemetry is the SNE-owned session contract Apollo renders. Optional metric
// values are deliberately pointers: absent data remains absent rather than
// becoming an appealing but false zero in a product surface.
type Telemetry struct {
	SchemaVersion string `json:"schema_version"`
	SessionID     string `json:"session_id"`
	EngineID      string `json:"engine_id"`
	// MachineID binds a session sample to the Horus instance that admitted it.
	// It is optional only for the existing single-device "this-mac" route;
	// any future selected peer must publish it before Pantheon renders the
	// sample for that plan.
	MachineID    string            `json:"machine_id,omitempty"`
	EmittedAt    time.Time         `json:"emitted_at"`
	TokensPerSec *float64          `json:"tokens_per_second,omitempty"`
	BandwidthBps *int64            `json:"bandwidth_bytes_per_second,omitempty"`
	MemoryBytes  *int64            `json:"memory_bytes,omitempty"`
	NetworkPct   *float64          `json:"network_saturation_percent,omitempty"`
	CPUResidency *float64          `json:"cpu_residency_percent,omitempty"`
	GPUResidency *float64          `json:"gpu_residency_percent,omitempty"`
	Estates      []EstateTelemetry `json:"chip_estates"`
}

type EstateTelemetry struct {
	ID             string   `json:"id"`
	ResidencyPct   *float64 `json:"residency_percent,omitempty"`
	MemoryBytes    *int64   `json:"memory_bytes,omitempty"`
	UtilizationPct *float64 `json:"utilization_percent,omitempty"`
}

type TelemetryRead struct {
	State     string     `json:"state"` // active|awaiting_session
	Telemetry *Telemetry `json:"telemetry,omitempty"`
	Reason    string     `json:"reason,omitempty"`
}

func TelemetryPath(home string) string {
	return filepath.Join(home, ".sirsi", "apollo", "session-telemetry.json")
}

// ReadTelemetry reads a complete, bounded SNE session sample. The absence of a
// sample is a normal awaiting-session result; malformed or unsafe input is an
// error and must not be rendered as active telemetry.
func ReadTelemetry(home string) (TelemetryRead, error) {
	path := TelemetryPath(home)
	f, err := openTelemetryFile(path)
	if os.IsNotExist(err) {
		return TelemetryRead{State: "awaiting_session", Reason: "SNE has not published an Apollo session sample"}, nil
	}
	if err != nil {
		return TelemetryRead{}, fmt.Errorf("open Apollo telemetry: %w", err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return TelemetryRead{}, fmt.Errorf("stat opened Apollo telemetry: %w", err)
	}
	if !validTelemetryFile(info) {
		return TelemetryRead{}, fmt.Errorf("Apollo telemetry must be a regular non-symlink file")
	}
	if info.Size() < 1 || info.Size() > maxTelemetryBytes {
		return TelemetryRead{}, fmt.Errorf("Apollo telemetry size is outside the allowed range")
	}
	data, err := readTelemetryBytes(f, info.Size())
	if err != nil {
		return TelemetryRead{}, err
	}
	if err := revalidateTelemetryPath(path, f, info); err != nil {
		return TelemetryRead{}, err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return TelemetryRead{}, fmt.Errorf("rewind Apollo telemetry: %w", err)
	}
	confirm, err := readTelemetryBytes(f, info.Size())
	if err != nil {
		return TelemetryRead{}, err
	}
	if !bytes.Equal(data, confirm) {
		return TelemetryRead{}, fmt.Errorf("Apollo telemetry changed while it was read")
	}
	if err := revalidateTelemetryPath(path, f, info); err != nil {
		return TelemetryRead{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var telemetry Telemetry
	if err := decoder.Decode(&telemetry); err != nil {
		return TelemetryRead{}, fmt.Errorf("decode Apollo telemetry: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return TelemetryRead{}, fmt.Errorf("Apollo telemetry contains trailing JSON values")
	}
	if err := validateTelemetry(telemetry); err != nil {
		return TelemetryRead{}, err
	}
	return TelemetryRead{State: "active", Telemetry: &telemetry}, nil
}

func validTelemetryFile(info os.FileInfo) bool {
	return info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0
}

func readTelemetryBytes(f *os.File, expectedSize int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(f, maxTelemetryBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read Apollo telemetry: %w", err)
	}
	if int64(len(data)) != expectedSize || len(data) > maxTelemetryBytes {
		return nil, fmt.Errorf("Apollo telemetry changed while it was read")
	}
	return data, nil
}

// revalidateTelemetryPath ensures the named input remains the exact regular
// object originally opened with no-follow semantics. Two matching reads make
// in-place mutation during decoding fail closed rather than rendering a torn
// or substituted sample.
func revalidateTelemetryPath(path string, f *os.File, expected os.FileInfo) error {
	opened, err := f.Stat()
	if err != nil {
		return fmt.Errorf("restat opened Apollo telemetry: %w", err)
	}
	if !validTelemetryFile(opened) || !os.SameFile(expected, opened) || expected.Size() != opened.Size() {
		return fmt.Errorf("Apollo telemetry descriptor changed while it was read")
	}
	current, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("restat Apollo telemetry path: %w", err)
	}
	if !validTelemetryFile(current) || !os.SameFile(expected, current) || expected.Size() != current.Size() {
		return fmt.Errorf("Apollo telemetry path changed while it was read")
	}
	return nil
}

func validateTelemetry(t Telemetry) error {
	if t.SchemaVersion != "apollo-session-telemetry/v1" {
		return fmt.Errorf("unsupported Apollo telemetry schema")
	}
	if strings.TrimSpace(t.SessionID) == "" || strings.TrimSpace(t.EngineID) == "" || t.EmittedAt.IsZero() {
		return fmt.Errorf("Apollo telemetry is missing session identity")
	}
	if t.MachineID != "" && strings.TrimSpace(t.MachineID) == "" {
		return fmt.Errorf("Apollo telemetry has an invalid machine identity")
	}
	if err := nonNegativeFinite("tokens_per_second", t.TokensPerSec); err != nil {
		return err
	}
	if err := nonNegativeInt("bandwidth_bytes_per_second", t.BandwidthBps); err != nil {
		return err
	}
	if err := nonNegativeInt("memory_bytes", t.MemoryBytes); err != nil {
		return err
	}
	if err := percentage("network_saturation_percent", t.NetworkPct); err != nil {
		return err
	}
	if err := percentage("cpu_residency_percent", t.CPUResidency); err != nil {
		return err
	}
	if err := percentage("gpu_residency_percent", t.GPUResidency); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, estate := range t.Estates {
		if strings.TrimSpace(estate.ID) == "" || seen[estate.ID] {
			return fmt.Errorf("Apollo telemetry has an invalid or duplicate chip estate")
		}
		seen[estate.ID] = true
		if err := percentage("chip estate residency_percent", estate.ResidencyPct); err != nil {
			return err
		}
		if err := percentage("chip estate utilization_percent", estate.UtilizationPct); err != nil {
			return err
		}
		if err := nonNegativeInt("chip estate memory_bytes", estate.MemoryBytes); err != nil {
			return err
		}
	}
	return nil
}

func nonNegativeFinite(name string, value *float64) error {
	if value != nil && (*value < 0 || math.IsNaN(*value) || math.IsInf(*value, 0)) {
		return fmt.Errorf("Apollo telemetry %s is invalid", name)
	}
	return nil
}
func nonNegativeInt(name string, value *int64) error {
	if value != nil && *value < 0 {
		return fmt.Errorf("Apollo telemetry %s is invalid", name)
	}
	return nil
}
func percentage(name string, value *float64) error {
	if err := nonNegativeFinite(name, value); err != nil {
		return err
	}
	if value != nil && *value > 100 {
		return fmt.Errorf("Apollo telemetry %s exceeds 100%%", name)
	}
	return nil
}
