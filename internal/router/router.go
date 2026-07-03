// Package router implements Ra's multi-agent work queue (Idea Router v3).
// Registered agents exchange proposals, reviews, and decisions through
// .agents/idea-router/ in the repo. Ra owns dispatch and orchestration;
// Thoth preserves router continuity; Ma'at validates governance.
package router

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// DocType identifies the kind of router document.
type DocType string

const (
	DocProposal DocType = "proposal"
	DocReview   DocType = "review"
	DocDecision DocType = "decision"
)

// Document represents a single router file.
type Document struct {
	ID      string    // filename without extension
	Type    DocType   // proposal, review, decision
	Path    string    // full filesystem path
	Author  string    // codex or claude
	Title   string    // extracted from first heading
	ModTime time.Time // file modification time
	Content string    // full file content
}

// State represents the router state.json file.
type State struct {
	Version         int             `json:"version"`
	ActiveTopics    []string        `json:"active_topics"`
	CompletedTopics []string        `json:"completed_topics,omitempty"`
	LastCodexRead   string          `json:"last_codex_read"`
	LastClaudeRead  string          `json:"last_claude_read"`
	Rules           map[string]bool `json:"rules"`

	// v3: Dynamic per-agent inbox keyed by agent ID
	Pending map[string][]string `json:"pending,omitempty"`

	// Legacy v2 fields — read for migration, written for backwards compat
	PendingForCodex  []string `json:"pending_for_codex,omitempty"`
	PendingForClaude []string `json:"pending_for_claude,omitempty"`
}

// PendingEntry is a display-friendly view of one registered inbox.
type PendingEntry struct {
	Agent string
	IDs   []string
}

// MigratePending moves legacy pending_for_codex/pending_for_claude into
// the dynamic Pending map. Call after reading state.json.
func (s *State) MigratePending() {
	if s.Pending == nil {
		s.Pending = make(map[string][]string)
	}
	merge := func(target string, ids []string) {
		for _, id := range ids {
			found := false
			for _, existing := range s.Pending[target] {
				if existing == id {
					found = true
					break
				}
			}
			if !found {
				s.Pending[target] = append(s.Pending[target], id)
			}
		}
	}
	// Migrate legacy fields if they have items not already in the dynamic map
	if len(s.PendingForCodex) > 0 {
		merge("codex-pantheon", s.PendingForCodex)
	}
	if len(s.PendingForClaude) > 0 {
		merge("claude-pantheon", s.PendingForClaude)
	}
}

// NormalizePending keeps v3 registered inboxes and legacy compatibility
// fields coherent, and avoids nil slices being written as JSON null.
func (s *State) NormalizePending() {
	s.MigratePending()
	for agent, ids := range s.Pending {
		if ids == nil {
			s.Pending[agent] = []string{}
		}
	}
	if ids, ok := s.Pending["codex-pantheon"]; ok {
		s.PendingForCodex = ids
	}
	if ids, ok := s.Pending["claude-pantheon"]; ok {
		s.PendingForClaude = ids
	}
	if s.PendingForCodex == nil {
		s.PendingForCodex = []string{}
	}
	if s.PendingForClaude == nil {
		s.PendingForClaude = []string{}
	}
}

// PendingEntries returns registered inboxes in stable order.
func (s *State) PendingEntries(includeEmpty bool) []PendingEntry {
	s.NormalizePending()
	agents := make([]string, 0, len(s.Pending))
	for agent, ids := range s.Pending {
		if !includeEmpty && len(ids) == 0 {
			continue
		}
		agents = append(agents, agent)
	}
	sort.Strings(agents)
	entries := make([]PendingEntry, 0, len(agents))
	for _, agent := range agents {
		ids := append([]string(nil), s.Pending[agent]...)
		entries = append(entries, PendingEntry{Agent: agent, IDs: ids})
	}
	return entries
}

// InboxFor returns the list of unread document IDs addressed to the given agent.
// Checks the dynamic Pending map first, falls back to legacy fields.
func (s *State) InboxFor(agent string) []string {
	s.NormalizePending()
	if s.Pending != nil {
		if items, ok := s.Pending[agent]; ok {
			return items
		}
	}
	// Legacy fallback
	switch agent {
	case "codex", "codex-pantheon":
		return s.PendingForCodex
	case "claude", "claude-pantheon":
		return s.PendingForClaude
	default:
		return nil
	}
}

