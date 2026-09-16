package router

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// EnforceCompletionProof is the ADR-037 completion-proof gate: in a repo with
// a .agents/completion.contract.json, a close must carry --proof, --blocked,
// or --ack — a bare close is a done-claim without evidence. Shared by the CLI
// (`sirsi router close`) and any other surface (e.g. MCP) that closes items,
// so neither can bypass the gate the other enforces.
func EnforceCompletionProof(repoRoot, itemID, proof string, blocked, ack bool, result string) error {
	contractPath := filepath.Join(repoRoot, ".agents", "completion.contract.json")
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
			return fmt.Errorf("completion proof required for %s: pass --proof .agents/proofs/%s.json, or use --blocked/--ack with --result", repoRoot, itemID)
		}
		return nil
	}
	if !hasContract {
		return fmt.Errorf("--proof supplied but no completion contract exists at %s", contractPath)
	}
	return ValidateCompletionProof(repoRoot, proof)
}

// ValidateCompletionProof shells out to the portfolio gate validator
// (tools/agent_completion_gate.py beside the repo, or
// SIRSI_COMPLETION_GATE_SCRIPT). The proof schema and validation rules live
// with the portfolio law, not in this binary.
func ValidateCompletionProof(repoRoot, proof string) error {
	script := os.Getenv("SIRSI_COMPLETION_GATE_SCRIPT")
	if script == "" {
		devRoot := filepath.Dir(repoRoot)
		script = filepath.Join(devRoot, "tools", "agent_completion_gate.py")
	}
	proofPath := proof
	if !filepath.IsAbs(proofPath) {
		proofPath = filepath.Join(repoRoot, proofPath)
	}
	out, err := exec.Command("python3", script, "validate", "--repo", repoRoot, "--proof", proofPath).CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("completion proof validation failed: %s", msg)
	}
	return nil
}
