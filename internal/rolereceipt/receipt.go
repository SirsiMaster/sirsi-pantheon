// Package rolereceipt validates the structural and operation-binding portion
// of Pantheon's host-neutral Stack Lab role receipts. Signature verification is
// deliberately external: a receipt cannot become authority until the caller
// also verifies it against the separately governed issuer/keyring policy.
package rolereceipt

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

const Schema = "pantheon.role-receipt/v1"

type Role string

const (
	RouterAuthority   Role = "router-authority"
	EvidenceAuthority Role = "evidence-authority"
	ExecutionHost     Role = "execution-host"
	ConstrainedClient Role = "constrained-client"
)

type HostProfile struct {
	ID        string `json:"id"`
	OS        string `json:"os"`
	Toolchain string `json:"toolchain"`
	Transport string `json:"transport"`
}
type ResourceFact struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type ObservedState struct {
	RouterNamespace    string         `json:"router_namespace"`
	ProtectedProcesses []string       `json:"protected_processes"`
	ResourceFacts      []ResourceFact `json:"resource_facts"`
}

// Receipt is intentionally a closed representation. It records opaque
// signature material, but does not interpret it: only an independently
// governed trust root may perform the cryptographic authentication step.
type Receipt struct {
	Schema              string        `json:"schema"`
	ReceiptID           string        `json:"receipt_id"`
	Role                Role          `json:"role"`
	HostProfile         HostProfile   `json:"host_profile"`
	Scope               []string      `json:"scope"`
	IssuedAt            time.Time     `json:"issued_at"`
	ExpiresAt           time.Time     `json:"expires_at"`
	RevocationReference string        `json:"revocation_reference"`
	Issuer              string        `json:"issuer"`
	KeyID               string        `json:"key_id"`
	PolicyVersion       string        `json:"policy_version"`
	Predecessor         string        `json:"predecessor,omitempty"`
	Handback            string        `json:"handback,omitempty"`
	ObservedState       ObservedState `json:"observed_state"`
	Signature           string        `json:"signature"`
}

// Constraints bind a structurally valid receipt to an operation. Empty fields
// intentionally mean “do not constrain this field”; callers that need a role
// must set it explicitly rather than treating structural validity as authority.
type Constraints struct {
	Now           time.Time
	Role          Role
	HostID        string
	Issuer        string
	KeyID         string
	RequiredScope []string
}

func Parse(body []byte) (Receipt, error) {
	if len(body) == 0 {
		return Receipt{}, errors.New("role receipt is empty")
	}
	if err := validateJSONNoDuplicateKeys(body); err != nil {
		return Receipt{}, fmt.Errorf("role receipt JSON is ambiguous: %w", err)
	}
	var receipt Receipt
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&receipt); err != nil {
		return Receipt{}, fmt.Errorf("decode role receipt: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return Receipt{}, errors.New("role receipt contains multiple JSON values")
		}
		return Receipt{}, fmt.Errorf("role receipt trailing JSON: %w", err)
	}
	if err := receipt.ValidateStructural(); err != nil {
		return Receipt{}, err
	}
	return receipt, nil
}

// validateJSONNoDuplicateKeys is kept local so the role-receipt package can
// be consumed by routerboard without an import cycle. Structural parsing and
// duplicate-key rejection remain part of the same closed receipt boundary.
func validateJSONNoDuplicateKeys(body []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := walkJSONValue(decoder); err != nil {
		return fmt.Errorf("invalid role receipt JSON: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return errors.New("role receipt contains multiple JSON values")
		}
		return fmt.Errorf("role receipt trailing JSON: %w", err)
	}
	return nil
}

func walkJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, isDelim := token.(json.Delim)
	if !isDelim {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]struct{}{}
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok {
				return errors.New("role receipt object key is not a string")
			}
			if _, exists := seen[name]; exists {
				return fmt.Errorf("role receipt duplicate object key %q", name)
			}
			seen[name] = struct{}{}
			if err := walkJSONValue(decoder); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil {
			return err
		}
		if end != json.Delim('}') {
			return errors.New("role receipt object did not terminate")
		}
	case '[':
		for decoder.More() {
			if err := walkJSONValue(decoder); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil {
			return err
		}
		if end != json.Delim(']') {
			return errors.New("role receipt array did not terminate")
		}
	default:
		return fmt.Errorf("role receipt unexpected delimiter %q", delim)
	}
	return nil
}

