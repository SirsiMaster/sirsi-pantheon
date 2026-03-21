package hapi

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// Snapshot represents an APFS or Time Machine local snapshot.
type Snapshot struct {
	Name   string `json:"name"`
	Date   string `json:"date"`
	Size   string `json:"size,omitempty"` // Human-readable estimate
	Volume string `json:"volume"`
}

// SnapshotResult contains all local snapshots found.
type SnapshotResult struct {
	Snapshots []Snapshot `json:"snapshots"`
	Total     int        `json:"total"`
}

// ListSnapshots enumerates APFS/Time Machine local snapshots.
func ListSnapshots() (*SnapshotResult, error) {
	if runtime.GOOS != "darwin" {
		return &SnapshotResult{}, nil
	}

	result := &SnapshotResult{}

	// List APFS snapshots via tmutil
	out, err := exec.Command("tmutil", "listlocalsnapshots", "/").Output()
	if err != nil {
		// Try without volume arg
		out, err = exec.Command("tmutil", "listlocalsnapshots", ".").Output()
		if err != nil {
			return result, nil // No snapshots or no permission
		}
	}

	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Format: com.apple.TimeMachine.2026-03-21-120000.local
		snap := Snapshot{
			Name:   line,
			Volume: "/",
		}

		// Extract date from snapshot name
		if strings.Contains(line, "TimeMachine.") {
			parts := strings.Split(line, "TimeMachine.")
			if len(parts) >= 2 {
				dateStr := strings.TrimSuffix(parts[1], ".local")
				snap.Date = dateStr
			}
		}

		result.Snapshots = append(result.Snapshots, snap)
	}

	result.Total = len(result.Snapshots)
	return result, nil
}

// PruneSnapshot deletes a specific local snapshot.
// Requires sudo on macOS.
func PruneSnapshot(name string, dryRun bool) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("snapshot pruning only supported on macOS")
	}

	if dryRun {
		return nil
	}

	cmd := exec.Command("tmutil", "deletelocalsnapshots", name)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("delete snapshot %s: %w (may need sudo)", name, err)
	}
	return nil
}
