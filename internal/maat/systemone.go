package maat

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
)

// SystemOneSchemaVersion is the closed local Ma'at screening contract. It is
// deliberately independent from an LLM provider: callers supply observations
// and confidence, while this package enforces the deterministic floor and the
// screen-to-escalate policy before anything can consume the result.
const SystemOneSchemaVersion = 1

const (
	defaultScreenConfidence  = 0.90
	boundaryScreenConfidence = 0.98
)

// GateDecision is the only machine-readable outcome a System One screen may
// publish. A screen is an advisory classifier; it never grants mutation or
// substitutes for the owning operation's acceptance.
type GateDecision string

const (
	GatePass     GateDecision = "pass"
	GateChanges  GateDecision = "changes"
	GateBlock    GateDecision = "block"
	GateEscalate GateDecision = "escalate"
)

// SystemOneRepairLaunchdDisabled is a closed reference to Ma'at's one
// currently implemented bounded host repair. It is deliberately an identifier,
// not a command: Casebook consumers can offer the known workflow, but never
// execute a producer-supplied string.
const SystemOneRepairLaunchdDisabled = "launchd-disabled"

// VerdictSubject pins a screen to the immutable change it assessed. A result
// cannot be reused after its head changes.
type VerdictSubject struct {
	Kind     string `json:"kind"`
	Repo     string `json:"repo"`
	Ref      string `json:"ref"`
	HeadSHA  string `json:"head_sha"`
	Boundary string `json:"boundary,omitempty"`
}

// ScreenFinding is a bounded, evidence-linked observation. Confidence belongs
// to the specific claim rather than being borrowed from the enclosing screen.
type ScreenFinding struct {
	ID         string  `json:"id"`
	Severity   string  `json:"severity"`
	Category   string  `json:"category"`
	File       string  `json:"file,omitempty"`
	Line       int     `json:"line,omitempty"`
	Claim      string  `json:"claim"`
	Evidence   string  `json:"evidence"`
	Confidence float64 `json:"confidence"`
	FixHint    string  `json:"fix_hint,omitempty"`
	// RepairID is optional and closed. It allows a System One host screen to
	// route a finding to a Ma'at-owned bounded repair without making FixHint or
	// any imported screen content executable input.
	RepairID string `json:"repair_id,omitempty"`
}

// FloorCheck is deterministic evidence. A screen is never allowed to turn a
// failed floor check into a pass, change request, or owner decision.
type FloorCheck struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
	Detail string `json:"detail,omitempty"`
}

type FloorResult struct {
	Passed bool         `json:"passed"`
	Checks []FloorCheck `json:"checks"`
}

// ModelStamp describes the classifier that produced the confidence input. The
// default local implementation remains useful without a model; a remote model
// must be conspicuous to every consumer.
type ModelStamp struct {
	Provider  string `json:"provider"`
	Version   string `json:"version"`
	Local     bool   `json:"local"`
	LatencyMS int    `json:"latency_ms"`
}

type Escalation struct {
	Reason     string  `json:"reason"`
	MissedConf float64 `json:"missed_conf"`
	ReviewTier string  `json:"review_tier"`
}

// MaatVerdict is the strict, portable System One result. It describes a
// screen only: GateEscalate is the required result for sensitive boundaries
// and low confidence, while a later owning reviewer may record an independent
// conclusion through the existing Casebook route.
type MaatVerdict struct {
	SchemaVersion int             `json:"schema_version"`
	Subject       VerdictSubject  `json:"subject"`
	FeatherWeight int             `json:"feather_weight"`
	Gate          GateDecision    `json:"gate"`
	Confidence    float64         `json:"confidence"`
	Findings      []ScreenFinding `json:"findings"`
	Floor         FloorResult     `json:"floor"`
	Escalation    *Escalation     `json:"escalation,omitempty"`
	Model         ModelStamp      `json:"model"`
}

// SystemOneScreen is the strict input to the local deterministic policy. It
// intentionally excludes Gate and Escalation: callers cannot choose their own
// outcome or bypass the confidence and boundary rules.
type SystemOneScreen struct {
	Subject       VerdictSubject  `json:"subject"`
	FeatherWeight int             `json:"feather_weight"`
	Confidence    float64         `json:"confidence"`
	Findings      []ScreenFinding `json:"findings"`
	Floor         FloorResult     `json:"floor"`
	Model         ModelStamp      `json:"model"`
}

