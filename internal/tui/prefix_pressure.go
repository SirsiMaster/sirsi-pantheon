package tui

import (
	"encoding/json"
	"fmt"
	"strings"
)

// prefixCachePressureView is the exact CLI projection exposed by Pantheon's
// receipt-backed prefix-cache-pressure route. It intentionally has no decision
// or execution fields: SNE owns those operations and the TUI only renders its
// returned evidence.
type prefixCachePressureView struct {
	State   string `json:"state"`
	Receipt struct {
		Observation struct {
			HostID    string `json:"host_id"`
			RequestID string `json:"request_id"`
		} `json:"observation"`
		ObservationSHA256 string `json:"observation_sha256"`
	} `json:"receipt"`
	Confirmation  *json.RawMessage `json:"confirmation,omitempty"`
	Authorization *struct {
		State string `json:"state"`
	} `json:"authorization,omitempty"`
}

// prefixCachePressureEvidence is the protected reader route's exact envelope.
// Receipt stays raw because SNE owns the schema and may extend it; this surface
// recognizes only truthful status fields and otherwise reports unavailable.
type prefixCachePressureEvidence struct {
	State        string          `json:"state"`
	EvidenceType string          `json:"evidence_type"`
	Identity     string          `json:"identity"`
	Receipt      json.RawMessage `json:"receipt,omitempty"`
}

type prefixCachePressurePanel struct {
	view         prefixCachePressureView
	execution    prefixCachePressureEvidence
	retention    prefixCachePressureEvidence
	prepareErr   error
	executionErr error
	retentionErr error
}

func (p prefixCachePressurePanel) lines(caps Capabilities) []string {
	lines := []string{"", "  " + Paint("SNE prefix-cache pressure", TokBrand, caps)}
	if p.prepareErr != nil {
		return append(lines,
			"  "+Paint("unavailable", TokWarn, caps)+" — observation/authorization evidence could not be read",
			"  "+Paint("SNE was not changed; retry with u update", TokDim, caps),
		)
	}

	state := strings.TrimSpace(p.view.State)
	if state == "" {
		state = "observation-only"
	}
	lines = append(lines, "  "+Paint(prefixCachePressureStateLabel(state), TokAccent, caps))
	if p.view.Receipt.Observation.HostID != "" {
		lines = append(lines, "  "+Paint("host:    ", TokDim, caps)+p.view.Receipt.Observation.HostID)
	}
	if p.view.Receipt.Observation.RequestID != "" {
		lines = append(lines, "  "+Paint("request: ", TokDim, caps)+p.view.Receipt.Observation.RequestID)
	}
	if p.view.Receipt.ObservationSHA256 != "" {
		lines = append(lines, "  "+Paint("observation receipt: ", TokDim, caps)+p.view.Receipt.ObservationSHA256)
	}
	if p.view.Confirmation != nil {
		lines = append(lines, "  "+Paint("owner confirmation is required; no SNE action has been authorized", TokWarn, caps))
	}
	if p.view.Authorization != nil {
		lines = append(lines, "  "+Paint("authorization accepted; SNE decision and execution remain external", TokDim, caps))
	}
	lines = append(lines, p.evidenceLines("execution", p.execution, p.executionErr, caps)...)
	lines = append(lines, p.evidenceLines("retention", p.retention, p.retentionErr, caps)...)
	return lines
}

func (p prefixCachePressurePanel) evidenceLines(kind string, evidence prefixCachePressureEvidence, err error, caps Capabilities) []string {
	if err != nil || evidence.State == "" || evidence.State == "unavailable" {
		return []string{"  " + Paint(kind+": unavailable", TokDim, caps) + " — no SNE-owned receipt is inferred"}
	}
	status, code := prefixCachePressureReceiptStatus(evidence.Receipt)
	if status == "" {
		return []string{"  " + Paint(kind+": unavailable", TokDim, caps) + " — receipt has no recognized terminal state"}
	}
	label := kind + ": " + status
	if status == "failed" && code == "cache_pressure_execution_interrupted" {
		label = kind + ": interrupted — recovered to failed"
	}
	return []string{"  " + Paint(label, TokAccent, caps)}
}

func prefixCachePressureStateLabel(state string) string {
	switch state {
	case "owner-confirmation-required":
		return "observation recorded — owner confirmation required"
	case "authorization-accepted":
		return "authorization accepted — awaiting SNE decision"
	case "expired", "rejected":
		return "authorization " + state
	case "started", "completed", "failed", "interrupted":
		return "SNE execution " + state
	case "retention-completed":
		return "SNE retention cleanup completed"
	case "unavailable":
		return "unavailable — no execution state inferred"
	default:
		return "observation only — SNE has not been authorized"
	}
}

func prefixCachePressureReceiptStatus(raw json.RawMessage) (string, string) {
	var receipt struct {
		Status    string `json:"status"`
		ErrorCode string `json:"error_code"`
	}
	if err := json.Unmarshal(raw, &receipt); err != nil {
		return "", ""
	}
	switch receipt.Status {
	case "started", "completed", "failed", "interrupted":
		return receipt.Status, receipt.ErrorCode
	default:
		return "", ""
	}
}

func (p prefixCachePressurePanel) String() string {
	return fmt.Sprintf("prefix-cache pressure: %s", p.view.State)
}
