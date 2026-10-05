package routerboard

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

const ControlFailureSchema = "pantheon.worker-control-failure/v1"

// ControlActionFailure is the negative-path counterpart to
// ControlActionResponse. It binds a rejected action to the exact request
// bytes, so a remote worker can distinguish a real router rejection from a
// detached or replayed error body.
type ControlActionFailure struct {
	Schema        string `json:"schema"`
	Authority     string `json:"authority"`
	Verb          string `json:"verb,omitempty"`
	Error         string `json:"error"`
	RequestSHA256 string `json:"request_sha256"`
	ReceiptSHA256 string `json:"receipt_sha256,omitempty"`
}

// ControlActionFailureError transports a verified canonical rejection across
// the M1 proxy without collapsing its request-bound receipt into plain text.
type ControlActionFailureError struct {
	Failure ControlActionFailure
}

func (e *ControlActionFailureError) Error() string {
	if e == nil {
		return "canonical worker action failed"
	}
	return e.Failure.Error
}

func (f *ControlActionFailure) SealControlActionFailure(requestBody []byte) error {
	if f == nil {
		return fmt.Errorf("control action failure receipt: response is nil")
	}
	requestSum := sha256.Sum256(requestBody)
	f.RequestSHA256 = hex.EncodeToString(requestSum[:])
	f.ReceiptSHA256 = ""
	canonical, err := json.Marshal(f)
	if err != nil {
		return fmt.Errorf("control action failure receipt: marshal response: %w", err)
	}
	receiptSum := sha256.Sum256(canonical)
	f.ReceiptSHA256 = hex.EncodeToString(receiptSum[:])
	return nil
}

func (f ControlActionFailure) VerifyControlActionFailure(requestBody []byte) error {
	if f.Schema != ControlFailureSchema || f.Authority != "canonical-routerstore" {
		return fmt.Errorf("control action failure identity is not canonical")
	}
	// An empty verb is reserved for failures raised before the request could be
	// decoded. Once a verb is present, it must be one of the closed router
	// capabilities; otherwise this receipt cannot be attributed to a valid
	// control operation.
	if f.Verb != "" && !isControlCapabilityVerb(f.Verb) {
		return fmt.Errorf("control action failure verb is not recognized")
	}
	if f.Error == "" || f.RequestSHA256 == "" || f.ReceiptSHA256 == "" {
		return fmt.Errorf("control action failure receipt is incomplete")
	}
	requestSum := sha256.Sum256(requestBody)
	expectedRequest := hex.EncodeToString(requestSum[:])
	if subtle.ConstantTimeCompare([]byte(f.RequestSHA256), []byte(expectedRequest)) != 1 {
		return fmt.Errorf("control action failure request digest mismatch")
	}
	expectedReceipt := f.ReceiptSHA256
	f.ReceiptSHA256 = ""
	canonical, err := json.Marshal(f)
	if err != nil {
		return fmt.Errorf("control action failure receipt: marshal response: %w", err)
	}
	receiptSum := sha256.Sum256(canonical)
	actualReceipt := hex.EncodeToString(receiptSum[:])
	if subtle.ConstantTimeCompare([]byte(expectedReceipt), []byte(actualReceipt)) != 1 {
		return fmt.Errorf("control action failure receipt digest mismatch")
	}
	return nil
}

// SealControlActionResponse binds a successful action response to the exact
// request bytes received by the router. It is an in-band receipt, not a second
// durable ledger: routerstore remains the only mutation authority.
func (r *ControlActionResponse) SealControlActionResponse(requestBody []byte) error {
	if r == nil {
		return fmt.Errorf("control action receipt: response is nil")
	}
	requestSum := sha256.Sum256(requestBody)
	r.RequestSHA256 = hex.EncodeToString(requestSum[:])
	r.ReceiptSHA256 = ""
	canonical, err := json.Marshal(r)
	if err != nil {
		return fmt.Errorf("control action receipt: marshal response: %w", err)
	}
	receiptSum := sha256.Sum256(canonical)
	r.ReceiptSHA256 = hex.EncodeToString(receiptSum[:])
	return nil
}

