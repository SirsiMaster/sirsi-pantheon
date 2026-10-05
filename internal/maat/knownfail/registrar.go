package knownfail

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
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
	if !testExists(repoDir, guardTest) {
		return fmt.Errorf("guard %q is not a test in %s: write the regression test first", guardTest, repoDir)
	}
	return mutate(path, func(c *Catalog) error {
		for i := range c.Entries {
			if c.Entries[i].ID == id {
				c.Entries[i].Status = "resolved"
				if fix.Kind == "" {
					fix.Kind = "guide"
				}
				c.Entries[i].Fix = fix
				c.Entries[i].Guard = Guard{Kind: "test", Ref: guardTest}
				return nil
			}
		}
		return fmt.Errorf("known failure %q is not registered", id)
	})
}

var testNameRe = regexp.MustCompile(`^Test[A-Za-z0-9_]+$`)

func testExists(repoDir, name string) bool {
	if !testNameRe.MatchString(name) {
		return false
	}
	out, err := exec.Command("git", "-C", repoDir, "grep", "--untracked", "-l", "func "+name+"(", "--", "*_test.go").Output()
	return err == nil && strings.TrimSpace(string(out)) != ""
}

func mutate(path string, fn func(*Catalog) error) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var c Catalog
	if err := json.Unmarshal(b, &c); err != nil {
		return err
	}
	if err := fn(&c); err != nil {
		return err
	}
	out, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if _, err := parse(out); err != nil { // never write a catalog the loader would reject
		return err
	}
	return os.WriteFile(path, append(out, '\n'), 0o644)
}
