//go:build darwin

package apprecovery

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"
)

func TestRecoveryHelperProcess(t *testing.T) {
	if os.Getenv("PANTHEON_TEST_RECOVERY_HELPER") != "1" && os.Getenv("PANTHEON_RECOVERY_STATE") != "verified" {
		return
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM)
	<-signals
}

func TestDarwinCheckpointRestoreUsesRealReplacementProcess(t *testing.T) {
	root := t.TempDir()
	help := filepath.Join(root, "pantheon-recovery-helper")
	copyExecutable(t, help)
	state := filepath.Join(root, "checkpoint.json")
	if err := os.WriteFile(state, []byte(`{"position":17}`), 0600); err != nil {
		t.Fatal(err)
	}

	initial := exec.Command(help, "-test.run=TestRecoveryHelperProcess")
	initial.Env = append(os.Environ(), "PANTHEON_TEST_RECOVERY_HELPER=1")
	if err := initial.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if initial.Process != nil {
			_ = initial.Process.Kill()
		}
	})
	waitForPID(t, DarwinDriver{}, Target{ExecutablePath: help}, initial.Process.Pid)

	target := Target{
		ID: "real-checkpoint-helper", Kind: KindCheckpointed,
		ExecutablePath: help, StatePaths: []string{state},
		StartArguments: []string{"-test.run=TestRecoveryHelperProcess"},
		ReadyTimeout:   5 * time.Second,
	}
	manager, err := NewManager([]Target{target}, DarwinDriver{}, FileStore{Root: filepath.Join(root, "receipts")})
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := manager.Recover(context.Background(), target.ID, ModeRestore)
	if err != nil {
		t.Fatal(err)
	}
	_ = initial.Wait()
	if receipt.Phase != PhaseReady || receipt.OldPID != initial.Process.Pid || receipt.NewPID <= 0 || receipt.NewPID == receipt.OldPID {
		t.Fatalf("invalid real recovery receipt: %+v", receipt)
	}
	if process, err := os.FindProcess(receipt.NewPID); err == nil {
		_ = process.Kill()
	}
}

func TestDarwinFreshRestartClearsDeclaredTransientFile(t *testing.T) {
	root := t.TempDir()
	help := filepath.Join(root, "pantheon-fresh-helper")
	copyExecutable(t, help)
	transient := filepath.Join(root, "pending-queue.json")
	if err := os.WriteFile(transient, []byte(`{"pending":3}`), 0600); err != nil {
		t.Fatal(err)
	}
	initial := exec.Command(help, "-test.run=TestRecoveryHelperProcess")
	initial.Env = append(os.Environ(), "PANTHEON_TEST_RECOVERY_HELPER=1")
	if err := initial.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if initial.Process != nil {
			_ = initial.Process.Kill()
		}
	})
	waitForPID(t, DarwinDriver{}, Target{ExecutablePath: help}, initial.Process.Pid)

	target := Target{
		ID: "real-fresh-helper", Kind: KindCheckpointed,
		ExecutablePath: help, FreshStatePaths: []string{transient},
		StartArguments: []string{"-test.run=TestRecoveryHelperProcess"},
		ReadyTimeout:   5 * time.Second,
	}
	manager, err := NewManager([]Target{target}, DarwinDriver{}, FileStore{Root: filepath.Join(root, "receipts")})
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := manager.Recover(context.Background(), target.ID, ModeFresh)
	if err != nil {
		t.Fatal(err)
	}
	_ = initial.Wait()
	if receipt.Phase != PhaseReady || receipt.Mode != ModeFresh || receipt.NewPID == receipt.OldPID {
		t.Fatalf("invalid real fresh receipt: %+v", receipt)
	}
	if _, err := os.Stat(transient); !os.IsNotExist(err) {
		t.Fatalf("registered transient file survived fresh restart: %v", err)
	}
	if process, err := os.FindProcess(receipt.NewPID); err == nil {
		_ = process.Kill()
	}
}