// AddToInbox marks a document as pending for the target agent.
// Uses the dynamic Pending map and syncs to legacy fields for backwards compat.
func (s *State) AddToInbox(target, docID string) {
	s.NormalizePending()
	if s.Pending == nil {
		s.Pending = make(map[string][]string)
	}
	for _, existing := range s.Pending[target] {
		if existing == docID {
			return
		}
	}
	s.Pending[target] = append(s.Pending[target], docID)

	// Sync to legacy fields for backwards compat
	switch target {
	case "codex", "codex-pantheon":
		s.PendingForCodex = s.Pending[target]
	case "claude", "claude-pantheon":
		s.PendingForClaude = s.Pending[target]
	}
}

// ClearInbox removes a document from the target agent's inbox.
func (s *State) ClearInbox(agent, docID string) {
	remove := func(ids []string, id string) []string {
		result := make([]string, 0, len(ids))
		for _, v := range ids {
			if v != id {
				result = append(result, v)
			}
		}
		return result
	}

	if s.Pending != nil {
		if items, ok := s.Pending[agent]; ok {
			s.Pending[agent] = remove(items, docID)
		}
		switch agent {
		case "codex":
			s.Pending["codex-pantheon"] = remove(s.Pending["codex-pantheon"], docID)
		case "codex-pantheon":
			s.Pending["codex"] = remove(s.Pending["codex"], docID)
		case "claude":
			s.Pending["claude-pantheon"] = remove(s.Pending["claude-pantheon"], docID)
		case "claude-pantheon":
			s.Pending["claude"] = remove(s.Pending["claude"], docID)
		}
	}

	// Sync legacy fields
	switch agent {
	case "codex", "codex-pantheon":
		s.PendingForCodex = remove(s.PendingForCodex, docID)
	case "claude", "claude-pantheon":
		s.PendingForClaude = remove(s.PendingForClaude, docID)
	}
	s.NormalizePending()
}

// Router provides access to the idea-router filesystem.
type Router struct {
	root string // path to .agents/idea-router/
}

// New creates a Router rooted at the given repo path.
// It looks for .agents/idea-router/ under repoRoot.
func New(repoRoot string) (*Router, error) {
	root := filepath.Join(repoRoot, ".agents", "idea-router")
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		return nil, fmt.Errorf("idea-router not found at %s", root)
	}
	return &Router{root: root}, nil
}

// FindRepoRoot walks up from cwd to find a .agents/idea-router/ directory.
func FindRepoRoot() (string, error) {
	// Canonical path: the MAIN worktree root (the shared `.git`'s parent), even
	// when called from inside a per-agent worktree. ADR-029 (per-agent session
	// worktrees) fixed `core.bare` corruption but each worktree carries its own
	// git-snapshot copy of `.agents/idea-router/`; resolving the router by a cwd
	// walk-up therefore lands in that STALE copy and fragments the relay — a
	// binding reviewer that never receives a routed review request is a broken
	// gate. The git common dir always points at the main tree, whose
	// `.agents/idea-router/` is the single live router, so we prefer it. We only
	// trust it when the live router actually lives there; otherwise we fall back
	// to the cwd walk-up (non-git checkouts, tests, unusual git dirs).
	if commonDir, ok := getGitCommonDirFn()(); ok {
		root := filepath.Dir(commonDir) // <mainroot>/.git -> <mainroot>
		if info, err := os.Stat(filepath.Join(root, ".agents", "idea-router")); err == nil && info.IsDir() {
			return root, nil
		}
	}

	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		candidate := filepath.Join(dir, ".agents", "idea-router")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			// LAST resort: the persisted marker (~/.sirsi/pantheon-repo). This is
			// what keeps app-context callers alive — a menubar lever spawned by
			// launchd has cwd=/ and no git tree, so both probes above fail even
			// though a perfectly good clone exists. The marker is validated on
			// read (.agents/idea-router must exist there); see reporootmarker.go.
			if root, ok := repoRootFromMarker(); ok {
				return root, nil
			}
			return "", fmt.Errorf("no .agents/idea-router/ found in any parent directory")
		}
		dir = parent
	}
}

// gitCommonDirFn resolves the absolute path of the shared `.git` directory (the
// MAIN worktree's `.git`, even when called from a linked worktree). Injectable
// (Rule A16) and RWMutex-guarded (Rule A21) so tests can exercise the
// canonical / fallback branches without a real repo. ("", false) means "not a
// git tree" — callers fall back to the cwd walk-up.
var (
	gitCommonDirMu sync.RWMutex
	gitCommonDirFn = defaultGitCommonDir
)

