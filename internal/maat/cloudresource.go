package maat

import (
	"fmt"
	"strings"
)

// CloudResource identifies one cloud account under a named provider, using the
// canonical cloud:<provider>:<account> form (e.g. cloud:gcp:01ABF6-FC7DBF-2BB83A).
// It is the first generic Resource shape in this package; the existing
// preflights (ReleaseCredentialPreflight, ReleaseContractPreflight) are each
// single-purpose and do not compose with a resource identifier.
type CloudResource struct {
	Provider string
	Account  string
}

func (r CloudResource) String() string {
	return fmt.Sprintf("cloud:%s:%s", r.Provider, r.Account)
}

func (r CloudResource) validate() error {
	if strings.TrimSpace(r.Provider) == "" || strings.TrimSpace(r.Account) == "" ||
		strings.ContainsAny(r.Provider+r.Account, ":\r\n\x00") {
		return fmt.Errorf("maat: invalid cloud resource %q", r)
	}
	return nil
}

// ParseCloudResource parses the canonical cloud:<provider>:<account> form.
func ParseCloudResource(s string) (CloudResource, error) {
	parts := strings.SplitN(s, ":", 3)
	if len(parts) != 3 || parts[0] != "cloud" {
		return CloudResource{}, fmt.Errorf("maat: %q is not a cloud:<provider>:<account> resource", s)
	}
	r := CloudResource{Provider: parts[1], Account: parts[2]}
	if err := r.validate(); err != nil {
		return CloudResource{}, err
	}
	return r, nil
}

// FundingStatus is the only admission result a caller may receive for a
// CloudResource. Unknown is failure-safe: no funding evidence source is wired
// for any provider yet, so every caller today MUST treat Unknown as a block,
// never as an implicit pass — mirroring PreflightDecision's unverifiable
// floor in memory.go.
type FundingStatus string

const (
	FundingUnknown  FundingStatus = "unknown"
	FundingFunded   FundingStatus = "funded"
	FundingUnfunded FundingStatus = "unfunded"
)

// ResourceAdmission is the generic evidence record for one CloudResource. It
// deliberately holds no dispatch decision: wiring this into dispatchgate is a
// separate, reviewed change once a real evidence source exists for at least
// one provider.
type ResourceAdmission struct {
	Resource CloudResource
	Status   FundingStatus
	Reason   string
}

// FundingEvidenceSource looks up funding evidence for a CloudResource. No
// implementation exists for any provider yet (GCP billing.viewer access is
// still unresolved). Callers MUST treat a nil source, or any error from one,
// as FundingUnknown, never as FundingFunded.
type FundingEvidenceSource func(CloudResource) (FundingStatus, string, error)

// PreflightCloudAdmission is fail-closed: it cannot return FundingFunded on
// its own. Without a working FundingEvidenceSource, every resource comes back
// FundingUnknown, which callers must treat as a block, not a pass.
func PreflightCloudAdmission(res CloudResource, source FundingEvidenceSource) (ResourceAdmission, error) {
	if err := res.validate(); err != nil {
		return ResourceAdmission{}, err
	}
	if source == nil {
		return ResourceAdmission{Resource: res, Status: FundingUnknown, Reason: "no funding evidence source is wired for this provider yet"}, nil
	}
	status, reason, err := source(res)
	if err != nil {
		return ResourceAdmission{Resource: res, Status: FundingUnknown, Reason: "funding evidence source error: " + err.Error()}, nil
	}
	if status != FundingFunded && status != FundingUnfunded {
		status = FundingUnknown
	}
	return ResourceAdmission{Resource: res, Status: status, Reason: reason}, nil
}