// Screen applies the closed local policy to a type-safe screen input. It runs
// no subprocess, invokes no model, and performs no mutation.
func Screen(input SystemOneScreen) (MaatVerdict, error) {
	input = canonicalSystemOneInput(input)
	if err := validateScreenInput(input); err != nil {
		return MaatVerdict{}, err
	}
	verdict := MaatVerdict{
		SchemaVersion: SystemOneSchemaVersion,
		Subject:       input.Subject,
		FeatherWeight: input.FeatherWeight,
		Confidence:    input.Confidence,
		Findings:      append([]ScreenFinding(nil), input.Findings...),
		Floor:         input.Floor,
		Model:         input.Model,
	}
	applyScreenPolicy(&verdict)
	return verdict, ValidateMaatVerdict(verdict)
}

// RecordSystemOne records a screened result in Ma'at's existing append-only
// journal. This gives the Casebook and all projections a shared evidence item
// without making the journal a release, router, or mutation authority.
func RecordSystemOne(j DecisionJournal, requester string, verdict MaatVerdict) (Decision, error) {
	if j == nil {
		return Decision{}, fmt.Errorf("maat system one: nil decision journal")
	}
	if err := ValidateMaatVerdict(verdict); err != nil {
		return Decision{}, err
	}
	requester = strings.TrimSpace(requester)
	if requester == "" {
		return Decision{}, fmt.Errorf("maat system one: requester is required")
	}
	raw, err := json.Marshal(verdict)
	if err != nil {
		return Decision{}, fmt.Errorf("maat system one: encode verdict: %w", err)
	}
	sum := sha256.Sum256(raw)
	decision := Decision{
		Kind:          "system one screen",
		Requester:     requester,
		Resource:      verdict.Subject.Repo,
		Assessed:      verdict.Subject.Kind + " " + verdict.Subject.Ref,
		Affected:      verdict.Subject.HeadSHA,
		Determination: string(verdict.Gate),
		Why:           systemOneReason(verdict),
		Evidence:      "maat-system-one:sha256=" + hex.EncodeToString(sum[:]),
		SystemOne:     &verdict,
	}
	if err := j.Append(decision); err != nil {
		return Decision{}, fmt.Errorf("maat system one: append screen: %w", err)
	}
	return decision, nil
}

// Calibration describes how often a screen pass was overturned by the
// independently supplied frontier result. It deliberately makes missing
// frontier evidence visible instead of treating it as a clean sample.
type Calibration struct {
	Samples          int     `json:"samples"`
	AutoPasses       int     `json:"auto_passes"`
	AutoPassOverturn int     `json:"auto_pass_overturns"`
	OverturnRate     float64 `json:"overturn_rate"`
}

type CalibrationSample struct {
	Screen   MaatVerdict  `json:"screen"`
	Frontier GateDecision `json:"frontier_gate"`
}

// CalibrationRecord binds one recorded local auto-pass to one distinct final
// outcome from an independent reviewer. Ma'at records the evidence link but
// does not claim to authenticate an external receipt or grant any authority.
type CalibrationRecord struct {
	SchemaVersion    int          `json:"schema_version"`
	ScreenEvidence   string       `json:"screen_evidence"`
	FrontierEvidence string       `json:"frontier_evidence"`
	ScreenGate       GateDecision `json:"screen_gate"`
	FrontierGate     GateDecision `json:"frontier_gate"`
}

func Calibrate(samples []CalibrationSample) (Calibration, error) {
	calibration := Calibration{Samples: len(samples)}
	for _, sample := range samples {
		if err := ValidateMaatVerdict(sample.Screen); err != nil {
			return Calibration{}, fmt.Errorf("maat system one: invalid screen sample: %w", err)
		}
		if !validGate(sample.Frontier) {
			return Calibration{}, fmt.Errorf("maat system one: unsupported frontier gate %q", sample.Frontier)
		}
		if sample.Screen.Gate != GatePass {
			continue
		}
		calibration.AutoPasses++
		if sample.Frontier == GateBlock || sample.Frontier == GateEscalate {
			calibration.AutoPassOverturn++
		}
	}
	if calibration.AutoPasses > 0 {
		calibration.OverturnRate = float64(calibration.AutoPassOverturn) / float64(calibration.AutoPasses)
	}
	return calibration, nil
}

