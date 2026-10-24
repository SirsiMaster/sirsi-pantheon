package maat

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sourceFixture() SourceScreen {
	return SourceScreen{Contract: SourceScreenContract, SchemaVersion: SourceScreenSchemaVersion,
		Subject:       VerdictSubject{Kind: "commit", Repo: "fixture/repo", Ref: "main", HeadSHA: strings.Repeat("a", 40)},
		FeatherWeight: 92, Confidence: 0.96, Findings: []ScreenFinding{},
		Floor: FloorResult{Passed: true, Checks: []FloorCheck{{Name: "gofmt", Passed: true}, {Name: "vet", Passed: true}, {Name: "golangci-lint", Passed: true}, {Name: "test-race", Passed: true}}},
		Model: ModelStamp{Provider: "local:fixture", Version: "fixture-v1", Local: true, EvidenceSetDigest: strings.Repeat("b", 64), CalibrationID: strings.Repeat("c", 64), Calibration: "unqualified"}}
}

func sourceFinding(id, severity string, confidence float64) ScreenFinding {
	return ScreenFinding{ID: id, Severity: severity, Category: "correctness", File: "fixture.go", Line: 1,
		Claim: "inert observation", Evidence: "fixture proof", EvidenceID: strings.Repeat("d", 64), Confidence: confidence}
}

