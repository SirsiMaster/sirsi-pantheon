// Package prd implements canon-to-PRD derivation for sirsi prd sync.
// A PRD is a machine-readable projection of a repo's canon + architecture:
// canon is the source of truth; the PRD is its agent-consumable view.
package prd

// PRD is the agent's completion contract, consumed by prd-keepalive.py.
// Schema mirrors .agents/prd/<agent>.json.
type PRD struct {
	Agent     string   `json:"agent"`
	App       string   `json:"app,omitempty"`
	DoneWhen  string   `json:"done_when"`
	CanonRefs []string `json:"canon_refs,omitempty"`
	Note      string   `json:"note,omitempty"`
	Tasks     []Task   `json:"tasks"`
}

// Task is one unit of work in the PRD.
type Task struct {
	ID         string `json:"id"`
	Desc       string `json:"desc"`
	Acceptance string `json:"acceptance,omitempty"`
	Status     Status `json:"status"`
	Note       string `json:"note,omitempty"`
	Ref        string `json:"ref,omitempty"`
	Source     string `json:"source,omitempty"` // "canon" | "adr" | "roadmap" | "manual"
}

// Status is the lifecycle state of a task.
type Status string

const (
	StatusOpen       Status = "open"
	StatusDone       Status = "done"
	StatusOwnerGated Status = "owner-gated"
	StatusBlocked    Status = "blocked"
)

// SyncOptions controls a single prd sync run.
type SyncOptions struct {
	// Agent is the agent ID (e.g. "claude-pantheon"). Empty => auto-detect from cwd.
	Agent string
	// RepoDir is the repo's root directory to read canon files from.
	// When empty, derived from the agents.json entry for Agent.
	RepoDir string
	// PRDDir is where .agents/prd/ lives (the sirsi-pantheon anchor).
	// Defaults to the result of FindRepoRoot() + "/.agents/prd/".
	PRDDir string
	// DryRun previews without writing.
	DryRun bool
	// Verbose emits per-step detail.
	Verbose bool
}

// SyncResult reports what changed.
type SyncResult struct {
	Agent   string
	Added   []string // task IDs added
	Updated []string // task IDs whose status changed
	Kept    []string // task IDs kept as-is
	Written bool
}
