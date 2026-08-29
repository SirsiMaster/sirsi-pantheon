package sne

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// PrefixCachePressureObservation is Pantheon's exact SNE pressure wire value.
// Pantheon measures it; SNE alone owns cache-policy mathematics.
type PrefixCachePressureObservation struct {
	RequestID         string `json:"request_id"`
	HostID            string `json:"host_id"`
	ObservedAtUnix    int64  `json:"observed_at_unix"`
	ExpiresAtUnix     int64  `json:"expires_at_unix"`
	TotalRAMBytes     uint64 `json:"total_ram_bytes"`
	AvailableRAMBytes uint64 `json:"available_ram_bytes"`
	SwapUsedBytes     uint64 `json:"swap_used_bytes"`
	SwapLimitBytes    uint64 `json:"swap_limit_bytes"`
	Pressure          string `json:"pressure"`
	PressureSource    string `json:"pressure_source"`
	SwapMeasured      bool   `json:"swap_measured"`
}
type PrefixCachePressureReceipt struct {
	Schema            string                         `json:"schema"`
	Observation       PrefixCachePressureObservation `json:"observation"`
	ObservationSHA256 string                         `json:"observation_sha256"`
}
type PrefixCachePressureDecisionBinding struct {
	Schema            string `json:"schema"`
	Action            string `json:"action"`
	ObservationSHA256 string `json:"observation_sha256"`
}

// PrefixCachePressureAuthorizationReceipt is Pantheon's explicit owner-action
// evidence. It is not an SNE policy decision or permission to start a model.
type PrefixCachePressureAuthorizationReceipt struct {
	Schema         string `json:"schema"`
	Operation      string `json:"operation"`
	State          string `json:"state"`
	HostID         string `json:"host_id"`
	RequestID      string `json:"request_id"`
	ArtifactSHA256 string `json:"artifact_sha256"`
	ExpiresAtUnix  int64  `json:"expires_at_unix"`
}

const (
	PrefixCachePressureReceiptSchema              = "pantheon.sne-prefix-cache-pressure-receipt.v1"
	PrefixCachePressureAuthorizationReceiptSchema = "pantheon.sne-prefix-cache-pressure-authorization.v1"
	PrefixCachePressureOperation                  = "prefix-cache-pressure"
)

func NewPrefixCachePressureReceipt(host string, a ResourceAdmission, now time.Time) (PrefixCachePressureReceipt, error) {
	if strings.TrimSpace(host) == "" || now.IsZero() {
		return PrefixCachePressureReceipt{}, fmt.Errorf("host identity and observation time are required")
	}
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return PrefixCachePressureReceipt{}, fmt.Errorf("create pressure request ID: %w", err)
	}
	o := PrefixCachePressureObservation{RequestID: "pressure-" + hex.EncodeToString(raw[:]), HostID: host, ObservedAtUnix: now.UTC().Unix(), ExpiresAtUnix: now.UTC().Add(5 * time.Minute).Unix(), TotalRAMBytes: a.TotalRAMBytes, AvailableRAMBytes: a.AvailableRAMBytes, SwapUsedBytes: a.SwapUsedBytes, SwapLimitBytes: a.SwapLimitBytes, Pressure: strings.ToLower(strings.TrimSpace(a.Pressure)), PressureSource: strings.TrimSpace(a.PressureSource), SwapMeasured: a.TotalRAMBytes > 0 && a.SwapLimitBytes > 0}
	d, err := PrefixCachePressureObservationSHA256(o)
	if err != nil {
		return PrefixCachePressureReceipt{}, err
	}
	return PrefixCachePressureReceipt{Schema: PrefixCachePressureReceiptSchema, Observation: o, ObservationSHA256: d}, nil
}
func PrefixCachePressureObservationSHA256(o PrefixCachePressureObservation) (string, error) {
	if strings.TrimSpace(o.RequestID) == "" || strings.TrimSpace(o.HostID) == "" || o.ObservedAtUnix <= 0 || o.ExpiresAtUnix <= o.ObservedAtUnix || o.ExpiresAtUnix-o.ObservedAtUnix > 300 || o.TotalRAMBytes == 0 || o.AvailableRAMBytes == 0 || !o.SwapMeasured || o.SwapLimitBytes == 0 || strings.TrimSpace(o.PressureSource) == "" {
		return "", fmt.Errorf("complete measured prefix-cache pressure observation required")
	}
	b, e := json.Marshal(o)
	if e != nil {
		return "", e
	}
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:]), nil
}
func ValidatePrefixCachePressureReceiptFor(r PrefixCachePressureReceipt, host string, now time.Time) error {
	if r.Schema != PrefixCachePressureReceiptSchema || host == "" || r.Observation.HostID != host {
		return fmt.Errorf("invalid or cross-host prefix-cache pressure receipt")
	}
	if now.UTC().Unix() < r.Observation.ObservedAtUnix || now.UTC().Unix() > r.Observation.ExpiresAtUnix {
		return fmt.Errorf("prefix-cache pressure receipt is stale")
	}
	d, e := PrefixCachePressureObservationSHA256(r.Observation)
	if e != nil || d != r.ObservationSHA256 {
		return fmt.Errorf("prefix-cache pressure receipt observation hash mismatch")
	}
	return nil
}
func ValidatePrefixCachePressureDecisionBinding(r PrefixCachePressureReceipt, d PrefixCachePressureDecisionBinding, host string, now time.Time) error {
	if e := ValidatePrefixCachePressureReceiptFor(r, host, now); e != nil {
		return e
	}
	if strings.TrimSpace(d.Schema) == "" || strings.TrimSpace(d.Action) == "" || d.ObservationSHA256 != r.ObservationSHA256 {
		return fmt.Errorf("prefix-cache pressure decision is not bound to the measured observation")
	}
	return nil
}

