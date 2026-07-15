package main_test

// Surfaces-canon regression tests: repo-scoped verbs shelled OUTSIDE a repo
// (the menubar runs everything from $HOME) must report "unmeasured" honestly
// — never a fabricated failure. 2026-07-05 popover: Ma'at showed "❌ 59/100
// fail" weighing the owner's home directory against fallback thresholds, and
// Net—Plan rendered "0.0% DRIFTING" from a hardcoded demo plan while its own
// warning promised 1.0.

import (
	"strings"
	"testing"
	"time"
)

// outsideRepoDir returns a temp dir guaranteed to have no go.mod above it...
// as close as a test can get: t.TempDir() lives under the darwin per-user
// temp root, which has no go.mod on any supported layout.
func requireOutsideRepo(t *testing.T, dir string) string {
	t.Helper()
	return dir
}

func TestMaatAuditOutsideRepoIsHonest(t *testing.T) {
	dir := requireOutsideRepo(t, t.TempDir())
	stdout, stderr, err := runSirsiInDir(t, dir, 60*time.Second, "maat", "audit")
	if err != nil {
		t.Fatalf("maat audit outside a repo must exit clean: %v\nstderr: %s", err, stderr)
	}
	combined := stdout + stderr
	if !strings.Contains(combined, "nothing to weigh") {
		t.Fatalf("expected the honest unmeasured message, got:\n%s", combined)
	}
	// Fabricated-verdict markers (the guidance text may legitimately mention
	// "coverage" — what must never appear is a verdict or score).
	for _, fabricated := range []string{"fail", "/100", "passed", "warnings"} {
		if strings.Contains(strings.ToLower(combined), fabricated) {
			t.Fatalf("outside a repo the audit must not fabricate %q:\n%s", fabricated, combined)
		}
	}
}

func TestNetStatusOutsideRepoIsHonest(t *testing.T) {
	dir := requireOutsideRepo(t, t.TempDir())
	stdout, stderr, err := runSirsiInDir(t, dir, 60*time.Second, "net", "status")
	if err != nil {
		t.Fatalf("net status outside a repo must exit clean: %v\nstderr: %s", err, stderr)
	}
	combined := stdout + stderr
	if !strings.Contains(combined, "nothing to weigh") {
		t.Fatalf("expected the honest unmeasured message, got:\n%s", combined)
	}
	for _, fabricated := range []string{"DRIFTING", "ALIGNED", "0.0%", "%"} {
		if strings.Contains(combined, fabricated) {
			t.Fatalf("outside a repo net status must not fabricate %q:\n%s", fabricated, combined)
		}
	}
}

func TestNetAlignOutsideRepoIsHonest(t *testing.T) {
	dir := requireOutsideRepo(t, t.TempDir())
	stdout, stderr, err := runSirsiInDir(t, dir, 60*time.Second, "net", "align")
	if err != nil {
		t.Fatalf("net align outside a repo must exit clean: %v\nstderr: %s", err, stderr)
	}
	combined := stdout + stderr
	if !strings.Contains(combined, "nothing to check") {
		t.Fatalf("expected the honest nothing-to-check message, got:\n%s", combined)
	}
	if strings.Contains(combined, "vet failed") || strings.Contains(combined, "build failed") {
		t.Fatalf("outside a repo align must not run vet/build against the cwd:\n%s", combined)
	}
}

// In-repo: net status names the real build log and still never fabricates a
// score (no session plan is recorded — alignment is explicitly unmeasured).
func TestNetStatusInRepoNamesTheLogAndNoFakeScore(t *testing.T) {
	stdout, stderr, err := runSirsi(t, 60*time.Second, "net", "status")
	if err != nil {
		t.Fatalf("net status: %v\nstderr: %s", err, stderr)
	}
	combined := stdout + stderr
	if !strings.Contains(combined, "BUILD_LOG.md") {
		t.Fatalf("in-repo net status should name the build log, got:\n%s", combined)
	}
	if strings.Contains(combined, "DRIFTING") || strings.Contains(combined, "0.0%") {
		t.Fatalf("net status must not fabricate a drift verdict:\n%s", combined)
	}
}
