package prd

import (
	"os/exec"
	"regexp"
	"strings"
)

// ShipmentChecker probes git/PR history to determine if a task is already
// shipped. It avoids network calls — reads only local git log.
type ShipmentChecker struct {
	repoDir string
	// gitLog is the recent git log (lazy-loaded).
	gitLog string
	loaded bool
}

// NewShipmentChecker returns a checker for repoDir.
func NewShipmentChecker(repoDir string) *ShipmentChecker {
	return &ShipmentChecker{repoDir: repoDir}
}

// IsShipped returns true when there is local git evidence the task is done.
// It uses the task ID, ref, or keywords from the description as search terms.
func (c *ShipmentChecker) IsShipped(t DerivedTask) bool {
	log := c.log()
	if log == "" {
		return false
	}

	terms := shipmentTerms(t)
	lower := strings.ToLower(log)
	for _, term := range terms {
		if strings.Contains(lower, strings.ToLower(term)) {
			return true
		}
	}
	return false
}

func (c *ShipmentChecker) log() string {
	if c.loaded {
		return c.gitLog
	}
	c.loaded = true
	out, err := exec.Command("git", "-C", c.repoDir, "log",
		"--oneline", "--no-decorate", "-500").Output()
	if err == nil {
		c.gitLog = string(out)
	}
	return c.gitLog
}

// shipmentTerms extracts candidate search strings for a DerivedTask.
func shipmentTerms(t DerivedTask) []string {
	var terms []string
	if t.Ref != "" {
		terms = append(terms, t.Ref)
	}
	if t.ID != "" {
		terms = append(terms, t.ID)
	}
	// Extract key nouns from the description (3+ char words, no stop words).
	words := keywordsFrom(t.Desc)
	terms = append(terms, words...)
	return terms
}

// stopWords is a minimal set to avoid false positives.
var stopWords = map[string]bool{
	"the": true, "and": true, "for": true, "via": true, "with": true,
	"that": true, "this": true, "from": true, "into": true, "its": true,
	"not": true, "nor": true, "are": true, "has": true, "was": true,
	"have": true, "been": true, "will": true, "can": true, "all": true,
	"each": true, "per": true, "new": true, "add": true, "fix": true,
}

// wordRe extracts alpha-numeric words.
var wordRe = regexp.MustCompile(`[A-Za-z][A-Za-z0-9]{2,}`)

func keywordsFrom(s string) []string {
	matches := wordRe.FindAllString(s, -1)
	var out []string
	seen := make(map[string]bool)
	for _, w := range matches {
		lower := strings.ToLower(w)
		if stopWords[lower] || seen[lower] {
			continue
		}
		seen[lower] = true
		out = append(out, w)
		if len(out) >= 3 { // limit to avoid over-matching
			break
		}
	}
	return out
}