func TestSourceScreenExactRulePrecedence(t *testing.T) {
	for _, tc := range []struct {
		name      string
		change    func(*SourceScreen)
		qualified bool
		gate      GateDecision
	}{
		{"R1-before-boundary", func(s *SourceScreen) { s.Floor.Passed = false; s.Subject.Boundary = "delivery" }, false, GateBlock},
		{"R1-missing-check", func(s *SourceScreen) { s.Floor.Checks = s.Floor.Checks[:3] }, false, GateBlock},
		{"R1-empty-floor", func(s *SourceScreen) { s.Floor = FloorResult{Checks: []FloorCheck{}} }, false, GateBlock},
		{"R2-unqualified", func(s *SourceScreen) {}, false, GateEscalate},
		{"R2-security", func(s *SourceScreen) { s.Subject.Boundary = "security" }, true, GateEscalate},
		{"R2-delivery", func(s *SourceScreen) { s.Subject.Boundary = "delivery" }, true, GateEscalate},
		{"R2-identity", func(s *SourceScreen) { s.Subject.Boundary = "identity" }, true, GateEscalate},
		{"R3-below", func(s *SourceScreen) {
			s.Confidence = .89999
			s.Findings = []ScreenFinding{sourceFinding("block", "block", 1)}
		}, true, GateEscalate},
		{"R4-equal-threshold", func(s *SourceScreen) {
			s.Confidence = .9
			s.Findings = []ScreenFinding{sourceFinding("block", "block", .9)}
		}, true, GateBlock},
		{"R4-before-low-major", func(s *SourceScreen) {
			s.Findings = []ScreenFinding{sourceFinding("block", "block", 1), sourceFinding("major", "major", .1)}
		}, true, GateBlock},
		{"R5-high-major", func(s *SourceScreen) { s.Findings = []ScreenFinding{sourceFinding("major", "major", 1)} }, true, GateEscalate},
		{"R5-low-major", func(s *SourceScreen) { s.Findings = []ScreenFinding{sourceFinding("major", "major", 0)} }, true, GateEscalate},
		{"R6-minor-nit", func(s *SourceScreen) {
			s.Findings = []ScreenFinding{sourceFinding("minor", "minor", 1), sourceFinding("nit", "nit", .1)}
		}, true, GateChanges},
		{"R7-score85", func(s *SourceScreen) { s.FeatherWeight = 85; s.Confidence = .9 }, true, GatePass},
		{"R8-score84", func(s *SourceScreen) { s.FeatherWeight = 84 }, true, GateEscalate},
		{"R8-unconfirmed-block", func(s *SourceScreen) { s.Findings = []ScreenFinding{sourceFinding("block", "block", .89)} }, true, GateEscalate},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := sourceFixture()
			tc.change(&s)
			if tc.qualified {
				s.Model.Calibration = "qualified"
			}
			v, err := screenSource(s, s.Subject.HeadSHA, tc.qualified)
			if err != nil || v.Gate != tc.gate {
				t.Fatalf("gate=%s err=%v want=%s", v.Gate, err, tc.gate)
			}
			if v.FloorEvidence != declaredFloorEvidence {
				t.Fatal("declared floor became executed proof")
			}
			if tc.qualified {
				if _, err := ScreenSource(s, s.Subject.HeadSHA); err == nil {
					t.Fatal("production accepted fixture qualification")
				}
			} else if err := ValidateMaatVerdict(v); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSourceScreenRejectsInvalidSemanticInputs(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*SourceScreen)
	}{
		{"version", func(s *SourceScreen) { s.SchemaVersion = 3 }},
		{"contract", func(s *SourceScreen) { s.Contract = "unknown" }},
		{"host", func(s *SourceScreen) { s.Subject.Kind = "host" }},
		{"boundary", func(s *SourceScreen) { s.Subject.Boundary = "other" }},
		{"score-negative", func(s *SourceScreen) { s.FeatherWeight = -1 }},
		{"score-high", func(s *SourceScreen) { s.FeatherWeight = 101 }},
		{"NaN", func(s *SourceScreen) { s.Confidence = math.NaN() }},
		{"Inf", func(s *SourceScreen) { s.Confidence = math.Inf(1) }},
		{"negative-confidence", func(s *SourceScreen) { s.Confidence = -.1 }},
		{"high-confidence", func(s *SourceScreen) { s.Confidence = 1.1 }},
		{"qualified", func(s *SourceScreen) { s.Model.Calibration = "qualified" }},
		{"calibration-absent", func(s *SourceScreen) { s.Model.Calibration = "" }},
		{"recipe-absent", func(s *SourceScreen) { s.Model.EvidenceSetDigest = "" }},
		{"calibration-id", func(s *SourceScreen) { s.Model.CalibrationID = "unknown" }},
		{"finding-evidence", func(s *SourceScreen) {
			s.Findings = []ScreenFinding{sourceFinding("x", "minor", 1)}
			s.Findings[0].EvidenceID = ""
		}},
		{"severity", func(s *SourceScreen) { s.Findings = []ScreenFinding{sourceFinding("x", "unknown", 1)} }},
		{"finding-confidence", func(s *SourceScreen) { s.Findings = []ScreenFinding{sourceFinding("x", "minor", math.Inf(-1))} }},
		{"duplicate-finding", func(s *SourceScreen) { f := sourceFinding("x", "minor", 1); s.Findings = []ScreenFinding{f, f} }},
		{"host-repair", func(s *SourceScreen) {
			f := sourceFinding("x", "minor", 1)
			f.RepairID = SystemOneRepairLaunchdDisabled
			s.Findings = []ScreenFinding{f}
		}},
		{"floor-unknown", func(s *SourceScreen) { s.Floor.Checks[0].Name = "unknown" }},
		{"floor-duplicate", func(s *SourceScreen) { s.Floor.Checks[0].Name = s.Floor.Checks[1].Name }},
		{"floor-contradiction", func(s *SourceScreen) { s.Floor.Checks[0].Passed = false; s.Floor.Checks[0].Detail = "failed" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := sourceFixture()
			tc.change(&s)
			if _, err := ScreenSource(s, s.Subject.HeadSHA); err == nil {
				t.Fatal("invalid source admitted")
			}
		})
	}
	s := sourceFixture()
	for _, head := range []string{"", strings.Repeat("f", 40)} {
		if _, err := ScreenSource(s, head); err == nil {
			t.Fatal("missing/stale expected head admitted")
		}
	}
	// Changing a version/recipe cannot create qualification; all still escalate.
	s.Model.Version = "new-version"
	s.Model.EvidenceSetDigest = strings.Repeat("e", 64)
	if v, err := ScreenSource(s, s.Subject.HeadSHA); err != nil || v.Gate != GateEscalate {
		t.Fatalf("new recipe: %+v %v", v, err)
	}
}

const sourceJSONFixture = `{"contract":"maat.adr070.source-screen","schema_version":2,"subject":{"kind":"commit","repo":"fixture/repo","ref":"main","head_sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","boundary":""},"feather_weight":92,"confidence":0.96,"findings":[{"id":"x","severity":"minor","category":"correctness","file":"fixture.go","line":0,"claim":"fixture","evidence":"inert proof","evidence_id":"dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd","confidence":0}],"floor":{"passed":true,"checks":[{"name":"gofmt","passed":true},{"name":"vet","passed":true},{"name":"golangci-lint","passed":true},{"name":"test-race","passed":true}]},"model":{"provider":"local:fixture","version":"fixture-v1","local":false,"latency_ms":0,"evidence_set_digest":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","calibration_id":"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc","calibration":"unqualified"}}`

