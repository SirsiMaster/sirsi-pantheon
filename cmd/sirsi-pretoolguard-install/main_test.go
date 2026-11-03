package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func fixtureIdentity(inode uint64, hash string) string {
	b, _ := json.Marshal(identity{1, inode, 501, 20, 0755, strings.Repeat(hash, 64)})
	return string(b)
}

func TestExactSelectorsAndRequests(t *testing.T) {
	for _, target := range []string{"codex-user", "claude-user", "codex-system"} {
		args := []string{"-action", "install", "-target", target, "-artifact", "/fixture", "-expected-current", fixtureIdentity(2, "a"), "-expected-original-provenance", `{"provenance_present":false,"provenance_sha256":""}`, "-expected-replacement-sha256", strings.Repeat("b", 64), "-expected-code-identifier", "invalid.example.fixture", "-expected-team-id", "9D382WV988"}
		if _, err := parse(args); err != nil {
			t.Fatalf("%s: %v", target, err)
		}
		args[3] = "/etc/codex/hooks/" + "sirsi-display-power-guard"
		if _, err := parse(args); err == nil {
			t.Fatal("path selector accepted")
		}
	}
	if _, err := parse([]string{"-action", "rollback", "-target", "codex-user", "-expected-current", fixtureIdentity(2, "a")}); err == nil {
		t.Fatal("missing transaction and backup accepted")
	}
}

func TestInstallRequiresIndependentProvenance(t *testing.T) {
	args := []string{"-action", "install", "-target", "codex-user", "-artifact", "/fixture", "-expected-current", fixtureIdentity(2, "a"), "-expected-replacement-sha256", strings.Repeat("b", 64), "-expected-code-identifier", "invalid.example.fixture", "-expected-team-id", "9D382WV988"}
	if _, err := parse(args); err == nil {
		t.Fatal("missing expected provenance accepted")
	}
	for _, proof := range []string{`{}`, `{"provenance_present":true,"provenance_sha256":""}`, `{"provenance_present":false,"provenance_sha256":"` + strings.Repeat("a", 64) + `"}`} {
		if _, err := parse(append(append([]string{}, args...), "-expected-original-provenance", proof)); err == nil {
			t.Fatalf("invalid proof accepted: %s", proof)
		}
	}
	for _, proof := range []string{`{"provenance_present":false,"provenance_sha256":""}`, `{"provenance_present":true,"provenance_sha256":"` + strings.Repeat("a", 64) + `"}`} {
		if _, err := parse(append(append([]string{}, args...), "-expected-original-provenance", proof)); err != nil {
			t.Fatalf("valid proof rejected: %v", err)
		}
	}
	proof := `{"provenance_present":true,"provenance_sha256":"` + strings.Repeat("a", 64) + `"}`
	if err := checkExpectedProvenance(proof, metadata{ProvenancePresent: true, ProvenanceSHA256: strings.Repeat("a", 64)}); err != nil {
		t.Fatal(err)
	}
	if checkExpectedProvenance(proof, metadata{ProvenancePresent: true, ProvenanceSHA256: strings.Repeat("b", 64)}) == nil || checkExpectedProvenance(proof, metadata{}) == nil {
		t.Fatal("mismatched independent expectation accepted")
	}
}