// RecordSystemOneCalibration records calibration only for one existing local
// auto-pass and one distinct final reviewer outcome. Replay, ambiguity, and
// non-pass screens fail closed so favorable history cannot be manufactured.
func RecordSystemOneCalibration(j DecisionJournal, requester, screenEvidence, frontierEvidence string, frontierGate GateDecision) (Decision, Calibration, error) {
	if j == nil {
		return Decision{}, Calibration{}, fmt.Errorf("maat system one: nil decision journal")
	}
	requester = strings.TrimSpace(requester)
	screenEvidence = strings.TrimSpace(screenEvidence)
	frontierEvidence = strings.TrimSpace(frontierEvidence)
	if requester == "" || screenEvidence == "" || frontierEvidence == "" {
		return Decision{}, Calibration{}, fmt.Errorf("maat system one: requester, screen evidence, and frontier evidence are required")
	}
	if len(screenEvidence) > 512 || len(frontierEvidence) > 512 {
		return Decision{}, Calibration{}, fmt.Errorf("maat system one: calibration evidence exceeds bounded record size")
	}
	if screenEvidence == frontierEvidence {
		return Decision{}, Calibration{}, fmt.Errorf("maat system one: frontier evidence must be distinct from screen evidence")
	}
	if !validFinalFrontierGate(frontierGate) {
		return Decision{}, Calibration{}, fmt.Errorf("maat system one: unsupported final frontier gate %q", frontierGate)
	}
	rows, err := j.Recent(1000)
	if err != nil {
		return Decision{}, Calibration{}, fmt.Errorf("maat system one: read calibration history: %w", err)
	}
	var screen *Decision
	screenMatches := 0
	for index := range rows {
		row := rows[index]
		if row.SystemOneCalibration != nil && row.SystemOneCalibration.ScreenEvidence == screenEvidence && row.SystemOneCalibration.FrontierEvidence == frontierEvidence {
			return Decision{}, Calibration{}, fmt.Errorf("maat system one: this screen/frontier calibration pair is already recorded")
		}
		if row.Evidence == screenEvidence && row.Kind == "system one screen" && row.SystemOne != nil {
			screen = &row
			screenMatches++
		}
	}
	if screen == nil {
		return Decision{}, Calibration{}, fmt.Errorf("maat system one: no matching recorded System One screen")
	}
	if screenMatches != 1 {
		return Decision{}, Calibration{}, fmt.Errorf("maat system one: ambiguous recorded System One screen evidence")
	}
	if screen.SystemOne.Gate != GatePass {
		return Decision{}, Calibration{}, fmt.Errorf("maat system one: calibration records only local auto-pass screens, got %q", screen.SystemOne.Gate)
	}
	record := CalibrationRecord{SchemaVersion: SystemOneSchemaVersion, ScreenEvidence: screenEvidence, FrontierEvidence: frontierEvidence, ScreenGate: GatePass, FrontierGate: frontierGate}
	if err := validateCalibrationRecord(record); err != nil {
		return Decision{}, Calibration{}, err
	}
	raw, err := json.Marshal(record)
	if err != nil {
		return Decision{}, Calibration{}, fmt.Errorf("maat system one: encode calibration: %w", err)
	}
	sum := sha256.Sum256(raw)
	decision := Decision{
		Kind: "system one calibration", Requester: requester, Resource: screen.Resource,
		Assessed: "System One auto-pass calibration", Affected: screen.Affected,
		Determination: string(frontierGate), Why: calibrationReason(record),
		Evidence: "maat-system-one-calibration:sha256=" + hex.EncodeToString(sum[:]), SystemOneCalibration: &record,
	}
	calibration, err := CalibrationFromDecisions(append(rows, decision))
	if err != nil {
		return Decision{}, Calibration{}, err
	}
	if err := j.Append(decision); err != nil {
		return Decision{}, Calibration{}, fmt.Errorf("maat system one: append calibration: %w", err)
	}
	return decision, calibration, nil
}

