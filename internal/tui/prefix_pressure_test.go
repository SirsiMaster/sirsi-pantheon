package tui

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestPrefixCachePressurePanelTruthfullyProjectsContractStates(t *testing.T) {
	caps := testCaps()
	panel := prefixCachePressurePanel{}
	panel.view.State = "owner-confirmation-required"
	panel.view.Receipt.Observation.HostID = "m5"
	panel.view.Receipt.Observation.RequestID = "request-1"
	panel.view.Receipt.ObservationSHA256 = "abc123"
	confirmation := json.RawMessage(`{"token":"not-rendered"}`)
	panel.view.Confirmation = &confirmation
	panel.execution = prefixCachePressureEvidence{State: "available", EvidenceType: "execution", Identity: "request-1", Receipt: json.RawMessage(`{"status":"started"}`)}
	panel.retention = prefixCachePressureEvidence{State: "available", EvidenceType: "retention", Identity: "cleanup-1", Receipt: json.RawMessage(`{"status":"completed"}`)}
	frame := strings.Join(panel.lines(caps), "\n")
	for _, want := range []string{
		"owner confirmation required", "host:    m5", "request: request-1", "execution: started", "retention: completed",
	} {
		if !strings.Contains(frame, want) {
			t.Errorf("panel missing %q:\n%s", want, frame)
		}
	}
}

func TestPrefixCachePressurePanelNeverInfersUnavailableOrInterrupted(t *testing.T) {
	caps := testCaps()
	panel := prefixCachePressurePanel{prepareErr: errors.New("offline")}
	frame := strings.Join(panel.lines(caps), "\n")
	if !strings.Contains(frame, "no SNE action has been authorized") && !strings.Contains(frame, "SNE was not changed") {
		t.Fatalf("unavailable panel implied an action:\n%s", frame)
	}

	panel = prefixCachePressurePanel{execution: prefixCachePressureEvidence{State: "available", Receipt: json.RawMessage(`{"status":"failed","error_code":"cache_pressure_execution_interrupted"}`)}}
	frame = strings.Join(panel.lines(caps), "\n")
	if !strings.Contains(frame, "interrupted — recovered to failed") || !strings.Contains(frame, "retention: unavailable") {
		t.Fatalf("terminal state was not rendered truthfully:\n%s", frame)
	}
}