func (r Receipt) ValidateStructural() error {
	if r.Schema != Schema {
		return fmt.Errorf("role receipt schema %q is unsupported", r.Schema)
	}
	for field, value := range map[string]string{
		"receipt_id":                      r.ReceiptID,
		"host_profile.id":                 r.HostProfile.ID,
		"host_profile.os":                 r.HostProfile.OS,
		"host_profile.toolchain":          r.HostProfile.Toolchain,
		"host_profile.transport":          r.HostProfile.Transport,
		"revocation_reference":            r.RevocationReference,
		"issuer":                          r.Issuer,
		"key_id":                          r.KeyID,
		"policy_version":                  r.PolicyVersion,
		"signature":                       r.Signature,
		"observed_state.router_namespace": r.ObservedState.RouterNamespace,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("role receipt %s is required", field)
		}
	}
	if !validRole(r.Role) {
		return fmt.Errorf("role receipt role %q is unsupported", r.Role)
	}
	if r.IssuedAt.IsZero() || r.ExpiresAt.IsZero() {
		return errors.New("role receipt issued_at and expires_at are required")
	}
	if !r.ExpiresAt.After(r.IssuedAt) {
		return errors.New("role receipt expires_at must be after issued_at")
	}
	if (strings.TrimSpace(r.Predecessor) == "") != (strings.TrimSpace(r.Handback) == "") {
		return errors.New("role receipt predecessor and handback must be present together")
	}
	if err := requireUnique("scope", r.Scope); err != nil {
		return err
	}
	if len(r.Scope) == 0 {
		return errors.New("role receipt scope is required")
	}
	if err := requireUnique("observed_state.protected_processes", r.ObservedState.ProtectedProcesses); err != nil {
		return err
	}
	if err := validateResourceFacts(r.ObservedState.ResourceFacts); err != nil {
		return err
	}
	return nil
}

func (r Receipt) ValidateFor(constraints Constraints) error {
	if err := r.ValidateStructural(); err != nil {
		return err
	}
	now := constraints.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if !now.Before(r.ExpiresAt) {
		return fmt.Errorf("role receipt %q expired at %s", r.ReceiptID, r.ExpiresAt.UTC().Format(time.RFC3339Nano))
	}
	if now.Before(r.IssuedAt) {
		return fmt.Errorf("role receipt %q is not valid before %s", r.ReceiptID, r.IssuedAt.UTC().Format(time.RFC3339Nano))
	}
	if constraints.Role != "" && r.Role != constraints.Role {
		return fmt.Errorf("role receipt role %q does not match required role %q", r.Role, constraints.Role)
	}
	if want := strings.TrimSpace(constraints.HostID); want != "" && r.HostProfile.ID != want {
		return fmt.Errorf("role receipt host %q does not match required host %q", r.HostProfile.ID, want)
	}
	if want := strings.TrimSpace(constraints.Issuer); want != "" && r.Issuer != want {
		return fmt.Errorf("role receipt issuer %q does not match required issuer %q", r.Issuer, want)
	}
	if want := strings.TrimSpace(constraints.KeyID); want != "" && r.KeyID != want {
		return fmt.Errorf("role receipt key_id %q does not match required key_id %q", r.KeyID, want)
	}
	actual := make(map[string]struct{}, len(r.Scope))
	for _, scope := range r.Scope {
		actual[scope] = struct{}{}
	}
	for _, required := range constraints.RequiredScope {
		required = strings.TrimSpace(required)
		if required == "" {
			return errors.New("required scope contains an empty entry")
		}
		if _, ok := actual[required]; !ok {
			return fmt.Errorf("role receipt scope does not authorize %q", required)
		}
	}
	return nil
}

func validRole(role Role) bool {
	switch role {
	case RouterAuthority, EvidenceAuthority, ExecutionHost, ConstrainedClient:
		return true
	default:
		return false
	}
}

func requireUnique(label string, values []string) error {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			return fmt.Errorf("role receipt %s contains an empty entry", label)
		}
		if _, duplicate := seen[value]; duplicate {
			return fmt.Errorf("role receipt %s contains duplicate %q", label, value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func validateResourceFacts(facts []ResourceFact) error {
	if len(facts) == 0 {
		return errors.New("role receipt observed_state.resource_facts is required")
	}
	seen := make(map[string]struct{}, len(facts))
	for _, fact := range facts {
		fact.Name = strings.TrimSpace(fact.Name)
		if fact.Name == "" || strings.TrimSpace(fact.Value) == "" {
			return errors.New("role receipt resource fact name and value are required")
		}
		if _, duplicate := seen[fact.Name]; duplicate {
			return fmt.Errorf("role receipt resource facts contain duplicate %q", fact.Name)
		}
		seen[fact.Name] = struct{}{}
	}
	return nil
}

// CanonicalScope returns a sorted copy suitable for deterministic receipt
// construction. It does not validate authority or produce signature bytes.
func CanonicalScope(scope []string) []string {
	out := append([]string(nil), scope...)
	for index := range out {
		out[index] = strings.TrimSpace(out[index])
	}
	sort.Strings(out)
	return out
}
