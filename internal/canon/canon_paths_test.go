package canon

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// canonEntry matches a numbered line in PANTHEON_RULES.md §4, capturing the
// backticked path and whether it carries the NOT-YET-WRITTEN marker.
var canonEntry = regexp.MustCompile("^[0-9]+\\.\\s+`([^`]+)`(.*)$")

// TestCanonicalDocumentsExistOrAreMarked is the guard for a defect that made
// Rule 16 unfollowable.
//
// §4 listed 22 "source of truth" documents. NINE of them did not exist: two had
// been renamed (ANUBIS_RULES.md -> PANTHEON_RULES.md, default_policies.yaml ->
// example_policy.yaml) and seven were never written. Rule 16 requires re-reading
// the relevant canonical documents before writing code, so a list naming files
// nobody can open does not merely rot — it silently teaches every agent to cite
// a document that does not exist, and no test, lint, or CI gate looked at it.
//
// The rule this enforces: an entry either RESOLVES on disk, or it says out loud
// that it does not. Ambiguity is the failure.
func TestCanonicalDocumentsExistOrAreMarked(t *testing.T) {
	root := "../.."
	raw, err := os.ReadFile(filepath.Join(root, "PANTHEON_RULES.md"))
	if err != nil {
		t.Fatalf("read canon: %v", err)
	}

	inSection := false
	var checked int
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(line, "## 4.") {
			inSection = true
			continue
		}
		if inSection && strings.HasPrefix(line, "## 5.") {
			break
		}
		if !inSection {
			continue
		}
		m := canonEntry.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		path, rest := m[1], m[2]
		checked++
		_, statErr := os.Stat(filepath.Join(root, path))
		marked := strings.Contains(rest, "NOT YET WRITTEN")

		if statErr == nil && marked {
			t.Errorf("%q EXISTS but is marked NOT YET WRITTEN — the marker is now a lie; delete it", path)
		}
		if statErr != nil && !marked {
			t.Errorf("%q is listed as a source of truth but does not exist and is not marked "+
				"NOT YET WRITTEN — Rule 16 cannot be followed against it. Create the file, "+
				"correct the path, or mark it.", path)
		}
	}

	if checked < 20 {
		t.Fatalf("only found %d canon entries; the §4 parser has drifted from the document "+
			"and this guard is no longer checking what it claims to", checked)
	}
}

// The canonical file is synced to CLAUDE.md and GEMINI.md. A correction applied
// to one and not the others produces three disagreeing copies of canon, which is
// worse than one stale copy because agents read different files.
func TestCanonSyncedCopiesAgreeOnSection4(t *testing.T) {
	root := "../.."
	section := func(name string) string {
		raw, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		s := string(raw)
		i := strings.Index(s, "## 4.")
		j := strings.Index(s, "## 5.")
		if i < 0 || j <= i {
			t.Fatalf("%s: could not locate §4", name)
		}
		return s[i:j]
	}
	want := section("PANTHEON_RULES.md")
	for _, copyName := range []string{"CLAUDE.md", "GEMINI.md"} {
		if got := section(copyName); got != want {
			t.Errorf("%s §4 differs from PANTHEON_RULES.md — the synced copies disagree about "+
				"which documents are canon", copyName)
		}
	}
}

// ruleHeading matches a rule DEFINITION heading (not a citation inside prose).
var ruleHeading = regexp.MustCompile(`^### (2\.[0-9]+) .*\(Rule (A[0-9]+)\)`)

// TestNoDuplicateRuleNumbers is the guard for the collision class that has now
// bitten this repo twice.
//
// §2.26 / Rule A29 was held by BOTH "Orchestration Brain" and "Scope The Check
// To The Claim". An agent resolving a citation to "A29" got whichever it found
// first — and a careful agent resolving by number is exactly the one that gets
// it wrong. The first instance of this class was the Rule 14/17 collision (PR
// #491), where every Ra-deployed agent was handed written permission to
// override "Do No Harm" because a markdown list ordinal had been written as a
// rule tag.
//
// Citations inside prose are fine and expected (A22's text legitimately cites
// A17). Only DEFINITIONS must be unique, which is why this matches headings.
func TestNoDuplicateRuleNumbers(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("../..", "PANTHEON_RULES.md"))
	if err != nil {
		t.Fatalf("read canon: %v", err)
	}
	sections := map[string]string{}
	rules := map[string]string{}
	for _, line := range strings.Split(string(raw), "\n") {
		m := ruleHeading.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		sec, rule := m[1], m[2]
		if prev, dup := sections[sec]; dup {
			t.Errorf("section §%s defined twice: %q and %q — a citation by section number is ambiguous", sec, prev, line)
		}
		if prev, dup := rules[rule]; dup {
			t.Errorf("Rule %s defined twice: %q and %q — an agent resolving this citation gets whichever it finds first", rule, prev, line)
		}
		sections[sec], rules[rule] = line, line
	}
	if len(rules) < 25 {
		t.Fatalf("only found %d rule definitions; the heading parser has drifted and this guard is no longer checking what it claims to", len(rules))
	}
}
