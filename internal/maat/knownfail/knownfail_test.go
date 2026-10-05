package knownfail

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestCatalogLoadsAndEveryResolvedEntryHasAGuard(t *testing.T) {
	c, err := Load()
	if err != nil || len(c.Entries) == 0 {
		t.Fatalf("catalog: %v (%d entries)", err, len(c.Entries))
	}
}

// The loop is only closed if the regression guard each resolved entry names really
// exists in the repo: a fix recorded without a test is a fix that can silently regress.
func TestEveryGuardTestExistsInTheRepo(t *testing.T) {
	c, _ := Load()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	for _, e := range c.Entries {
		if e.Status != "resolved" {
			continue
		}
		out, err := gitCmd(repoRoot(t), "grep", "--untracked", "-l", "func "+e.Guard.Ref+"(", "--", "*_test.go").Output()
		if err != nil || strings.TrimSpace(string(out)) == "" {
			t.Errorf("known failure %q names guard %s, which is not a test in the repo", e.ID, e.Guard.Ref)
		}
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	out, err := gitCmd(".", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		wd, _ := os.Getwd()
		return wd
	}
	return strings.TrimSpace(string(out))
}

// Recognition both ways: real failure text matches its entry; unrelated text does not.
func TestMatchRecognizesKnownFailuresOnly(t *testing.T) {
	got := Match(`ERROR codex: UnsupportedOperation("writable root /Users/x/.sirsi/relay contains symlink component /Users/x/.sirsi")`)
	if len(got) != 1 || got[0].ID != "codex-symlinked-writable-root" {
		t.Fatalf("symlink failure not recognized: %+v", got)
	}
	if got := Match("rpc: no claimable task in the ledger (rows may be done"); len(got) != 1 || got[0].ID != "claim-refused-no-claimable-task" {
		t.Fatalf("claim refusal not recognized: %+v", got)
	}
	if got := Match("everything is fine, 3 items closed"); len(got) != 0 {
		t.Fatalf("healthy text matched: %+v", got)
	}
}

func TestSummaryTellsAnOutdatedHostToUpgradeButNotADevBuild(t *testing.T) {
	e := Match("caller's session does not own this lease")[0]
	if s := e.Summary("0.24.63"); !strings.Contains(s, "upgrade") {
		t.Fatalf("old host must be told to upgrade: %s", s)
	}
	if s := e.Summary("0.24.68"); strings.Contains(s, "upgrade") {
		t.Fatalf("fixed host must not be told to upgrade: %s", s)
	}
	if s := e.Summary("dev"); strings.Contains(s, "upgrade") {
		t.Fatalf("a dev build must not be told to upgrade: %s", s)
	}
}

func TestParseRejectsAResolvedEntryWithoutAGuard(t *testing.T) {
	bad := `{"schema":"x","entries":[{"id":"a","signature":"s","cause":"c","status":"resolved","fix":{"kind":"guide","text":"t","fixed_in":"1.0.0"},"guard":{}}]}`
	if _, err := parse([]byte(bad)); err == nil {
		t.Fatal("a resolved entry with no regression guard must be rejected")
	}
}

// Registering opens a problem; resolving it is refused without a real guard test
// and accepted with one (both directions).
func TestRegisterThenResolveRequiresARealGuard(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/catalog.json"
	if err := os.WriteFile(path, []byte(`{"schema":"x","entries":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Register(path, Entry{ID: "new-one", Signature: "boom", Cause: "it booms"}); err != nil {
		t.Fatal(err)
	}
	if err := Register(path, Entry{ID: "new-one", Signature: "boom", Cause: "again"}); err == nil {
		t.Fatal("a duplicate id must be refused")
	}
	root := repoRoot(t)
	fix := Fix{Text: "upgrade", FixedIn: "1.2.3", Ref: "PR"}
	if err := Resolve(path, root, "new-one", fix, "TestDoesNotExistAnywhere"); err == nil {
		t.Fatal("a guard that is not a real test must be refused")
	}
	if err := Resolve(path, root, "new-one", fix, "TestMatchRecognizesKnownFailuresOnly"); err != nil {
		t.Fatalf("a real guard must be accepted: %v", err)
	}
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), `"status": "resolved"`) {
		t.Fatalf("entry not resolved: %s", b)
	}
}
