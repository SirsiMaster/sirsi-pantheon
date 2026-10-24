package maat

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPreflightReleaseContractConstructsAStableDeliveryVerdict(t *testing.T) {
	root := makeReleaseContractFixture(t, false)
	first, err := PreflightReleaseContract(root)
	if err != nil {
		t.Fatal(err)
	}
	second, err := PreflightReleaseContract(root)
	if err != nil {
		t.Fatal(err)
	}
	if first.Fingerprint != second.Fingerprint || first.Verdict.Gate != GateEscalate || first.Verdict.Floor.Passed != true {
		t.Fatalf("first=%+v second=%+v", first, second)
	}
	if first.Verdict.Subject.Boundary != "delivery" || !strings.Contains(first.Verdict.Subject.HeadSHA, first.Fingerprint) {
		t.Fatalf("subject = %+v", first.Verdict.Subject)
	}
}

func TestPreflightReleaseContractBlocksAChangedOrMissingContract(t *testing.T) {
	root := makeReleaseContractFixture(t, true)
	preflight, err := PreflightReleaseContract(root)
	if err != nil {
		t.Fatal(err)
	}
	if preflight.Verdict.Gate != GateBlock || preflight.Verdict.Floor.Passed || len(preflight.Verdict.Findings) == 0 {
		t.Fatalf("preflight = %+v", preflight)
	}
	if preflight.Verdict.Findings[0].FixHint == "" || preflight.Verdict.Findings[0].Evidence == "" {
		t.Fatalf("finding = %+v", preflight.Verdict.Findings[0])
	}
}

func TestPreflightReleaseContractRejectsSymlinkedContractInput(t *testing.T) {
	root := makeReleaseContractFixture(t, false)
	source := filepath.Join(root, "scripts", "build-dmg.sh")
	copy := filepath.Join(root, "scripts", "build-dmg-copy.sh")
	if err := os.Rename(source, copy); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(copy, source); err != nil {
		t.Fatal(err)
	}
	preflight, err := PreflightReleaseContract(root)
	if err != nil {
		t.Fatal(err)
	}
	if preflight.Verdict.Gate != GateBlock || preflight.Verdict.Floor.Passed {
		t.Fatalf("preflight = %+v", preflight)
	}
}

func TestPreflightReleaseContractBlocksDuplicateCaskPublisher(t *testing.T) {
	root := makeReleaseContractFixture(t, false)
	workflow := filepath.Join(root, ".github", "workflows", "release.yml")
	raw, err := os.ReadFile(workflow)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(workflow, append(raw, []byte("\nperl -0pi\n")...), 0o600); err != nil {
		t.Fatal(err)
	}
	preflight, err := PreflightReleaseContract(root)
	if err != nil {
		t.Fatal(err)
	}
	if preflight.Verdict.Gate != GateBlock || preflight.Verdict.Floor.Passed {
		t.Fatalf("preflight = %+v", preflight)
	}
	for _, finding := range preflight.Verdict.Findings {
		if finding.ID == "canonical-cask-publication-workflow" && strings.Contains(finding.Claim, "perl -0pi") && finding.FixHint != "" {
			return
		}
	}
	t.Fatalf("cask publication finding = %+v", preflight.Verdict.Findings)
}

