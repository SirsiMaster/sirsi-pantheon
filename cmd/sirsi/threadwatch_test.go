package main_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
// pinned to a temp home for files AND an explicit private actuator for launchd.
// HOME does not isolate gui/<UID>; fixed fixture labels must never be booted out
// in the real launchd domain. Use the actual built CLI, including main,
// executeSirsi and its local-receipt guard, not a lighter test-binary entry.
func runThreadWatch(t *testing.T, dir, home string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	return runThreadWatchEnv(t, dir, home, nil, args...)
}

// runThreadWatchEnv is runThreadWatch with extra environment (e.g. the session's
// own SIRSI_AGENT_ID, which --uninstall requires to match --agent).
func runThreadWatchEnv(t *testing.T, dir, home string, extra []string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	return runThreadWatchActuator(t, dir, home, newThreadWatchActuator(t), extra, args...)
}

const threadWatchActuatorScript = "#!/bin/sh\nprintf '%s\\n' \"$@\" >> \"$0.calls\"\n"

func newThreadWatchActuator(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "launchctl")
	if err := os.WriteFile(path, []byte(threadWatchActuatorScript), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func checkThreadWatchActuator(path string) error {
	physical, err := filepath.EvalSymlinks(path)
	if err != nil || !filepath.IsAbs(path) || physical != path || filepath.Base(path) != "launchctl" {
		return fmt.Errorf("thread-watch fixture: actuator must be an existing physical absolute launchctl path")
	}
	dir, err := os.Lstat(filepath.Dir(path))
	if err != nil || !dir.IsDir() || dir.Mode().Perm() != 0o700 {
		return fmt.Errorf("thread-watch fixture: actuator directory is not private")
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o700 || info.Size() != int64(len(threadWatchActuatorScript)) {
		return fmt.Errorf("thread-watch fixture: actuator is not the private executable")
	}
	data, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(data, []byte(threadWatchActuatorScript)) {
		return fmt.Errorf("thread-watch fixture: actuator bytes differ")
	}
	return nil
}

func runThreadWatchActuator(t *testing.T, dir, home, actuator string, extra []string, args ...string) (stdout, stderr string, err error) {
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
	env = append(env, extra...)
	// No inherited or caller-supplied PATH entry may restore real launchctl.
	// The fake uses absolute /bin/sh and printf is a shell builtin. FindRepoRoot's
	// optional git probe may fail normally, then walk this private fixture cwd.
	// No git, system-tool directory, or real-actuator fallback is needed here.
	closed := make([]string, 0, len(env)+1)
	for _, kv := range env {
		if !strings.HasPrefix(kv, "PATH=") {
			closed = append(closed, kv)
		}
	}
	cmd.Env = append(closed, "PATH="+filepath.Dir(actuator))
	cmd.Stdin = nil
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &outBuf, &errBuf
	// Refuse missing/changed fixtures before launching even the built CLI.
	// A later deletion still cannot fall through to a system PATH directory.
	if err := checkThreadWatchActuator(actuator); err != nil {
		return "", "", err
	}
	err = cmd.Run()
	return outBuf.String(), errBuf.String(), err
}

func TestThreadWatchUninstallUsesOnlyPrivateActuator(t *testing.T) {
	tmp, home := setupTempRouter(t), t.TempDir()
	seedAgent(t, tmp, "gemma-private-actuator", "gemma")
	actuator := newThreadWatchActuator(t)
	poison := newThreadWatchActuator(t)
	env := []string{"SIRSI_AGENT_ID=gemma-private-actuator", "PATH=" + filepath.Dir(poison) + string(os.PathListSeparator) + filepath.Dir(testBinary)}
	run := func(args ...string) {
		t.Helper()
		if _, stderr, err := runThreadWatchActuator(t, tmp, home, actuator, env, args...); err != nil {
			t.Fatalf("thread watch %q: %v\n%s", args, err, stderr)
		}
	}
	run("--agent", "gemma-private-actuator", "--install")
	foreign := []string{"SIRSI_AGENT_ID=gemma-foreign", "PATH=" + filepath.Dir(poison)}
	if _, _, err := runThreadWatchActuator(t, tmp, home, actuator, foreign, "--agent", "gemma-private-actuator", "--uninstall"); err == nil {
		t.Fatal("foreign identity must still be refused")
	}
	if _, err := os.Stat(actuator + ".calls"); !os.IsNotExist(err) {
		t.Fatalf("refused uninstall reached actuator: %v", err)
	}
	run("--agent", "gemma-private-actuator", "--uninstall")
	want := fmt.Sprintf("bootout\ngui/%d/ai.sirsi.router.wake.gemma-private-actuator\n", os.Getuid())
	got, err := os.ReadFile(actuator + ".calls")
	if err != nil || string(got) != want {
		t.Fatalf("private actuator calls=%q, err=%v; want %q", got, err, want)
	}
	run("--agent", "gemma-private-actuator", "--uninstall")
	got, err = os.ReadFile(actuator + ".calls")
	if err != nil || string(got) != want {
		t.Fatalf("idempotent uninstall added an actuator call: %q, %v", got, err)
	}
	if _, err := os.Stat(poison + ".calls"); !os.IsNotExist(err) {
		t.Fatalf("PATH actuator must never run: %v", err)
	}

	run("--agent", "gemma-private-actuator", "--install")
	if err := os.Remove(actuator); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runThreadWatchActuator(t, tmp, home, actuator, env, "--agent", "gemma-private-actuator", "--uninstall"); err == nil {
		t.Fatal("missing explicit actuator must fail, never fall back to PATH")
	}
	plist := filepath.Join(home, "Library", "LaunchAgents", "ai.sirsi.router.wake.gemma-private-actuator.plist")
	if _, err := os.Stat(plist); err != nil {
		t.Fatalf("missing actuator must refuse before removing plist: %v", err)
	}
	if _, err := os.Stat(poison + ".calls"); !os.IsNotExist(err) {
		t.Fatalf("missing actuator fell back to PATH: %v", err)
	}
	if err := os.WriteFile(actuator, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runThreadWatchActuator(t, tmp, home, actuator, env, "--agent", "gemma-private-actuator", "--uninstall"); err == nil {
		t.Fatal("changed actuator bytes must be refused")
	}
	if _, err := os.Stat(plist); err != nil {
		t.Fatalf("changed actuator must refuse before removing plist: %v", err)
	}
}

func TestThreadWatchMainRejectsInvalidLocalReceipt(t *testing.T) {
	tmp, home := setupTempRouter(t), t.TempDir()
	seedAgent(t, tmp, "gemma-main-receipt", "gemma")
	actuator := newThreadWatchActuator(t)
	if _, stderr, err := runThreadWatchActuator(t, tmp, home, actuator, nil, "--agent", "gemma-main-receipt", "--install"); err != nil {
		t.Fatalf("install: %v\n%s", err, stderr)
	}
	receipt := filepath.Join(t.TempDir(), "invalid-local-receipt.json")
	if err := os.WriteFile(receipt, []byte(`{"schema":"fixture-invalid"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(receipt, 0o600); err != nil {
		t.Fatal(err)
	}
	env := []string{"SIRSI_AGENT_ID=gemma-main-receipt", "SIRSI_MAAT_LOCAL_EXECUTION_RECEIPT=" + receipt}
	_, stderr, err := runThreadWatchActuator(t, tmp, home, actuator, env, "--agent", "gemma-main-receipt", "--uninstall")
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 1 || !strings.Contains(stderr, "local admission:") {
		t.Fatalf("actual CLI main must reject the invalid receipt: err=%v stderr=%q", err, stderr)
	}
	plist := filepath.Join(home, "Library", "LaunchAgents", "ai.sirsi.router.wake.gemma-main-receipt.plist")
	if _, err := os.Stat(plist); err != nil {
		t.Fatalf("receipt refusal must precede uninstall: %v", err)
	}
	if _, err := os.Stat(actuator + ".calls"); !os.IsNotExist(err) {
		t.Fatalf("receipt refusal reached actuator: %v", err)
	}
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
	if _, stderr, err = runThreadWatchEnv(t, tmp, home, own, "--agent", "gemma-tw", "--uninstall"); err != nil {
		t.Fatalf("uninstall: %v\n%s", err, stderr)
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
		if _, stderr, err := runThreadWatchEnv(t, tmp, home, env, "--agent", "gemma-a", "--uninstall"); err == nil {
			t.Fatalf("env %v: uninstall of another lane must be refused", env)
		} else if !bytes.Contains([]byte(stderr), []byte("never disarms another lane")) {
			t.Fatalf("env %v: wrong refusal: %s", env, stderr)
		}
		if _, statErr := os.Stat(plist); statErr != nil {
			t.Fatalf("env %v: plist must survive a refused uninstall: %v", env, statErr)
		}
	}
	if _, stderr, err := runThreadWatchEnv(t, tmp, home, []string{"SIRSI_AGENT_ID=gemma-a"}, "--agent", "gemma-a", "--uninstall"); err != nil {
		t.Fatalf("own-lane uninstall: %v\n%s", err, stderr)
	}
	if _, statErr := os.Stat(plist); !os.IsNotExist(statErr) {
		t.Fatalf("own-lane uninstall must remove the plist; stat err=%v", statErr)
	}
}