func getGitCommonDirFn() func() (string, bool) {
	gitCommonDirMu.RLock()
	defer gitCommonDirMu.RUnlock()
	return gitCommonDirFn
}

func setGitCommonDirFn(fn func() (string, bool)) func() (string, bool) {
	gitCommonDirMu.Lock()
	defer gitCommonDirMu.Unlock()
	old := gitCommonDirFn
	gitCommonDirFn = fn
	return old
}

// defaultGitCommonDir shells `git rev-parse --git-common-dir` (absolute) to find
// the shared `.git`. In a linked worktree this returns the main tree's `.git`,
// so its parent is the canonical repo root.
func defaultGitCommonDir() (string, bool) {
	out, err := exec.Command("git", "rev-parse", "--path-format=absolute", "--git-common-dir").Output()
	if err != nil {
		return "", false
	}
	dir := strings.TrimSpace(string(out))
	if dir == "" {
		return "", false
	}
	return dir, true
}

// ReadState returns the current router state.
func (r *Router) ReadState() (*State, error) {
	data, err := os.ReadFile(filepath.Join(r.root, "state.json"))
	if err != nil {
		return nil, fmt.Errorf("read state: %w", err)
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parse state: %w", err)
	}
	return &state, nil
}

// WriteState persists the router state.
func (r *Router) WriteState(state *State) error {
	state.NormalizePending()
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	return os.WriteFile(filepath.Join(r.root, "state.json"), data, 0o644)
}

// validAuthors is the whitelist of allowed author values.
var validAuthors = map[string]bool{
	"codex":  true,
	"claude": true,
}

// ValidateAuthor checks that author is a legacy whitelist value (codex/claude).
// For v3 registered agent IDs, use ValidateAgent instead.
func ValidateAuthor(author string) error {
	if !validAuthors[author] {
		return fmt.Errorf("author %q is not allowed (must be 'codex' or 'claude')", author)
	}
	return nil
}

// ValidateAgent checks that an agent ID is either a legacy author or a
// registered agent in agents.json. This is the v3 validation path.
func (r *Router) ValidateAgent(agentID string) error {
	// Legacy whitelist
	if validAuthors[agentID] {
		return nil
	}
	// Check registry
	reg, err := LoadRegistry(r.root)
	if err != nil {
		return fmt.Errorf("cannot load agent registry: %w", err)
	}
	if reg.IsRegistered(agentID) {
		return nil
	}
	return fmt.Errorf("agent %q not registered — add to .agents/idea-router/agents.json or use 'codex'/'claude'", agentID)
}

// Submit writes a new document to the router and updates the state.
// Returns the document ID (filename stem).
func (r *Router) Submit(docType DocType, author, title, content string) (string, error) {
	if err := ValidateAuthor(author); err != nil {
		return "", err
	}

	ts := time.Now().Format("20060102-1504")
	slug := slugify(title)
	id := fmt.Sprintf("%s-%s-%s", ts, author, slug)

	var dir string
	switch docType {
	case DocProposal:
		dir = "proposals"
	case DocReview:
		dir = "reviews"
	case DocDecision:
		dir = "decisions"
	default:
		return "", fmt.Errorf("unknown doc type: %s", docType)
	}

	path := filepath.Join(r.root, dir, id+".md")

	// Path containment: verify the resolved path stays under the expected directory
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}
	absDir, err := filepath.Abs(filepath.Join(r.root, dir))
	if err != nil {
		return "", fmt.Errorf("resolve dir: %w", err)
	}
	if !strings.HasPrefix(absPath, absDir+string(os.PathSeparator)) {
		return "", fmt.Errorf("path traversal blocked: %q escapes %q", absPath, absDir)
	}

	if writeErr := os.WriteFile(path, []byte(content), 0o644); writeErr != nil {
		return "", fmt.Errorf("write %s: %w", path, writeErr)
	}

	// Update state: mark last read time for the submitting agent
	state, err := r.ReadState()
	if err != nil {
		return id, nil // file written, state update failed — non-fatal
	}
	now := time.Now().Format(time.RFC3339)
	switch author {
	case "claude":
		state.LastClaudeRead = now
	case "codex":
		state.LastCodexRead = now
	}
	_ = r.WriteState(state)

	return id, nil
}

