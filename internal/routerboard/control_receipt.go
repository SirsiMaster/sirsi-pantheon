package routerboard

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

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
