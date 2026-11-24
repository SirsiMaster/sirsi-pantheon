// Package completionproof validates evidence before application work is closed.
package completionproof

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const maxCompletionProofBytes = 4 << 20
const completionValidatorTimeout = 30 * time.Second
const completionValidatorWaitDelay = 2 * time.Second
const completionPython = "/usr/bin/python3"
const completionGit = "/usr/bin/git"

// This is production source, shipped with the Go binary. Caller paths, Git,
// HOME and SIRSI_* variables cannot select or reopen its implementation.
//
//go:embed validator/agent_completion_gate.py
var productionValidator string

// ValidatorSHA256 identifies exact standalone bytes, not release approval or
// interpreter provenance.
func ValidatorSHA256() string { return fmt.Sprintf("%x", sha256.Sum256([]byte(productionValidator))) }

// ExportValidator writes the same bytes to an explicitly named absent file.
// It never discovers an install directory, creates parents or overwrites a file.
func ExportValidator(path string) error { return exportValidator(path, []byte(productionValidator)) }

// Enforce requires an independently resolved caller root, never proof-derived
// authority. It does not replace the caller's actor, session or lease fences.
func Enforce(callerRoot, itemID, proof string, blocked, ack bool, result string) error {
	return enforce(callerRoot, "", itemID, proof, blocked, ack, result)
}

// The router-root argument remains API-compatible, NOT helper authority.
func enforce(callerRoot, _ string, itemID, proof string, blocked, ack bool, result string) error {
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
	_, err := os.Stat(filepath.Join(callerRoot, ".agents", "completion.contract.json"))
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("check completion contract: %w", err)
	}
	if proof == "" {
		if err == nil {
			return fmt.Errorf("completion proof required: pass --proof, or use --blocked/--ack with --result")
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("--proof supplied but no completion contract exists")
	}
	root, relative, display, err := resolveProofCapture(callerRoot, proof)
	if err != nil {
		return err
	}
	contents, err := captureCompletionProof(root, relative)
	if err != nil {
		return err
	}
	return runCompletionValidator(root, itemID, display, contents)
}

func RequiresCallerRoot(blocked, ack bool) bool { return !blocked && !ack }

// EnforceCurrentCaller discovers only the invoking worktree, with fixed Git
// and without inherited Git configuration/redirectors. The router-root argument
// is intentionally irrelevant to validator authority.
func EnforceCurrentCaller(canonicalRouterRoot, itemID, proof string, blocked, ack bool, result string) error {
	if !RequiresCallerRoot(blocked, ack) {
		return enforce("", canonicalRouterRoot, itemID, proof, blocked, ack, result)
	}
	root, err := currentCallerRoot()
	if err != nil {
		return err
	}
	return enforce(root, canonicalRouterRoot, itemID, proof, blocked, ack, result)
}

func currentCallerRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve completion caller directory: %w", err)
	}
	actualCWD, err := filepath.EvalSymlinks(cwd)
	if err != nil {
		return "", fmt.Errorf("resolve physical caller directory: %w", err)
	}
	boundary, err := nearestGitBoundary(actualCWD)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), completionValidatorTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, completionGit, "-C", actualCWD, "rev-parse", "--show-toplevel")
	cmd.Dir = actualCWD
	cmd.Env = append(completionValidatorEnv(os.Environ()), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
	cmd.WaitDelay = completionValidatorWaitDelay
	var output limitedOutput
	cmd.Stdout, cmd.Stderr = &output, io.Discard
	if err := cmd.Run(); err != nil || ctx.Err() != nil || output.exceeded {
		return "", fmt.Errorf("cannot resolve actual Git caller worktree for completion proof")
	}
	root := strings.TrimSuffix(string(output.data), "\n")
	if !filepath.IsAbs(root) || strings.ContainsAny(root, "\r\n\x00") {
		return "", fmt.Errorf("invalid Git caller worktree")
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("resolve Git caller worktree: %w", err)
	}
	if root != boundary {
		return "", fmt.Errorf("Git worktree does not match nearest physical caller boundary")
	}
	return root, nil
}

// nearestGitBoundary is independent of Git config, including core.worktree.
// Stop at the first physical marker: never follow a symlink or skip an invalid
// marker to adopt an ancestor. Git still validates regular gitfiles (linked
// worktrees/submodules) and directory-backed repositories in currentCallerRoot.
// This pathname observation does not establish immutable ancestry across rename.
func nearestGitBoundary(physicalCWD string) (string, error) {
	for dir := physicalCWD; ; dir = filepath.Dir(dir) {
		marker, err := os.Lstat(filepath.Join(dir, ".git"))
		if err == nil {
			if marker.IsDir() || marker.Mode().IsRegular() {
				return dir, nil
			}
			return "", fmt.Errorf("invalid physical Git boundary marker; symlinks and special files are refused")
		}
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("inspect physical Git boundary marker: %w", err)
		}
		if filepath.Dir(dir) == dir {
			return "", fmt.Errorf("cannot resolve nearest physical Git caller boundary")
		}
	}
}

