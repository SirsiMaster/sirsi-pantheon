package setup

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
)

// ── 𓆄 Ma'at pre-push gate ────────────────────────────────────────────────────
//
// The repo ships a pre-push gate at .githooks/pre-push (gofmt + go vet +
// golangci-lint matching CI + diff tests — Rule A17, Ma'at the QA Sovereign).
// Git only runs it when core.hooksPath points at .githooks; a fresh clone
// defaults to .git/hooks (empty), so the gate ships disarmed. ArmMaatGate wires
// it during `sirsi setup` so contributors push pre-gated instead of discovering
// failures in CI after the fact.

// ArmMaatGate sets git core.hooksPath to .githooks for the current Pantheon
// source clone. Idempotent. Skips cleanly when not inside a clone that ships the
// gate (e.g. an end-user `brew install`), so it only ever arms for contributors
// working in the source tree.
func ArmMaatGate() InstallResult {
	res := InstallResult{Surface: SurfaceMaatGate}

	repoRoot, err := router.FindRepoRoot()
	if err != nil {
		res.Status, res.Message = StatusSkipped, "not inside a Pantheon source clone"
		return res
	}
	if _, err := os.Stat(filepath.Join(repoRoot, ".githooks", "pre-push")); err != nil {
		res.Status, res.Message = StatusSkipped, "no .githooks/pre-push — not a Pantheon source clone"
		return res
	}

	// Already armed? Treat as success so re-running setup is a no-op.
	if out, err := exec.Command("git", "-C", repoRoot, "config", "--get", "core.hooksPath").Output(); err == nil {
		if strings.TrimSpace(string(out)) == ".githooks" {
			res.Status, res.Message = StatusOK, "Ma'at pre-push gate already armed"
			return res
		}
	}

	if err := exec.Command("git", "-C", repoRoot, "config", "core.hooksPath", ".githooks").Run(); err != nil {
		res.Status, res.Message = StatusFailed, "git config core.hooksPath failed: "+err.Error()
		return res
	}
	res.Status, res.Message = StatusOK, "Ma'at pre-push gate armed (core.hooksPath=.githooks)"
	return res
}
