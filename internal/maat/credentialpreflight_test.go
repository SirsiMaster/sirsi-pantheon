package maat

import (
	"errors"
	"strings"
	"testing"
)

func TestPreflightReleaseCredentialsBindsRequiredDeveloperIDIdentities(t *testing.T) {
	original := securityFindIdentity
	t.Cleanup(func() { securityFindIdentity = original })
	securityFindIdentity = func() ([]byte, error) {
		return []byte(`  1) ABCDEF0123456789ABCDEF0123456789ABCDEF01 "Developer ID Application: Sirsi Technologies Inc. (9D382WV988)"
  2) 0123456789ABCDEF0123456789ABCDEF01234567 "Developer ID Installer: Sirsi Technologies Inc. (9D382WV988)"
     2 valid identities found
`), nil
	}
	preflight, err := PreflightReleaseCredentials()
	if err != nil {
		t.Fatal(err)
	}
	if len(preflight.DeveloperIdentities) != 2 || !hasIdentity(preflight.DeveloperIdentities, "application") || !hasIdentity(preflight.DeveloperIdentities, "installer") {
		t.Fatalf("identities = %+v", preflight.DeveloperIdentities)
	}
	if len(preflight.ObservedNonDeveloperIdentityTypes) != 0 {
		t.Fatalf("unexpected non-Developer ID types: %+v", preflight.ObservedNonDeveloperIdentityTypes)
	}
	if preflight.NotarizationObserved || preflight.Verdict.Floor.Passed || preflight.Verdict.Gate != GateBlock {
		t.Fatalf("preflight = %+v", preflight)
	}
	if !strings.HasPrefix(preflight.Fingerprint, "sha256=") {
		t.Fatalf("fingerprint = %q", preflight.Fingerprint)
	}
	if got, want := preflight.RecoveryPlan, []string{
		"Have the protected release workflow validate one complete notarization credential set without exposing its secrets to Ma'at.",
		"Recheck readiness, then run the signed, notarized DMG and PKG workflow only after every required proof is present.",
	}; !sameStrings(got, want) {
		t.Fatalf("recovery plan = %q, want %q", got, want)
	}
}

func TestPreflightReleaseCredentialsRejectsWrongTeamAndUnrelatedIdentity(t *testing.T) {
	original := securityFindIdentity
	t.Cleanup(func() { securityFindIdentity = original })
	securityFindIdentity = func() ([]byte, error) {
		return []byte(`  1) ABCDEF0123456789ABCDEF0123456789ABCDEF01 "Developer ID Application: Other Corp (0000000000)"
  2) 0123456789ABCDEF0123456789ABCDEF01234567 "Apple Distribution: Sirsi Technologies Inc. (9D382WV988)"
`), nil
	}
	preflight, err := PreflightReleaseCredentials()
	if err != nil {
		t.Fatal(err)
	}
	if len(preflight.DeveloperIdentities) != 0 || preflight.Verdict.Floor.Passed {
		t.Fatalf("preflight = %+v", preflight)
	}
	if got, want := preflight.ObservedNonDeveloperIdentityTypes, []string{"Apple Distribution"}; !sameStrings(got, want) {
		t.Fatalf("observed non-Developer ID types = %v, want %v", got, want)
	}
	claims := strings.Join(findingClaims(preflight.Verdict.Findings), "\n")
	if !strings.Contains(claims, "Developer ID Application") || !strings.Contains(claims, "Developer ID Installer") {
		t.Fatalf("findings = %+v", preflight.Verdict.Findings)
	}
	if strings.Contains(claims, "Apple Distribution") || strings.Contains(claims, "cannot substitute") {
		t.Fatalf("extra identity type became a finding: %+v", preflight.Verdict.Findings)
	}
	steps := strings.Join(preflight.RecoveryPlan, "\n")
	if !strings.Contains(steps, "Developer ID Application") || !strings.Contains(steps, "Developer ID Installer") || !strings.Contains(steps, "notarization credential") {
		t.Fatalf("recovery plan = %q", preflight.RecoveryPlan)
	}
}

func TestPreflightReleaseCredentialsTreatsAdditionalIdentityAsNeutralMetadata(t *testing.T) {
	original := securityFindIdentity
	t.Cleanup(func() { securityFindIdentity = original })
	securityFindIdentity = func() ([]byte, error) {
		return []byte(`  1) ABCDEF0123456789ABCDEF0123456789ABCDEF01 "Developer ID Application: Sirsi Technologies Inc. (9D382WV988)"
  2) 0123456789ABCDEF0123456789ABCDEF01234567 "Developer ID Installer: Sirsi Technologies Inc. (9D382WV988)"
  3) 1123456789ABCDEF0123456789ABCDEF01234567 "Apple Distribution: Sirsi Technologies Inc. (9D382WV988)"
`), nil
	}
	preflight, err := PreflightReleaseCredentials()
	if err != nil {
		t.Fatal(err)
	}
	if len(preflight.DeveloperIdentities) != 2 || !hasIdentity(preflight.DeveloperIdentities, "application") || !hasIdentity(preflight.DeveloperIdentities, "installer") {
		t.Fatalf("required identities = %+v", preflight.DeveloperIdentities)
	}
	if !sameStrings(preflight.ObservedNonDeveloperIdentityTypes, []string{"Apple Distribution"}) {
		t.Fatalf("additional identity metadata = %v", preflight.ObservedNonDeveloperIdentityTypes)
	}
	if preflight.NotarizationObserved || preflight.Verdict.Floor.Passed || preflight.Verdict.Gate != GateBlock {
		t.Fatalf("credential floor/gate changed: %+v", preflight)
	}
	for _, finding := range preflight.Verdict.Findings {
		if finding.ID == "non-developer-identity-observed" || strings.Contains(finding.FixHint, "Developer ID") {
			t.Fatalf("extra identity caused an actionable finding: %+v", finding)
		}
	}
}

