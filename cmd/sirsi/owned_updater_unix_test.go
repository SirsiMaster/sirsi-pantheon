//go:build unix

package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestOwnedUpdaterDeadlineAndCancellationStopOnlyTheStartedProcessGroup(t *testing.T) {
	for _, test := range []struct {
		name string
		ctx  func() (context.Context, context.CancelFunc)
		want error
	}{
		{name: "deadline", ctx: func() (context.Context, context.CancelFunc) {
			return context.WithTimeout(context.Background(), time.Second)
		}, want: context.DeadlineExceeded},
		{name: "caller cancellation", ctx: func() (context.Context, context.CancelFunc) {
			return context.WithCancel(context.Background())
		}, want: context.Canceled},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			groupPath := filepath.Join(root, "group-pid")
			childPath := filepath.Join(root, "child-pid")
			script := filepath.Join(root, "stalled-updater.sh")
			body := "#!/bin/sh\necho $$ > " + strconv.Quote(groupPath) + "\n/bin/sleep 30 &\necho $! > " + strconv.Quote(childPath) + "\nwait\n"
			if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := test.ctx()
			command := exec.Command("/bin/sh", script)
			done := make(chan error, 1)
			joined := false
			defer func() {
				cancel()
				if !joined {
					select {
					case <-done:
					case <-time.After(4 * time.Second):
						t.Errorf("fixture cleanup did not join the owned updater goroutine")
					}
				}
			}()
			go func() { done <- runOwnedUpdaterCommand(ctx, command) }()
			waitForFile(t, groupPath)
			waitForFile(t, childPath)
			groupPID := readPID(t, groupPath)
			childPID := readPID(t, childPath)
			if syscall.Kill(childPID, 0) != nil {
				t.Fatalf("stalled updater child %d was not alive before cancellation", childPID)
			}
			if test.name == "caller cancellation" {
				cancel()
			}
			select {
			case err := <-done:
				joined = true
				if !errors.Is(err, test.want) {
					t.Fatalf("updater terminal error = %v, want %v", err, test.want)
				}
			case <-time.After(4 * time.Second):
				t.Fatal("owned updater did not terminate and reap within the cleanup bound")
			}
			if updaterGroupExists(groupPID) {
				t.Fatalf("owned updater process group remains after terminal return (leader=%d child=%d)", groupPID, childPID)
			}
			if command.ProcessState == nil {
				t.Fatal("owned direct updater child has no terminal wait state")
			}
			status, ok := command.ProcessState.Sys().(syscall.WaitStatus)
			if !ok || !status.Signaled() || (status.Signal() != syscall.SIGTERM && status.Signal() != syscall.SIGKILL) {
				t.Fatalf("owned direct updater child wait status = %#v; want reaped SIGTERM/SIGKILL termination", command.ProcessState.Sys())
			}
		})
	}
}

func TestOwnedUpdaterCancellationBeforeLaunchDoesNotStartTheCommand(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "launched")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	command := exec.Command("/bin/sh", "-c", "touch "+strconv.Quote(marker))
	err := runOwnedUpdaterCommand(ctx, command)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-launch cancellation = %v, want context.Canceled", err)
	}
	if _, statErr := os.Lstat(marker); !os.IsNotExist(statErr) {
		t.Fatalf("canceled updater command launched: marker stat = %v", statErr)
	}
}

func TestOwnedUpdaterUnconfirmedCleanupReturnsBoundedUnknownAndLeavesReplayDebt(t *testing.T) {
	root := t.TempDir()
	groupPath := filepath.Join(root, "group-pid")
	childPath := filepath.Join(root, "child-pid")
	script := filepath.Join(root, "stalled-updater.sh")
	body := "#!/bin/sh\necho $$ > " + strconv.Quote(groupPath) + "\n/bin/sleep 30 &\necho $! > " + strconv.Quote(childPath) + "\nwait $!\n"
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	oldCleanup, oldWait := updaterGroupCleanup, updaterDirectWait
	oldWaitHook := updaterWaitCompleted
	updaterGroupCleanup = func(int) error { return errors.New("fixture cannot confirm group cleanup") }
	updaterDirectWait = 40 * time.Millisecond
	waitCompleted := make(chan struct{}, 1)
	updaterWaitCompleted = func(*exec.Cmd, error) { waitCompleted <- struct{}{} }
	t.Cleanup(func() {
		updaterGroupCleanup, updaterDirectWait, updaterWaitCompleted = oldCleanup, oldWait, oldWaitHook
	})
	ctx, cancel := context.WithCancel(context.Background())
	command := exec.Command("/bin/sh", script)
	done := make(chan error, 1)
	go func() { done <- runOwnedUpdaterCommand(ctx, command) }()
	var groupPID int
	joined := false
	waitObserved := false
	defer func() {
		cancel()
		if groupPID > 0 {
			_ = syscall.Kill(-groupPID, syscall.SIGKILL)
		}
		if !joined {
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				t.Errorf("unconfirmed updater fixture did not return during cleanup")
			}
		}
		if !waitObserved {
			select {
			case <-waitCompleted:
			case <-time.After(3 * time.Second):
				t.Errorf("fixture did not join the direct-child Wait goroutine")
			}
		}
	}()
	waitForFile(t, groupPath)
	groupPID = readPID(t, groupPath)
	waitForFile(t, childPath)
	childPID := readPID(t, childPath)
	if syscall.Kill(childPID, 0) != nil {
		t.Fatalf("stalled fixture child %d was not running before cancellation", childPID)
	}
	cancel()
	select {
	case err := <-done:
		joined = true
		if !errors.Is(err, errUpdaterExecutionUnknown) {
			t.Fatalf("unconfirmed cleanup result = %v; want retained unknown execution debt", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("unconfirmed cleanup did not return within the direct-wait bound")
	}
	// The fixture, not production code, now cleans up its deliberately surviving
	// test group. Join cmd.Wait through its completion signal before examining
	// ProcessState; never race a read against os/exec's Wait goroutine.
	_ = syscall.Kill(-groupPID, syscall.SIGKILL)
	select {
	case <-waitCompleted:
		waitObserved = true
	case <-time.After(2 * time.Second):
		t.Fatal("fixture failed to join its direct child after cleanup")
	}
	if command.ProcessState == nil {
		t.Fatal("fixture failed to reap its deliberately surviving direct child")
	}
}

func waitForFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("stalled updater fixture did not create %s", path)
}

func readPID(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		t.Fatalf("fixture PID %q: %v", data, err)
	}
	return pid
}
