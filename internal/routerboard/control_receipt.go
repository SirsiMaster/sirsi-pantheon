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
	if err := validateRoleReceiptReference(r.RoleReceiptID, r.RoleReceiptSHA256); err != nil {
		return fmt.Errorf("control action receipt: role receipt reference: %w", err)
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
	if err := validateRoleReceiptReference(r.RoleReceiptID, r.RoleReceiptSHA256); err != nil {
		return fmt.Errorf("control action receipt role reference: %w", err)
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

func validateRoleReceiptReference(receiptID, receiptSHA256 string) error {
	originalID, originalSHA256 := receiptID, receiptSHA256
	receiptID = strings.TrimSpace(receiptID)
	receiptSHA256 = strings.TrimSpace(receiptSHA256)
	if receiptID == "" && receiptSHA256 == "" {
		return nil
	}
	if originalID != receiptID || originalSHA256 != receiptSHA256 {
		return fmt.Errorf("role receipt id and sha256 must not contain surrounding whitespace")
	}
	if receiptID == "" || receiptSHA256 == "" {
		return fmt.Errorf("role receipt id and sha256 must be supplied together")
	}
	if len(receiptID) > 256 {
		return fmt.Errorf("role receipt id exceeds 256 bytes")
	}
	for _, character := range receiptID {
		if character < 0x20 || character == 0x7f {
			return fmt.Errorf("role receipt id contains a control character")
		}
	}
	if len(receiptSHA256) != sha256.Size*2 {
		return fmt.Errorf("role receipt sha256 must be %d hexadecimal characters", sha256.Size*2)
	}
	if receiptSHA256 != strings.ToLower(receiptSHA256) {
		return fmt.Errorf("role receipt sha256 must use lowercase hexadecimal")
	}
	if _, err := hex.DecodeString(receiptSHA256); err != nil {
		return fmt.Errorf("role receipt sha256 is not hexadecimal: %w", err)
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