func TestDarwinLaunchdFreshUsesSupervisedReplacement(t *testing.T) {
	root := t.TempDir()
	help := filepath.Join(root, "pantheon-launchd-helper")
	copyExecutable(t, help)
	label := fmt.Sprintf("ai.sirsi.test.recovery.%d", os.Getpid())
	domain := fmt.Sprintf("gui/%d", os.Getuid())
	targetName := domain + "/" + label
	plist := filepath.Join(root, label+".plist")
	plistBody := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Label</key><string>%s</string>
<key>ProgramArguments</key><array><string>%s</string><string>-test.run=TestRecoveryHelperProcess</string></array>
<key>EnvironmentVariables</key><dict><key>PANTHEON_TEST_RECOVERY_HELPER</key><string>1</string></dict>
<key>RunAtLoad</key><true/><key>KeepAlive</key><true/>
</dict></plist>`, label, help)
	if err := os.WriteFile(plist, []byte(plistBody), 0600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("/bin/launchctl", "bootstrap", domain, plist).CombinedOutput(); err != nil {
		t.Fatalf("bootstrap fixture: %v: %s", err, output)
	}
	t.Cleanup(func() { _ = exec.Command("/bin/launchctl", "bootout", targetName).Run() })
	target := Target{ID: "real-launchd-helper", Kind: KindLaunchd, ExecutablePath: help, LaunchdTarget: targetName, ReadyTimeout: 5 * time.Second}
	oldPID := waitForAnyPID(t, DarwinDriver{}, target)

	manager, err := NewManager([]Target{target}, DarwinDriver{}, FileStore{Root: filepath.Join(root, "receipts")})
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := manager.Recover(context.Background(), target.ID, ModeFresh)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Phase != PhaseReady || receipt.OldPID != oldPID || receipt.NewPID <= 0 || receipt.NewPID == oldPID {
		t.Fatalf("invalid launchd replacement receipt: %+v", receipt)
	}
}

func TestDarwinNewManagerResumesDurableStoppedReceipt(t *testing.T) {
	root := t.TempDir()
	help := filepath.Join(root, "pantheon-resume-helper")
	copyExecutable(t, help)
	state := filepath.Join(root, "checkpoint.json")
	if err := os.WriteFile(state, []byte(`{"position":29}`), 0600); err != nil {
		t.Fatal(err)
	}
	initial := exec.Command(help, "-test.run=TestRecoveryHelperProcess")
	initial.Env = append(os.Environ(), "PANTHEON_TEST_RECOVERY_HELPER=1")
	if err := initial.Start(); err != nil {
		t.Fatal(err)
	}
	waitForPID(t, DarwinDriver{}, Target{ExecutablePath: help}, initial.Process.Pid)

	target := Target{ID: "real-resume-helper", Kind: KindCheckpointed, ExecutablePath: help, StatePaths: []string{state}, StartArguments: []string{"-test.run=TestRecoveryHelperProcess"}, ReadyTimeout: 5 * time.Second}
	driver := DarwinDriver{}
	snapshot, err := driver.Capture(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	if err := driver.Stop(context.Background(), target, initial.Process.Pid); err != nil {
		t.Fatal(err)
	}
	_ = initial.Wait()
	waitForNoPID(t, driver, target)
	store := FileStore{Root: filepath.Join(root, "receipts")}
	now := time.Now().UTC()
	if err := store.Save(Receipt{Schema: "pantheon.app-recovery.v1", TargetID: target.ID, Kind: target.Kind, Mode: ModeRestore, Phase: PhaseStopped, OldPID: initial.Process.Pid, Snapshot: snapshot, StartedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}

	replacementManager, err := NewManager([]Target{target}, driver, store)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := replacementManager.Resume(context.Background(), target.ID)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Phase != PhaseReady || receipt.NewPID <= 0 || receipt.NewPID == receipt.OldPID {
		t.Fatalf("new manager did not resume durable recovery: %+v", receipt)
	}
	if process, err := os.FindProcess(receipt.NewPID); err == nil {
		_ = process.Kill()
	}
}

func TestDarwinAppSavedStateRelaunchConsumesPersistedSession(t *testing.T) {
	root := t.TempDir()
	appPath := filepath.Join(root, "Pantheon Recovery Fixture.app")
	state := filepath.Join(root, "session.txt")
	ready := filepath.Join(root, "restored.txt")
	bundleID := fmt.Sprintf("ai.sirsi.test.recovery.app.%d", os.Getpid())
	if err := os.WriteFile(state, []byte("tab-17"), 0600); err != nil {
		t.Fatal(err)
	}
	script := fmt.Sprintf(`property statePath : %s
property readyPath : %s
on run
  do shell script "/bin/cp " & quoted form of statePath & " " & quoted form of readyPath
end run
on idle
  return 1
end idle
on quit
  continue quit
end quit`, strconv.Quote(state), strconv.Quote(ready))
	scriptPath := filepath.Join(root, "fixture.applescript")
	if err := os.WriteFile(scriptPath, []byte(script), 0600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("/usr/bin/osacompile", "-s", "-o", appPath, scriptPath).CombinedOutput(); err != nil {
		t.Fatalf("compile app fixture: %v: %s", err, output)
	}
	infoPlist := filepath.Join(appPath, "Contents", "Info.plist")
	if output, err := exec.Command("/usr/bin/plutil", "-replace", "CFBundleIdentifier", "-string", bundleID, infoPlist).CombinedOutput(); err != nil {
		t.Fatalf("set fixture bundle id: %v: %s", err, output)
	}
	if output, err := exec.Command("/usr/bin/plutil", "-insert", "LSUIElement", "-bool", "YES", infoPlist).CombinedOutput(); err != nil {
		t.Fatalf("make fixture background-only: %v: %s", err, output)
	}
	lsregister := "/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister"
	if output, err := exec.Command(lsregister, "-f", appPath).CombinedOutput(); err != nil {
		t.Fatalf("register fixture app: %v: %s", err, output)
	}
	t.Cleanup(func() {
		_ = exec.Command("/usr/bin/osascript", "-e", `tell application id "`+bundleID+`" to quit`).Run()
		_ = exec.Command(lsregister, "-u", appPath).Run()
	})
	if output, err := exec.Command("/usr/bin/open", appPath).CombinedOutput(); err != nil {
		t.Fatalf("launch fixture app: %v: %s", err, output)
	}
	executable := filepath.Join(appPath, "Contents", "MacOS", "applet")
	target := Target{ID: "real-app-helper", Kind: KindAppSavedState, BundleID: bundleID, ExecutablePath: executable, StatePaths: []string{state}, ReadyTimeout: 10 * time.Second}
	oldPID := waitForAnyPID(t, DarwinDriver{}, target)
	waitForFileContent(t, ready, "tab-17")
	if err := os.Remove(ready); err != nil {
		t.Fatal(err)
	}

	manager, err := NewManager([]Target{target}, DarwinDriver{}, FileStore{Root: filepath.Join(root, "receipts")})
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := manager.Recover(context.Background(), target.ID, ModeRestore)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Phase != PhaseReady || receipt.OldPID != oldPID || receipt.NewPID <= 0 || receipt.NewPID == oldPID {
		t.Fatalf("invalid app saved-state receipt: %+v", receipt)
	}
	waitForFileContent(t, ready, "tab-17")
}

func copyExecutable(t *testing.T, destination string) {
	t.Helper()
	sourcePath, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.Open(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	dest, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0700)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(dest, source); err != nil {
		dest.Close()
		t.Fatal(err)
	}
	if err := dest.Close(); err != nil {
		t.Fatal(err)
	}
}

func waitForPID(t *testing.T, driver DarwinDriver, target Target, expected int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		pid, err := driver.PID(context.Background(), target)
		if err == nil && pid == expected {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("helper PID %d did not become observable", expected)
}

func waitForAnyPID(t *testing.T, driver DarwinDriver, target Target) int {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		pid, err := driver.PID(context.Background(), target)
		if err == nil && pid > 0 {
			return pid
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("recovery target process did not become observable")
	return 0
}

func waitForNoPID(t *testing.T, driver DarwinDriver, target Target) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		pid, err := driver.PID(context.Background(), target)
		if err == nil && pid == 0 {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("original helper process remained observable")
}

func waitForFileContent(t *testing.T, path, expected string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil && string(data) == expected {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("file %s did not contain expected restored state", path)
}
