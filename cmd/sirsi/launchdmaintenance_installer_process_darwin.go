//go:build darwin

package main

import (
	"context"
	"os/exec"
)

func newMaintenanceInstallerProcess(ctx context.Context, exe string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Env = []string{"PATH=/usr/bin:/bin:/usr/sbin:/sbin", "LC_ALL=C"}
	return cmd
}
