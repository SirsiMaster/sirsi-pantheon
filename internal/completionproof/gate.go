// Package completionproof validates the evidence required before a router item
// may be closed from an application worktree.
package completionproof

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
)

const maxCompletionProofBytes = 4 << 20

// Enforce applies the ADR-037 completion-proof gate to callerRoot. callerRoot
// must be the independently resolved invoking application/worktree root; it
// is never derived from the proof argument.
func Enforce(callerRoot, itemID, proof string, blocked, ack bool, result string) error {
	return enforce(callerRoot, "", itemID, proof, blocked, ack, result)
}

func enforce(callerRoot, canonicalRouterRoot, itemID, proof string, blocked, ack bool, result string) error {
	if !RequiresCallerRoot(blocked, ack) {
		if strings.TrimSpace(result) == "" {
			if blocked {
				return fmt.Errorf("--blocked requires --result explaining the blocker")
			}
			return fmt.Errorf("--ack requires --result explaining what was acknowledged")
		}
		return nil
	}
	if strings.TrimSpace(callerRoot) == "" {
		return fmt.Errorf("cannot resolve actual caller worktree for completion proof")
	}
	contractPath := filepath.Join(callerRoot, ".agents", "completion.contract.json")
	_, contractErr := os.Stat(contractPath)
	hasContract := contractErr == nil
	if contractErr != nil && !os.IsNotExist(contractErr) {
		return fmt.Errorf("check completion contract: %w", contractErr)
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
	return validateForCompletion(callerRoot, canonicalRouterRoot, proof)
}

// RequiresCallerRoot reports whether a close needs a caller worktree. Explicit
// coordination outcomes are independently auditable through their non-empty
// result and must keep working for an attended non-Git client; implementation
// completion (bare or proof-bearing) always needs an actual Git caller.
func RequiresCallerRoot(blocked, ack bool) bool {
	return !blocked && !ack
}

// EnforceCurrentCaller applies Enforce after resolving a Git worktree only
// where implementation completion needs one. It deliberately has no marker or
// environment fallback: an unproven completion must fail closed.
func EnforceCurrentCaller(canonicalRouterRoot, itemID, proof string, blocked, ack bool, result string) error {
	if !RequiresCallerRoot(blocked, ack) {
		return enforce("", canonicalRouterRoot, itemID, proof, blocked, ack, result)
	}
	callerRoot, err := router.FindCallerRepoRoot()
	if err != nil {
		return fmt.Errorf("cannot resolve actual caller worktree for completion proof: %w", err)
	}
	return enforce(callerRoot, canonicalRouterRoot, itemID, proof, blocked, ack, result)
}

// Validate runs the portfolio validator over a proof that is contained by the
// actual caller worktree. Both relative and absolute proof paths are allowed,
// but a proof from another repository (including one reached through a
// symlink) is refused before the validator executes.
func Validate(callerRoot, proof string) error {
	root, proofPath, err := resolveProof(callerRoot, proof)
	if err != nil {
		return err
	}
	return runTrustedValidator(root, "", proofPath)
}

func validateForCompletion(callerRoot, canonicalRouterRoot, proof string) error {
	root, proofPath, err := resolveProof(callerRoot, proof)
	if err != nil {
		return err
	}
	contents, err := captureCompletionProof(proofPath)
	if err != nil {
		return err
	}
	return runCompletionValidator(root, canonicalRouterRoot, proofPath, contents)
}

func resolveProof(callerRoot, proof string) (string, string, error) {
	root, err := filepath.EvalSymlinks(callerRoot)
	if err != nil {
		return "", "", fmt.Errorf("resolve caller worktree: %w", err)
	}
	proofPath := proof
	if !filepath.IsAbs(proofPath) {
		proofPath = filepath.Join(root, proofPath)
	}
	proofPath, err = filepath.EvalSymlinks(proofPath)
	if err != nil {
		return "", "", fmt.Errorf("resolve completion proof: %w", err)
	}
	rel, err := filepath.Rel(root, proofPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", "", fmt.Errorf("completion proof must be inside caller worktree %s", root)
	}
	return root, proofPath, nil
}

func runTrustedValidator(root, canonicalRouterRoot, proofPath string) error {
	script, err := trustedValidator(canonicalRouterRoot)
	if err != nil {
		return err
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

// runCompletionValidator sends the one captured proof document to the
// completion-aware validator. That validator treats --proof as diagnostic
// metadata only; it must never reopen the mutable proof pathname.
func runCompletionValidator(root, canonicalRouterRoot, proofPath string, contents []byte) error {
	script, err := trustedValidator(canonicalRouterRoot)
	if err != nil {
		return err
	}
	cmd := exec.Command("python3", script, "validate-completion", "--repo", root, "--proof", proofPath)
	cmd.Stdin = bytes.NewReader(contents)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("completion proof validation failed: %s", msg)
	}
	return nil
}

func captureCompletionProof(proofPath string) ([]byte, error) {
	proofFile, err := openCompletionProof(proofPath)
	if err != nil {
		return nil, fmt.Errorf("open completion proof: %w", err)
	}
	defer proofFile.Close()

	info, err := proofFile.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat completion proof: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("completion proof must be a regular file: %s", proofPath)
	}
	if info.Size() > maxCompletionProofBytes {
		return nil, fmt.Errorf("completion proof exceeds %d-byte limit", maxCompletionProofBytes)
	}
	contents, err := io.ReadAll(io.LimitReader(proofFile, maxCompletionProofBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read completion proof: %w", err)
	}
	if len(contents) > maxCompletionProofBytes {
		return nil, fmt.Errorf("completion proof exceeds %d-byte limit", maxCompletionProofBytes)
	}
	return contents, nil
}

// trustedValidator resolves portfolio law from the canonical router checkout,
// never from the caller worktree. A linked application worktree may live under
// .worktrees/ or any other sibling layout; it is valid proof authority but is
// not allowed to choose the validator that judges it.
func trustedValidator(canonicalRouterRoot string) (string, error) {
	if script := strings.TrimSpace(os.Getenv("SIRSI_COMPLETION_GATE_SCRIPT")); script != "" {
		return script, nil
	}
	canonicalRoot := strings.TrimSpace(canonicalRouterRoot)
	if canonicalRoot == "" {
		var err error
		canonicalRoot, err = router.FindRepoRoot()
		if err != nil {
			return "", fmt.Errorf("locate trusted completion validator: canonical router root: %w", err)
		}
	}
	script := filepath.Join(filepath.Dir(filepath.Clean(canonicalRoot)), "tools", "agent_completion_gate.py")
	info, err := os.Stat(script)
	if err != nil {
		return "", fmt.Errorf("locate trusted completion validator at %s: %w", script, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("trusted completion validator is not a regular file: %s", script)
	}
	return script, nil
}