func TestPreflightReleaseCredentialsRequestsOnlyMissingRequiredKind(t *testing.T) {
	for _, test := range []struct {
		name       string
		identity   string
		missing    string
		unexpected string
	}{
		{name: "missing installer", identity: `Developer ID Application: Sirsi Technologies Inc. (9D382WV988)`, missing: "Developer ID Installer", unexpected: "Developer ID Application"},
		{name: "missing application", identity: `Developer ID Installer: Sirsi Technologies Inc. (9D382WV988)`, missing: "Developer ID Application", unexpected: "Developer ID Installer"},
	} {
		t.Run(test.name, func(t *testing.T) {
			original := securityFindIdentity
			t.Cleanup(func() { securityFindIdentity = original })
			securityFindIdentity = func() ([]byte, error) {
				return []byte(`  1) ABCDEF0123456789ABCDEF0123456789ABCDEF01 "` + test.identity + `"
  2) 1123456789ABCDEF0123456789ABCDEF01234567 "Apple Distribution: Sirsi Technologies Inc. (9D382WV988)"
`), nil
			}
			preflight, err := PreflightReleaseCredentials()
			if err != nil {
				t.Fatal(err)
			}
			var claims, hints []string
			for _, finding := range preflight.Verdict.Findings {
				claims = append(claims, finding.Claim)
				hints = append(hints, finding.FixHint)
			}
			all := strings.Join(append(claims, hints...), "\n")
			if !strings.Contains(all, test.missing) || strings.Contains(all, test.unexpected) {
				t.Fatalf("findings requested wrong identity kind: %+v", preflight.Verdict.Findings)
			}
		})
	}
}

func TestProtectedReleaseRecoveryPlanDoesNotRequestSecretsWhenCredentialsAreObserved(t *testing.T) {
	identities := []ReleaseSigningIdentity{
		{Kind: "application", Name: "Developer ID Application: Sirsi Technologies Inc. (9D382WV988)"},
		{Kind: "installer", Name: "Developer ID Installer: Sirsi Technologies Inc. (9D382WV988)"},
	}
	got := ProtectedReleaseRecoveryPlan(PantheonDeveloperTeamID, identities, true)
	want := []string{"Recheck readiness, then run the signed, notarized DMG and PKG workflow only after every required proof is present."}
	if !sameStrings(got, want) {
		t.Fatalf("recovery plan = %q, want %q", got, want)
	}
}

func TestPreflightReleaseCredentialsKeepsSecurityObservationFailureActionable(t *testing.T) {
	original := securityFindIdentity
	t.Cleanup(func() { securityFindIdentity = original })
	securityFindIdentity = func() ([]byte, error) {
		return []byte(`  1) ABCDEF0123456789ABCDEF0123456789ABCDEF01 "Developer ID Application: Sirsi Technologies Inc. (9D382WV988)"`), errors.New("keychain unavailable")
	}
	preflight, err := PreflightReleaseCredentials()
	if err != nil {
		t.Fatal(err)
	}
	if preflight.Verdict.Gate != GateBlock || preflight.Verdict.Floor.Passed {
		t.Fatalf("preflight = %+v", preflight)
	}
	claims := strings.Join(findingClaims(preflight.Verdict.Findings), "\n")
	if !strings.Contains(claims, "macOS could not complete") {
		t.Fatalf("findings = %+v", preflight.Verdict.Findings)
	}
	if len(preflight.DeveloperIdentities) != 0 || len(preflight.ObservedNonDeveloperIdentityTypes) != 0 {
		t.Fatalf("partial output was treated as an observation: %+v", preflight)
	}
	if strings.Contains(claims, "was not observed") || strings.Contains(strings.Join(preflight.RecoveryPlan, "\n"), "Developer ID Application identity available") || strings.Contains(strings.Join(preflight.RecoveryPlan, "\n"), "Developer ID Installer identity available") {
		t.Fatalf("observation failure was diagnosed as absence: %+v", preflight)
	}
}

func findingClaims(findings []ScreenFinding) []string {
	claims := make([]string, 0, len(findings))
	for _, finding := range findings {
		claims = append(claims, finding.Claim)
	}
	return claims
}

func sameStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
