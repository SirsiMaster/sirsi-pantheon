package routerboard

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/rolereceipt"
)

// ControlSchema is the stable machine-facing envelope used by remote workers.
// The board payload remains the presentation contract; this envelope makes the
// authority and supported control verbs explicit so an M1 client cannot infer
// a second router or silently fall back to a local store.
const ControlSchema = "pantheon.worker-control/v1"

// ControlCapability describes one operation owned by the canonical router.
// Commands are documentation for constrained clients, not an invitation to
// execute arbitrary shell strings.
type ControlCapability struct {
	Verb        string `json:"verb"`
	Command     string `json:"command"`
	Mutates     bool   `json:"mutates"`
	Description string `json:"description"`
}

// ControlEnvelope is the single worker-facing read model. State is the same
// Payload used by the board and SSE stream, so remote inspection cannot drift
// from the local UI or menubar projection.
type ControlEnvelope struct {
	Schema            string              `json:"schema"`
	Authority         string              `json:"authority"`
	Revision          uint64              `json:"revision"`
	GeneratedAt       string              `json:"generated_at"`
	StateSHA256       string              `json:"state_sha256"`
	RoleReceiptID     string              `json:"role_receipt_id,omitempty"`
	RoleReceiptSHA256 string              `json:"role_receipt_sha256,omitempty"`
	Capabilities      []ControlCapability `json:"capabilities"`
	State             Payload             `json:"state"`
}

var controlCapabilities = []ControlCapability{
	{Verb: "inspect", Command: "sirsi router control", Description: "Read the canonical live worker, task, event, and evidence projection."},
	{Verb: "message", Command: "sirsi router send <agent>", Mutates: true, Description: "Send a durable message through the canonical router store."},
	{Verb: "review_request", Command: "sirsi router send <agent> --type review", Mutates: true, Description: "Create a review request as a durable router item."},
	{Verb: "delegate", Command: "sirsi router task add <agent> <task-id>", Mutates: true, Description: "Register durable work in the canonical task ledger."},
	{Verb: "claim", Command: "sirsi router task claim <agent>", Mutates: true, Description: "Atomically claim work with a fenced lease."},
	{Verb: "cancel_handback", Command: "sirsi router task release <agent> <task-id>", Mutates: true, Description: "Release leased work with an explicit handback reason."},
	{Verb: "result_return", Command: "sirsi router task complete <agent> <task-id>", Mutates: true, Description: "Return a result through the fenced task lease."},
}

func cloneCapabilities() []ControlCapability {
	return append([]ControlCapability(nil), controlCapabilities...)
}

// ControlCapabilities returns the canonical closed capability registry for
// clients that construct or inspect a control envelope in another package.
// The returned slice is a copy and cannot mutate the router's registry.
func ControlCapabilities() []ControlCapability { return cloneCapabilities() }

