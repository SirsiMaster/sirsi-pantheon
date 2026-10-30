package maat

import (
	"bytes"
	"context"
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"
)

const LifecycleSchema = "maat.source-lifecycle-receipt.v1"

// Installed acceptance is an offline release input. Empty evidence classes are
// intentional until a reviewed release accepts real, profile-bound operands.
// Inert package fixtures must never be copied into these inventories.
//
//go:embed qualificationacceptance-v1.json
var lifecycleAcceptanceBytes embed.FS

// Profile identity is reusable across source heads, but never across a change
// to the model artifact/version, prompt, evidence recipe or reviewed policy.
type SourceProfileIdentity struct {
	Producer     string `json:"producer"`
	ArtifactSHA  string `json:"artifact_sha256"`
	ModelVersion string `json:"model_version"`
	PromptSHA    string `json:"prompt_sha256"`
	RecipeSHA    string `json:"recipe_sha256"`
	PolicySHA    string `json:"policy_sha256"`
}

type QualificationPolicy struct {
	ID                string  `json:"id"`
	Threshold         float64 `json:"threshold"`
	BoundaryThreshold float64 `json:"boundary_threshold"`
	MinimumPass       int     `json:"minimum_pass_samples"`
	MinimumChanges    int     `json:"minimum_changes_samples"`
}

type LifecycleSample struct {
	ID         string       `json:"id"`
	Screen     SourceScreen `json:"screen"`
	OutcomeRef string       `json:"outcome_ref"`
}

// All keys are required, including empty arrays/strings for inapplicable fields.
// The object is evidence, not a qualification request or a model invocation.
type LifecycleReceipt struct {
	Schema        string                `json:"schema"`
	Kind          string                `json:"kind"`
	Profile       SourceProfileIdentity `json:"profile"`
	Repo          string                `json:"repo"`
	HeadSHA       string                `json:"head_sha"`
	PolicyRef     string                `json:"policy_ref"`
	Policy        QualificationPolicy   `json:"policy"`
	DatasetSHA    string                `json:"dataset_sha256"`
	ResultsSHA    string                `json:"results_sha256"`
	RunID         string                `json:"run_id"`
	Samples       []LifecycleSample     `json:"samples"`
	SelectedIDs   []string              `json:"selected_ids"`
	ScreenSHA     string                `json:"screen_sha256"`
	FinalGate     GateDecision          `json:"final_gate"`
	CandidateGate GateDecision          `json:"candidate_gate"`
}

