package knownfail

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// Register adds an OPEN problem to the catalog file at path (the repo's
// catalog.json): a signature, the cause, and nothing claimed as fixed. The change
// goes through a PR like any other, so a registered problem is on origin.
func Register(path string, e Entry) error {
	e.Status = "open"
	e.Fix, e.Guard = Fix{}, Guard{}
	return mutate(path, func(c *Catalog) error {
		for _, x := range c.Entries {
			if x.ID == e.ID {
				return fmt.Errorf("known failure %q is already registered", e.ID)
			}
		}
		c.Entries = append(c.Entries, e)
		return nil
	})
}

// Resolve records the fix for a registered problem. It refuses unless the named
// regression guard is a test that really exists in repoDir, which is what keeps a
// "fixed" claim from being a note: the fix is on record with the version it shipped
// in and a test that fails if it regresses.
func Resolve(path, repoDir, id string, fix Fix, guardTest string) error {
	if strings.TrimSpace(guardTest) == "" || strings.TrimSpace(fix.FixedIn) == "" || strings.TrimSpace(fix.Text) == "" {
		return fmt.Errorf("resolving %q needs --fixed-in, --text and --guard (a regression test name)", id)
	}
	kind := GuardKindFor(guardTest)
	if err := guardExists(repoDir, kind, guardTest); err != nil {
		return fmt.Errorf("guard %q: %w: write the regression guard first", guardTest, err)
	}
	return mutate(path, func(c *Catalog) error {
		for i := range c.Entries {
			if c.Entries[i].ID == id {
				c.Entries[i].Status = "resolved"
				if fix.Kind == "" {
					fix.Kind = "guide"
				}
				c.Entries[i].Fix = fix
				c.Entries[i].Guard = Guard{Kind: kind, Ref: guardTest}
				return nil
			}
		}
		return fmt.Errorf("known failure %q is not registered", id)
	})
}

var testNameRe = regexp.MustCompile(`^Test[A-Za-z0-9_]+$`)

// crossRepoGuardRef matches a guard that names a commit in another Sirsi repo:
// "owner/repo@commit:path". The commit (not a branch) makes the guard an
// immutable fact instead of a moving target (Ra decision 20261007-110326,
// answering mercury's known-failures-resolve question 20261007-042548).
var crossRepoGuardRef = regexp.MustCompile(`^([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+)@([0-9a-fA-F]{7,40}):(.+)$`)

// GuardKindFor says which kind of guard a reference names: "owner/repo@commit:path"
// lives in another Sirsi repo's local checkout; any other path is a script this
// repo's CI runs; anything else is a Go test name.
func GuardKindFor(ref string) string {
	if crossRepoGuardRef.MatchString(ref) {
		return "cross-repo-script"
	}
	if strings.Contains(ref, "/") {
		return "script"
	}
	return "test"
}

// guardExists proves a guard is real. A test must be a Go test function in the repo. A
// script must be an executable file in the repo that the CI workflow actually runs, so a
// "fixed" claim cannot rest on a script nobody executes. A cross-repo-script is the same
// proof read from another Sirsi repo's own local checkout at a pinned commit.
func guardExists(repoDir, kind, ref string) error {
	if kind == "cross-repo-script" {
		return crossRepoGuardExists(ref)
	}
	if kind == "script" {
		clean := filepath.Clean(ref)
		if filepath.IsAbs(clean) || strings.HasPrefix(clean, "..") {
			return fmt.Errorf("a script guard must be a path inside the repo")
		}
		info, err := os.Stat(filepath.Join(repoDir, clean))
		if err != nil || info.IsDir() || info.Mode()&0o111 == 0 {
			return fmt.Errorf("%s is not an executable file in the repo", clean)
		}
		ci, err := os.ReadFile(filepath.Join(repoDir, ".github", "workflows", "ci.yml"))
		if err != nil || !strings.Contains(string(ci), clean) {
			return fmt.Errorf("%s is not run by .github/workflows/ci.yml", clean)
		}
		return nil
	}
	if !testExists(repoDir, ref) {
		return fmt.Errorf("not a test in the repo")
	}
	return nil
}

// crossRepoGuardExists proves an "owner/repo@commit:path" guard the same way
// guardExists proves a local one, but reads the OTHER repo's own local checkout
// (convention: ~/Development/<repo>) at the pinned commit: the commit exists
// there, the blob at that commit is executable, and that repo's CI workflow at
// that commit runs it. It never trusts the current working tree of that repo,
// so a later uncommitted edit there can't forge the guard retroactively.
func crossRepoGuardExists(ref string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	return crossRepoGuardExistsIn(filepath.Join(home, "Development"), ref)
}

func crossRepoGuardExistsIn(devRoot, ref string) error {
	m := crossRepoGuardRef.FindStringSubmatch(ref)
	if m == nil {
		return fmt.Errorf("must be owner/repo@commit:path")
	}
	owner, repo, commit, path := m[1], m[2], m[3], m[4]
	dir := filepath.Join(devRoot, repo)
	if _, err := os.Stat(dir); err != nil {
		return fmt.Errorf("no local checkout of %s/%s at %s", owner, repo, dir)
	}
	if err := gitCmd(dir, "cat-file", "-e", commit+"^{commit}").Run(); err != nil {
		return fmt.Errorf("commit %s not found in local checkout %s", commit, dir)
	}
	out, err := gitCmd(dir, "ls-tree", commit, "--", path).Output()
	if err != nil || strings.TrimSpace(string(out)) == "" {
		return fmt.Errorf("%s not found at commit %s in %s", path, commit, dir)
	}
	if fields := strings.Fields(string(out)); len(fields) == 0 || fields[0] != "100755" {
		return fmt.Errorf("%s at commit %s is not mode 100755", path, commit)
	}
	ci, err := gitCmd(dir, "show", commit+":.github/workflows/ci.yml").Output()
	if err != nil || !strings.Contains(string(ci), path) {
		return fmt.Errorf("%s is not run by %s/%s's .github/workflows/ci.yml at commit %s", path, owner, repo, commit)
	}
	return nil
}

func testExists(repoDir, name string) bool {
	if !testNameRe.MatchString(name) {
		return false
	}
	out, err := gitCmd(repoDir, "grep", "--untracked", "-l", "func "+name+"(", "--", "*_test.go").Output()
	return err == nil && strings.TrimSpace(string(out)) != ""
}

// gitCmd runs git in dir with the GIT_* environment removed: inside a git hook
// GIT_DIR/GIT_INDEX_FILE point at the pushing repository and would silently redirect
// the query.
func gitCmd(dir string, args ...string) *exec.Cmd {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GIT_") {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	return cmd
}

func mutate(path string, fn func(*Catalog) error) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var c Catalog
	if err = json.Unmarshal(b, &c); err != nil {
		return err
	}
	if err = fn(&c); err != nil {
		return err
	}
	out, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if _, err = parse(out); err != nil { // never write a catalog the loader would reject
		return err
	}
	return os.WriteFile(path, append(out, '\n'), 0o644)
}
