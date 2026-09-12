package rolereceipt

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Authenticator is the external trust-root boundary. The Pantheon parser does
// not own issuer keys, revocation state, or signature policy; callers must
// supply a separately governed verifier before a receipt can become authority.
type Authenticator interface {
	VerifyRoleReceipt(raw []byte, receipt Receipt) error
}

// AuthenticatedReceipt is the only receipt type that represents an
// authenticated role. Its fields are private so callers cannot manufacture
// one from a structurally valid Receipt or by copying an unverified signature.
type AuthenticatedReceipt struct {
	receipt Receipt
	rawHash string
}

// AuthenticateAndValidate performs structural, temporal, operation, and
// external trust-root checks in one explicit sequence. It never falls back to
// structural validation when the trust root is absent.
func AuthenticateAndValidate(raw []byte, constraints Constraints, authenticator Authenticator) (AuthenticatedReceipt, error) {
	if authenticator == nil {
		return AuthenticatedReceipt{}, errors.New("role receipt authenticator is required")
	}
	receipt, err := Parse(raw)
	if err != nil {
		return AuthenticatedReceipt{}, err
	}
	if err := receipt.ValidateFor(constraints); err != nil {
		return AuthenticatedReceipt{}, err
	}
	if err := authenticator.VerifyRoleReceipt(raw, receipt); err != nil {
		return AuthenticatedReceipt{}, fmt.Errorf("authenticate role receipt: %w", err)
	}
	sum := sha256.Sum256(raw)
	return AuthenticatedReceipt{receipt: receipt, rawHash: hex.EncodeToString(sum[:])}, nil
}

// Receipt returns the authenticated receipt as a value copy. The returned
// receipt remains evidence; authorization consumers should retain the wrapper
// and its raw-byte digest together.
func (r AuthenticatedReceipt) Receipt() Receipt { return r.receipt }

// RawSHA256 identifies the exact bytes that the external trust root accepted.
func (r AuthenticatedReceipt) RawSHA256() string { return r.rawHash }

// Authorizes rechecks the authenticated receipt at the point of control-plane
// use. The zero value is deliberately invalid, and the operation must remain
// within the receipt's scope and validity window; structural evidence alone
// never becomes an authorization.
func (r AuthenticatedReceipt) Authorizes(operation string, now time.Time) error {
	if r.rawHash == "" || r.receipt.ReceiptID == "" {
		return errors.New("authenticated role receipt is unbound")
	}
	operation = strings.TrimSpace(operation)
	if operation == "" {
		return errors.New("authorized operation is required")
	}
	return r.receipt.ValidateFor(Constraints{Now: now, RequiredScope: []string{operation}})
}