// Validate verifies the complete control envelope before it is published or
// consumed. The capability list is part of the authority contract: a client
// must not accept an envelope that advertises a different command surface.
func (e ControlEnvelope) Validate() error {
	if e.Schema != ControlSchema {
		return fmt.Errorf("control envelope schema %q is unsupported", e.Schema)
	}
	if e.Authority != "canonical-routerstore" {
		return fmt.Errorf("control envelope authority %q is not canonical-routerstore", e.Authority)
	}
	if e.Revision == 0 {
		return errors.New("control envelope revision is zero")
	}
	if err := validateRoleReceiptReference(e.RoleReceiptID, e.RoleReceiptSHA256); err != nil {
		return fmt.Errorf("control envelope role reference: %w", err)
	}
	if strings.TrimSpace(e.GeneratedAt) == "" || e.GeneratedAt != e.State.GeneratedAt {
		return errors.New("control envelope generated_at does not match canonical state")
	}
	if _, err := time.Parse(time.RFC3339Nano, e.GeneratedAt); err != nil {
		return fmt.Errorf("control envelope generated_at is not RFC3339: %w", err)
	}
	canonicalState, err := json.Marshal(e.State)
	if err != nil {
		return fmt.Errorf("control envelope state: %w", err)
	}
	stateSum := sha256.Sum256(canonicalState)
	if e.StateSHA256 != hex.EncodeToString(stateSum[:]) {
		return errors.New("control envelope state digest mismatch")
	}
	if len(e.Capabilities) != len(controlCapabilities) {
		return fmt.Errorf("control envelope capability count %d does not match canonical count %d", len(e.Capabilities), len(controlCapabilities))
	}
	expected := make(map[string]ControlCapability, len(controlCapabilities))
	for _, capability := range controlCapabilities {
		expected[capability.Verb] = capability
	}
	seen := make(map[string]struct{}, len(e.Capabilities))
	for _, capability := range e.Capabilities {
		if capability.Verb == "" {
			return errors.New("control envelope contains an empty capability verb")
		}
		if _, duplicate := seen[capability.Verb]; duplicate {
			return fmt.Errorf("control envelope contains duplicate capability %q", capability.Verb)
		}
		seen[capability.Verb] = struct{}{}
		canonical, ok := expected[capability.Verb]
		if !ok {
			return fmt.Errorf("control envelope contains unknown capability %q", capability.Verb)
		}
		if capability != canonical {
			return fmt.Errorf("control envelope capability %q differs from canonical registry", capability.Verb)
		}
	}
	for _, capability := range controlCapabilities {
		if _, ok := seen[capability.Verb]; !ok {
			return fmt.Errorf("control envelope is missing capability %q", capability.Verb)
		}
	}
	return nil
}

// BindControlEnvelopeRoleReceipt adds the exact authenticated role proof to a
// canonical inspect response. It validates the existing envelope first and
// revalidates after binding so callers cannot attach proof to malformed state.
func BindControlEnvelopeRoleReceipt(body []byte, receipt rolereceipt.AuthenticatedReceipt) ([]byte, error) {
	if receipt.RawSHA256() == "" || strings.TrimSpace(receipt.Receipt().ReceiptID) == "" {
		return nil, errors.New("control envelope role receipt is unbound")
	}
	if err := ValidateJSONNoDuplicateKeys(body); err != nil {
		return nil, fmt.Errorf("control envelope JSON is ambiguous: %w", err)
	}
	var envelope ControlEnvelope
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil {
		return nil, fmt.Errorf("decode control envelope: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, errors.New("control envelope contains multiple JSON values")
		}
		return nil, fmt.Errorf("control envelope trailing JSON: %w", err)
	}
	if err := envelope.Validate(); err != nil {
		return nil, err
	}
	envelope.RoleReceiptID = receipt.Receipt().ReceiptID
	envelope.RoleReceiptSHA256 = receipt.RawSHA256()
	if err := envelope.Validate(); err != nil {
		return nil, err
	}
	out, err := json.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("encode control envelope: %w", err)
	}
	return out, nil
}

// SnapshotControl wraps the latest successful board poll. A zero version is
// deliberately an error: a missing poll is not an empty fleet.
func (b *Board) SnapshotControl() ([]byte, uint64, error) {
	body, version := b.Snapshot()
	if version == 0 || len(body) == 0 {
		return nil, 0, nil
	}
	var state Payload
	if err := json.Unmarshal(body, &state); err != nil {
		return nil, version, err
	}
	if strings.TrimSpace(state.GeneratedAt) == "" {
		return nil, version, fmt.Errorf("control snapshot: generated_at is required")
	}
	if _, err := time.Parse(time.RFC3339Nano, state.GeneratedAt); err != nil {
		return nil, version, fmt.Errorf("control snapshot: generated_at is not RFC3339: %w", err)
	}
	canonicalState, err := json.Marshal(state)
	if err != nil {
		return nil, version, err
	}
	stateSum := sha256.Sum256(canonicalState)
	envelope := ControlEnvelope{
		Schema:       ControlSchema,
		Authority:    "canonical-routerstore",
		Revision:     version,
		GeneratedAt:  state.GeneratedAt,
		StateSHA256:  hex.EncodeToString(stateSum[:]),
		Capabilities: cloneCapabilities(),
		State:        state,
	}
	if err := envelope.Validate(); err != nil {
		return nil, version, err
	}
	out, err := json.Marshal(envelope)
	if err != nil {
		return nil, version, err
	}
	return out, version, nil
}