func TestSourceScreenStrictJSONRequiredClosedKeys(t *testing.T) {
	if s, err := DecodeSourceScreen([]byte(sourceJSONFixture)); err != nil {
		t.Fatal(err)
	} else if _, err = ScreenSource(s, s.Subject.HeadSHA); err != nil {
		t.Fatal(err)
	}
	// Every required key at every nested object: omission, null, case alias,
	// semantic duplicate and unknown extras, including required zero/false scalars.
	var document map[string]any
	if err := json.Unmarshal([]byte(sourceJSONFixture), &document); err != nil {
		t.Fatal(err)
	}
	var walk func(map[string]any, string)
	walk = func(object map[string]any, path string) {
		keys := make([]string, 0, len(object))
		for key := range object {
			keys = append(keys, key)
		}
		for _, key := range keys {
			value := object[key]
			for _, mode := range []string{"missing", "null", "alias", "duplicate-alias"} {
				t.Run(path+key+"/"+mode, func(t *testing.T) {
					delete(object, key)
					switch mode {
					case "null":
						object[key] = nil
					case "alias":
						object[strings.ToUpper(key)] = value
					case "duplicate-alias":
						object[key] = value
						object[strings.ToUpper(key)] = value
					}
					raw, err := json.Marshal(document)
					if err != nil {
						t.Fatal(err)
					}
					if _, err = DecodeSourceScreen(raw); err == nil {
						t.Fatal("invalid required/exact key admitted")
					}
					delete(object, strings.ToUpper(key))
					object[key] = value
				})
			}
			switch v := value.(type) {
			case map[string]any:
				walk(v, path+key+".")
			case []any:
				if len(v) > 0 {
					if child, ok := v[0].(map[string]any); ok {
						walk(child, path+key+"[].")
					}
				}
			}
		}
	}
	// Local document rather than any production input/provider; never parallel.
	walk(document, "")
	for _, bad := range []string{
		strings.Replace(sourceJSONFixture, `"schema_version":2`, `"schema_version":2,"schema_version":2`, 1),
		strings.Replace(sourceJSONFixture, `"line":0`, `"line":0,"extra":0`, 1),
		sourceJSONFixture + ` {}`, sourceJSONFixture + ` trailing`, sourceJSONFixture[:len(sourceJSONFixture)-1],
		strings.Replace(sourceJSONFixture, `"confidence":0.96`, `"confidence":"0.96"`, 1),
		sourceJSONFixture + strings.Repeat(" ", SourceScreenMaxBytes),
	} {
		if _, err := DecodeSourceScreen([]byte(bad)); err == nil {
			t.Fatal("malformed/duplicate/trailing/oversized input admitted")
		}
	}
}

func TestSourceScreenJournalAndLegacyCompatibility(t *testing.T) {
	j := &FileDecisionJournal{Path: filepath.Join(t.TempDir(), "decisions.jsonl")}
	legacy, err := Screen(validSystemOneInput())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = RecordSystemOne(j, "fixture-legacy", legacy); err != nil {
		t.Fatal(err)
	}
	s := sourceFixture()
	v, err := ScreenSource(s, s.Subject.HeadSHA)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = RecordSystemOne(j, "fixture-v2", v); err != nil {
		t.Fatal(err)
	}
	rows, err := j.Recent(10)
	if err != nil || len(rows) != 2 {
		t.Fatalf("strict reader: %+v %v", rows, err)
	}
	if rows[0].SystemOne.Gate != GateEscalate || rows[1].SystemOne.Gate != GatePass {
		t.Fatal("v2/v1 policy crossed")
	}
	for _, gate := range []GateDecision{GatePass, GateChanges, GateBlock} {
		forged := v
		forged.Gate = gate
		if err := ValidateMaatVerdict(forged); err == nil {
			t.Fatal("forged v2 gate admitted")
		}
	}
	v.Model.Calibration = "qualified"
	if _, err = RecordSystemOne(j, "fixture-forged", v); err == nil {
		t.Fatal("qualified journal bypass")
	}
	// This reader compatibility test never reads the actual installed journal.
	if _, err = os.Stat(j.Path); err != nil {
		t.Fatal(err)
	}
}
