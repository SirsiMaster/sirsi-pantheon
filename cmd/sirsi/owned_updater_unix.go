//go:build unix

package main

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"syscall"
	"time"
)

const (
	updaterTermGrace = 500 * time.Millisecond
	updaterKillWait  = 2 * time.Second
)

var updaterGroupCleanup = terminateUpdaterGroup
var updaterWaitCompleted = func(*exec.Cmd, error) {}

// runOwnedUpdaterCommand gives one updater invocation a fresh process group.
// Cancellation and deadline cleanup address only that group, never a global
// Homebrew process discovered by name or PID.
func runOwnedUpdaterCommand(ctx context.Context, cmd *exec.Cmd) error {
	if ctx == nil {
		return errors.New("supported updater requires a cancellation context")
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("updater canceled before launch: %w", err)
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start supported updater: %w", err)
	}
	pgid := cmd.Process.Pid
	waited := make(chan error, 1)
	go func() {
		waitErr := cmd.Wait()
		updaterWaitCompleted(cmd, waitErr)
		waited <- waitErr
	}()

	select {
	case commandErr := <-waited:
		if groupErr := awaitUpdaterGroupExit(ctx, pgid); groupErr != nil {
			cleanupErr := updaterGroupCleanup(pgid)
			if cleanupErr != nil {
				return errors.Join(errUpdaterExecutionUnknown, fmt.Errorf("updater process exited but its owned child group did not: %w", groupErr), cleanupErr)
			}
			return fmt.Errorf("updater process exited while owned child work remained; that work was stopped, so no successful update is claimed: %w", groupErr)
		}
		return commandErr
	case <-ctx.Done():
		cleanupErr := updaterGroupCleanup(pgid)
		waitErr, reaped := waitForOwnedUpdater(waited, updaterDirectWait)
		if !reaped {
			return errors.Join(errUpdaterExecutionUnknown, fmt.Errorf("updater canceled (%w); direct-child wait did not complete within %s", ctx.Err(), updaterDirectWait), cleanupErr)
		}
		if cleanupErr != nil {
			return errors.Join(errUpdaterExecutionUnknown, fmt.Errorf("updater canceled (%w); owned process-group cleanup is unconfirmed: %v", ctx.Err(), cleanupErr), waitErr)
		}
		return fmt.Errorf("updater canceled; its owned process group has exited: %w", ctx.Err())
	}
}

func waitForOwnedUpdater(waited <-chan error, bound time.Duration) (error, bool) {
	timer := time.NewTimer(bound)
	defer timer.Stop()
	select {
	case err := <-waited:
		return err, true
	case <-timer.C:
		return nil, false
	}
}

func awaitUpdaterGroupExit(ctx context.Context, pgid int) error {
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		if !updaterGroupExists(pgid) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func terminateUpdaterGroup(pgid int) error {
	if pgid <= 0 || !updaterGroupExists(pgid) {
		return nil
	}
	var termErr error
	if err := syscall.Kill(-pgid, syscall.SIGTERM); err != nil && err != syscall.ESRCH {
		termErr = fmt.Errorf("signal owned updater group: %w", err)
	}
	if termErr == nil && waitUpdaterGroupGone(pgid, updaterTermGrace) {
		return nil
	}
	if !updaterGroupExists(pgid) {
		return nil
	}
	var killErr error
	if err := syscall.Kill(-pgid, syscall.SIGKILL); err != nil && err != syscall.ESRCH {
		killErr = fmt.Errorf("force-stop owned updater group after grace: %w", err)
	}
	if !waitUpdaterGroupGone(pgid, updaterKillWait) {
		return errors.Join(termErr, killErr, errors.New("owned updater process group remained after SIGKILL; terminal state is unknown"))
	}
	return nil
}

func waitUpdaterGroupGone(pgid int, bound time.Duration) bool {
	deadline := time.Now().Add(bound)
	for time.Now().Before(deadline) {
		if !updaterGroupExists(pgid) {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return !updaterGroupExists(pgid)
}

func updaterGroupExists(pgid int) bool {
	if pgid <= 0 {
		return false
	}
	err := syscall.Kill(-pgid, 0)
	return err == nil || err == syscall.EPERM
}