// CalibrationFromDecisions derives calibration from the durable decision
// history. Invalid, replayed, or unbound records are errors rather than data.
func CalibrationFromDecisions(rows []Decision) (Calibration, error) {
	calibration := Calibration{}
	seen := map[string]bool{}
	screens := make(map[string]GateDecision)
	for _, row := range rows {
		if row.Kind != "system one screen" || row.SystemOne == nil {
			continue
		}
		if err := ValidateMaatVerdict(*row.SystemOne); err != nil {
			return Calibration{}, fmt.Errorf("maat system one: invalid recorded screen: %w", err)
		}
		if previous, exists := screens[row.Evidence]; exists && previous != row.SystemOne.Gate {
			return Calibration{}, fmt.Errorf("maat system one: conflicting recorded screen evidence")
		}
		screens[row.Evidence] = row.SystemOne.Gate
	}
	for _, row := range rows {
		record := row.SystemOneCalibration
		if record == nil {
			continue
		}
		if err := validateCalibrationRecord(*record); err != nil {
			return Calibration{}, err
		}
		key := record.ScreenEvidence + "\x00" + record.FrontierEvidence
		if seen[key] {
			return Calibration{}, fmt.Errorf("maat system one: duplicate durable calibration pair")
		}
		if screens[record.ScreenEvidence] != GatePass {
			return Calibration{}, fmt.Errorf("maat system one: calibration has no matching recorded local auto-pass screen")
		}
		seen[key] = true
		calibration.Samples++
		calibration.AutoPasses++
		if record.FrontierGate == GateBlock {
			calibration.AutoPassOverturn++
		}
	}
	if calibration.AutoPasses > 0 {
		calibration.OverturnRate = float64(calibration.AutoPassOverturn) / float64(calibration.AutoPasses)
	}
	return calibration, nil
}

// ValidateMaatVerdict enforces both the schema and the derived policy. It is
// intentionally suitable for a future router or release consumer: a verdict
// that merely looks plausible but contradicts its deterministic inputs fails.
func ValidateMaatVerdict(verdict MaatVerdict) error {
	if verdict.SchemaVersion != SystemOneSchemaVersion {
		return fmt.Errorf("maat system one: unsupported schema version %d", verdict.SchemaVersion)
	}
	input := SystemOneScreen{
		Subject: verdict.Subject, FeatherWeight: verdict.FeatherWeight,
		Confidence: verdict.Confidence, Findings: verdict.Findings,
		Floor: verdict.Floor, Model: verdict.Model,
	}
	if err := validateScreenInput(input); err != nil {
		return err
	}
	if !sameCanonicalSystemOneInput(input, canonicalSystemOneInput(input)) {
		return fmt.Errorf("maat system one: verdict findings and deterministic floor checks must use canonical order")
	}
	expected := verdict
	applyScreenPolicy(&expected)
	if verdict.Gate != expected.Gate {
		return fmt.Errorf("maat system one: gate %q contradicts screen policy %q", verdict.Gate, expected.Gate)
	}
	if !sameEscalation(verdict.Escalation, expected.Escalation) {
		return fmt.Errorf("maat system one: escalation contradicts screen policy")
	}
	return nil
}

func applyScreenPolicy(verdict *MaatVerdict) {
	threshold := confidenceThreshold(verdict.Subject.Boundary)
	if !verdict.Floor.Passed {
		verdict.Gate = GateBlock
		if verdict.FeatherWeight > 49 {
			verdict.FeatherWeight = 49
		}
		verdict.Escalation = nil
		return
	}
	if verdict.Subject.Boundary != "" {
		verdict.Gate = GateEscalate
		verdict.Escalation = &Escalation{
			Reason: "sensitive boundary requires frontier review", ReviewTier: "frontier",
			MissedConf: missedConfidence(threshold, verdict.Confidence),
		}
		return
	}
	if verdict.Confidence < threshold || hasLowConfidenceMajor(verdict.Findings, threshold) {
		verdict.Gate = GateEscalate
		verdict.Escalation = &Escalation{
			Reason: "screen confidence is below the binding threshold", ReviewTier: "frontier",
			MissedConf: missedConfidence(threshold, verdict.Confidence),
		}
		return
	}
	if hasBlockingFinding(verdict.Findings, threshold) {
		verdict.Gate = GateBlock
		verdict.Escalation = nil
		return
	}
	if len(verdict.Findings) > 0 || verdict.FeatherWeight < 85 {
		verdict.Gate = GateChanges
		verdict.Escalation = nil
		return
	}
	verdict.Gate = GatePass
	verdict.Escalation = nil
}

