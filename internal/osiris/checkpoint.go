package osiris

// Checkpoint COMMIT — the lever behind the risk finding (ADR-033: a CRITICAL
// "commit now!" with no button is an alarm without a remediation; Rule A18
// makes checkpoint commits canon). Local commit ONLY — no push: the lever
// stays reversible (git reset) and never touches a remote.

import (
	"fmt"
	"strings"
)

// CheckpointResult reports one checkpoint attempt.
type CheckpointResult struct {
	Committed      bool   `json:"committed"`
	Hash           string `json:"hash,omitempty"`
	FilesCommitted int    `json:"files_committed"`
	Message        string `json:"message"`
}

// CommitCheckpoint stages everything and commits a Rule-A18 checkpoint in
// repoDir. A clean tree is a successful no-op, not an error.
func CommitCheckpoint(repoDir string) (*CheckpointResult, error) {
	// Bare repos have no work tree — checkpoint the dirtiest linked worktree,
	// the SAME target Assess reports on (2026-07-09: the button errored on the
	// bare sirsi-pantheon root while the view showed real worktree risk).
	if bare, _ := runCommand(repoDir, "git", "rev-parse", "--is-bare-repository"); bare == "true" {
		if wt := dirtiestWorktree(repoDir); wt != "" {
			repoDir = wt
		} else {
			return nil, fmt.Errorf("%s is a bare repository with no linked worktrees", repoDir)
		}
	}
	if out, err := runCommand(repoDir, "git", "rev-parse", "--is-inside-work-tree"); err != nil || strings.TrimSpace(out) != "true" {
		return nil, fmt.Errorf("not a git work tree: %s", repoDir)
	}
	status, err := runCommand(repoDir, "git", "status", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("read status: %w", err)
	}
	var files int
	for _, l := range strings.Split(status, "\n") {
		if strings.TrimSpace(l) != "" {
			files++
		}
	}
	if files == 0 {
		return &CheckpointResult{Committed: false, Message: "working tree clean — nothing to checkpoint"}, nil
	}
	if _, err := runCommand(repoDir, "git", "add", "-A"); err != nil {
		return nil, fmt.Errorf("stage changes: %w", err)
	}
	msg := fmt.Sprintf("chore: checkpoint — %d file(s) secured by Osiris (Rule A18)", files)
	if _, err := runCommand(repoDir, "git", "commit", "-m", msg); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	hash, _ := runCommand(repoDir, "git", "rev-parse", "--short", "HEAD")
	return &CheckpointResult{Committed: true, Hash: strings.TrimSpace(hash), FilesCommitted: files, Message: msg}, nil
}
