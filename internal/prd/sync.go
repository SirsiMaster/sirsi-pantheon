package prd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// AgentRepoMap is the canonical agent-id → repo-dir mapping, built from
// agents.json in the idea-router directory.
type AgentRepoMap map[string]string

// LoadAgentRepoMap reads .agents/idea-router/agents.json and returns the
// cwd for each registered agent.
func LoadAgentRepoMap(routerHome string) (AgentRepoMap, error) {
	path := filepath.Join(routerHome, "idea-router", "agents.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("agents.json: %w", err)
	}
	var raw struct {
		Agents map[string]struct {
			CWD string `json:"cwd"`
		} `json:"agents"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("agents.json parse: %w", err)
	}
	out := make(AgentRepoMap, len(raw.Agents))
	for id, entry := range raw.Agents {
		out[id] = entry.CWD
	}
	return out, nil
}

// Sync derives and writes (or previews) the PRD for one agent.
func Sync(opts SyncOptions) (*SyncResult, error) {
	if opts.Agent == "" {
		return nil, fmt.Errorf("agent ID required (--agent <id>)")
	}

	// Resolve paths.
	prdDir := opts.PRDDir
	if prdDir == "" {
		root, err := FindRouterHome()
		if err != nil {
			return nil, err
		}
		prdDir = filepath.Join(root, "prd")
	}

	repoDir := opts.RepoDir
	if repoDir == "" {
		// Load from agents.json.
		m, err := LoadAgentRepoMap(filepath.Dir(prdDir))
		if err != nil {
			return nil, fmt.Errorf("cannot resolve repo dir for %q: %w", opts.Agent, err)
		}
		d, ok := m[opts.Agent]
		if !ok {
			return nil, fmt.Errorf("agent %q not in agents.json; use --repo-dir to specify", opts.Agent)
		}
		repoDir = d
	}

	// Load existing PRD (if any).
	prdPath := filepath.Join(prdDir, opts.Agent+".json")
	existing, err := loadExistingPRD(prdPath)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("load existing PRD: %w", err)
	}

	// Derive tasks from canon.
	reader := NewCanonReader(repoDir)
	derived, canonRefs, err := reader.Derive()
	if err != nil {
		return nil, fmt.Errorf("canon derivation: %w", err)
	}

	// Build shipment checker.
	checker := NewShipmentChecker(repoDir)

	// Merge: existing tasks are preserved with their status/notes.
	// Canon-derived tasks that are new are added as "open" (or "done" if shipped).
	merged, result := mergeTasks(existing, derived, checker, opts.Agent)
	result.Agent = opts.Agent

	// Build the output PRD.
	out := &PRD{
		Agent:     opts.Agent,
		DoneWhen:  "all tasks done or owner-gated",
		CanonRefs: mergeRefs(existing, canonRefs),
		Note: fmt.Sprintf(
			"Derived from repo canon + architecture by `sirsi prd sync`. Last synced: %s. Canon is the single source of truth — re-run to reconcile.",
			time.Now().Format("2006-01-02"),
		),
		Tasks: merged,
	}

	// Preserve app name from existing.
	if existing != nil && existing.App != "" {
		out.App = existing.App
	}

	if opts.DryRun {
		result.Written = false
		return result, nil
	}

	// Write.
	if err := writePRD(prdPath, out); err != nil {
		return nil, fmt.Errorf("write PRD: %w", err)
	}
	result.Written = true
	return result, nil
}

// SyncAll runs Sync for all agents that have a repo mapping in agents.json.
func SyncAll(opts SyncOptions) ([]*SyncResult, error) {
	prdDir := opts.PRDDir
	if prdDir == "" {
		root, err := FindRouterHome()
		if err != nil {
			return nil, err
		}
		prdDir = filepath.Join(root, "prd")
	}

	m, err := LoadAgentRepoMap(filepath.Dir(prdDir))
	if err != nil {
		return nil, err
	}

	// Only sync agents whose cwd is an actual directory (skip cloud/home entries).
	var results []*SyncResult
	for id, dir := range m {
		if !isRealRepoDir(dir) {
			continue
		}
		// Skip -web sub-entries (they share the parent repo's canon).
		if strings.HasSuffix(id, "-web") {
			continue
		}
		o := opts
		o.Agent = id
		o.RepoDir = dir
		o.PRDDir = prdDir
		r, err := Sync(o)
		if err != nil {
			// Non-fatal: report and continue.
			results = append(results, &SyncResult{Agent: id})
			continue
		}
		results = append(results, r)
	}
	return results, nil
}

// FindRouterHome walks up from cwd looking for .agents/idea-router/.
func FindRouterHome() (string, error) {
	dir, _ := os.Getwd()
	for {
		candidate := filepath.Join(dir, ".agents")
		if _, err := os.Stat(filepath.Join(candidate, "idea-router")); err == nil {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("no .agents/idea-router/ found; run from within the sirsi-pantheon repo")
}

// --- internal helpers ---

func loadExistingPRD(path string) (*PRD, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var p PRD
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

func writePRD(path string, p *PRD) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// mergeTasks merges existing PRD tasks with canon-derived tasks.
// Existing tasks (manually curated) are always preserved with their status.
// New canon tasks are added as "open" (or "done" if shipment evidence found).
// Re-runs are idempotent: same canon → same result.
func mergeTasks(existing *PRD, derived []DerivedTask, checker *ShipmentChecker, agent string) ([]Task, *SyncResult) {
	result := &SyncResult{}

	// Index existing tasks by ID for O(1) lookup.
	existingByID := make(map[string]Task)
	if existing != nil {
		for _, t := range existing.Tasks {
			existingByID[t.ID] = t
		}
	}

	var out []Task

	// Keep all existing tasks (they carry human-set status/notes).
	if existing != nil {
		for _, t := range existing.Tasks {
			out = append(out, t)
			result.Kept = append(result.Kept, t.ID)
		}
	}

	// Add canon-derived tasks that are genuinely new (not already present).
	prefix := agentPrefix(agent)
	canonCounter := nextCanonCounter(existing)

	for _, d := range derived {
		// Skip if an existing task already references this canon item.
		if existingHasRef(existing, d.ID, d.Desc) {
			continue
		}

		id := fmt.Sprintf("%s%d", prefix, canonCounter)
		canonCounter++

		status := StatusOpen
		if d.Shipped || checker.IsShipped(d) {
			status = StatusDone
		}

		t := Task{
			ID:         id,
			Desc:       d.Desc,
			Acceptance: d.Acceptance,
			Status:     status,
			Ref:        d.Ref,
			Source:     string(d.Source),
		}
		out = append(out, t)
		result.Added = append(result.Added, id)
	}

	return out, result
}

// agentPrefix derives a short task-ID prefix from the agent name.
// "claude-pantheon" → "CP", "claude-finalwishes" → "CF", etc.
func agentPrefix(agent string) string {
	parts := strings.Split(agent, "-")
	if len(parts) < 2 {
		return "T"
	}
	// Use first letter of each meaningful segment.
	var sb strings.Builder
	for i, p := range parts {
		if i == 0 && strings.EqualFold(p, "claude") {
			continue // skip "claude"
		}
		if p != "" {
			sb.WriteByte(strings.ToUpper(p)[0])
		}
	}
	s := sb.String()
	if s == "" {
		return "T"
	}
	return s
}

// nextCanonCounter finds the next available numeric suffix for auto-generated task IDs.
func nextCanonCounter(existing *PRD) int {
	n := 100 // canon-derived tasks start at 100 to avoid colliding with manual Pxx IDs
	if existing == nil {
		return n
	}
	for _, t := range existing.Tasks {
		var num int
		// Extract trailing digits.
		i := len(t.ID) - 1
		for i >= 0 && t.ID[i] >= '0' && t.ID[i] <= '9' {
			i--
		}
		suffix := t.ID[i+1:]
		if suffix != "" {
			_, _ = fmt.Sscan(suffix, &num)
			if num >= n {
				n = num + 1
			}
		}
	}
	return n
}

// existingHasRef returns true when the existing PRD already covers the derived task.
func existingHasRef(existing *PRD, id, desc string) bool {
	if existing == nil {
		return false
	}
	descLower := strings.ToLower(desc)
	for _, t := range existing.Tasks {
		if t.Ref == id {
			return true
		}
		// Fuzzy match: if the existing task description contains the key phrase.
		if descLower != "" && strings.Contains(strings.ToLower(t.Desc), descLower[:min(len(descLower), 40)]) {
			return true
		}
	}
	return false
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func mergeRefs(existing *PRD, canonRefs []string) []string {
	seen := make(map[string]bool)
	var out []string
	if existing != nil {
		for _, r := range existing.CanonRefs {
			if !seen[r] {
				seen[r] = true
				out = append(out, r)
			}
		}
	}
	for _, r := range canonRefs {
		if !seen[r] {
			seen[r] = true
			out = append(out, r)
		}
	}
	return out
}

func isRealRepoDir(dir string) bool {
	if dir == "" {
		return false
	}
	// Home directory entries are not repo dirs.
	home, _ := os.UserHomeDir()
	if dir == home {
		return false
	}
	info, err := os.Stat(dir)
	return err == nil && info.IsDir()
}