func validateScreenInput(input SystemOneScreen) error {
	if err := validateSubject(input.Subject); err != nil {
		return err
	}
	if input.FeatherWeight < 0 || input.FeatherWeight > 100 {
		return fmt.Errorf("maat system one: feather weight must be between 0 and 100")
	}
	if err := validateConfidence("screen", input.Confidence); err != nil {
		return err
	}
	if err := validateFloor(input.Floor); err != nil {
		return err
	}
	if err := validateModel(input.Model); err != nil {
		return err
	}
	seenFindings := map[string]bool{}
	for _, finding := range input.Findings {
		if err := validateFinding(finding); err != nil {
			return err
		}
		if seenFindings[finding.ID] {
			return fmt.Errorf("maat system one: duplicate finding id %q", finding.ID)
		}
		seenFindings[finding.ID] = true
	}
	return nil
}

// canonicalSystemOneInput makes equivalent local observations yield the same
// ordered verdict and therefore the same recorded evidence hash. It does not
// infer, rewrite, or execute any observation; it only normalizes independent
// finding and deterministic-floor collection order.
func canonicalSystemOneInput(input SystemOneScreen) SystemOneScreen {
	input.Floor.Checks = append([]FloorCheck(nil), input.Floor.Checks...)
	sort.Slice(input.Floor.Checks, func(i, j int) bool {
		return input.Floor.Checks[i].Name < input.Floor.Checks[j].Name
	})
	input.Findings = append([]ScreenFinding(nil), input.Findings...)
	sort.Slice(input.Findings, func(i, j int) bool {
		return input.Findings[i].ID < input.Findings[j].ID
	})
	return input
}

func sameCanonicalSystemOneInput(a, b SystemOneScreen) bool {
	if len(a.Floor.Checks) != len(b.Floor.Checks) || len(a.Findings) != len(b.Findings) {
		return false
	}
	for i := range a.Floor.Checks {
		if a.Floor.Checks[i] != b.Floor.Checks[i] {
			return false
		}
	}
	for i := range a.Findings {
		if a.Findings[i] != b.Findings[i] {
			return false
		}
	}
	return true
}

func validateSubject(subject VerdictSubject) error {
	// A System One subject is an immutable observation, not exclusively a source
	// change. A local host diagnostic is likewise a bounded, hashed observation
	// that Ma'at must be able to retain and resolve through the same Casebook.
	if !oneOf(subject.Kind, "pr", "diff", "file", "commit", "host") {
		return fmt.Errorf("maat system one: unsupported subject kind %q", subject.Kind)
	}
	for name, value := range map[string]string{"repo": subject.Repo, "ref": subject.Ref, "head_sha": subject.HeadSHA} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("maat system one: subject %s is required", name)
		}
	}
	if !oneOf(subject.Boundary, "", "security", "delivery", "identity") {
		return fmt.Errorf("maat system one: unsupported subject boundary %q", subject.Boundary)
	}
	return nil
}

func validateFinding(finding ScreenFinding) error {
	for name, value := range map[string]string{"finding id": finding.ID, "finding category": finding.Category, "finding claim": finding.Claim, "finding evidence": finding.Evidence} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("maat system one: %s is required", name)
		}
	}
	if !oneOf(finding.Severity, "block", "major", "minor", "nit") {
		return fmt.Errorf("maat system one: unsupported finding severity %q", finding.Severity)
	}
	if finding.Line < 0 {
		return fmt.Errorf("maat system one: finding line cannot be negative")
	}
	if !oneOf(finding.RepairID, "", SystemOneRepairLaunchdDisabled) {
		return fmt.Errorf("maat system one: unsupported closed repair id %q", finding.RepairID)
	}
	return validateConfidence("finding", finding.Confidence)
}