// SubmitAddressed writes a document and marks it as pending for the target agent.
// The target agent will see it when calling PollInbox.
// Returns an error if addressedTo is invalid — validates before writing the file.
func (r *Router) SubmitAddressed(docType DocType, author, title, content, addressedTo string) (string, error) {
	// Validate addressed_to BEFORE writing the file to avoid false-success
	if addressedTo != "" {
		if err := r.ValidateAgent(addressedTo); err != nil {
			return "", fmt.Errorf("invalid addressed_to: %w", err)
		}
	}

	id, err := r.Submit(docType, author, title, content)
	if err != nil {
		return "", err
	}

	if addressedTo != "" {
		state, err := r.ReadState()
		if err != nil {
			return id, fmt.Errorf("document written (ID: %s) but inbox update failed: %w", id, err)
		}
		state.AddToInbox(addressedTo, id)
		if err := r.WriteState(state); err != nil {
			return id, fmt.Errorf("document written (ID: %s) but inbox update failed: %w", id, err)
		}
	}

	return id, nil
}

// PollInbox returns the unread document IDs for the given agent WITHOUT
// clearing them. Call AckInbox to acknowledge and clear specific items.
func (r *Router) PollInbox(agent string) ([]string, error) {
	if err := r.ValidateAgent(agent); err != nil {
		return nil, err
	}

	state, err := r.ReadState()
	if err != nil {
		return nil, err
	}

	return state.InboxFor(agent), nil
}

// AckInbox acknowledges and removes specific document IDs from the agent's inbox.
// Only acknowledged items are cleared — unacknowledged items remain pending.
func (r *Router) AckInbox(agent string, ids []string) error {
	if err := r.ValidateAgent(agent); err != nil {
		return err
	}

	state, err := r.ReadState()
	if err != nil {
		return err
	}

	for _, id := range ids {
		state.ClearInbox(agent, id)
	}
	now := time.Now().Format(time.RFC3339)
	switch {
	case agent == "claude" || strings.HasPrefix(agent, "claude-"):
		state.LastClaudeRead = now
	case agent == "codex" || strings.HasPrefix(agent, "codex-"):
		state.LastCodexRead = now
	}
	return r.WriteState(state)
}

// List returns all documents across proposals, reviews, and decisions.
func (r *Router) List() ([]Document, error) {
	var docs []Document

	for _, dt := range []DocType{DocProposal, DocReview, DocDecision} {
		dir := filepath.Join(r.root, string(dt)+"s")
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			info, _ := e.Info()
			doc := Document{
				ID:      strings.TrimSuffix(e.Name(), ".md"),
				Type:    dt,
				Path:    filepath.Join(dir, e.Name()),
				ModTime: info.ModTime(),
			}
			// Extract title from first heading
			if data, err := os.ReadFile(doc.Path); err == nil {
				doc.Content = string(data)
				for _, line := range strings.Split(doc.Content, "\n") {
					if strings.HasPrefix(line, "# ") {
						doc.Title = strings.TrimPrefix(line, "# ")
						break
					}
				}
				// Extract author from content
				for _, line := range strings.Split(doc.Content, "\n") {
					trimmed := strings.TrimSpace(line)
					if strings.HasPrefix(trimmed, "author:") || strings.HasPrefix(trimmed, "reviewer:") {
						parts := strings.SplitN(trimmed, ":", 2)
						if len(parts) == 2 {
							doc.Author = strings.TrimSpace(parts[1])
						}
						break
					}
				}
			}
			docs = append(docs, doc)
		}
	}

	sort.Slice(docs, func(i, j int) bool {
		return docs[i].ModTime.After(docs[j].ModTime)
	})
	return docs, nil
}

// Get returns a single document by ID (searches all directories).
func (r *Router) Get(id string) (*Document, error) {
	docs, err := r.List()
	if err != nil {
		return nil, err
	}
	for _, d := range docs {
		if d.ID == id {
			return &d, nil
		}
	}
	return nil, fmt.Errorf("document %q not found", id)
}

// PollSince returns documents modified after the given timestamp.
func (r *Router) PollSince(since time.Time, limit int) ([]Document, error) {
	docs, err := r.List()
	if err != nil {
		return nil, err
	}
	var filtered []Document
	for _, d := range docs {
		if d.ModTime.After(since) {
			filtered = append(filtered, d)
			if limit > 0 && len(filtered) >= limit {
				break
			}
		}
	}
	return filtered, nil
}

func slugify(s string) string {
	s = strings.ToLower(s)
	s = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			return r
		}
		if r == ' ' || r == '-' || r == '_' {
			return '-'
		}
		return -1
	}, s)
	// Collapse multiple dashes
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	s = strings.Trim(s, "-")
	if len(s) > 40 {
		s = s[:40]
	}
	return s
}
