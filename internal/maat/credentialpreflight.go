package maat

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strings"
)

// PantheonDeveloperTeamID is the team that owns Pantheon's commercial
// Developer ID artifacts. This is public certificate metadata, never a key.
const PantheonDeveloperTeamID = "9D382WV988"

// ReleaseCredentialPreflight is a deliberately narrow, non-secret readiness
// observation. It can establish whether the two local Developer ID identities
// are usable, but it never reads a private key, a keychain password, or an
// Apple-notarization credential, and it never contacts Apple.
type ReleaseCredentialPreflight struct {
	SchemaVersion        int                      `json:"schema_version"`
	TeamID               string                   `json:"team_id"`
	Fingerprint          string                   `json:"fingerprint"`
	DeveloperIdentities  []ReleaseSigningIdentity `json:"developer_identities"`
	NotarizationObserved bool                     `json:"notarization_observed"`
	Verdict              MaatVerdict              `json:"verdict"`
}

// ReleaseSigningIdentity is public certificate metadata emitted by macOS's
// security tool. Fingerprints make the observed identity reviewable without
// exposing certificate or private-key bytes.
type ReleaseSigningIdentity struct {
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Fingerprint string `json:"fingerprint"`
}

var securityFindIdentity = func() ([]byte, error) {
	return exec.Command("/usr/bin/security", "find-identity", "-v").CombinedOutput()
}

var securityIdentityLine = regexp.MustCompile(`^\s*\d+\)\s+([0-9A-F]{40})\s+"([^"]+)"\s*$`)

// PreflightReleaseCredentials observes only public local certificate names and
// fingerprints. A green source floor is not a credential proof; conversely,
// this result is intentionally blocked until the protected workflow verifies
// notarization separately. That gives the operator an exact next action rather
// than an opaque "release unavailable" state.
func PreflightReleaseCredentials() (ReleaseCredentialPreflight, error) {
	raw, runErr := securityFindIdentity()
	identities := parseDeveloperIDIdentities(string(raw), PantheonDeveloperTeamID)
	checks := []FloorCheck{
		{Name: "developer-id-application", Passed: hasIdentity(identities, "application"), Detail: "one usable Team " + PantheonDeveloperTeamID + " Developer ID Application identity"},
		{Name: "developer-id-installer", Passed: hasIdentity(identities, "installer"), Detail: "one usable Team " + PantheonDeveloperTeamID + " Developer ID Installer identity"},
		{Name: "notarization-credential", Passed: false, Detail: "not observed: protected notarization workflow is required"},
	}
	if runErr != nil {
		checks[0].Passed = false
		checks[1].Passed = false
		checks[0].Detail = "macOS security identity observation failed"
		checks[1].Detail = "macOS security identity observation failed"
	}
	findings := make([]ScreenFinding, 0, 3)
	if !checks[0].Passed {
		findings = append(findings, credentialFinding("developer-id-application", "A usable Team "+PantheonDeveloperTeamID+" Developer ID Application identity was not observed.", "Make one usable Team "+PantheonDeveloperTeamID+" Developer ID Application identity available through the protected release workflow."))
	}
	if !checks[1].Passed {
		findings = append(findings, credentialFinding("developer-id-installer", "A usable Team "+PantheonDeveloperTeamID+" Developer ID Installer identity was not observed.", "Make one usable Team "+PantheonDeveloperTeamID+" Developer ID Installer identity available through the protected release workflow."))
	}
	findings = append(findings, credentialFinding("notarization-credential", "Notarization readiness was intentionally not inspected: Ma'at does not read secrets or contact Apple in this preflight.", "Use the protected release workflow to validate one complete notarization credential set without exposing its secrets."))
	if runErr != nil {
		findings = append(findings, credentialFinding("security-identity-observation", "macOS could not complete the public identity observation.", "Unlock the release keychain/session, then re-run this readiness check. No identity was inferred."))
	}

	fingerprint := credentialFingerprint(identities, runErr)
	floorPassed := true
	for _, check := range checks {
		floorPassed = floorPassed && check.Passed
	}
	verdict, err := Screen(SystemOneScreen{
		Subject:       VerdictSubject{Kind: "host", Repo: "local-macos", Ref: "release-credential-readiness", HeadSHA: "release-credentials:sha256=" + fingerprint, Boundary: "delivery"},
		FeatherWeight: 100,
		Confidence:    1,
		Findings:      findings,
		Floor:         FloorResult{Passed: floorPassed, Checks: checks},
		Model:         ModelStamp{Provider: "local:maat-release-credentials", Version: "v1", Local: true},
	})
	if err != nil {
		return ReleaseCredentialPreflight{}, fmt.Errorf("maat credential preflight: construct verdict: %w", err)
	}
	return ReleaseCredentialPreflight{SchemaVersion: SystemOneSchemaVersion, TeamID: PantheonDeveloperTeamID, Fingerprint: "sha256=" + fingerprint, DeveloperIdentities: identities, NotarizationObserved: false, Verdict: verdict}, nil
}

func parseDeveloperIDIdentities(raw, teamID string) []ReleaseSigningIdentity {
	identities := make([]ReleaseSigningIdentity, 0, 2)
	for _, line := range strings.Split(raw, "\n") {
		match := securityIdentityLine.FindStringSubmatch(line)
		if len(match) != 3 {
			continue
		}
		kind := ""
		switch {
		case strings.HasPrefix(match[2], "Developer ID Application: "):
			kind = "application"
		case strings.HasPrefix(match[2], "Developer ID Installer: "):
			kind = "installer"
		}
		if kind == "" || !strings.HasSuffix(match[2], "("+teamID+")") {
			continue
		}
		identities = append(identities, ReleaseSigningIdentity{Kind: kind, Name: match[2], Fingerprint: match[1]})
	}
	sort.Slice(identities, func(i, j int) bool {
		if identities[i].Kind == identities[j].Kind {
			return identities[i].Fingerprint < identities[j].Fingerprint
		}
		return identities[i].Kind < identities[j].Kind
	})
	return identities
}

func hasIdentity(identities []ReleaseSigningIdentity, kind string) bool {
	return anyIdentity(identities, kind)
}

func anyIdentity(identities []ReleaseSigningIdentity, kind string) bool {
	for _, identity := range identities {
		if identity.Kind == kind {
			return true
		}
	}
	return false
}

func credentialFingerprint(identities []ReleaseSigningIdentity, runErr error) string {
	hash := sha256.New()
	for _, identity := range identities {
		fmt.Fprintf(hash, "%s\x00%s\x00%s\n", identity.Kind, identity.Name, identity.Fingerprint)
	}
	if runErr != nil {
		fmt.Fprint(hash, "security-observation-error\n")
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func credentialFinding(id, claim, hint string) ScreenFinding {
	return ScreenFinding{ID: id, Severity: "block", Category: "release-credentials", Claim: claim, Evidence: "local-public-certificate-observation", Confidence: 1, FixHint: hint}
}