// VerifyControlActionResponse verifies both the request binding and the
// response digest. The receipt field is removed only in the local copy used to
// recompute the digest; the caller's decoded response is not mutated.
func (r ControlActionResponse) VerifyControlActionResponse(requestBody []byte) error {
	if r.Schema != ControlSchema || r.Authority != "canonical-routerstore" || r.Verb == "" || !isControlCapabilityVerb(r.Verb) {
		return fmt.Errorf("control action identity is not canonical")
	}
	if r.RequestSHA256 == "" || r.ReceiptSHA256 == "" {
		return fmt.Errorf("control action receipt is incomplete")
	}
	requestSum := sha256.Sum256(requestBody)
	expectedRequest := hex.EncodeToString(requestSum[:])
	if subtle.ConstantTimeCompare([]byte(r.RequestSHA256), []byte(expectedRequest)) != 1 {
		return fmt.Errorf("control action request digest mismatch")
	}
	expectedReceipt := r.ReceiptSHA256
	r.ReceiptSHA256 = ""
	canonical, err := json.Marshal(r)
	if err != nil {
		return fmt.Errorf("control action receipt: marshal response: %w", err)
	}
	receiptSum := sha256.Sum256(canonical)
	actualReceipt := hex.EncodeToString(receiptSum[:])
	if subtle.ConstantTimeCompare([]byte(expectedReceipt), []byte(actualReceipt)) != 1 {
		return fmt.Errorf("control action receipt digest mismatch")
	}
	return nil
}

// VerifyControlActionResponseForRequest checks both the receipt digest and the
// action-specific result proof. All clients use this method so a response that
// is correctly hashed but detached from the requested operation is never
// accepted as a successful worker action.
func (r ControlActionResponse) VerifyControlActionResponseForRequest(requestBody []byte) error {
	request, err := DecodeControlActionRequest(requestBody)
	if err != nil {
		return fmt.Errorf("control action request is invalid: %w", err)
	}
	if err := r.VerifyControlActionResponse(requestBody); err != nil {
		return err
	}
	verb := strings.TrimSpace(request.Verb)
	if r.Verb != verb {
		return fmt.Errorf("control action response verb %q does not match %q", r.Verb, verb)
	}
	switch verb {
	case "message", "review_request":
		if strings.TrimSpace(r.ItemID) == "" {
			return fmt.Errorf("control action response omitted item_id")
		}
	case "delegate", "cancel_handback", "result_return":
		if strings.TrimSpace(r.TaskID) == "" {
			return fmt.Errorf("control action response omitted task_id")
		}
		if strings.TrimSpace(r.TaskID) != strings.TrimSpace(request.TaskID) {
			return fmt.Errorf("control action response task_id %q does not match requested task_id %q", r.TaskID, request.TaskID)
		}
	case "claim":
		if r.Lease == nil || strings.TrimSpace(r.Lease.Token) == "" || strings.TrimSpace(r.Lease.TaskID) == "" {
			return fmt.Errorf("control action response omitted lease proof")
		}
		lease := r.Lease
		if strings.TrimSpace(lease.Agent) != strings.TrimSpace(request.Agent) {
			return fmt.Errorf("control action lease agent %q does not match requested agent %q", lease.Agent, request.Agent)
		}
		if strings.TrimSpace(lease.Worker) != strings.TrimSpace(request.Worker) {
			return fmt.Errorf("control action lease worker %q does not match requested worker %q", lease.Worker, request.Worker)
		}
		if strings.TrimSpace(lease.ThreadID) != strings.TrimSpace(request.ThreadID) {
			return fmt.Errorf("control action lease thread_id %q does not match requested thread_id %q", lease.ThreadID, request.ThreadID)
		}
		if requestedTaskID := strings.TrimSpace(request.TaskID); requestedTaskID != "" && strings.TrimSpace(lease.TaskID) != requestedTaskID {
			return fmt.Errorf("control action lease task_id %q does not match requested task_id %q", lease.TaskID, requestedTaskID)
		}
		if strings.TrimSpace(r.TaskID) != strings.TrimSpace(lease.TaskID) || lease.Expires.IsZero() || lease.Attempt <= 0 {
			return fmt.Errorf("control action lease proof is incomplete or detached from task_id")
		}
	default:
		return fmt.Errorf("control action request omitted or has unsupported verb %q", verb)
	}
	if verb == "result_return" {
		if strings.TrimSpace(r.ResultRef) == "" {
			return fmt.Errorf("control action response omitted result_ref")
		}
		if strings.TrimSpace(r.ResultRef) != strings.TrimSpace(request.ResultRef) {
			return fmt.Errorf("control action response result_ref does not match requested result_ref")
		}
	}
	return nil
}

func isControlCapabilityVerb(verb string) bool {
	for _, capability := range controlCapabilities {
		if capability.Verb == verb {
			return true
		}
	}
	return false
}
