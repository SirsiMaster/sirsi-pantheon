package main_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// seedAgent writes a minimal agents.json under the router dir so `reg.Lookup`
// resolves agentID (a registered agent config, distinct from a live thread).
func seedAgent(t *testing.T, dir, agentID, agentType string) {
	t.Helper()
	reg := map[string]any{
		"agents": map[string]any{
			agentID: map[string]any{
				"id":   agentID,
				"type": agentType,
				"wake": map[string]any{"mechanism": "launchagent"},
			},
		},
	}
	blob, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		t.Fatalf("marshal agents.json: %v", err)
	}
	path := filepath.Join(dir, ".agents", "idea-router", "agents.json")
	if err := os.WriteFile(path, blob, 0o644); err != nil {
		t.Fatalf("write agents.json: %v", err)
	}
}

// runThreadWatch runs `sirsi thread watch ...` in an isolated router dir with HOME
// pinned to a temp home, so the wake LaunchAgent is written/read under
// <home>/Library/LaunchAgents — never the developer's real ~/Library/LaunchAgents
// or launchd domain (the launchctl bootout is a harmless no-op on an unloaded
// label). This is the injected-writer boundary for the CLI path (Rule A16).
func runThreadWatch(t *testing.T, dir, home string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, testBinary, append([]string{"thread", "watch"}, args...)...)
	cmd.Dir = dir
	env := sirsiTestEnv(dir, filepath.Join(dir, "router-test.db"))
	env = append(env,
		"HOME="+home,
		"XDG_CONFIG_HOME="+filepath.Join(home, ".config"),
		"XDG_CACHE_HOME="+filepath.Join(home, ".cache"),
	)
	cmd.Env = env
	cmd.Stdin = nil
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &outBuf, &errBuf
	err = cmd.Run()
	return outBuf.String(), errBuf.String(), err
}

// TestThreadWatchInstallUninstall exercises the thin alias end to end: with
// --agent given, `thread watch --install` installs the SAME durable wake
// LaunchAgent as `router wake-install`, status reports it, and --uninstall removes
// it. It proves the equivalence (same label/plist path) and the clean-off path.
func TestThreadWatchInstallUninstall(t *testing.T) {
	tmp := setupTempRouter(t)
	home := t.TempDir()
	seedAgent(t, tmp, "gemma-tw", "gemma")

	laDir := filepath.Join(home, "Library", "LaunchAgents")
	label := "ai.sirsi.router.wake.gemma-tw"
	plist := filepath.Join(laDir, label+".plist")

	// Status before install → not installed.
	stdout, stderr, err := runThreadWatch(t, tmp, home, "--agent", "gemma-tw", "--json")
	if err != nil {
		t.Fatalf("status: %v\n%s", err, stderr)
	}
	var st map[string]any
	if jerr := json.Unmarshal([]byte(stdout), &st); jerr != nil {
		t.Fatalf("decode status json: %v\n%s", jerr, stdout)
	}
	if st["installed"] != false {
		t.Fatalf("status before install: installed=%v, want false", st["installed"])
	}

	// Install → plist appears under the pinned HOME, not the real one.
	if _, stderr, err = runThreadWatch(t, tmp, home, "--agent", "gemma-tw", "--install"); err != nil {
		t.Fatalf("install: %v\n%s", err, stderr)
	}
	if _, statErr := os.Stat(plist); statErr != nil {
		t.Fatalf("wake plist not written at %s: %v", plist, statErr)
	}

	// Install again → idempotent (no error).
	if _, stderr, err = runThreadWatch(t, tmp, home, "--agent", "gemma-tw", "--install"); err != nil {
		t.Fatalf("install (repeat): %v\n%s", err, stderr)
	}

	// Uninstall → plist gone.
	if _, stderr, err = runThreadWatch(t, tmp, home, "--agent", "gemma-tw", "--uninstall"); err != nil {
		t.Fatalf("uninstall: %v\n%s", err, stderr)
	}
	if _, statErr := os.Stat(plist); !os.IsNotExist(statErr) {
		t.Fatalf("wake plist must be gone after uninstall; stat err=%v", statErr)
	}

	// Uninstall again → clean no-op.
	if _, stderr, err = runThreadWatch(t, tmp, home, "--agent", "gemma-tw", "--uninstall"); err != nil {
		t.Fatalf("uninstall (repeat): %v\n%s", err, stderr)
	}
}

// TestThreadWatchBothFlagsError rejects passing --install and --uninstall together.
func TestThreadWatchBothFlagsError(t *testing.T) {
	tmp := setupTempRouter(t)
	home := t.TempDir()
	seedAgent(t, tmp, "gemma-tw2", "gemma")
	_, _, err := runThreadWatch(t, tmp, home, "--agent", "gemma-tw2", "--install", "--uninstall")
	if err == nil {
		t.Fatal("thread watch --install --uninstall must error")
	}
}

// TestThreadWatchUnresolvedAgentErrors fails helpfully when the current agent
// cannot be resolved (no --agent, no marker, no sole live thread).
func TestThreadWatchUnresolvedAgentErrors(t *testing.T) {
	tmp := setupTempRouter(t)
	home := t.TempDir()
	// No thread registered, no --agent, no SIRSI_AGENT_ID → must error, not guess.
	_, stderr, err := runThreadWatch(t, tmp, home)
	if err == nil {
		t.Fatalf("thread watch with no resolvable agent must error; stderr=%s", stderr)
	}
}
