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
	if !strings.Contains(claims, "Apple Distribution") || !strings.Contains(claims, "cannot substitute") {
		t.Fatalf("identity type mismatch was not actionable: %+v", preflight.Verdict.Findings)
	}
	steps := strings.Join(preflight.RecoveryPlan, "\n")
	if !strings.Contains(steps, "Developer ID Application") || !strings.Contains(steps, "Developer ID Installer") || !strings.Contains(steps, "notarization credential") {
		t.Fatalf("recovery plan = %q", preflight.RecoveryPlan)
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
	securityFindIdentity = func() ([]byte, error) { return nil, errors.New("keychain unavailable") }
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