func lifecycleDigest(v any) string {
	raw, _ := json.Marshal(v)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func (p SourceProfileIdentity) ID() string { return lifecycleDigest(p) }

func lifecycleHead(s string) bool {
	if len(s) != 40 || strings.ToLower(s) != s {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

func DecodeLifecycleReceipt(raw []byte) (LifecycleReceipt, error) {
	var receipt LifecycleReceipt
	if len(raw) > 256*1024 {
		return receipt, fmt.Errorf("lifecycle receipt exceeds 256 KiB")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	if err := sourceJSONValue(d, reflect.TypeOf(receipt), 0); err != nil {
		return receipt, err
	}
	if _, err := d.Token(); err != io.EOF {
		return receipt, fmt.Errorf("lifecycle receipt has trailing input")
	}
	if err := json.Unmarshal(raw, &receipt); err != nil {
		return receipt, err
	}
	return receipt, receipt.validate()
}

func (r LifecycleReceipt) validate() error {
	if r.Schema != LifecycleSchema || !oneOf(r.Kind, "policy", "evaluation", "outcome") ||
		strings.TrimSpace(r.Repo) == "" || !lifecycleHead(r.HeadSHA) || strings.TrimSpace(r.RunID) == "" ||
		strings.TrimSpace(r.Profile.Producer) == "" || strings.TrimSpace(r.Profile.ModelVersion) == "" {
		return fmt.Errorf("lifecycle schema/kind/subject/run/profile is missing or invalid")
	}
	for _, digest := range []string{r.Profile.ArtifactSHA, r.Profile.PromptSHA, r.Profile.RecipeSHA, r.Profile.PolicySHA} {
		if !sourceDigest(digest) {
			return fmt.Errorf("lifecycle profile digest required")
		}
	}
	if r.Policy.ID == "" || r.Policy.MinimumPass < 1 || r.Policy.MinimumChanges < 1 ||
		r.Policy.MinimumPass > 10000 || r.Policy.MinimumChanges > 10000 ||
		r.Policy.Threshold < .90 || r.Policy.Threshold >= 1 || r.Policy.BoundaryThreshold < .98 || r.Policy.BoundaryThreshold > 1 || r.Policy.BoundaryThreshold < r.Policy.Threshold {
		return fmt.Errorf("explicit nonempty policy and finite supporting populations required")
	}
	if err := validateConfidence("policy", r.Policy.Threshold); err != nil {
		return err
	}
	if err := validateConfidence("boundary policy", r.Policy.BoundaryThreshold); err != nil {
		return err
	}
	if lifecycleDigest(r.Policy) != r.Profile.PolicySHA {
		return fmt.Errorf("policy/profile digest mismatch")
	}
	if len(r.Samples) > 512 || len(r.SelectedIDs) > 512 {
		return fmt.Errorf("lifecycle population exceeds 512")
	}
	if r.Kind == "evaluation" {
		if r.PolicyRef == "" || !sourceDigest(r.DatasetSHA) || !sourceDigest(r.ResultsSHA) || len(r.Samples) == 0 || len(r.SelectedIDs) == 0 {
			return fmt.Errorf("evaluation requires pinned dataset/results, policy and selected population")
		}
		seen := map[string]bool{}
		screens := map[string]bool{}
		for _, sample := range r.Samples {
			if sample.ID == "" || seen[sample.ID] || sample.Screen.Subject.Repo != r.Repo || !lifecycleHead(sample.Screen.Subject.HeadSHA) || sample.Screen.Model.Version != r.Profile.ModelVersion {
				return fmt.Errorf("duplicate or mismatched evaluation sample")
			}
			if _, err := ScreenSource(sample.Screen, sample.Screen.Subject.HeadSHA); err != nil {
				return err
			}
			key := lifecycleDigest(sample.Screen)
			if screens[key] {
				return fmt.Errorf("duplicate evaluation screen")
			}
			screens[key] = true
			seen[sample.ID] = true
		}
		selected := map[string]bool{}
		for _, id := range r.SelectedIDs {
			if !seen[id] || selected[id] {
				return fmt.Errorf("unknown or duplicate selected sample")
			}
			selected[id] = true
		}
		if lifecycleDigest(r.Samples) != r.ResultsSHA {
			return fmt.Errorf("actual result-set digest mismatch")
		}
		identities := make([]string, 0, len(r.Samples))
		for _, sample := range r.Samples {
			identities = append(identities, sample.ID+":"+sample.Screen.Subject.Repo+":"+sample.Screen.Subject.HeadSHA)
		}
		if lifecycleDigest(identities) != r.DatasetSHA {
			return fmt.Errorf("dataset inventory digest mismatch")
		}
	} else if len(r.Samples) != 0 || len(r.SelectedIDs) != 0 {
		return fmt.Errorf("non-evaluation receipt contains a population")
	}
	if r.Kind == "outcome" && (!sourceDigest(r.ScreenSHA) || !oneOf(string(r.FinalGate), "pass", "changes", "block") || !oneOf(string(r.CandidateGate), "pass", "changes")) {
		return fmt.Errorf("outcome must bind exact screen digest and final gate")
	}
	return nil
}

// Acceptance is deliberately not serialized or caller-constructible. The
// canonical Get adapter supplies exact custody only; the installed verifier
// or replay-checked owner-adoption journal is the only production setter.
type LifecycleEvidence struct {
	ID, SHA, Issuer, Reason string
	Receipt                 LifecycleReceipt
	accepted                bool
}

// LifecycleRawRecord is an exact canonical custody operand. Acceptance code
// always hashes and decodes RawInstructions itself; caller-supplied parsed
// receipts or acceptance booleans are not inputs to the verifier.
type LifecycleRawRecord struct {
	ID, Issuer      string
	RawInstructions []byte
}

// InstalledEngineAuthority is an opaque proof minted only after the current
// canonical packaged engine passes the existing platform signature check.
// Its zero value never authorizes acceptance.
type InstalledEngineAuthority struct{ enginePath string }

func (a InstalledEngineAuthority) Valid() bool { return a.enginePath != "" }

type lifecycleAcceptanceInventory struct {
	Schema     string                   `json:"schema"`
	Policy     []lifecycleAcceptancePin `json:"policy"`
	Evaluation []lifecycleAcceptancePin `json:"actual_evaluation"`
	Outcome    []lifecycleAcceptancePin `json:"actual_independent_outcome"`
}

type lifecycleAcceptancePin struct {
	ReceiptID           string `json:"receipt_id"`
	ReceiptSHA          string `json:"receipt_sha256"`
	Kind                string `json:"kind"`
	Repo                string `json:"repo"`
	HeadSHA             string `json:"head_sha"`
	ProfileID           string `json:"profile_id"`
	PolicySHA           string `json:"policy_sha256"`
	DatasetSHA          string `json:"dataset_sha256"`
	ResultsSHA          string `json:"results_sha256"`
	ScreenSHA           string `json:"screen_sha256"`
	OriginReceiptID     string `json:"origin_receipt_id"`
	OriginReceiptSHA    string `json:"origin_receipt_sha256"`
	IssuerClaim         string `json:"issuer_claim"`
	ReceiptInstructions string `json:"receipt_instructions_base64"`
}

func (inventory lifecycleAcceptanceInventory) find(evidence LifecycleEvidence) (lifecycleAcceptancePin, bool) {
	classes := map[string][]lifecycleAcceptancePin{
		"policy":     inventory.Policy,
		"evaluation": inventory.Evaluation,
		"outcome":    inventory.Outcome,
	}
	for _, pin := range classes[evidence.Receipt.Kind] {
		r := evidence.Receipt
		if pin.ReceiptID == evidence.ID && pin.ReceiptSHA == evidence.SHA && pin.Kind == r.Kind && pin.IssuerClaim == evidence.Issuer &&
			pin.Repo == r.Repo && pin.HeadSHA == r.HeadSHA && pin.ProfileID == r.Profile.ID() &&
			pin.PolicySHA == r.Profile.PolicySHA && pin.DatasetSHA == r.DatasetSHA &&
			pin.ResultsSHA == r.ResultsSHA && pin.ScreenSHA == r.ScreenSHA {
			return pin, true
		}
	}
	return lifecycleAcceptancePin{}, false
}

func (inventory lifecycleAcceptanceInventory) validate() error {
	if inventory.Schema != "maat.lifecycle-installed-acceptance.v1" || inventory.Policy == nil || inventory.Evaluation == nil || inventory.Outcome == nil {
		return fmt.Errorf("invalid installed lifecycle acceptance inventory envelope")
	}
	classes := []struct {
		kind string
		pins []lifecycleAcceptancePin
	}{
		{"policy", inventory.Policy}, {"evaluation", inventory.Evaluation}, {"outcome", inventory.Outcome},
	}
	byID := make(map[string]LifecycleEvidence)
	for _, class := range classes {
		for _, pin := range class.pins {
			if pin.Kind != class.kind || pin.ReceiptID == "" || pin.IssuerClaim == "" ||
				!sourceDigest(pin.ReceiptSHA) || !sourceDigest(pin.ProfileID) || !sourceDigest(pin.PolicySHA) ||
				!lifecycleHead(pin.HeadSHA) || strings.TrimSpace(pin.Repo) == "" {
				return fmt.Errorf("invalid installed %s evidence pin", class.kind)
			}
			raw, err := base64.StdEncoding.DecodeString(pin.ReceiptInstructions)
			if err != nil || len(raw) == 0 || len(raw) > 256*1024 || lifecycleRawSHA(raw) != pin.ReceiptSHA {
				return fmt.Errorf("installed %s raw receipt pin digest mismatch", class.kind)
			}
			evidence, err := lifecycleEvidenceFromRaw(LifecycleRawRecord{ID: pin.ReceiptID, Issuer: pin.IssuerClaim, RawInstructions: raw})
			if err != nil || evidence.Receipt.Kind != class.kind || evidence.Receipt.Repo != pin.Repo || evidence.Receipt.HeadSHA != pin.HeadSHA ||
				evidence.Receipt.Profile.ID() != pin.ProfileID || evidence.Receipt.Profile.PolicySHA != pin.PolicySHA ||
				evidence.Receipt.DatasetSHA != pin.DatasetSHA || evidence.Receipt.ResultsSHA != pin.ResultsSHA || evidence.Receipt.ScreenSHA != pin.ScreenSHA {
				return fmt.Errorf("installed %s pin does not match its exact receipt", class.kind)
			}
			if class.kind == "outcome" && pin.IssuerClaim == evidence.Receipt.Profile.Producer {
				return fmt.Errorf("installed outcome reviewer attribution is not independent")
			}
			if _, exists := byID[pin.ReceiptID]; exists {
				return fmt.Errorf("duplicate installed lifecycle receipt ID")
			}
			byID[pin.ReceiptID] = evidence
		}
	}
	for _, pin := range inventory.Evaluation {
		evaluation := byID[pin.ReceiptID].Receipt
		policy, ok := byID[evaluation.PolicyRef]
		if !ok || policy.Receipt.Kind != "policy" || policy.Receipt.Profile != evaluation.Profile || policy.Receipt.Policy != evaluation.Policy || pin.OriginReceiptID != "" || pin.OriginReceiptSHA != "" {
			return fmt.Errorf("installed evaluation lacks its exact accepted policy dependency")
		}
	}
	for _, pin := range inventory.Outcome {
		outcome := byID[pin.ReceiptID]
		origin, ok := byID[pin.OriginReceiptID]
		if !ok || !sourceDigest(pin.OriginReceiptSHA) || origin.SHA != pin.OriginReceiptSHA || origin.Receipt.Kind != "evaluation" ||
			origin.Receipt.Profile != outcome.Receipt.Profile || origin.Receipt.Repo != outcome.Receipt.Repo ||
			!lifecycleOriginContainsOutcome(origin.Receipt, outcome) {
			return fmt.Errorf("installed outcome lacks its exact originating evaluation dependency")
		}
	}
	for _, pin := range inventory.Policy {
		if pin.OriginReceiptID != "" || pin.OriginReceiptSHA != "" {
			return fmt.Errorf("policy pin cannot declare an origin receipt")
		}
	}
	return nil
}

func lifecycleEvidenceFromRaw(record LifecycleRawRecord) (LifecycleEvidence, error) {
	var evidence LifecycleEvidence
	if record.ID == "" || len(record.ID) > 512 || strings.TrimSpace(record.Issuer) == "" || len(record.Issuer) > 512 || len(record.RawInstructions) == 0 {
		return evidence, fmt.Errorf("exact raw lifecycle custody record required")
	}
	receipt, err := DecodeLifecycleReceipt(record.RawInstructions)
	if err != nil {
		return evidence, err
	}
	sum := sha256.Sum256(record.RawInstructions)
	return LifecycleEvidence{ID: record.ID, SHA: hex.EncodeToString(sum[:]), Issuer: record.Issuer,
		Receipt: receipt, Reason: "Canonical raw custody only; installed acceptance was not established."}, nil
}

// DecodeLifecycleCustodyRecord validates exact bytes but never establishes
// acceptance. It is the safe fallback when the canonical installed-engine
// signature boundary is unavailable.
func DecodeLifecycleCustodyRecord(record LifecycleRawRecord) (LifecycleEvidence, error) {
	return lifecycleEvidenceFromRaw(record)
}

func readLifecycleAcceptanceInventory() (lifecycleAcceptanceInventory, error) {
	var inventory lifecycleAcceptanceInventory
	raw, err := lifecycleAcceptanceBytes.ReadFile("qualificationacceptance-v1.json")
	if err != nil {
		return inventory, fmt.Errorf("embedded lifecycle acceptance inventory is unavailable")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err := sourceJSONValue(decoder, reflect.TypeOf(inventory), 0); err != nil {
		return inventory, fmt.Errorf("embedded lifecycle acceptance inventory is malformed")
	}
	if _, err := decoder.Token(); err != io.EOF {
		return inventory, fmt.Errorf("embedded lifecycle acceptance inventory has trailing input")
	}
	if err := json.Unmarshal(raw, &inventory); err != nil {
		return inventory, fmt.Errorf("embedded lifecycle acceptance inventory is malformed")
	}
	if err := inventory.validate(); err != nil {
		return inventory, fmt.Errorf("embedded lifecycle acceptance inventory is malformed: %w", err)
	}
	return inventory, nil
}

// InstalledLifecycleRawRecord returns the exact retained bytes of an embedded
// receipt for offline replay. Its caller must separately establish the signed
// canonical-engine boundary before using these bytes as acceptance evidence.
func InstalledLifecycleRawRecord(id string) (LifecycleRawRecord, bool) {
	if id == "" || len(id) > 512 {
		return LifecycleRawRecord{}, false
	}
	inventory, err := readLifecycleAcceptanceInventory()
	if err != nil {
		return LifecycleRawRecord{}, false
	}
	for _, pins := range [][]lifecycleAcceptancePin{inventory.Policy, inventory.Evaluation, inventory.Outcome} {
		for _, pin := range pins {
			if pin.ReceiptID != id {
				continue
			}
			raw, err := base64.StdEncoding.DecodeString(pin.ReceiptInstructions)
			if err != nil {
				return LifecycleRawRecord{}, false
			}
			return LifecycleRawRecord{ID: id, Issuer: pin.IssuerClaim, RawInstructions: raw}, true
		}
	}
	return LifecycleRawRecord{}, false
}

// VerifyInstalledLifecycleRecord applies the closed, embedded release
// inventory to exact canonical bytes. The current inventory contains no real
// empirical evaluation or independent outcome pins, so no model profile can
// become qualified from this adapter yet.
func VerifyInstalledLifecycleRecord(ctx context.Context, authority InstalledEngineAuthority, record LifecycleRawRecord, resolver LifecycleRawResolver) (LifecycleEvidence, error) {
	if !authority.Valid() {
		return LifecycleEvidence{}, fmt.Errorf("verified canonical installed-engine authority required")
	}
	inventory, err := readLifecycleAcceptanceInventory()
	if err != nil {
		return LifecycleEvidence{}, fmt.Errorf("embedded lifecycle acceptance inventory is unavailable; acceptance remains unavailable")
	}
	return verifyInstalledLifecycleRecordWithInventory(ctx, authority, record, resolver, inventory)
}

// verifyInstalledLifecycleRecordWithInventory is the bounded test seam for
// exercising the production pin/dependency verifier without changing the
// release-embedded empirical inventory. Callers still cannot construct the
// authority accepted by VerifyInstalledLifecycleRecord.
func verifyInstalledLifecycleRecordWithInventory(ctx context.Context, authority InstalledEngineAuthority, record LifecycleRawRecord,
	resolver LifecycleRawResolver, inventory lifecycleAcceptanceInventory) (LifecycleEvidence, error) {
	if !authority.Valid() {
		return LifecycleEvidence{}, fmt.Errorf("verified canonical installed-engine authority required")
	}
	evidence, err := lifecycleEvidenceFromRaw(record)
	if err != nil {
		return LifecycleEvidence{}, err
	}
	if err := inventory.validate(); err != nil {
		return LifecycleEvidence{}, fmt.Errorf("installed lifecycle acceptance inventory is malformed: %w", err)
	}
	pin, accepted := inventory.find(evidence)
	if accepted && evidence.Receipt.Kind == "outcome" {
		if resolver == nil || pin.OriginReceiptID == "" || !sourceDigest(pin.OriginReceiptSHA) {
			accepted = false
		} else {
			originRecord, resolveErr := resolver.ResolveLifecycleRaw(ctx, pin.OriginReceiptID)
			origin, decodeErr := lifecycleEvidenceFromRaw(originRecord)
			accepted = resolveErr == nil && decodeErr == nil && origin.ID == pin.OriginReceiptID && origin.SHA == pin.OriginReceiptSHA &&
				origin.Receipt.Kind == "evaluation" && origin.Receipt.Profile == evidence.Receipt.Profile &&
				origin.Receipt.Repo == evidence.Receipt.Repo &&
				lifecycleOriginContainsOutcome(origin.Receipt, evidence)
		}
	}
	if accepted {
		evidence.accepted = true
		evidence.Reason = "Exact receipt and dependency bytes match closed reviewed installed acceptance pins; this does not authorize a gate."
	} else {
		evidence.Reason = "Exact receipt bytes are not in the installed accepted-evidence inventory; model profile remains unqualified."
	}
	return evidence, nil
}

func lifecycleOriginContainsOutcome(origin LifecycleReceipt, outcome LifecycleEvidence) bool {
	selected := make(map[string]bool, len(origin.SelectedIDs))
	for _, id := range origin.SelectedIDs {
		selected[id] = true
	}
	for _, sample := range origin.Samples {
		if selected[sample.ID] && sample.OutcomeRef == outcome.ID && sample.Screen.Subject.Repo == outcome.Receipt.Repo &&
			sample.Screen.Subject.HeadSHA == outcome.Receipt.HeadSHA && lifecycleDigest(sample.Screen) == outcome.Receipt.ScreenSHA &&
			shadowSourceGate(sample.Screen, origin.Policy.Threshold) == outcome.Receipt.CandidateGate {
			return true
		}
	}
	return false
}

type LifecycleResolver interface {
	ResolveLifecycle(context.Context, string) (LifecycleEvidence, error)
}

type QualificationEvent struct {
	Schema     string                `json:"schema"`
	ReceiptID  string                `json:"receipt_id"`
	ReceiptSHA string                `json:"receipt_sha256"`
	Profile    SourceProfileIdentity `json:"profile"`
	HeadSHA    string                `json:"head_sha"`
}

// LifecycleOwnerAdoption is an exact-byte owner adoption of a retained
// independent review and its originating evaluation receipt. Reviewer is the
// canonical From attribution shown to the owner, not authenticated identity.
type LifecycleOwnerAdoption struct {
	Schema             string                `json:"schema"`
	ReviewID           string                `json:"review_id"`
	ReviewSHA          string                `json:"review_sha256"`
	OriginID           string                `json:"origin_receipt_id"`
	OriginSHA          string                `json:"origin_receipt_sha256"`
	Repo               string                `json:"repo"`
	HeadSHA            string                `json:"head_sha"`
	Profile            SourceProfileIdentity `json:"profile"`
	ScreenSHA          string                `json:"screen_sha256"`
	CandidateGate      GateDecision          `json:"candidate_gate"`
	FinalGate          GateDecision          `json:"final_gate"`
	Producer           string                `json:"producer"`
	ReviewerClaim      string                `json:"reviewer_claim"`
	OriginIssuerClaim  string                `json:"origin_issuer_claim"`
	ReviewInstructions string                `json:"review_instructions_base64"`
	OriginInstructions string                `json:"origin_instructions_base64"`
}

func (a LifecycleOwnerAdoption) validate() error {
	if a.Schema != "maat.source-lifecycle-owner-adoption.v1" || a.ReviewID == "" || len(a.ReviewID) > 512 ||
		a.OriginID == "" || len(a.OriginID) > 512 || !sourceDigest(a.ReviewSHA) || !sourceDigest(a.OriginSHA) ||
		strings.TrimSpace(a.Repo) == "" || !lifecycleHead(a.HeadSHA) || !sourceDigest(a.ScreenSHA) ||
		!oneOf(string(a.CandidateGate), "pass", "changes") || !oneOf(string(a.FinalGate), "pass", "changes", "block") ||
		a.Producer == "" || a.Profile.ModelVersion == "" || strings.TrimSpace(a.ReviewerClaim) == "" ||
		a.ReviewerClaim != strings.TrimSpace(a.ReviewerClaim) || a.ReviewerClaim == a.Producer || strings.TrimSpace(a.OriginIssuerClaim) == "" ||
		a.OriginIssuerClaim != strings.TrimSpace(a.OriginIssuerClaim) {
		return fmt.Errorf("invalid owner adoption identity or non-independent reviewer attribution")
	}
	for _, digest := range []string{a.Profile.ArtifactSHA, a.Profile.PromptSHA, a.Profile.RecipeSHA, a.Profile.PolicySHA} {
		if !sourceDigest(digest) {
			return fmt.Errorf("invalid owner adoption profile")
		}
	}
	reviewRaw, err := base64.StdEncoding.DecodeString(a.ReviewInstructions)
	if err != nil {
		return fmt.Errorf("owner adoption review bytes are not valid base64")
	}
	originRaw, err := base64.StdEncoding.DecodeString(a.OriginInstructions)
	if err != nil || len(reviewRaw)+len(originRaw) > 96*1024 {
		return fmt.Errorf("owner adoption raw receipts exceed the bounded journal envelope")
	}
	if lifecycleRawSHA(reviewRaw) != a.ReviewSHA || lifecycleRawSHA(originRaw) != a.OriginSHA {
		return fmt.Errorf("owner adoption raw receipt digest mismatch")
	}
	review, err := DecodeLifecycleReceipt(reviewRaw)
	if err != nil || review.Kind != "outcome" || review.Profile != a.Profile || review.Repo != a.Repo || review.HeadSHA != a.HeadSHA ||
		review.ScreenSHA != a.ScreenSHA || review.CandidateGate != a.CandidateGate || review.FinalGate != a.FinalGate {
		return fmt.Errorf("owner adoption review bytes do not match the adopted subject")
	}
	origin, err := DecodeLifecycleReceipt(originRaw)
	if err != nil || origin.Kind != "evaluation" || origin.Profile != a.Profile || origin.Repo != a.Repo {
		return fmt.Errorf("owner adoption origin bytes do not match the adopted evaluation")
	}
	if !lifecycleOriginContainsOutcome(origin, LifecycleEvidence{ID: a.ReviewID, Receipt: review}) {
		return fmt.Errorf("owner adoption is not bound to the exact selected evaluation sample")
	}
	return nil
}

func lifecycleRawSHA(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

type lifecycleAdoptedResolver struct {
	base     LifecycleResolver
	byReview map[string]LifecycleOwnerAdoption
	byOrigin map[string]LifecycleOwnerAdoption
}

func withLifecycleAdoptions(base LifecycleResolver, adoptions []LifecycleOwnerAdoption) LifecycleResolver {
	byReview := make(map[string]LifecycleOwnerAdoption, len(adoptions))
	byOrigin := make(map[string]LifecycleOwnerAdoption, len(adoptions))
	for _, adoption := range adoptions {
		byReview[adoption.ReviewID] = adoption
		byOrigin[adoption.OriginID] = adoption
	}
	return lifecycleAdoptedResolver{base: base, byReview: byReview, byOrigin: byOrigin}
}

func (r lifecycleAdoptedResolver) ResolveLifecycle(ctx context.Context, id string) (LifecycleEvidence, error) {
	if adoption, ok := r.byOrigin[id]; ok {
		if err := adoption.validate(); err != nil {
			return LifecycleEvidence{}, err
		}
		if r.base != nil {
			if installed, err := r.base.ResolveLifecycle(ctx, id); err == nil && installed.ID == id && installed.SHA == adoption.OriginSHA && installed.accepted {
				return installed, nil
			}
		}
		raw, _ := base64.StdEncoding.DecodeString(adoption.OriginInstructions)
		return lifecycleEvidenceFromRaw(LifecycleRawRecord{ID: adoption.OriginID, Issuer: adoption.OriginIssuerClaim, RawInstructions: raw})
	}
	a, ok := r.byReview[id]
	if !ok {
		return r.base.ResolveLifecycle(ctx, id)
	}
	if err := a.validate(); err != nil {
		return LifecycleEvidence{}, err
	}
	reviewRaw, _ := base64.StdEncoding.DecodeString(a.ReviewInstructions)
	originRaw, _ := base64.StdEncoding.DecodeString(a.OriginInstructions)
	evidence, err := lifecycleEvidenceFromRaw(LifecycleRawRecord{ID: a.ReviewID, Issuer: a.ReviewerClaim, RawInstructions: reviewRaw})
	if err != nil || evidence.ID != a.ReviewID || evidence.SHA != a.ReviewSHA || evidence.Receipt.Kind != "outcome" ||
		evidence.Receipt.Profile != a.Profile || evidence.Receipt.Repo != a.Repo || evidence.Receipt.HeadSHA != a.HeadSHA ||
		evidence.Receipt.ScreenSHA != a.ScreenSHA || evidence.Receipt.CandidateGate != a.CandidateGate ||
		evidence.Receipt.FinalGate != a.FinalGate || evidence.Receipt.Profile.Producer != a.Producer || evidence.Issuer != a.ReviewerClaim {
		return LifecycleEvidence{}, fmt.Errorf("owner-adopted review bytes or subject changed")
	}
	origin, err := lifecycleEvidenceFromRaw(LifecycleRawRecord{ID: a.OriginID, Issuer: a.OriginIssuerClaim, RawInstructions: originRaw})
	if err != nil || origin.ID != a.OriginID || origin.SHA != a.OriginSHA || origin.Receipt.Kind != "evaluation" ||
		origin.Receipt.Profile != a.Profile || origin.Receipt.Repo != a.Repo {
		return LifecycleEvidence{}, fmt.Errorf("owner-adopted originating evaluation changed or mismatched")
	}
	if !lifecycleOriginContainsOutcome(origin.Receipt, evidence) {
		return LifecycleEvidence{}, fmt.Errorf("owner-adopted review is not bound to the exact originating evaluation sample")
	}
	evidence.accepted = true
	evidence.Reason = "Owner adopted exact retained independent-review bytes and originating evaluation reference; From remains an unauthenticated attribution claim."
	return evidence, nil
}

func (e QualificationEvent) validate() error {
	if e.Schema != LifecycleSchema || e.ReceiptID == "" || len(e.ReceiptID) > 512 || !sourceDigest(e.ReceiptSHA) || !lifecycleHead(e.HeadSHA) || strings.TrimSpace(e.Profile.Producer) == "" || strings.TrimSpace(e.Profile.ModelVersion) == "" {
		return fmt.Errorf("invalid lifecycle event identity")
	}
	for _, s := range []string{e.Profile.ArtifactSHA, e.Profile.PromptSHA, e.Profile.RecipeSHA, e.Profile.PolicySHA} {
		if !sourceDigest(s) {
			return fmt.Errorf("invalid lifecycle event profile")
		}
	}
	return nil
}

type QualificationState struct {
	ProfileID           string  `json:"profile_id"`
	Status              string  `json:"status"`
	Reason              string  `json:"reason"`
	Epoch               int     `json:"epoch"`
	Threshold           float64 `json:"threshold"`
	ThresholdMustExceed float64 `json:"threshold_must_exceed"`
	Declared            int     `json:"declared_receipts"`
	Verified            int     `json:"verified_receipts"`
	PassPopulation      int     `json:"pass_population"`
	ChangesPopulation   int     `json:"changes_population"`
	PassSamples         int     `json:"pass_samples"`
	ChangesSamples      int     `json:"changes_samples"`
	Missing             int     `json:"missing_outcomes"`
	Overturns           int     `json:"overturns"`
	Conflicts           int     `json:"conflicting_outcomes"`
	CoverageKnown       bool    `json:"coverage_known"`
}

// Shadow classification is factual policy evaluation, never qualified emission.
// The public v2 input still MUST say unqualified; no MaatVerdict is promoted.
func shadowSourceGate(s SourceScreen, threshold float64) GateDecision {
	if !s.Floor.Passed || len(s.Floor.Checks) != 4 {
		return GateBlock
	}
	if s.Subject.Boundary != "" || s.Confidence < threshold {
		return GateEscalate
	}
	if hasBlockingFinding(s.Findings, threshold) {
		return GateBlock
	}
	if sourceHasSeverity(s.Findings, "major") {
		return GateEscalate
	}
	if len(s.Findings) > 0 && !sourceHasSeverity(s.Findings, "block") {
		return GateChanges
	}
	if len(s.Findings) == 0 && s.FeatherWeight >= 85 {
		return GatePass
	}
	return GateEscalate
}

// Replay uses file order, not time sorting or the newest display window.
func ReplayQualification(ctx context.Context, profile SourceProfileIdentity, events []QualificationEvent, resolver LifecycleResolver) (QualificationState, error) {
	state := QualificationState{ProfileID: profile.ID(), Status: "unqualified", Reason: "No established reviewed policy/evaluation acceptance.", Threshold: .90}
	if resolver == nil {
		return state, fmt.Errorf("lifecycle resolver required")
	}
	seen := map[string]string{}
	comparisons := map[string]string{}
	invalidated := map[string]bool{}
	cache := map[string]LifecycleEvidence{}
	uncertain := false
	uncertainReason := ""
	resolve := func(id string) (LifecycleEvidence, error) {
		if err := ctx.Err(); err != nil {
			return LifecycleEvidence{}, err
		}
		if v, ok := cache[id]; ok {
			return v, nil
		}
		if len(cache) >= 512 {
			return LifecycleEvidence{}, fmt.Errorf("lifecycle receipt resolution bound exceeded")
		}
		v, err := resolver.ResolveLifecycle(ctx, id)
		if err == nil {
			if v.ID != id || !sourceDigest(v.SHA) {
				return v, fmt.Errorf("invalid resolved receipt identity")
			}
			if err = v.Receipt.validate(); err != nil {
				return v, err
			}
			cache[id] = v
		}
		return v, err
	}
	for _, event := range events {
		if err := event.validate(); err != nil {
			return state, err
		}
		if event.Profile.ID() != profile.ID() {
			continue
		}
		if previous, ok := seen[event.ReceiptID]; ok {
			if previous != event.ReceiptSHA {
				return state, fmt.Errorf("conflicting lifecycle receipt bytes")
			}
			continue
		}
		seen[event.ReceiptID] = event.ReceiptSHA
		state.Epoch++
		state.Declared++
		v, err := resolve(event.ReceiptID)
		if err != nil {
			if ctx.Err() != nil {
				return state, ctx.Err()
			}
			state.Status = "unqualified"
			state.Reason = "Receipt unavailable: " + event.ReceiptID
			if uncertainReason == "" {
				uncertainReason = state.Reason
			}
			state.Missing++
			uncertain = true
			continue
		}
		if v.ID != event.ReceiptID || v.SHA != event.ReceiptSHA || v.Receipt.Profile != profile || v.Receipt.HeadSHA != event.HeadSHA {
			return state, fmt.Errorf("receipt/head/profile drift")
		}
		if err := v.Receipt.validate(); err != nil {
			return state, err
		}
		if !v.accepted {
			uncertain = true
			state.Status = "unqualified"
			state.Reason = v.Reason
			if state.Reason == "" {
				state.Reason = "Authoritative policy/evaluation acceptance is unestablished."
			}
			if uncertainReason == "" {
				uncertainReason = state.Reason
			}
			continue
		}
		state.Verified++
		state.Threshold = v.Receipt.Policy.Threshold
		if v.Receipt.Kind == "outcome" {
			if v.Issuer == profile.Producer || v.Issuer == "" {
				return state, fmt.Errorf("independent reviewer is not established")
			}
			key := v.Receipt.Repo + ":" + v.Receipt.HeadSHA + ":" + v.Receipt.ScreenSHA
			if old, ok := comparisons[key]; ok {
				if old != string(v.Receipt.FinalGate) {
					state.Conflicts++
				} else {
					continue
				}
			}
			comparisons[key] = string(v.Receipt.FinalGate)
			if v.Receipt.CandidateGate == GatePass && v.Receipt.FinalGate == GateBlock {
				if !invalidated[key] {
					state.Overturns++
					invalidated[key] = true
				}
				state.Status = "requalification_required"
				state.Reason = "Independent block requires a new reviewed policy/evaluation; raising a threshold alone is insufficient."
				state.ThresholdMustExceed = v.Receipt.Policy.Threshold
			}
		}
		if v.Receipt.Kind != "evaluation" {
			continue
		}
		p, err := resolve(v.Receipt.PolicyRef)
		if err != nil || !p.accepted || p.Receipt.Kind != "policy" || p.Receipt.Profile != profile || p.Receipt.Policy != v.Receipt.Policy {
			uncertain = true
			state.Status = "unqualified"
			state.Reason = "Pinned reviewed policy acceptance is missing: " + v.Receipt.PolicyRef
			if uncertainReason == "" {
				uncertainReason = state.Reason
			}
			continue
		}
		selected := map[string]bool{}
		state.CoverageKnown = true
		for _, id := range v.Receipt.SelectedIDs {
			selected[id] = true
		}
		state.PassPopulation = 0
		state.ChangesPopulation = 0
		state.PassSamples = 0
		state.ChangesSamples = 0
		state.Missing = 0
		for _, sample := range v.Receipt.Samples {
			gate := shadowSourceGate(sample.Screen, v.Receipt.Policy.Threshold)
			if gate == GatePass {
				state.PassPopulation++
			}
			if gate == GateChanges {
				state.ChangesPopulation++
			}
			if !selected[sample.ID] {
				continue
			}
			if gate != GatePass && gate != GateChanges {
				return state, fmt.Errorf("selected sample is not candidate PASS/CHANGES")
			}
			o, err := resolve(sample.OutcomeRef)
			if err != nil || !o.accepted {
				uncertain = true
				if uncertainReason == "" {
					uncertainReason = "Independent outcome acceptance is missing for sample " + sample.ID + ": " + sample.OutcomeRef
				}
				state.Missing++
				continue
			}
			if o.Receipt.Kind != "outcome" || o.Receipt.Profile != profile || o.Receipt.Repo != sample.Screen.Subject.Repo || o.Receipt.HeadSHA != sample.Screen.Subject.HeadSHA || o.Receipt.ScreenSHA != lifecycleDigest(sample.Screen) || o.Receipt.CandidateGate != gate || o.Issuer == "" || o.Issuer == profile.Producer {
				return state, fmt.Errorf("independent outcome subject/profile/reviewer mismatch")
			}
			if gate == GatePass {
				state.PassSamples++
			} else {
				state.ChangesSamples++
			}
			key := o.Receipt.Repo + ":" + o.Receipt.HeadSHA + ":" + o.Receipt.ScreenSHA
			if old, ok := comparisons[key]; ok && old != string(o.Receipt.FinalGate) {
				state.Conflicts++
			}
			comparisons[key] = string(o.Receipt.FinalGate)
			if gate == GatePass && o.Receipt.FinalGate == GateBlock {
				if !invalidated[key] {
					state.Overturns++
					invalidated[key] = true
				}
			}
		}
		if state.Overturns > 0 || state.Conflicts > 0 {
			state.Status = "requalification_required"
			state.Reason = "Independent auto-pass overturn retained; new profile/policy evaluation required."
			state.ThresholdMustExceed = v.Receipt.Policy.Threshold
		} else if state.Missing == 0 && state.PassSamples >= v.Receipt.Policy.MinimumPass && state.ChangesSamples >= v.Receipt.Policy.MinimumChanges {
			state.Status = "qualified"
			state.Reason = "Exact reviewed evaluation/policy accepted; informational only, no gate activation."
			state.Threshold = v.Receipt.Policy.Threshold
		} else {
			state.Status = "unqualified"
			state.Reason = "PASS/CHANGES selection coverage or independent labels incomplete."
		}
	}
	if state.Overturns > 0 || state.Conflicts > 0 {
		state.Status = "requalification_required"
		state.Reason = "Retained independent auto-pass overturn or conflicting outcome; new profile/policy evaluation required."
		if state.ThresholdMustExceed < state.Threshold {
			state.ThresholdMustExceed = state.Threshold
		}
	} else if uncertain {
		state.Status = "unqualified"
		state.Reason = uncertainReason
		state.CoverageKnown = false
	}
	return state, nil
}