// Validate is diagnostic: draft/blocked evidence remains supported. It uses
// captured bytes and embedded policy but does not authorize an item.
func Validate(callerRoot, proof string) error {
	root, relative, display, err := resolveProofCapture(callerRoot, proof)
	if err != nil {
		return err
	}
	contents, err := captureCompletionProof(root, relative)
	if err != nil {
		return err
	}
	return runValidatorCommand(root, []string{"validate", "--proof-stdin", "--repo", root, "--proof", display}, contents, completionValidatorTimeout)
}

func runCompletionValidator(root, itemID, display string, contents []byte) error {
	return runValidatorCommand(root, []string{"validate-completion", "--repo", root, "--proof", display,
		"--expected-item-id", itemID}, contents, completionValidatorTimeout)
}

func runValidatorCommand(root string, args []string, stdin []byte, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, completionPython, append([]string{"-I", "-S", "-c", productionValidator}, args...)...)
	cmd.Dir = root
	cmd.Env = completionValidatorEnv(os.Environ())
	cmd.Stdin = bytes.NewReader(stdin)
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	cmd.WaitDelay = completionValidatorWaitDelay
	err := cmd.Run()
	if ctx.Err() != nil {
		return fmt.Errorf("completion proof validator timed out")
	}
	if err != nil {
		return fmt.Errorf("completion proof validation failed")
	}
	return nil
}

// Fixed system launchers still depend on configured system tooling/libraries.
// Filtering removes known startup/loader/developer selectors, not that separate
// provenance obligation. CommandContext does not guarantee descendant cleanup.
func completionValidatorEnv(environ []string) []string {
	filtered := make([]string, 0, len(environ))
	for _, entry := range environ {
		key, _, _ := strings.Cut(entry, "=")
		key = strings.ToUpper(key)
		if strings.HasPrefix(key, "PYTHON") || strings.HasPrefix(key, "LD_") || strings.HasPrefix(key, "DYLD_") ||
			strings.HasPrefix(key, "XCRUN_") || strings.HasPrefix(key, "GIT_") || strings.HasPrefix(key, "SIRSI_") ||
			key == "DEVELOPER_DIR" || key == "SDKROOT" || key == "TOOLCHAINS" || key == "GLIBC_TUNABLES" || key == "GCONV_PATH" {
			continue
		}
		filtered = append(filtered, entry)
	}
	return filtered
}

// Lexical containment then descriptor-relative no-follow traversal pins a read,
// not ancestry against rename, hardlink ownership, in-place writes, or a single
// immutable snapshot of contract/canon/proof.
func resolveProofCapture(callerRoot, proof string) (root, relative, display string, err error) {
	root, err = filepath.Abs(callerRoot)
	if err == nil {
		root, err = filepath.EvalSymlinks(root)
	}
	if err != nil {
		return "", "", "", fmt.Errorf("resolve caller worktree: %w", err)
	}
	path := proof
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	relative, err = filepath.Rel(root, filepath.Clean(path))
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return "", "", "", fmt.Errorf("completion proof must be inside caller worktree")
	}
	return root, relative, filepath.Join(root, relative), nil
}

func captureCompletionProof(root, relative string) ([]byte, error) {
	f, err := openCompletionProof(root, relative)
	if err != nil {
		return nil, fmt.Errorf("open completion proof: %w", err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat completion proof: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("completion proof must be a regular file")
	}
	if info.Size() > maxCompletionProofBytes {
		return nil, fmt.Errorf("completion proof exceeds byte limit")
	}
	contents, err := io.ReadAll(io.LimitReader(f, maxCompletionProofBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read completion proof: %w", err)
	}
	if len(contents) > maxCompletionProofBytes {
		return nil, fmt.Errorf("completion proof exceeds byte limit")
	}
	return contents, nil
}

type limitedOutput struct {
	data     []byte
	exceeded bool
}

func (o *limitedOutput) Write(p []byte) (int, error) {
	n := len(p)
	const limit = 16 << 10
	if n > limit-len(o.data) {
		o.exceeded = true
		p = p[:limit-len(o.data)]
	}
	o.data = append(o.data, p...)
	return n, nil
}