func validateFloor(floor FloorResult) error {
	if len(floor.Checks) == 0 {
		return fmt.Errorf("maat system one: deterministic floor checks are required")
	}
	passed := true
	seen := map[string]bool{}
	for _, check := range floor.Checks {
		name := strings.TrimSpace(check.Name)
		if name == "" {
			return fmt.Errorf("maat system one: floor check name is required")
		}
		if seen[name] {
			return fmt.Errorf("maat system one: duplicate floor check %q", name)
		}
		seen[name] = true
		if !check.Passed {
			passed = false
			if strings.TrimSpace(check.Detail) == "" {
				return fmt.Errorf("maat system one: failed floor check %q requires detail", name)
			}
		}
	}
	if floor.Passed != passed {
		return fmt.Errorf("maat system one: floor passed flag contradicts checks")
	}
	return nil
}

func validateModel(model ModelStamp) error {
	if strings.TrimSpace(model.Provider) == "" || strings.TrimSpace(model.Version) == "" {
		return fmt.Errorf("maat system one: model provider and version are required")
	}
	if model.LatencyMS < 0 {
		return fmt.Errorf("maat system one: model latency cannot be negative")
	}
	return nil
}

func validateConfidence(label string, confidence float64) error {
	if math.IsNaN(confidence) || math.IsInf(confidence, 0) || confidence < 0 || confidence > 1 {
		return fmt.Errorf("maat system one: %s confidence must be between 0 and 1", label)
	}
	return nil
}

func confidenceThreshold(boundary string) float64 {
	if boundary != "" {
		return boundaryScreenConfidence
	}
	return defaultScreenConfidence
}

func missedConfidence(threshold, confidence float64) float64 {
	if confidence >= threshold {
		return 0
	}
	return threshold - confidence
}

func hasBlockingFinding(findings []ScreenFinding, threshold float64) bool {
	for _, finding := range findings {
		if finding.Severity == "block" && finding.Confidence >= threshold {
			return true
		}
	}
	return false
}

func hasLowConfidenceMajor(findings []ScreenFinding, threshold float64) bool {
	for _, finding := range findings {
		if (finding.Severity == "block" || finding.Severity == "major") && finding.Confidence < threshold {
			return true
		}
	}
	return false
}

func sameEscalation(a, b *Escalation) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Reason == b.Reason && a.ReviewTier == b.ReviewTier && a.MissedConf == b.MissedConf
}

func validateCalibrationRecord(record CalibrationRecord) error {
	if record.SchemaVersion != SystemOneSchemaVersion {
		return fmt.Errorf("maat system one: unsupported calibration schema version %d", record.SchemaVersion)
	}
	if strings.TrimSpace(record.ScreenEvidence) == "" || strings.TrimSpace(record.FrontierEvidence) == "" {
		return fmt.Errorf("maat system one: calibration evidence is required")
	}
	if len(record.ScreenEvidence) > 512 || len(record.FrontierEvidence) > 512 {
		return fmt.Errorf("maat system one: calibration evidence exceeds bounded record size")
	}
	if record.ScreenEvidence == record.FrontierEvidence {
		return fmt.Errorf("maat system one: calibration evidence references must be distinct")
	}
	if record.ScreenGate != GatePass {
		return fmt.Errorf("maat system one: calibration screen gate must be pass, got %q", record.ScreenGate)
	}
	if !validFinalFrontierGate(record.FrontierGate) {
		return fmt.Errorf("maat system one: unsupported final frontier gate %q", record.FrontierGate)
	}
	return nil
}

func validFinalFrontierGate(gate GateDecision) bool {
	return gate == GatePass || gate == GateChanges || gate == GateBlock
}

func calibrationReason(record CalibrationRecord) string {
	if record.FrontierGate == GateBlock {
		return "independent review overturned a local System One auto-pass"
	}
	return "independent review confirmed or refined a local System One auto-pass"
}

func validGate(gate GateDecision) bool {
	return gate == GatePass || gate == GateChanges || gate == GateBlock || gate == GateEscalate
}

func oneOf(value string, options ...string) bool {
	for _, option := range options {
		if value == option {
			return true
		}
	}
	return false
}

func systemOneReason(verdict MaatVerdict) string {
	if verdict.Gate == GateEscalate && verdict.Escalation != nil {
		return verdict.Escalation.Reason
	}
	if verdict.Gate == GateBlock && !verdict.Floor.Passed {
		return "deterministic floor failed; System One cannot override it"
	}
	if len(verdict.Findings) > 0 {
		return verdict.Findings[0].Claim
	}
	return "local System One screen completed without findings"
}
