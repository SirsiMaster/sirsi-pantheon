package sne

import (
	"strings"
	"testing"
	"time"
)

func TestPrefixCachePressureReceiptBindsExactObservationAndDecision(t *testing.T) {
	now := time.Unix(1_788_000_000, 0).UTC()
	receipt, err := NewPrefixCachePressureReceipt("m5", ResourceAdmission{
		TotalRAMBytes: 48 << 30, AvailableRAMBytes: 20 << 30,
		SwapUsedBytes: 1 << 30, SwapLimitBytes: 3 << 30,
		Pressure: "warning", PressureSource: "host_statistics64",
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(receipt.Observation.RequestID, "pressure-") || len(receipt.ObservationSHA256) != 64 {
		t.Fatalf("receipt identity = %+v", receipt)
	}
	if err := ValidatePrefixCachePressureDecisionBinding(receipt, PrefixCachePressureDecisionBinding{
		Schema: "sne.prefix-cache.pressure-policy.v1", Action: "trim", ObservationSHA256: receipt.ObservationSHA256,
	}, "m5", now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
}

func TestPrefixCachePressureReceiptFailsClosed(t *testing.T) {
	now := time.Unix(1_788_000_000, 0).UTC()
	receipt, err := NewPrefixCachePressureReceipt("m5", ResourceAdmission{
		TotalRAMBytes: 48 << 30, AvailableRAMBytes: 20 << 30,
		SwapLimitBytes: 3 << 30, Pressure: "normal", PressureSource: "host_statistics64",
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*PrefixCachePressureReceipt){
		"cross-host": func(r *PrefixCachePressureReceipt) { r.Observation.HostID = "m1" },
		"tampered":   func(r *PrefixCachePressureReceipt) { r.Observation.AvailableRAMBytes-- },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := receipt
			mutate(&candidate)
			if err := ValidatePrefixCachePressureReceiptFor(candidate, "m5", now.Add(time.Second)); err == nil {
				t.Fatal("accepted invalid receipt")
			}
		})
	}
	if err := ValidatePrefixCachePressureReceiptFor(receipt, "m5", now.Add(301*time.Second)); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("stale receipt error = %v", err)
	}
	if err := ValidatePrefixCachePressureDecisionBinding(receipt, PrefixCachePressureDecisionBinding{
		Schema: "sne.prefix-cache.pressure-policy.v1", Action: "trim", ObservationSHA256: strings.Repeat("0", 64),
	}, "m5", now); err == nil {
		t.Fatal("accepted unbound decision")
	}
}

func TestPrefixCachePressureAuthorizationBindsOwnerActionToObservation(t *testing.T) {
	now := time.Unix(1_788_000_000, 0).UTC()
	receipt, err := NewPrefixCachePressureReceipt("m5", ResourceAdmission{
		TotalRAMBytes: 48 << 30, AvailableRAMBytes: 20 << 30,
		SwapLimitBytes: 3 << 30, Pressure: "warning", PressureSource: "host_statistics64",
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	authorization, err := IssuePrefixCachePressureAuthorizationReceipt(receipt, "m5", now.Add(time.Minute), now)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidatePrefixCachePressureAuthorizationReceiptFor(receipt, authorization, "m5", now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*PrefixCachePressureAuthorizationReceipt){
		"operation": func(value *PrefixCachePressureAuthorizationReceipt) { value.Operation = "start" },
		"host":      func(value *PrefixCachePressureAuthorizationReceipt) { value.HostID = "m1" },
		"request":   func(value *PrefixCachePressureAuthorizationReceipt) { value.RequestID = "other-request" },
		"artifact":  func(value *PrefixCachePressureAuthorizationReceipt) { value.ArtifactSHA256 = strings.Repeat("0", 64) },
		"expired":   func(value *PrefixCachePressureAuthorizationReceipt) { value.ExpiresAtUnix = now.Unix() },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := authorization
			mutate(&candidate)
			if err := ValidatePrefixCachePressureAuthorizationReceiptFor(receipt, candidate, "m5", now.Add(time.Second)); err == nil {
				t.Fatal("accepted invalid authorization")
			}
		})
	}
	if _, err := IssuePrefixCachePressureAuthorizationReceipt(receipt, "m5", now.Add(301*time.Second), now); err == nil {
		t.Fatal("issued authorization after observation expiry")
	}
}
