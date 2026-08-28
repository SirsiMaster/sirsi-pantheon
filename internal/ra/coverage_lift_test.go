package ra

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// NOTE: none of the tests in this file use t.Parallel — they swap the
// package-level execCommand seam and redirect $HOME,
// so they must run sequentially (repo law from PRs #129/#131).

// swapExec replaces the execCommand seam for the duration of the test.
func swapExec(t *testing.T, fn func(name string, args ...string) *exec.Cmd) {
	t.Helper()
	orig := execCommand
	execCommand = fn
	t.Cleanup(func() { execCommand = orig })
}

// execAlwaysTrue is a fake exec that succeeds without side effects.
func execAlwaysTrue(string, ...string) *exec.Cmd { return exec.Command("true") }

// execAlwaysFalse is a fake exec that fails without side effects.
func execAlwaysFalse(string, ...string) *exec.Cmd { return exec.Command("false") }

// ── terminal.go ─────────────────────────────────────────────────────────────

func spawnConfigForTest(t *testing.T, name string) SpawnConfig {
	t.Helper()
	tmp := t.TempDir()
	promptFile := filepath.Join(tmp, "prompt.md")
	if err := os.WriteFile(promptFile, []byte("do the work"), 0o644); err != nil {
		t.Fatal(err)
	}
	return SpawnConfig{
		Name:       name,
		Title:      "𓇶 Ra: " + name,
		WorkDir:    tmp,
		PromptFile: promptFile,
		LogFile:    filepath.Join(tmp, "logs", name+".log"),
		ExitFile:   filepath.Join(tmp, "exits", name+".exit"),
		PIDFile:    filepath.Join(tmp, "pids", name+".pid"),
	}
}

func TestSpawnWindow_Success(t *testing.T) {
	swapExec(t, execAlwaysTrue)

	cfg := spawnConfigForTest(t, "alpha")
	cfg.Sprints = 3 // exercise the multi-sprint loop

	res, err := SpawnWindow(cfg)
	if err != nil {
		t.Fatalf("SpawnWindow() error = %v", err)
	}
	if res.Name != "alpha" {
		t.Errorf("Name = %q, want alpha", res.Name)
	}
	if res.PIDFile != cfg.PIDFile || res.LogFile != cfg.LogFile {
		t.Errorf("tracking files not propagated: %+v", res)
	}
	for _, d := range []string{filepath.Dir(cfg.LogFile), filepath.Dir(cfg.ExitFile), filepath.Dir(cfg.PIDFile)} {
		if fi, err := os.Stat(d); err != nil || !fi.IsDir() {
			t.Errorf("tracking dir %s not created", d)
		}
	}
}

func TestSpawnWindow_ITerm2AndDefaultSprints(t *testing.T) {
	swapExec(t, execAlwaysTrue)

	cfg := spawnConfigForTest(t, "beta")
	cfg.UseITerm2 = true
	cfg.Sprints = 0 // must default to 1

	if _, err := SpawnWindow(cfg); err != nil {
		t.Fatalf("SpawnWindow(iTerm2) error = %v", err)
	}
}

func TestSpawnWindow_OsascriptFails(t *testing.T) {
	swapExec(t, execAlwaysFalse)

	cfg := spawnConfigForTest(t, "gamma")
	_, err := SpawnWindow(cfg)
	if err == nil || !strings.Contains(err.Error(), "osascript failed") {
		t.Fatalf("expected osascript failure, got %v", err)
	}
}

