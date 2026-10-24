package maat

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"errors"
)

// ActiveIncidentGuardID and ActiveIncidentGuardVersion name the closed local
// recurrence guard, not a host-health probe or permission to repair anything.
const (
	ActiveIncidentGuardID      = "maat.active-incident"
	ActiveIncidentGuardVersion = "v1"
)

// Bind the actual compiled-in implementation source, including the closed
// dispatch below, rather than hashing a caller's guard label. This is source
// identity, not binary provenance or authentication of imported incidents.
//
//go:embed memoryguard.go
var memoryGuardSource string

// Include the typed inputs, validation, and production preflight dispatch too.
//
//go:embed memory.go
var memoryStoreSource string

type memoryGuardInput struct {
	Action   Scope
	Incident Incident
}

type memoryGuard struct {
	identity GuardEvaluation
	evaluate func(memoryGuardInput) (CheckOutcome, error)
}

// No registration API, plugin, imported command, or mutable global registry.
func resolveMemoryGuard(id, version string) (memoryGuard, bool) {
	if id != ActiveIncidentGuardID || version != ActiveIncidentGuardVersion {
		return memoryGuard{}, false
	}
	digest := sha256.Sum256([]byte(memoryGuardSource + "\x00" + memoryStoreSource))
	return memoryGuard{
		identity: GuardEvaluation{ID: id, Version: version, SHA256: hex.EncodeToString(digest[:])},
		evaluate: evaluateActiveIncident,
	}, true
}

// evaluateActiveIncident is pure: its only inputs are the verified incident
// and exact requested action. It asserts recurrence exclusion, not that the
// original host failure still exists. Recovery text is never executed.
func evaluateActiveIncident(input memoryGuardInput) (CheckOutcome, error) {
	i := input.Incident
	if input.Action.validate() != nil || i.validate() != nil || i.Status != IncidentActive ||
		!i.Scope.matches(input.Action) || i.GuardID != ActiveIncidentGuardID || i.GuardVersion != ActiveIncidentGuardVersion {
		return CheckOutcome{}, errors.New("maat memory: invalid active-incident guard input")
	}
	return CheckOutcome{
		IncidentKey: i.Key, EvidenceSHA256: i.EvidenceSHA256,
		Status: string(i.Status), Outcome: "active-incident-recurrence-rejected",
	}, nil
}
