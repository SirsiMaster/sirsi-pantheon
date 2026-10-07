// Package knownfail is the closed loop for recurring failures: a problem is
// registered once with its signature and cause, the fix is recorded with the
// version it shipped in and a regression guard that must exist, and from then on
// the router recognizes the failure by its signature and says what the fix is
// instead of quarantining a lane and waiting for a person. The catalog is the Stack
// Lab artifact (contracts/stacklab/ra-horus-fabric-recipe-v1.json names it), so a
// fix recorded here does not have to be rediscovered.
package knownfail

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

//go:embed catalog.json
var catalogJSON []byte

type Fix struct {
	Kind    string `json:"kind"` // guide (text only); a closed action may be added later, never a free command
	Text    string `json:"text"`
	FixedIn string `json:"fixed_in"` // first release containing the fix; empty while open
	Ref     string `json:"ref"`
}

type Guard struct {
	Kind string `json:"kind"` // test | script | cross-repo-script
	Ref  string `json:"ref"`  // test: a Go test name; script: a repo-relative executable that CI runs; cross-repo-script: "owner/repo@commit:path" in another Sirsi repo's own checkout
}

type Entry struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Signature string `json:"signature"` // case-insensitive regexp over failure text
	Cause     string `json:"cause"`
	Status    string `json:"status"` // open | resolved
	Fix       Fix    `json:"fix"`
	Guard     Guard  `json:"guard"`
	re        *regexp.Regexp
}

type Catalog struct {
	Schema  string  `json:"schema"`
	Entries []Entry `json:"entries"`
}

var (
	once   sync.Once
	loaded Catalog
	loadEr error
)

// Load parses and validates the embedded catalog.
func Load() (Catalog, error) {
	once.Do(func() { loaded, loadEr = parse(catalogJSON) })
	return loaded, loadEr
}

func parse(b []byte) (Catalog, error) {
	var c Catalog
	if err := json.Unmarshal(b, &c); err != nil {
		return Catalog{}, fmt.Errorf("known-failure catalog: %w", err)
	}
	seen := map[string]bool{}
	for i := range c.Entries {
		e := &c.Entries[i]
		if e.ID == "" || e.Signature == "" || e.Cause == "" {
			return Catalog{}, fmt.Errorf("known-failure %q: id, signature and cause are required", e.ID)
		}
		if seen[e.ID] {
			return Catalog{}, fmt.Errorf("known-failure %q: duplicate id", e.ID)
		}
		seen[e.ID] = true
		re, err := regexp.Compile("(?i)" + e.Signature)
		if err != nil {
			return Catalog{}, fmt.Errorf("known-failure %q: bad signature: %w", e.ID, err)
		}
		e.re = re
		switch e.Status {
		case "open":
		case "resolved":
			if e.Fix.FixedIn == "" || e.Fix.Text == "" {
				return Catalog{}, fmt.Errorf("known-failure %q: resolved needs fix.text and fix.fixed_in", e.ID)
			}
			if (e.Guard.Kind != "test" && e.Guard.Kind != "script" && e.Guard.Kind != "cross-repo-script") || e.Guard.Ref == "" {
				return Catalog{}, fmt.Errorf("known-failure %q: resolved needs a regression guard (guard.kind=test or script, guard.ref)", e.ID)
			}
		default:
			return Catalog{}, fmt.Errorf("known-failure %q: status must be open or resolved", e.ID)
		}
	}
	return c, nil
}

// Match returns every entry whose signature appears in text, catalog order.
func Match(text string) []Entry {
	c, err := Load()
	if err != nil || text == "" {
		return nil
	}
	var out []Entry
	for _, e := range c.Entries {
		if e.re.MatchString(text) {
			out = append(out, e)
		}
	}
	return out
}

// Summary is the one-line recognition a lane or operator sees.
func (e Entry) Summary(installed string) string {
	s := fmt.Sprintf("KNOWN failure %q: %s. Fix: %s", e.ID, e.Cause, e.Fix.Text)
	if e.Status == "resolved" && installed != "" && versionLess(installed, e.Fix.FixedIn) {
		s += fmt.Sprintf(" (this host runs %s; fixed in %s: upgrade)", installed, e.Fix.FixedIn)
	}
	return s
}

// versionLess compares dotted numeric versions ("0.24.9" < "0.24.10"); anything
// unparseable compares as not-less so a dev build is never told to upgrade.
func versionLess(a, b string) bool {
	pa, pb := nums(a), nums(b)
	if pa == nil || pb == nil {
		return false
	}
	for i := 0; i < len(pa) && i < len(pb); i++ {
		if pa[i] != pb[i] {
			return pa[i] < pb[i]
		}
	}
	return len(pa) < len(pb)
}

func nums(v string) []int {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	var out []int
	for _, p := range strings.Split(v, ".") {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil
		}
		out = append(out, n)
	}
	return out
}