func TestSpawnWindow_MkdirFails(t *testing.T) {
	swapExec(t, execAlwaysTrue)

	tmp := t.TempDir()
	blocker := filepath.Join(tmp, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := spawnConfigForTest(t, "delta")
	cfg.LogFile = filepath.Join(blocker, "sub", "delta.log") // parent is a file

	_, err := SpawnWindow(cfg)
	if err == nil || !strings.Contains(err.Error(), "create dir") {
		t.Fatalf("expected create dir failure, got %v", err)
	}
}

func TestKillWindow_Errors(t *testing.T) {
	if err := KillWindow(filepath.Join(t.TempDir(), "nope.pid")); err == nil {
		t.Error("expected error for missing pid file")
	}

	bad := filepath.Join(t.TempDir(), "bad.pid")
	os.WriteFile(bad, []byte("not-a-pid"), 0o644)
	if err := KillWindow(bad); err == nil {
		t.Error("expected error for unparseable pid")
	}

	// PID far beyond the macOS/Linux pid range → deterministic ESRCH.
	dead := filepath.Join(t.TempDir(), "dead.pid")
	os.WriteFile(dead, []byte("99999999"), 0o644)
	if err := KillWindow(dead); err == nil || !strings.Contains(err.Error(), "terminate pid") {
		t.Errorf("expected terminate error for dead pid, got %v", err)
	}
}

func TestKillWindow_TerminatesOwnChild(t *testing.T) {
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start sleep: %v", err)
	}
	defer cmd.Wait() // reap after SIGTERM

	pidFile := filepath.Join(t.TempDir(), "child.pid")
	os.WriteFile(pidFile, []byte(fmt.Sprintf("%d\n", cmd.Process.Pid)), 0o644)

	if err := KillWindow(pidFile); err != nil {
		t.Fatalf("KillWindow() error = %v", err)
	}
}

func TestKillAll_NoPidsDir(t *testing.T) {
	swapExec(t, execAlwaysTrue)
	if err := KillAll(t.TempDir()); err != nil {
		t.Fatalf("KillAll(no pids dir) = %v, want nil", err)
	}
}

func TestKillAll_AggregatesErrors(t *testing.T) {
	swapExec(t, execAlwaysTrue)

	raDir := t.TempDir()
	pids := filepath.Join(raDir, "pids")
	os.MkdirAll(filepath.Join(pids, "subdir"), 0o755) // dirs are skipped
	os.WriteFile(filepath.Join(pids, "bad.pid"), []byte("garbage"), 0o644)

	err := KillAll(raDir)
	if err == nil || !strings.Contains(err.Error(), "1 errors") {
		t.Fatalf("expected aggregated error, got %v", err)
	}
}

func TestKillAll_KillsOwnChild(t *testing.T) {
	var calls []string
	swapExec(t, func(name string, args ...string) *exec.Cmd {
		calls = append(calls, name)
		return exec.Command("true")
	})

	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start sleep: %v", err)
	}
	defer cmd.Wait()

	raDir := t.TempDir()
	pids := filepath.Join(raDir, "pids")
	os.MkdirAll(pids, 0o755)
	os.WriteFile(filepath.Join(pids, "child.pid"), []byte(fmt.Sprintf("%d", cmd.Process.Pid)), 0o644)

	if err := KillAll(raDir); err != nil {
		t.Fatalf("KillAll() error = %v", err)
	}
	if len(calls) != 1 || calls[0] != "osascript" {
		t.Errorf("expected one osascript close call, got %v", calls)
	}
}

func TestProtectFrontWindow(t *testing.T) {
	var calls []string
	swapExec(t, func(name string, args ...string) *exec.Cmd {
		calls = append(calls, name)
		return exec.Command("true")
	})

	ProtectFrontWindow()
	if len(calls) != 1 || calls[0] != "osascript" {
		t.Errorf("expected one osascript call, got %v", calls)
	}
}

func TestIsWatchRunning(t *testing.T) {
	swapExec(t, func(string, ...string) *exec.Cmd { return exec.Command("echo", "4242") })
	if !isWatchRunning() {
		t.Error("expected watch running when pgrep prints a pid")
	}

	swapExec(t, execAlwaysFalse)
	if isWatchRunning() {
		t.Error("expected watch not running when pgrep fails")
	}
}

