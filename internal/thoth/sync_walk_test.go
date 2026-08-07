package thoth

import (
	"os"
	"path/filepath"
	"testing"
)

// The bug: estimateTestCount pruned NOTHING while estimateLineCount pruned six
// directories — same file, two walks, two ideas of scope. Rooted at $HOME
// (findRepoRoot walks UP to ~/.thoth, which exists) the unpruned walk covered the
// whole home tree and could not finish inside the SessionEnd hook's 60s budget.
// Measured 2026-08-07: killed at 120s, never completed, every home-rooted session.
//
// NEGATIVE CONTROL: revert walkSource in estimateTestCount (restore the bare
// filepath.Walk) and this test MUST fail — it counts the node_modules test file.
func TestEstimateTestCountPrunesNonSourceDirs(t *testing.T) {
	root := t.TempDir()

	// One real test file at the root — this one must be counted.
	mustWrite(t, filepath.Join(root, "real_test.go"), "package p\n\nfunc TestReal(t *testing.T) {}\n")

	// Identical files buried in directories a project sync must never walk.
	for _, dir := range []string{"node_modules", ".git", "vendor", "Library", ".cache", "dist"} {
		mustWrite(t, filepath.Join(root, dir, "pkg", "buried_test.go"),
			"package p\n\nfunc TestBuried(t *testing.T) {}\n")
	}

	if got := estimateTestCount(root); got != 1 {
		t.Errorf("estimateTestCount = %d, want 1 — the walk is descending into "+
			"pruned directories (node_modules/.git/vendor/Library/.cache/dist)", got)
	}
}

// Both estimators must agree on scope. They disagreeing is what caused the hang,
// so a future edit that prunes in one walk but not the other should fail here.
func TestBothEstimatorsShareTheSamePruning(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.go"), "package p\n")
	// 4KB of .go inside node_modules: counted only if the walk descends.
	big := make([]byte, 4096)
	for i := range big {
		big[i] = 'x'
	}
	mustWrite(t, filepath.Join(root, "node_modules", "huge.go"), string(big))

	// estimateLineCount divides total bytes by 65; the root file alone is tiny,
	// so descending into node_modules would push this well above zero.
	if got := estimateLineCount(root); got > 10 {
		t.Errorf("estimateLineCount = %d, want <=10 — it counted node_modules", got)
	}
}

func TestSkipWalkDirPrunesByShapeNotJustName(t *testing.T) {
	// Dotted directories are pruned by shape, so a cache dir invented tomorrow
	// does not reopen this bug.
	for _, d := range []string{".git", ".thoth", ".cache", ".venv", ".next", ".claude"} {
		if !skipWalkDir(d) {
			t.Errorf("skipWalkDir(%q) = false, want true (dotted dirs are never source)", d)
		}
	}
	for _, d := range []string{"internal", "cmd", "docs", "src"} {
		if skipWalkDir(d) {
			t.Errorf("skipWalkDir(%q) = true, want false (real source dir)", d)
		}
	}
	// A bare "." must NOT be pruned — it is the walk root itself.
	if skipWalkDir(".") {
		t.Error(`skipWalkDir(".") = true, want false — that would prune the root`)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
