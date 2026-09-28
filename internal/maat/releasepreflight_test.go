package maat

import (
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
	if err := os.WriteFile(workflow, []byte("scripts/build-dmg.sh --release scripts/build-pkg.sh --release Publish canonical Homebrew Cask cask-release render cask-release verify perl -0pi"), 0o600); err != nil {
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
	files := map[string]string{
		"scripts/build-dmg.sh":          "--development --release DEVELOPER_ID_APPLICATION APPLE_ID APPLE_TEAM_ID APPLE_APP_PASSWORD SirsiPantheon-${VERSION}-dev-${ARCH}.dmg xcrun notarytool submit xcrun stapler validate",
		"scripts/build-pkg.sh":          "--development --release DEVELOPER_ID_INSTALLER APPLE_ID APPLE_TEAM_ID APPLE_APP_PASSWORD SirsiPantheon-${VERSION}-dev-${ARCH}.pkg xcrun notarytool submit xcrun stapler validate",
		".github/workflows/release.yml": "scripts/build-dmg.sh --release scripts/build-pkg.sh --release Preflight imported Developer ID Installer identity DEVELOPER_ID_INSTALLER is required for the commercial Pantheon PKG Temporary signing keychain does not contain a usable Team 9D382WV988 Developer ID Installer identity Configured Developer ID Installer identity does not match the identity imported into the temporary signing keychain Publish canonical Homebrew Cask cask-release render cask-release verify",
		"Makefile":                      "dmg-dev:\npkg-dev:\nrelease-dmg:\nrelease-pkg:\nmacapp/Package.swift\nStackLab",
		"contracts/stacklab/pantheon-release-artifact-recipe-v1.json": "stacklab.recipe.pantheon-release-artifact release-artifact-class-contract release-native-payload-composition commercial-sign-notary-publication-route canonical-cask-publication",
		"internal/caskrelease/cask.go":                                "func Render func Verify exact canonical rendering",
		"cmd/sirsi/cask_release.go":                                   "cask-release caskrelease.Render caskrelease.Verify",
		"scripts/verify-commercial-release-contract.sh":               "commercial release contract: pass pantheon-release-artifact-recipe-v1.json",
	}
	if broken {
		files["scripts/build-dmg.sh"] = "--development"
	}
	for relative, content := range files {
		path := filepath.Join(root, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}
