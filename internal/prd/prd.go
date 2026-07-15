// Package prd loads, validates, and reconciles agent PRDs
// (.agents/prd/<agent>.json) against repo canon. The owner directive is that a
// PRD is DERIVED from canon (RULES + ARCHITECTURE + ADR-INDEX + ROADMAP), not
// hand-authored — so the first job is to keep each PRD honest: well-formed,
// valid statuses, its canon references actually present, and to surface drift
// (open items, stale-vs-canon) for reconciliation. Semantic task derivation
// (generating tasks from the roadmap text) is the next iteration; it needs a
// canon↔task linkage convention that isn't established yet, and a fuzzy guesser
// that silently mis-statuses tasks would be worse than honest reporting.
package prd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// validStatuses are the only task states a PRD may use.
var validStatuses = map[string]bool{
	"open": true, "done": true, "owner-gated": true, "blocked": true,
}

// Task is one PRD work item.
type Task struct {
	ID         string `json:"id"`
	Desc       string `json:"desc"`
	Acceptance string `json:"acceptance,omitempty"`
	Status     string `json:"status"`
	Ref        string `json:"ref,omitempty"`
	Note       string `json:"note,omitempty"`
}

// PRD is an agent's product-requirements doc.
type PRD struct {
	Agent     string   `json:"agent"`
	App       string   `json:"app,omitempty"`
	DoneWhen  string   `json:"done_when,omitempty"`
	CanonRefs []string `json:"canon_refs,omitempty"`
	Note      string   `json:"note,omitempty"`
	Tasks     []Task   `json:"tasks"`
}

// Load reads and parses a PRD JSON file.
func Load(path string) (*PRD, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var p PRD
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parse %s: %w", filepath.Base(path), err)
	}
	return &p, nil
}

// Save writes a PRD back as indented JSON (stable shape for diffs).
func Save(path string, p *PRD) error {
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// Summary counts tasks by status.
type Summary struct {
	Total    int            `json:"total"`
	ByStatus map[string]int `json:"by_status"`
	OpenIDs  []string       `json:"open_ids"`
	Complete bool           `json:"complete"` // every task done or owner-gated
}

// Summarize rolls up task statuses. "Complete" matches the keepalive's notion of
// done: nothing left open or blocked (owner-gated counts as parked, not pending).
func Summarize(p *PRD) Summary {
	s := Summary{ByStatus: map[string]int{}, Complete: true}
	for _, t := range p.Tasks {
		s.Total++
		s.ByStatus[t.Status]++
		if t.Status == "open" || t.Status == "blocked" {
			s.OpenIDs = append(s.OpenIDs, t.ID)
			s.Complete = false
		}
	}
	sort.Strings(s.OpenIDs)
	return s
}

// Validate returns the list of problems with a PRD: malformed tasks, invalid
// statuses, duplicate IDs, and canon_refs whose files are missing from repoRoot
// (globs allowed, e.g. "docs/ARCHITECTURE*.md"). An empty result means the PRD
// is well-formed and its canon references resolve.
func Validate(p *PRD, repoRoot string) []string {
	var problems []string
	if strings.TrimSpace(p.Agent) == "" {
		problems = append(problems, "missing top-level \"agent\"")
	}
	seen := map[string]bool{}
	for i, t := range p.Tasks {
		where := t.ID
		if where == "" {
			where = fmt.Sprintf("task[%d]", i)
		}
		if t.ID == "" {
			problems = append(problems, fmt.Sprintf("%s: missing id", where))
		} else if seen[t.ID] {
			problems = append(problems, fmt.Sprintf("%s: duplicate id", t.ID))
		}
		seen[t.ID] = true
		if strings.TrimSpace(t.Desc) == "" {
			problems = append(problems, fmt.Sprintf("%s: empty desc", where))
		}
		if !validStatuses[t.Status] {
			problems = append(problems, fmt.Sprintf("%s: invalid status %q (want open|done|owner-gated|blocked)", where, t.Status))
		}
		if t.Status == "blocked" && strings.TrimSpace(t.Note) == "" {
			problems = append(problems, fmt.Sprintf("%s: blocked without a note explaining the blocker", where))
		}
	}
	// canon_refs must resolve — a PRD claiming to be derived from canon that
	// points at missing files is stale.
	for _, ref := range p.CanonRefs {
		matches, _ := filepath.Glob(filepath.Join(repoRoot, ref))
		if len(matches) == 0 {
			problems = append(problems, fmt.Sprintf("canon_ref not found in repo: %s", ref))
		}
	}
	return problems
}
