package main_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
// or the real launchd controller. A temporary HOME does not isolate the logged-in
// user's GUI domain: the child also gets a private inert launchctl on PATH.
// This is a fixture transport boundary, not a product uninstall bypass.
func runThreadWatch(t *testing.T, dir, home string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	return runThreadWatchEnv(t, dir, home, nil, args...)
}

// runThreadWatchEnv is runThreadWatch with extra environment (e.g. the session's
// own SIRSI_AGENT_ID, which --uninstall requires to match --agent).
func runThreadWatchEnv(t *testing.T, dir, home string, extra []string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	stdout, stderr, _, err = runThreadWatchEnvCaptured(t, dir, home, extra, args...)
	return stdout, stderr, err
}

// Capture external-controller argv without invoking any real service controller.
// A fresh executable/log per invocation avoids sharing fixture state across tests.
func runThreadWatchEnvCaptured(t *testing.T, dir, home string, extra []string, args ...string) (stdout, stderr, calls string, err error) {
	t.Helper()
	controllerDir := t.TempDir()
	controllerLog := filepath.Join(controllerDir, "launchctl-args")
	controller := filepath.Join(controllerDir, "launchctl")
	if werr := os.WriteFile(controller, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" >> \"$SIRSI_TEST_LAUNCHCTL_LOG\"\n"), 0o700); werr != nil {
		t.Fatal(werr)
	}
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
	env = append(env, extra...)
	// Last entries win; callers cannot accidentally restore the real controller
	// through an extra PATH. Keep other tools available after this inert front door.
	env = append(env,
		"PATH="+controllerDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"SIRSI_TEST_LAUNCHCTL_LOG="+controllerLog,
	)
	cmd.Env = env
	cmd.Stdin = nil
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &outBuf, &errBuf
	err = cmd.Run()
	if raw, rerr := os.ReadFile(controllerLog); rerr == nil {
		calls = string(raw)
	} else if !os.IsNotExist(rerr) {
		t.Fatal(rerr)
	}
	return outBuf.String(), errBuf.String(), calls, err
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
	own := []string{"SIRSI_AGENT_ID=gemma-tw"}
	_, stderr, calls, err := runThreadWatchEnvCaptured(t, tmp, home, own, "--agent", "gemma-tw", "--uninstall")
	if err != nil {
		t.Fatalf("uninstall: %v\n%s", err, stderr)
	}
	if want := fmt.Sprintf("bootout\ngui/%d/%s\n", os.Getuid(), label); calls != want {
		t.Fatalf("owned uninstall must reach only the inert controller with exact argv: %q, want %q", calls, want)
	}
	if _, statErr := os.Stat(plist); !os.IsNotExist(statErr) {
		t.Fatalf("wake plist must be gone after uninstall; stat err=%v", statErr)
	}

	// Uninstall again → clean no-op.
	if _, stderr, err = runThreadWatchEnv(t, tmp, home, own, "--agent", "gemma-tw", "--uninstall"); err != nil {
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

// TestThreadWatchUninstallRefusesAnotherLane: --uninstall acts only on the
// session's own lane. A session identified as gemma-b cannot remove gemma-a's
// wake channel through --agent; the plist survives; gemma-a itself can.
func TestThreadWatchUninstallRefusesAnotherLane(t *testing.T) {
	tmp := setupTempRouter(t)
	home := t.TempDir()
	reg := map[string]any{"agents": map[string]any{
		"gemma-a": map[string]any{"id": "gemma-a", "type": "gemma", "wake": map[string]any{"mechanism": "launchagent"}},
		"gemma-b": map[string]any{"id": "gemma-b", "type": "gemma", "wake": map[string]any{"mechanism": "launchagent"}},
	}}
	blob, _ := json.MarshalIndent(reg, "", "  ")
	if err := os.WriteFile(filepath.Join(tmp, ".agents", "idea-router", "agents.json"), blob, 0o644); err != nil {
		t.Fatal(err)
	}
	plist := filepath.Join(home, "Library", "LaunchAgents", "ai.sirsi.router.wake.gemma-a.plist")
	if _, stderr, err := runThreadWatch(t, tmp, home, "--agent", "gemma-a", "--install"); err != nil {
		t.Fatalf("install: %v\n%s", err, stderr)
	}
	// Another lane, and an unidentified session, are both refused.
	for _, env := range [][]string{{"SIRSI_AGENT_ID=gemma-b"}, nil} {
		_, stderr, calls, err := runThreadWatchEnvCaptured(t, tmp, home, env, "--agent", "gemma-a", "--uninstall")
		if err == nil {
			t.Fatalf("env %v: uninstall of another lane must be refused", env)
		} else if !bytes.Contains([]byte(stderr), []byte("never disarms another lane")) {
			t.Fatalf("env %v: wrong refusal: %s", env, stderr)
		}
		if calls != "" {
			t.Fatalf("refused uninstall reached the controller: %q", calls)
		}
		if _, statErr := os.Stat(plist); statErr != nil {
			t.Fatalf("env %v: plist must survive a refused uninstall: %v", env, statErr)
		}
	}
	_, stderr, calls, err := runThreadWatchEnvCaptured(t, tmp, home, []string{"SIRSI_AGENT_ID=gemma-a"}, "--agent", "gemma-a", "--uninstall")
	if err != nil {
		t.Fatalf("own-lane uninstall: %v\n%s", err, stderr)
	}
	if want := fmt.Sprintf("bootout\ngui/%d/ai.sirsi.router.wake.gemma-a\n", os.Getuid()); calls != want {
		t.Fatalf("own-lane uninstall did not reach the exact inert controller: got %q, want %q", calls, want)
	}
	if _, statErr := os.Stat(plist); !os.IsNotExist(statErr) {
		t.Fatalf("own-lane uninstall must remove the plist; stat err=%v", statErr)
	}
}

// Even when a controller exists in the inherited PATH, the fixture must shadow
// it. The ambient controller here is itself an inert spy, so a regression in the
// fixture boundary can be shown red without reaching real launchctl.
func TestThreadWatchFixtureShadowsAmbientLaunchctl(t *testing.T) {
	tmp := setupTempRouter(t)
	home := t.TempDir()
	seedAgent(t, tmp, "gemma-tw", "gemma")
	ambientDir := t.TempDir()
	marker := filepath.Join(ambientDir, "ambient-reached")
	spy := "#!/bin/sh\nprintf reached > \"$SIRSI_TEST_AMBIENT_LAUNCHCTL_MARKER\"\nexit 99\n"
	if err := os.WriteFile(filepath.Join(ambientDir, "launchctl"), []byte(spy), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", ambientDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("SIRSI_TEST_AMBIENT_LAUNCHCTL_MARKER", marker)
	if _, stderr, err := runThreadWatch(t, tmp, home, "--agent", "gemma-tw", "--install"); err != nil {
		t.Fatalf("install: %v\n%s", err, stderr)
	}
	_, stderr, calls, err := runThreadWatchEnvCaptured(t, tmp, home, []string{"SIRSI_AGENT_ID=gemma-tw"}, "--agent", "gemma-tw", "--uninstall")
	if err != nil {
		t.Fatalf("uninstall: %v\n%s", err, stderr)
	}
	if want := fmt.Sprintf("bootout\ngui/%d/ai.sirsi.router.wake.gemma-tw\n", os.Getuid()); calls != want {
		t.Errorf("private controller did not capture exact uninstall: got %q, want %q", calls, want)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Errorf("fixture reached ambient launchctl, or cannot verify isolation: %v", err)
	}
}
