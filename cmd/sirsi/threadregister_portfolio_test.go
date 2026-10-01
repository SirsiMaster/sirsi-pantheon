package main_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// threadRegisterJSON mirrors the subset of `thread register --json` output
// this test cares about (thread_id, surface, repo).
type threadRegisterJSON struct {
	ThreadID string `json:"thread_id"`
	Repo     string `json:"repo"`
}

// registerThreadPortfolio runs `sirsi thread register --json` in dir, with
// the shared-router marker pointed at markerTarget (empty = no marker).
// GIT_* and SIRSI_AGENT_ID are stripped by sirsiTestEnv so FindRepoRoot
// resolves purely from dir's own cwd walk-up and the marker — never the real
// developer clone. Output is read from --json, not by reading a registry file
// off disk, since the registry's actual backing store (JSON file vs.
// routerstore-backed cutover) is an implementation detail router.FindRepoRoot
// callers must not assume.
func registerThreadPortfolio(t *testing.T, dir, home, markerTarget string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	fullArgs := append([]string{"thread", "register", "--anchor-pid", "1", "--json"}, args...)
	cmd := exec.CommandContext(ctx, testBinary, fullArgs...)
	cmd.Dir = dir
	markerPath := filepath.Join(home, ".sirsi", "pantheon-repo-marker")
	env := sirsiTestEnv(dir, filepath.Join(dir, "router-test.db"),
		"HOME="+home,
		"SIRSI_PANTHEON_REPO_MARKER="+markerPath,
		// Force the legacy JSON-file registry backend so this test's
		// assertions aren't coupled to the routerstore cutover state of
		// whatever host runs it (see routercfg.StoreWake).
		"SIRSI_ROUTER_STORE_WAKE=0",
	)
	if markerTarget != "" {
		if mkErr := os.MkdirAll(filepath.Dir(markerPath), 0o755); mkErr != nil {
			t.Fatalf("mkdir marker dir: %v", mkErr)
		}
		if wErr := os.WriteFile(markerPath, []byte(markerTarget+"\n"), 0o644); wErr != nil {
			t.Fatalf("write marker: %v", wErr)
		}
	}
	cmd.Env = env
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &outBuf, &errBuf
	err = cmd.Run()
	return outBuf.String(), errBuf.String(), err
}

// TestThreadRegister_PortfolioRepoSharesAuthoritativeRouter: a target repo
// that correctly has no .agents/idea-router of its own (a portfolio repo like
// FinalWishes/sirsi-io) must still register successfully by resolving the
// authoritative Pantheon router through the existing marker-based workspace
// resolver, while the registered thread keeps the portfolio repo as its Repo
// metadata (item 20260930-230920).
func TestThreadRegister_PortfolioRepoSharesAuthoritativeRouter(t *testing.T) {
	home := t.TempDir()
	routerHome := setupTempRouter(t)
	portfolioRepo := t.TempDir() // no .agents/idea-router here

	stdout, stderr, err := registerThreadPortfolio(t, portfolioRepo, home, routerHome,
		"--agent", "claude-portfolio", "--surface", "claude", "--repo", portfolioRepo)
	if err != nil {
		t.Fatalf("register failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	var out threadRegisterJSON
	if jsonErr := json.Unmarshal([]byte(stdout), &out); jsonErr != nil {
		t.Fatalf("decode register --json: %v\nstdout=%s", jsonErr, stdout)
	}
	if out.Repo != portfolioRepo {
		t.Fatalf("expected registered thread to retain the portfolio repo as its Repo metadata, got %q want %q", out.Repo, portfolioRepo)
	}

	regPath := filepath.Join(routerHome, ".agents", "idea-router", "threads.json")
	blob, readErr := os.ReadFile(regPath)
	if readErr != nil {
		t.Fatalf("expected the thread to land in the authoritative router's registry at %s: %v", regPath, readErr)
	}
	if !strings.Contains(string(blob), portfolioRepo) {
		t.Fatalf("expected the authoritative registry to record the portfolio repo, got %s", blob)
	}
}

// TestThreadRegister_OrdinaryPantheonRepoStillWorks is the control: a target
// repo that IS the Pantheon router root continues to register exactly as
// before.
func TestThreadRegister_OrdinaryPantheonRepoStillWorks(t *testing.T) {
	home := t.TempDir()
	pantheonRepo := setupTempRouter(t)

	stdout, stderr, err := registerThreadPortfolio(t, pantheonRepo, home, "",
		"--agent", "claude-pantheon-test", "--surface", "claude", "--repo", pantheonRepo)
	if err != nil {
		t.Fatalf("register failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	var out threadRegisterJSON
	if jsonErr := json.Unmarshal([]byte(stdout), &out); jsonErr != nil {
		t.Fatalf("decode register --json: %v\nstdout=%s", jsonErr, stdout)
	}
	if out.Repo != pantheonRepo {
		t.Fatalf("expected Repo %q, got %q", pantheonRepo, out.Repo)
	}

	regPath := filepath.Join(pantheonRepo, ".agents", "idea-router", "threads.json")
	if _, statErr := os.Stat(regPath); statErr != nil {
		t.Fatalf("expected the thread registry at %s: %v", regPath, statErr)
	}
}

// TestThreadRegister_MissingAuthoritativeRouterFails_NoRepoFlag: no --repo and
// no router reachable by workspace resolution must fail, exactly as before —
// no new router home is ever invented.
func TestThreadRegister_MissingAuthoritativeRouterFails_NoRepoFlag(t *testing.T) {
	home := t.TempDir()
	bareDir := t.TempDir()

	_, stderr, err := registerThreadPortfolio(t, bareDir, home, "",
		"--agent", "claude-x", "--surface", "claude")
	if err == nil {
		t.Fatalf("expected failure with no router reachable, got success")
	}
	if !strings.Contains(stderr, "idea-router") {
		t.Fatalf("expected an idea-router resolution error, got stderr=%s", stderr)
	}
}

// TestThreadRegister_ExplicitWrongScopeFails: an explicit --repo that has no
// router of its own, with no shared/authoritative router reachable either,
// must fail — it must NOT silently fork a new router home at --repo.
func TestThreadRegister_ExplicitWrongScopeFails(t *testing.T) {
	home := t.TempDir()
	wrongRepo := t.TempDir() // no .agents/idea-router, no marker set up

	_, stderr, err := registerThreadPortfolio(t, wrongRepo, home, "",
		"--agent", "claude-x", "--surface", "claude", "--repo", wrongRepo)
	if err == nil {
		t.Fatalf("expected failure for an explicit --repo with no reachable authoritative router, got success")
	}
	if !strings.Contains(stderr, "no authoritative router found") {
		t.Fatalf("expected the explicit-scope error, got stderr=%s", stderr)
	}
	if _, statErr := os.Stat(filepath.Join(wrongRepo, ".agents")); statErr == nil {
		t.Fatalf("must not have forked a new router home at --repo %s", wrongRepo)
	}
}
