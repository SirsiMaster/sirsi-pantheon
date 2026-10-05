package sne

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func TestHardwareObservationAdmission(t *testing.T) {
	now := time.Date(2026, 10, 5, 11, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name, node, overall, protected string
		age                            time.Duration
		allow                          bool
	}{
		{"fresh", "M1", "PASS_OBSERVATION_ONLY", "PASS", time.Minute, true},
		{"peer cannot admit", "M5", "PASS_OBSERVATION_ONLY", "PASS", time.Minute, false},
		{"hold", "M1", "HOLD_M1_SWAP", "PASS", time.Minute, false},
		{"unknown protection", "M1", "PASS_OBSERVATION_ONLY", "UNKNOWN", time.Minute, false},
		{"expired", "M1", "PASS_OBSERVATION_ONLY", "PASS", 5 * time.Minute, false},
		{"future", "M1", "PASS_OBSERVATION_ONLY", "PASS", -time.Second, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var p SupervisorProfile
			p.SNE.HardwareObservation = "receipt.json"
			p.SNE.HardwareNodeID = "M1"
			p.SNE.HardwareCollectorSHA256 = strings.Repeat("a", 64)
			raw, _ := json.Marshal(map[string]any{"schema": "sirsi.stacklab.hardware-observation.v2", "owner": "sirsi-hardware-admin", "observed_at": now.Add(-tc.age), "collector_sha256": p.SNE.HardwareCollectorSHA256, "hosts": map[string]any{tc.node: map[string]any{"capture_result": "PASS", "admission": map[string]any{"overall": tc.overall, "thermal": "PASS", "swap": "PASS", "protected_state": tc.protected, "filevault": "PASS", "assessment_policy": "PASS", "missing_sections": []string{}}}}})
			err := checkHardwareObservation(p, now, func(string) ([]byte, error) { return raw, nil })
			if (err == nil) != tc.allow {
				t.Fatalf("allow=%v error=%v", tc.allow, err)
			}
		})
	}
}

func TestHardwareObservationStopsSupervisorBeforeLaunch(t *testing.T) {
	s := &Supervisor{}
	s.profile.SNE.HardwareObservation = "missing.json"
	s.profile.SNE.HardwareNodeID = "M1"
	s.profile.SNE.HardwareCollectorSHA256 = strings.Repeat("a", 64)
	if err := s.startLocked(context.Background(), false); err == nil || !strings.Contains(err.Error(), "hardware qualification") {
		t.Fatalf("expected hold before process launch: %v", err)
	}
	if err := checkHardwareObservation(SupervisorProfile{}, time.Now(), os.ReadFile); err != nil {
		t.Fatal(err)
	}
}
