package main

import (
	"encoding/base64"
	"errors"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
)

type maatLivenessMemoryInspection struct {
	maat.DefaultStoreStatus
	IncompleteAttempt    bool                           `json:"incomplete_attempt"`
	ProjectionDebt       bool                           `json:"projection_debt"`
	PreparedPreflight    *maat.PreparedPreflightReceipt `json:"prepared_preflight,omitempty"`
	ReceiptRecoveryError string                         `json:"receipt_recovery_error,omitempty"`
}

func addPreparedMaatReceiptInspection(result *maatLivenessMemoryInspection, store maatLivenessStore) {
	inspector, ok := store.(interface {
		InspectPreparedPreflightReceipt() (*maat.PreparedPreflightReceipt, error)
	})
	if !ok {
		result.ReceiptRecoveryError = "prepared-inspection-unavailable"
		return
	}
	prepared, err := inspector.InspectPreparedPreflightReceipt()
	if err != nil {
		result.ReceiptRecoveryError = "prepared-receipt-unavailable"
		result.State = maat.DefaultStoreIncomplete
		result.Detail = "Prepared receipt custody is unverifiable. Missing, changed or unrelated debt remains retained; no recovery or action was authorized."
		return
	}
	result.PreparedPreflight = prepared
	if prepared != nil {
		result.State = maat.DefaultStoreIncomplete
		result.Detail = "A complete exact preflight receipt write is prepared. Inspect and confirm historical receipt recovery, then perform a fresh assessment; no action was authorized."
	}
}

func recoverMaatPreparedReceipt(store *maat.Store, digest, encoded string) (maat.PreflightReceiptRead, error) {
	if encoded == "" {
		return store.RecoverPreflightReceipt(digest)
	}
	if len(encoded) > base64.StdEncoding.EncodedLen(8192) {
		return maat.PreflightReceiptRead{}, errors.New("prepared receipt target exceeds bound")
	}
	raw, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil {
		return maat.PreflightReceiptRead{}, err
	}
	target, err := maat.DecodePreparedPreflightTarget(raw)
	if err != nil {
		return maat.PreflightReceiptRead{}, err
	}
	if target.Reference.ReceiptSHA256 != digest {
		return maat.PreflightReceiptRead{}, errors.New("prepared target does not match selected receipt")
	}
	return store.RecoverPreparedPreflightReceipt(target)
}