func makeReleaseContractFixture(t *testing.T, broken bool) string {
	t.Helper()
	root := t.TempDir()
	// Notary/client inputs retain published v0.24.130 bytes (60b9963d73f66ed497e90fa127263cc1113f506a).
	// The builders deliberately bind the reviewed corrective134 mode/upgrade
	// source at6364a72f58847744716d3168f34b8d53d92e9f0c, not private-mode133.
	// The workflow pin adds draft-first publication on released132 base
	// 08843c54272a7ad6f6d2a431724bad98123d823f, not a policy stub.
	// The successor to published134 ba2a07bb adds only explicit Python -B
	// invocation flags to these three pinned scripts; signing/cask policy stays.
	// No copied script is executed. Future route changes must deliberately
	// update this provenance and the observer together.
	pinned := map[string]string{
		"scripts/build-dmg.sh":                 "aad1c9c9d0404e96e6e80a54e83c1992f7094b359dac282eb896cbaa0b05e3c4",
		"scripts/build-pkg.sh":                 "5a95ed756f6b632b044c12bbdfd18eb81e327bb649f03293eceb01135b4f627a",
		"scripts/notarize-release-artifact.sh": "cf4112f04f90f1b8ac70e9aecd17ed87652e1718bb82dcecc6801efd03eab433",
		".github/workflows/release.yml":        "358eb25b8064ca362254809d53ff2d8aaaf4bb09563483f629b5edf3c57715ee",
		"tools/signing/sirsi-sign":             "805123e96ce78ff225641bebd2bf32f9c15d5023f381963f096facda4d2aedd1",
	}
	for _, requirement := range releaseContractRequirements {
		relative := requirement.Path
		content, err := os.ReadFile(filepath.Join("..", "..", filepath.FromSlash(relative)))
		if err != nil {
			t.Fatal(err)
		}
		if expected, ok := pinned[relative]; ok && fmt.Sprintf("%x", sha256.Sum256(content)) != expected {
			t.Fatalf("pinned release source changed: %s; update provenance and contract deliberately", relative)
		}
		path := filepath.Join(root, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, content, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if broken {
		if err := os.WriteFile(filepath.Join(root, "scripts", "build-dmg.sh"), []byte("--development"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestPreflightReleaseContractMissingControlsStillBlock(t *testing.T) {
	for _, tc := range []struct{ name, path, control, check string }{
		{"explicit-mode", "scripts/build-dmg.sh", `[[ -n "$MODE" ]] ||`, "dmg-explicit-artifact-mode"},
		{"mutually-exclusive-mode", "scripts/build-pkg.sh", `[[ -z "$MODE" ]] ||`, "pkg-explicit-artifact-mode"},
		{"application-identity", "scripts/build-dmg.sh", `for required in DEVELOPER_ID_APPLICATION; do`, "dmg-explicit-artifact-mode"},
		{"installer-identity", "scripts/build-pkg.sh", `for required in DEVELOPER_ID_INSTALLER; do`, "pkg-explicit-artifact-mode"},
		{"remote-client", "scripts/build-dmg.sh", `[[ -x "${PANTHEON_SIGN_CLIENT:-}" ]] ||`, "dmg-explicit-artifact-mode"},
		{"notary-delegation", "scripts/build-pkg.sh", `"${PROJECT_ROOT}/scripts/notarize-release-artifact.sh" "$PKG_PATH"`, "pkg-explicit-artifact-mode"},
		{"notary-wait", "scripts/notarize-release-artifact.sh", `--timeout 20m --wait`, "delegated-notarization-contract"},
		{"notary-failure", "scripts/notarize-release-artifact.sh", `exit "$status"`, "delegated-notarization-contract"},
		{"staple-validation", "scripts/build-dmg.sh", `xcrun stapler validate "${DMG_PATH}"`, "dmg-explicit-artifact-mode"},
		{"remote-return-identity", "tools/signing/sirsi-sign", `[ $vok = 1 ] ||`, "delegated-signing-return-verification"},
		{"selected-keychain-existence", ".github/workflows/release.yml", `test -f "$KEYCHAIN"`, "installer-identity-keychain-preflight"},
		{"selected-keychain-absence-refusal", ".github/workflows/release.yml", `|| { echo "Selected signing keychain is absent: $KEYCHAIN" >&2; exit 1; }`, "installer-identity-keychain-preflight"},
		{"selected-keychain-comparison", ".github/workflows/release.yml", `if [ "$IMPORTED_INSTALLER" != "$DEVELOPER_ID_INSTALLER" ]; then`, "installer-identity-keychain-preflight"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := makeReleaseContractFixture(t, false)
			path := filepath.Join(root, filepath.FromSlash(tc.path))
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(raw), tc.control) {
				t.Fatalf("mutation target absent: %q", tc.control)
			}
			if err := os.WriteFile(path, []byte(strings.ReplaceAll(string(raw), tc.control, "REMOVED_CONTROL")), 0o600); err != nil {
				t.Fatal(err)
			}
			result, err := PreflightReleaseContract(root)
			if err != nil {
				t.Fatal(err)
			}
			if result.Verdict.Gate != GateBlock || result.Verdict.Floor.Passed {
				t.Fatalf("missing control accepted: %+v", result)
			}
			if len(result.Verdict.Findings) != 1 || result.Verdict.Findings[0].ID != tc.check {
				t.Fatalf("not a causal single-check refusal: %+v", result.Verdict.Findings)
			}
		})
	}
}