// IssuePrefixCachePressureAuthorizationReceipt creates the only Pantheon
// authorization value SNE may consume. The caller must have completed a
// visible owner confirmation before calling this function.
func IssuePrefixCachePressureAuthorizationReceipt(receipt PrefixCachePressureReceipt, hostID string, expiresAt time.Time, now time.Time) (PrefixCachePressureAuthorizationReceipt, error) {
	if err := ValidatePrefixCachePressureReceiptFor(receipt, hostID, now); err != nil {
		return PrefixCachePressureAuthorizationReceipt{}, err
	}
	if expiresAt.IsZero() || expiresAt.UTC().Unix() > receipt.Observation.ExpiresAtUnix || expiresAt.UTC().Unix() <= now.UTC().Unix() {
		return PrefixCachePressureAuthorizationReceipt{}, fmt.Errorf("prefix-cache pressure authorization expiry is invalid")
	}
	return PrefixCachePressureAuthorizationReceipt{
		Schema:         PrefixCachePressureAuthorizationReceiptSchema,
		Operation:      PrefixCachePressureOperation,
		State:          "accepted",
		HostID:         hostID,
		RequestID:      receipt.Observation.RequestID,
		ArtifactSHA256: receipt.ObservationSHA256,
		ExpiresAtUnix:  expiresAt.UTC().Unix(),
	}, nil
}

// ValidatePrefixCachePressureAuthorizationReceiptFor fails closed before SNE
// calculates or executes any cache policy.
func ValidatePrefixCachePressureAuthorizationReceiptFor(receipt PrefixCachePressureReceipt, authorization PrefixCachePressureAuthorizationReceipt, hostID string, now time.Time) error {
	if err := ValidatePrefixCachePressureReceiptFor(receipt, hostID, now); err != nil {
		return err
	}
	if authorization.Schema != PrefixCachePressureAuthorizationReceiptSchema || authorization.Operation != PrefixCachePressureOperation || authorization.State != "accepted" || authorization.HostID != hostID || authorization.RequestID != receipt.Observation.RequestID || authorization.ArtifactSHA256 != receipt.ObservationSHA256 || authorization.ExpiresAtUnix <= now.UTC().Unix() || authorization.ExpiresAtUnix > receipt.Observation.ExpiresAtUnix {
		return fmt.Errorf("prefix-cache pressure authorization is invalid or does not bind the observation")
	}
	return nil
}
