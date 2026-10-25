package provider

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// CapabilityState separates absent evidence from an explicit refusal. The zero
// value is unknown; protocol compatibility alone is never positive evidence.
type CapabilityState string

const (
	CapabilityUnknown     CapabilityState = ""
	CapabilityUnsupported CapabilityState = "unsupported"
	CapabilitySupported   CapabilityState = "supported"
)

type ResponseFormat string

const (
	ResponseFormatText       ResponseFormat = "text"
	ResponseFormatJSONObject ResponseFormat = "json_object"
)

// DeterminismMode names the producer's batch-invariant contract, not a promise
// of universal repeatability. SNE reports launch-time state, not a request flag.
// No installed OpenAICompat resolver currently qualifies this requirement.
type DeterminismMode string

const DeterminismBatchInvariant DeterminismMode = "batch_invariant"

var (
	ErrRequestRequirements = errors.New("invalid generation requirements")
	ErrCapability          = errors.New("generation capability unqualified")
	ErrInvalidJSONObject   = errors.New("provider did not return one complete JSON object")
)

// ValidateRequirements is shared by routing and direct provider calls. It has
// no side effects and must run before model discovery, availability or HTTP.
func ValidateRequirements(req Request, stream bool) error {
	switch req.ResponseFormat {
	case "", ResponseFormatText, ResponseFormatJSONObject:
	default:
		return fmt.Errorf("%w: unsupported response format", ErrRequestRequirements)
	}
	if req.Determinism != "" && req.Determinism != DeterminismBatchInvariant {
		return fmt.Errorf("%w: unsupported determinism contract", ErrRequestRequirements)
	}
	if req.ResponseFormat == ResponseFormatJSONObject {
		if stream {
			return fmt.Errorf("%w: json_object with streaming is not supported", ErrRequestRequirements)
		}
		if len(req.Tools) > 0 || req.RequireTool {
			return fmt.Errorf("%w: json_object cannot be combined with tools", ErrRequestRequirements)
		}
	}
	return nil
}

func capabilityDescription(state CapabilityState) string {
	if state == CapabilityUnsupported {
		return "declared unsupported"
	}
	return "unknown"
}

// QualifyRequirements checks all requested generation capabilities together.
// A mode name and supported state must agree; malformed states fail closed.
func QualifyRequirements(req Request, caps Caps) error {
	if req.ResponseFormat == ResponseFormatJSONObject && caps.JSONObject != CapabilitySupported {
		return fmt.Errorf("%w: json_object is %s", ErrCapability, capabilityDescription(caps.JSONObject))
	}
	if req.Determinism != "" && (caps.Determinism != CapabilitySupported || caps.DeterminismMode != req.Determinism) {
		return fmt.Errorf("%w: batch_invariant determinism is %s", ErrCapability, capabilityDescription(caps.Determinism))
	}
	return nil
}

// ValidateResponse never accepts a valid prefix, tool-only result or unfinished
// completion as JSON success. No retries or inference replay occur here.
func ValidateResponse(req Request, resp Response) error {
	if req.ResponseFormat != ResponseFormatJSONObject {
		return nil
	}
	text := strings.TrimSpace(resp.Text)
	if resp.FinishReason != "stop" || len(resp.ToolCalls) != 0 || len(text) == 0 || text[0] != '{' || !json.Valid([]byte(text)) {
		return ErrInvalidJSONObject
	}
	return nil
}
