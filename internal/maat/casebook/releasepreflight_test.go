package casebook

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
)

func TestReleaseSourcePreflightNextActionDoesNotDiagnoseCredentials(t *testing.T) {
	// Observe/copy source only, never invoke the release route or read a keychain.
	repo := filepath.Join("..", "..", "..")
	baseline, err := maat.PreflightReleaseContract(repo)
	if err != nil {
		t.Fatal(err)
	}
	if !baseline.Verdict.Floor.Passed || baseline.Verdict.Gate != maat.GateEscalate {
		t.Fatalf("current release source contract: %+v", baseline.Verdict)
	}
	root := t.TempDir()
	for _, file := range baseline.Files {
		raw, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(file.Path)))
		if err != nil {
			t.Fatal(err)
		}
		dst := filepath.Join(root, filepath.FromSlash(file.Path))
		if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dst, raw, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	workflow := filepath.Join(root, ".github", "workflows", "release.yml")
	raw, err := os.ReadFile(workflow)
	if err != nil {
		t.Fatal(err)
	}
	const control = `if [ "$IMPORTED_INSTALLER" != "$DEVELOPER_ID_INSTALLER" ]; then`
	if strings.Count(string(raw), control) != 1 {
		t.Fatal("selected-keychain identity comparison mutation target changed")
	}
	if err := os.WriteFile(workflow, []byte(strings.Replace(string(raw), control, "REMOVED_CONTROL", 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	observation, err := maat.PreflightReleaseContract(root)
	if err != nil {
		t.Fatal(err)
	}
	if observation.Verdict.Gate != maat.GateBlock || len(observation.Verdict.Findings) != 1 || observation.Verdict.Findings[0].ID != "installer-identity-keychain-preflight" {
		t.Fatalf("expected exactly the removed source control: %+v", observation.Verdict)
	}
	view := Build([]maat.Decision{{
		Time: "2026-10-24T00:00:00Z", Host: "fixture", Kind: "system one screen",
		Requester: "sirsi maat preflight release", Assessed: "release source",
		Determination: string(observation.Verdict.Gate), Why: "source control missing",
		Evidence: observation.Fingerprint, SystemOne: &observation.Verdict,
	}})
	if len(view.Cases) != 1 || view.Cases[0].NextAction == nil {
		t.Fatalf("missing recovery route: %+v", view)
	}
	action := view.Cases[0].NextAction
	if action.Kind != "system_one_floor_recovery" || action.ActionID != "" || !action.RequiresConfirmation || len(action.Steps) != 3 {
		t.Fatalf("unexpected executable/automatic recovery: %+v", action)
	}
	correction := action.Steps[1].Detail
	if correction != observation.Verdict.Findings[0].FixHint || !strings.Contains(correction, "Restore the workflow's selected-keychain") || !strings.Contains(correction, "does not establish a missing certificate") || !strings.Contains(correction, "separate public certificate-readiness") {
		t.Fatalf("untruthful source/credential recovery: %+v", action)
	}
	for _, prescription := range []string{"Provide the matching", "import a certificate", "export a certificate", "APPLE_APP_PASSWORD"} {
		if strings.Contains(correction, prescription) {
			t.Fatalf("source absence prescribed owner credentials: %s", correction)
		}
	}
}