func TestSpawnWatchWindow(t *testing.T) {
	t.Setenv("SIRSI_BINARY", "/opt/fake/sirsi") // deterministic setup.SirsiBinaryPath

	var calls []string
	swapExec(t, func(name string, args ...string) *exec.Cmd {
		calls = append(calls, name)
		return exec.Command("true")
	})

	SpawnWatchWindow(false)
	SpawnWatchWindow(true)

	// Each call: one kill-existing osascript + one spawn osascript.
	if len(calls) != 4 {
		t.Errorf("expected 4 osascript calls, got %d (%v)", len(calls), calls)
	}
}

// ── monitor.go ──────────────────────────────────────────────────────────────

func writeDeployment(t *testing.T, raDir string, scopes []string) {
	t.Helper()
	os.MkdirAll(raDir, 0o755)
	meta := deploymentMeta{StartedAt: time.Now().Add(-time.Minute).Format(time.RFC3339), Scopes: scopes}
	data, _ := json.Marshal(meta)
	if err := os.WriteFile(filepath.Join(raDir, "deployment.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestWaitAll_DoneImmediately(t *testing.T) {
	raDir := t.TempDir()
	writeDeployment(t, raDir, nil) // no scopes → AllDone on first poll

	status, err := WaitAll(raDir, time.Second)
	if err != nil {
		t.Fatalf("WaitAll() error = %v", err)
	}
	if !status.AllDone {
		t.Error("expected AllDone")
	}
}

func TestWaitAll_Timeout(t *testing.T) {
	raDir := t.TempDir()
	writeDeployment(t, raDir, []string{"busy"})
	os.MkdirAll(filepath.Join(raDir, "pids"), 0o755)
	// Our own PID is alive → the window reports "running" forever.
	os.WriteFile(filepath.Join(raDir, "pids", "busy.pid"), []byte(fmt.Sprintf("%d", os.Getpid())), 0o644)

	_, err := WaitAll(raDir, -time.Second) // deadline already passed
	if err == nil || !strings.Contains(err.Error(), "timeout") {
		t.Fatalf("expected timeout error, got %v", err)
	}
}

func TestWaitAll_MonitorError(t *testing.T) {
	if _, err := WaitAll(t.TempDir(), time.Second); err == nil {
		t.Error("expected error when deployment.json is missing")
	}
}

// ── pipeline.go ─────────────────────────────────────────────────────────────

func TestNewPipeline(t *testing.T) {
	p := NewPipeline("/repo/root")
	if p.ThothDir != filepath.Join("/repo/root", ".thoth") {
		t.Errorf("ThothDir = %q", p.ThothDir)
	}
	if p.Filter == nil || p.ThothAdapter == nil {
		t.Error("Filter/ThothAdapter not initialized")
	}
	if p.RepoRoot != "/repo/root" {
		t.Errorf("RepoRoot = %q", p.RepoRoot)
	}
}

func TestParseOutput(t *testing.T) {
	p := &Pipeline{}
	task := Task{Subcmd: "health"}

	t.Run("plain text", func(t *testing.T) {
		items := p.parseOutput(task, "all systems go", "")
		if len(items) != 1 || items[0].Summary != "all systems go" {
			t.Fatalf("items = %+v", items)
		}
		if !strings.Contains(items[0].Title, "[Ra]") || !strings.Contains(items[0].Title, "health") {
			t.Errorf("Title = %q", items[0].Title)
		}
	})

	t.Run("stderr fallback", func(t *testing.T) {
		items := p.parseOutput(task, "", "warning: hmm")
		if len(items) != 1 || items[0].Summary != "warning: hmm" {
			t.Fatalf("items = %+v", items)
		}
	})

	t.Run("empty output", func(t *testing.T) {
		if items := p.parseOutput(task, "  \n ", ""); items != nil {
			t.Fatalf("expected nil, got %+v", items)
		}
	})

	t.Run("long output truncated", func(t *testing.T) {
		items := p.parseOutput(task, strings.Repeat("x", 5000), "")
		if len(items) != 1 || !strings.Contains(items[0].Summary, "[... truncated]") {
			t.Fatal("expected truncation marker")
		}
	})

	t.Run("extra args in title", func(t *testing.T) {
		items := p.parseOutput(Task{Subcmd: "task", ExtraArgs: []string{"repo1", "fix it"}}, "done", "")
		if len(items) != 1 || !strings.Contains(items[0].Title, "repo1 fix it") {
			t.Fatalf("items = %+v", items)
		}
	})

	t.Run("json array", func(t *testing.T) {
		out := `[{"repo":"a","result":"ok"},{"name":"b","status":"fail"},{"other":1}]`
		items := p.parseOutput(task, out, "")
		if len(items) != 3 {
			t.Fatalf("expected 3 items, got %d", len(items))
		}
		if !strings.Contains(items[0].Title, "a") || items[0].Summary != "ok" {
			t.Errorf("item0 = %+v", items[0])
		}
		if !strings.Contains(items[1].Title, "b") || items[1].Summary != "fail" {
			t.Errorf("item1 = %+v", items[1])
		}
		if !strings.Contains(items[2].Title, "result") || !strings.Contains(items[2].Summary, "other") {
			t.Errorf("item2 = %+v", items[2])
		}
	})

	t.Run("single json object", func(t *testing.T) {
		items := p.parseOutput(task, `{"k":"v"}`, "")
		if len(items) != 1 || !strings.Contains(items[0].Summary, `"k"`) {
			t.Fatalf("items = %+v", items)
		}
	})
}

func TestTryParseJSON_EmptyShapes(t *testing.T) {
	if got := tryParseJSON("not json at all"); got != nil {
		t.Errorf("plain text → %+v", got)
	}
	if got := tryParseJSON("[]"); got != nil {
		t.Errorf("empty array → %+v", got)
	}
	if got := tryParseJSON("{}"); got != nil {
		t.Errorf("empty object → %+v", got)
	}
}

func TestRunAndRecord_Success(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // keep any stele/thoth writes out of the real home
	repo := t.TempDir()

	p := NewPipeline(repo)
	pr, err := p.recordCapturedOutput(Task{Subcmd: "health"}, `[{"repo":"r1","result":"ok"}]`, "")
	if err != nil {
		t.Fatalf("RunAndRecord() error = %v", err)
	}
	if pr.ItemsIngested != 1 {
		t.Errorf("ItemsIngested = %d, want 1", pr.ItemsIngested)
	}
	if pr.ThothSynced {
		t.Error("expected ThothSynced=false without .thoth/memory.yaml")
	}

	// Status was recorded and reads back.
	status, err := p.ReadStatus()
	if err != nil || status == nil {
		t.Fatalf("ReadStatus() = (%+v, %v)", status, err)
	}
	if status.ItemCount != 1 {
		t.Errorf("ItemCount = %d, want 1", status.ItemCount)
	}
	if status.LastRecorded.IsZero() {
		t.Error("LastRecorded not set")
	}
}

func TestRunAndRecord_EmptyOutputSkipsExport(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repo := t.TempDir()

	p := NewPipeline(repo)
	pr, err := p.recordCapturedOutput(Task{Subcmd: "lint"}, "", "")
	if err != nil {
		t.Fatalf("RunAndRecord() error = %v", err)
	}
	if pr.ItemsIngested != 0 {
		t.Errorf("ItemsIngested = %d, want 0", pr.ItemsIngested)
	}
	if _, err := os.Stat(filepath.Join(repo, ".thoth", "seshat")); !os.IsNotExist(err) {
		t.Error("export should have been skipped for zero items")
	}
}

func TestRunAndRecord_ExportFails(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	tmp := t.TempDir()
	blocker := filepath.Join(tmp, "blocker")
	os.WriteFile(blocker, []byte("x"), 0o644)

	p := NewPipeline(blocker) // .thoth under a regular file → MkdirAll fails
	if _, err := p.recordCapturedOutput(Task{Subcmd: "health"}, "some output", ""); err == nil ||
		!strings.Contains(err.Error(), "seshat export failed") {
		t.Fatalf("expected export failure, got %v", err)
	}
}

func TestRecordStatus_MkdirFails(t *testing.T) {
	tmp := t.TempDir()
	blocker := filepath.Join(tmp, "blocker")
	os.WriteFile(blocker, []byte("x"), 0o644)

	p := &Pipeline{ThothDir: filepath.Join(blocker, "sub")}
	if err := p.recordStatus(1, true); err == nil {
		t.Error("expected mkdir error")
	}
}

func TestReadStatus_CorruptAndSynced(t *testing.T) {
	tmp := t.TempDir()
	p := &Pipeline{ThothDir: tmp}

	os.WriteFile(p.statusFile(), []byte("{not json"), 0o644)
	if _, err := p.ReadStatus(); err == nil {
		t.Error("expected parse error for corrupt status")
	}

	now := time.Now().Format(time.RFC3339)
	body := fmt.Sprintf(`{"last_recorded":%q,"item_count":7,"thoth_synced":%q}`, now, now)
	os.WriteFile(p.statusFile(), []byte(body), 0o644)
	status, err := p.ReadStatus()
	if err != nil {
		t.Fatalf("ReadStatus() error = %v", err)
	}
	if status.ItemCount != 7 || status.LastRecorded.IsZero() || status.ThothSynced.IsZero() {
		t.Errorf("status = %+v", status)
	}
}

// ── deploy.go ───────────────────────────────────────────────────────────────

func writeScopeConfig(t *testing.T, configDir, name, repoPath string, sprints int) {
	t.Helper()
	os.MkdirAll(configDir, 0o755)
	body := fmt.Sprintf(
		"name: %s\ndisplay_name: %s\nrepo_path: %s\ndeadline: soon\npriority: high\nscope_of_work: Ship it\nsprints: %d\n",
		name, strings.ToUpper(name[:1])+name[1:], repoPath, sprints)
	if err := os.WriteFile(filepath.Join(configDir, name+".yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDeploy_LoadScopesError(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	_, err := Deploy(DeployOptions{ConfigDir: t.TempDir(), DryRun: true})
	if err == nil || !strings.Contains(err.Error(), "load scopes") {
		t.Fatalf("expected load scopes error, got %v", err)
	}
}

func TestDeploy_NoScopesMatched(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	configDir := t.TempDir()
	writeScopeConfig(t, configDir, "alpha", t.TempDir(), 1)

	_, err := Deploy(DeployOptions{ConfigDir: configDir, ScopeNames: []string{"nope"}, DryRun: true})
	if err == nil || !strings.Contains(err.Error(), "no scopes matched") {
		t.Fatalf("expected no-match error, got %v", err)
	}
}

func TestDeploy_DryRun(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	configDir := t.TempDir()
	repo := t.TempDir()
	writeScopeConfig(t, configDir, "alpha", repo, 1)
	writeScopeConfig(t, configDir, "beta", repo, 1)

	res, err := Deploy(DeployOptions{
		ConfigDir:  configDir,
		ScopeNames: []string{"beta"},
		DryRun:     true,
	})
	if err != nil {
		t.Fatalf("Deploy(dry-run) error = %v", err)
	}
	if len(res.Spawned) != 1 || res.Spawned[0] != "beta" {
		t.Fatalf("Spawned = %v, want [beta]", res.Spawned)
	}
	if res.Status != nil || res.Results != nil {
		t.Error("dry-run must not carry status/results")
	}

	// Neith wrote the prompt under the (temp) home.
	prompt := filepath.Join(home, ".config", "ra", "scopes", "beta-prompt.md")
	if _, err := os.Stat(prompt); err != nil {
		t.Errorf("prompt file not written: %v", err)
	}
}

func TestDeploy_FullRunHermetic(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SIRSI_BINARY", "/opt/fake/sirsi")

	// pgrep → "found" (skip watch window); every osascript → success.
	swapExec(t, func(name string, args ...string) *exec.Cmd {
		if name == "pgrep" {
			return exec.Command("echo", "4242")
		}
		return exec.Command("true")
	})

	configDir := t.TempDir()
	repo := t.TempDir()
	writeScopeConfig(t, configDir, "alpha", repo, 2)

	res, err := Deploy(DeployOptions{
		ConfigDir: configDir,
		Wait:      true,
		Record:    true,
		RepoRoot:  repo,
	})
	if err != nil {
		t.Fatalf("Deploy() error = %v", err)
	}
	if len(res.Spawned) != 1 || res.Spawned[0] != "alpha" {
		t.Fatalf("Spawned = %v, want [alpha]", res.Spawned)
	}
	if res.Status == nil || !res.Status.AllDone {
		t.Fatalf("expected completed status, got %+v", res.Status)
	}
	// The fake osascript never started a shell → no PID file → "crashed".
	if got := res.Status.Windows[0].State; got != "crashed" {
		t.Errorf("window state = %q, want crashed", got)
	}
	if len(res.Results) != 1 || res.Results[0].ExitCode != -1 {
		t.Errorf("Results = %+v", res.Results)
	}

	// Deployment metadata landed in the temp home.
	if _, statErr := os.Stat(filepath.Join(home, ".config", "ra", "deployment.json")); statErr != nil {
		t.Errorf("deployment.json not written: %v", statErr)
	}
	// --record ran the Seshat → Thoth pipeline into the repo root.
	entries, err := os.ReadDir(filepath.Join(repo, ".thoth", "seshat"))
	if err != nil || len(entries) == 0 {
		t.Errorf("expected seshat exports in repo root, err=%v", err)
	}
}

func TestIngestWindowResults(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repo := t.TempDir()

	// Give thoth.Sync a memory.yaml so the synced branch is exercised too.
	os.MkdirAll(filepath.Join(repo, ".thoth"), 0o755)
	os.WriteFile(filepath.Join(repo, ".thoth", "memory.yaml"), []byte("module_count: 1\n"), 0o644)

	results := []WindowResult{
		{Name: "ok-scope", ExitCode: 0, LogText: "did the thing", Duration: time.Minute},
		{Name: "fail-scope", ExitCode: 2, LogText: strings.Repeat("y", 5000), Duration: time.Minute},
	}

	pr, err := IngestWindowResults(repo, results)
	if err != nil {
		t.Fatalf("IngestWindowResults() error = %v", err)
	}
	if pr.ItemsIngested != 2 {
		t.Errorf("ItemsIngested = %d, want 2", pr.ItemsIngested)
	}
	if !pr.ThothSynced {
		t.Error("expected ThothSynced with memory.yaml present")
	}

	entries, err := os.ReadDir(filepath.Join(repo, ".thoth", "seshat"))
	if err != nil || len(entries) != 2 {
		t.Fatalf("expected 2 exported items, got %d (err=%v)", len(entries), err)
	}
}

func TestIngestWindowResults_ExportError(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	tmp := t.TempDir()
	blocker := filepath.Join(tmp, "blocker")
	os.WriteFile(blocker, []byte("x"), 0o644)

	_, err := IngestWindowResults(blocker, []WindowResult{{Name: "s", ExitCode: 0, LogText: "log"}})
	if err == nil || !strings.Contains(err.Error(), "seshat export") {
		t.Fatalf("expected export error, got %v", err)
	}
}

func TestExpandHomeAndSyncStatus(t *testing.T) {
	t.Setenv("HOME", "/tmp/fakehome")
	if got := expandHome("~/repo"); got != filepath.Join("/tmp/fakehome", "repo") {
		t.Errorf("expandHome = %q", got)
	}
	if got := expandHome("/abs/path"); got != "/abs/path" {
		t.Errorf("expandHome(abs) = %q", got)
	}
	if syncStatus(true) == syncStatus(false) {
		t.Error("syncStatus must differ by outcome")
	}
}
