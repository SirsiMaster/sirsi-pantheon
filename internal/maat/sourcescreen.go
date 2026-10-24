package maat

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"
)

const (
	SourceScreenContract      = "maat.adr070.source-screen"
	SourceScreenSchemaVersion = 2
	SourceScreenMaxBytes      = 1 << 20
	declaredFloorEvidence     = "declared-only; checks not executed or authenticated by import"
)

// SourceScreen is the amended, nonbinding source import contract. Unlike host
// triage and historical v1, every scalar is required (even false/zero/empty
// boundary). Digests identify declared evidence, not its truth or qualification.
type SourceScreen struct {
	Contract      string          `json:"contract"`
	SchemaVersion int             `json:"schema_version"`
	Subject       VerdictSubject  `json:"subject"`
	FeatherWeight int             `json:"feather_weight"`
	Confidence    float64         `json:"confidence"`
	Findings      []ScreenFinding `json:"findings"`
	Floor         FloorResult     `json:"floor"`
	Model         ModelStamp      `json:"model"`
}

// DecodeSourceScreen validates presence, exact case, duplicates, closed keys,
// types, nulls and EOF before any Go zero value can stand for an absent field.
// The input and recursion bounds apply to this source contract only.
func DecodeSourceScreen(raw []byte) (SourceScreen, error) {
	var input SourceScreen
	if len(raw) > SourceScreenMaxBytes {
		return input, fmt.Errorf("maat source screen: input exceeds 1 MiB")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	if err := sourceJSONValue(d, reflect.TypeOf(input), 0); err != nil {
		return input, err
	}
	if _, err := d.Token(); err != io.EOF {
		return input, fmt.Errorf("maat source screen: trailing JSON or invalid EOF")
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return input, fmt.Errorf("maat source screen: decode: %w", err)
	}
	return input, nil
}

// ScreenSource is the sole production v2 evaluator. No caller-controlled stamp,
// provider, history row or boolean can qualify a model. Qualified claims reject,
// rather than being silently downgraded. Expected head is separate consumer
// context; it is required and never inferred from the imported packet.
func ScreenSource(input SourceScreen, expectedHead string) (MaatVerdict, error) {
	return screenSource(input, expectedHead, false)
}

// qualified is package-private policy-fixture context ONLY. There is no public
// qualified path or qualification writer. Authenticated calibration/independent
// outcomes, drift and requalification lifecycle remain OPEN, not implemented here.
func screenSource(input SourceScreen, expectedHead string, qualified bool) (MaatVerdict, error) {
	if err := validateSourceInput(input, expectedHead, qualified); err != nil {
		return MaatVerdict{}, err
	} // R0
	canonical := canonicalSystemOneInput(SystemOneScreen{Subject: input.Subject, FeatherWeight: input.FeatherWeight,
		Confidence: input.Confidence, Findings: input.Findings, Floor: input.Floor, Model: input.Model})
	v := MaatVerdict{Contract: SourceScreenContract, SchemaVersion: SourceScreenSchemaVersion,
		FloorEvidence: declaredFloorEvidence, Subject: canonical.Subject, FeatherWeight: canonical.FeatherWeight,
		Confidence: canonical.Confidence, Findings: canonical.Findings, Floor: canonical.Floor, Model: canonical.Model}
	if !v.Floor.Passed || len(v.Floor.Checks) != 4 { // R1: failed or incomplete declared floor
		v.Floor.Passed = false
		v.Gate = GateBlock
		if v.FeatherWeight > 49 {
			v.FeatherWeight = 49
		}
		return v, nil
	}
	escalate := func(reason string) {
		v.Gate = GateEscalate
		v.Escalation = &Escalation{Reason: reason,
			ReviewTier: "frontier", MissedConf: missedConfidence(defaultScreenConfidence, v.Confidence)}
	}
	switch {
	case v.Subject.Boundary != "" || !qualified: // R2, independent of findings
		escalate("boundary or unqualified calibration requires independent review; screen is nonbinding")
	case v.Confidence < defaultScreenConfidence: // R3
		escalate("source screen confidence below provisional threshold")
	case hasBlockingFinding(v.Findings, defaultScreenConfidence): // R4 BEFORE any major
		v.Gate = GateBlock
	case sourceHasSeverity(v.Findings, "major"): // R5, any confidence
		escalate("major finding requires independent review")
	case len(v.Findings) > 0 && !sourceHasSeverity(v.Findings, "block"): // R6: ONLY minor/nit
		v.Gate = GateChanges
	case len(v.Findings) == 0 && v.FeatherWeight >= 85: // R7
		v.Gate = GatePass
	default: // R8, including unconfirmed block and no-findings score below 85
		escalate("source screen has no clean qualified outcome")
	}
	return v, nil
}

func validateSourceInput(input SourceScreen, expectedHead string, qualified bool) error {
	if input.Contract != SourceScreenContract || input.SchemaVersion != SourceScreenSchemaVersion {
		return fmt.Errorf("maat source screen: unsupported contract/version")
	}
	if !oneOf(input.Subject.Kind, "pr", "diff", "file", "commit") {
		return fmt.Errorf("maat source screen: source subject required")
	}
	if err := validateSubject(input.Subject); err != nil {
		return err
	}
	if strings.TrimSpace(expectedHead) == "" || input.Subject.HeadSHA != expectedHead {
		return fmt.Errorf("maat source screen: expected head mismatch or missing")
	}
	if input.FeatherWeight < 0 || input.FeatherWeight > 100 {
		return fmt.Errorf("maat source screen: score outside 0..100")
	}
	if err := validateConfidence("source screen", input.Confidence); err != nil {
		return err
	}
	if err := validateModel(input.Model); err != nil {
		return err
	}
	if !oneOf(input.Model.Calibration, "unqualified", "qualified") {
		return fmt.Errorf("maat source screen: calibration state required")
	}
	if input.Model.Calibration == "qualified" && !qualified {
		return fmt.Errorf("maat source screen: imported qualified claim has no authority")
	}
	if qualified && input.Model.Calibration != "qualified" {
		return fmt.Errorf("maat source screen: fixture qualification contradicts stamp")
	}
	if !sourceDigest(input.Model.EvidenceSetDigest) || !sourceDigest(input.Model.CalibrationID) {
		return fmt.Errorf("maat source screen: recipe and calibration SHA256 identities required")
	}
	seen := map[string]bool{}
	for _, f := range input.Findings {
		if err := validateFinding(f); err != nil {
			return err
		}
		if f.RepairID != "" || !sourceDigest(f.EvidenceID) || seen[f.ID] {
			return fmt.Errorf("maat source screen: duplicate finding, invalid evidence identity or host repair reference")
		}
		seen[f.ID] = true
	}
	checks := map[string]bool{}
	passed := true
	for _, c := range input.Floor.Checks {
		if !oneOf(c.Name, "gofmt", "vet", "golangci-lint", "test-race") || checks[c.Name] {
			return fmt.Errorf("maat source screen: unknown or duplicate floor check")
		}
		checks[c.Name] = true
		if !c.Passed {
			passed = false
			if strings.TrimSpace(c.Detail) == "" {
				return fmt.Errorf("maat source screen: failed check requires declared detail")
			}
		}
	}
	if input.Floor.Passed && !passed {
		return fmt.Errorf("maat source screen: contradictory declared floor")
	}
	return nil
}

func validateSourceVerdict(v MaatVerdict) error {
	if v.FloorEvidence != declaredFloorEvidence {
		return fmt.Errorf("maat source screen: declared-only floor label required")
	}
	input := SourceScreen{Contract: v.Contract, SchemaVersion: v.SchemaVersion, Subject: v.Subject,
		FeatherWeight: v.FeatherWeight, Confidence: v.Confidence, Findings: v.Findings, Floor: v.Floor, Model: v.Model}
	expected, err := ScreenSource(input, v.Subject.HeadSHA)
	if err != nil {
		return err
	}
	if v.Gate != expected.Gate || v.FeatherWeight != expected.FeatherWeight || v.Floor.Passed != expected.Floor.Passed || !sameEscalation(v.Escalation, expected.Escalation) {
		return fmt.Errorf("maat source screen: verdict contradicts unqualified production policy")
	}
	ordered := SystemOneScreen{Findings: v.Findings, Floor: v.Floor}
	if !sameCanonicalSystemOneInput(ordered, canonicalSystemOneInput(ordered)) {
		return fmt.Errorf("maat source screen: noncanonical evidence order")
	}
	return nil
}

func sourceDigest(s string) bool {
	if len(s) != 64 || s != strings.ToLower(s) {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

func sourceHasSeverity(findings []ScreenFinding, severity string) bool {
	for _, f := range findings {
		if f.Severity == severity {
			return true
		}
	}
	return false
}

func hasSourceProvenance(model ModelStamp, findings []ScreenFinding) bool {
	if model.EvidenceSetDigest != "" || model.CalibrationID != "" || model.Calibration != "" {
		return true
	}
	for _, f := range findings {
		if f.EvidenceID != "" {
			return true
		}
	}
	return false
}

// sourceJSONValue walks only the bounded source packet. Optional legacy fields
// are required here except fix_hint/detail; host repair_id is never accepted.
func sourceJSONValue(d *json.Decoder, typ reflect.Type, depth int) error {
	if depth > 16 {
		return fmt.Errorf("maat source screen: JSON nesting exceeds limit")
	}
	token, err := d.Token()
	if err != nil {
		return fmt.Errorf("maat source screen: JSON: %w", err)
	}
	if token == nil {
		return fmt.Errorf("maat source screen: null required value")
	}
	switch typ.Kind() {
	case reflect.Struct:
		if token != json.Delim('{') {
			return fmt.Errorf("maat source screen: object required")
		}
		fields := map[string]reflect.Type{}
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			key := strings.Split(f.Tag.Get("json"), ",")[0]
			if key != "repair_id" {
				fields[key] = f.Type
			}
		}
		seen := map[string]bool{}
		for d.More() {
			keyToken, e := d.Token()
			if e != nil {
				return e
			}
			key, ok := keyToken.(string)
			field, exists := fields[key]
			if !ok || !exists || seen[key] {
				return fmt.Errorf("maat source screen: unknown/exact-case or duplicate field %q", key)
			}
			seen[key] = true
			if e := sourceJSONValue(d, field, depth+1); e != nil {
				return e
			}
		}
		if _, e := d.Token(); e != nil {
			return e
		}
		for key := range fields {
			if key != "fix_hint" && key != "detail" && !seen[key] {
				return fmt.Errorf("maat source screen: required field %q missing", key)
			}
		}
	case reflect.Slice:
		if token != json.Delim('[') {
			return fmt.Errorf("maat source screen: array required")
		}
		for d.More() {
			if e := sourceJSONValue(d, typ.Elem(), depth+1); e != nil {
				return e
			}
		}
		_, err = d.Token()
		return err
	default:
		if _, compound := token.(json.Delim); compound {
			return fmt.Errorf("maat source screen: scalar required")
		}
	}
	return nil
}
