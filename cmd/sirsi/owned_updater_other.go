//go:build !unix

package main

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"time"
)

// Non-Unix fallback still bounds and reaps the directly owned updater child.
// Unix platforms use a fresh process group so inherited updater children are
// also terminated without touching unrelated processes.
func runOwnedUpdaterCommand(ctx context.Context, cmd *exec.Cmd) error {
	if ctx == nil {
		return fmt.Errorf("supported updater requires a cancellation context")
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("updater canceled before launch: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start supported updater: %w", err)
	}
	waited := make(chan error, 1)
	go func() { waited <- cmd.Wait() }()
	select {
	case err := <-waited:
		return err
	case <-ctx.Done():
		killErr := cmd.Process.Kill()
		timer := time.NewTimer(updaterDirectWait)
		defer timer.Stop()
		select {
		case <-waited:
			if killErr != nil {
				return errors.Join(errUpdaterExecutionUnknown, fmt.Errorf("updater canceled (%w); direct-child kill was not confirmed: %v", ctx.Err(), killErr))
			}
		case <-timer.C:
			return errors.Join(errUpdaterExecutionUnknown, fmt.Errorf("updater canceled (%w); direct-child wait did not complete within %s", ctx.Err(), updaterDirectWait), killErr)
		}
		return fmt.Errorf("updater canceled: %w", ctx.Err())
	}
}
