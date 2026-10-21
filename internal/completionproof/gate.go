// Package completionproof validates the evidence required before a router item
// may be closed from an application worktree.
package completionproof

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Enforce applies the ADR-037 completion-proof gate to callerRoot. callerRoot
// must be the independently resolved invoking application/worktree root; it
// is never derived from the proof argument.
func Enforce(callerRoot, itemID, proof string, blocked, ack bool, result string) error {
	contractPath := filepath.Join(callerRoot, ".agents", "completion.contract.json")
	_, contractErr := os.Stat(contractPath)
	hasContract := contractErr == nil
	if contractErr != nil && !os.IsNotExist(contractErr) {
		return fmt.Errorf("check completion contract: %w", contractErr)
	}

	if blocked {
		if strings.TrimSpace(result) == "" {
			return fmt.Errorf("--blocked requires --result explaining the blocker")
		}
		return nil
	}
	if ack {
		if strings.TrimSpace(result) == "" {
			return fmt.Errorf("--ack requires --result explaining what was acknowledged")
		}
		return nil
	}
	if proof == "" {
		if hasContract {
			return fmt.Errorf("completion proof required for %s: pass --proof .agents/proofs/%s.json, or use --blocked/--ack with --result", callerRoot, itemID)
		}
		return nil
	}
	if !hasContract {
		return fmt.Errorf("--proof supplied but no completion contract exists at %s", contractPath)
	}
	return Validate(callerRoot, proof)
}

// Validate runs the portfolio validator over a proof that is contained by the
// actual caller worktree. Both relative and absolute proof paths are allowed,
// but a proof from another repository (including one reached through a
// symlink) is refused before the validator executes.
func Validate(callerRoot, proof string) error {
	root, err := filepath.EvalSymlinks(callerRoot)
	if err != nil {
		return fmt.Errorf("resolve caller worktree: %w", err)
	}
	proofPath := proof
	if !filepath.IsAbs(proofPath) {
		proofPath = filepath.Join(root, proofPath)
	}
	proofPath, err = filepath.EvalSymlinks(proofPath)
	if err != nil {
		return fmt.Errorf("resolve completion proof: %w", err)
	}
	rel, err := filepath.Rel(root, proofPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return fmt.Errorf("completion proof must be inside caller worktree %s", root)
	}

	script := os.Getenv("SIRSI_COMPLETION_GATE_SCRIPT")
	if script == "" {
		devRoot := filepath.Dir(root)
		script = filepath.Join(devRoot, "tools", "agent_completion_gate.py")
	}
	out, err := exec.Command("python3", script, "validate", "--repo", root, "--proof", proofPath).CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("completion proof validation failed: %s", msg)
	}
	return nil
}
