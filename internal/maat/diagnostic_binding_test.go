package maat

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestDiagnosticHostBindingClosedAndLegacyExact(t *testing.T) {
	host := "fixture-mac"
	sum := sha256.Sum256([]byte(host))
	base := VerdictSubject{Kind: "host", Ref: "diagnostic", Repo: "local-host-" + hex.EncodeToString(sum[:]), HostBinding: DiagnosticHostBinding}
	for _, tc := range []struct {
		name    string
		subject VerdictSubject
		host    string
		want    bool
	}{
		{"typed same", base, host, true}, {"typed foreign", base, "other-mac", false},
		{"typed missing", base, "", false},
		{"legacy same", VerdictSubject{Kind: "host", Ref: "diagnostic", Repo: host}, host, true},
		{"legacy foreign", VerdictSubject{Kind: "host", Ref: "diagnostic", Repo: host}, "other-mac", false},
		{"legacy hash cannot infer format", VerdictSubject{Kind: "host", Ref: "diagnostic", Repo: base.Repo}, host, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := DiagnosticHostMatches(tc.subject, tc.host); got != tc.want {
				t.Fatalf("matches=%v want%v", got, tc.want)
			}
		})
	}
	for _, mutate := range []func(*VerdictSubject){func(s *VerdictSubject) { s.HostBinding = "unknown" }, func(s *VerdictSubject) { s.Repo += "x" }, func(s *VerdictSubject) { s.Ref = "source" }, func(s *VerdictSubject) { s.Repo = strings.ToUpper(s.Repo) }} {
		value := base
		mutate(&value)
		if DiagnosticHostMatches(value, host) {
			t.Fatal("malformed binding accepted")
		}
	}
}

func TestDiagnosticUnknownCodePreservesFreshCanonicalIdentityWithoutProse(t *testing.T) {
	for _, check := range []string{"Memory Death Spiral", " /Users/PRIVATE_UNKNOWN_CHECK --token SECRET_CANARY "} {
		code := DiagnosticCheckCode(check)
		o := DiagnosticObservation{Schema: DiagnosticObservationSchema, SourceSHA256: strings.Repeat("a", 64), HostSHA256: strings.Repeat("b", 64), ObservedAtUTC: time.Now().UTC(), Score: 70, Statuses: []DiagnosticStatus{{Check: code, Severity: 2}}}
		v, err := o.Verdict()
		if err != nil {
			t.Fatal(err)
		}
		if len(v.Findings) != 1 || v.Findings[0].ID != CanonicalHostFindingID(check) || v.Findings[0].Evidence != CanonicalHostFindingEvidence(o.SourceSHA256, check) {
			t.Fatalf("fresh-check identity changed: %+v", v.Findings)
		}
		if !IsCanonicalHostDiagnosticFinding(v, v.Findings[0], check) {
			t.Fatal("actual canonical consumer rejected projection")
		}
		raw, _ := json.Marshal(struct {
			Observation DiagnosticObservation
			Verdict     MaatVerdict
		}{o, v})
		for _, secret := range []string{check, "PRIVATE_UNKNOWN_CHECK", "SECRET_CANARY", "/Users/"} {
			if strings.Contains(string(raw), secret) {
				t.Fatal("unknown producer text leaked")
			}
		}
		if IsCanonicalHostDiagnosticFinding(v, v.Findings[0], check+"changed") {
			t.Fatal("changed fresh check matched")
		}
	}
}

func TestDiagnosticHistoricalV1ProjectionIsNotSilentlyRewritten(t *testing.T) {
	code := DiagnosticCheckCode("Memory Death Spiral")
	o := DiagnosticObservation{Schema: diagnosticObservationSchemaV1, SourceSHA256: strings.Repeat("a", 64), HostSHA256: strings.Repeat("b", 64), ObservedAtUTC: time.Now().UTC(), Score: 70, Statuses: []DiagnosticStatus{{Check: code, Severity: 2}}}
	old, err := o.Verdict()
	if err != nil {
		t.Fatal(err)
	}
	if old.Subject.HostBinding != "" || old.Findings[0].ID != CanonicalHostFindingID(code) || old.Findings[0].Evidence != CanonicalHostFindingEvidence(o.SourceSHA256, code) {
		t.Fatal("historical bytes were upgraded")
	}
	o.Schema = DiagnosticObservationSchema
	fresh, err := o.Verdict()
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Subject.HostBinding != DiagnosticHostBinding || fresh.Findings[0].ID != CanonicalHostFindingID("Memory Death Spiral") {
		t.Fatal("versioned new producer not corrected")
	}
}
