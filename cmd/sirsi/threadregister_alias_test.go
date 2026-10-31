package main

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestAttendedRegistrationUsesCurrentCodexThreadWhenIDsDiffer(t *testing.T) {
	t.Setenv("CLAUDE_CODE_SESSION_ID", "")
	t.Setenv("CODEX_SESSION_ID", "parent-session")
	t.Setenv("CODEX_THREAD_ID", "current-thread")
	beforeSurface, beforeAgent, beforeRepo := threadRegSurface, threadRegAgent, threadRegRepo
	threadRegSurface = "codex"
	threadRegAgent, threadRegRepo = "", ""
	t.Cleanup(func() {
		threadRegSurface, threadRegAgent, threadRegRepo = beforeSurface, beforeAgent, beforeRepo
	})
	// Missing --agent stops before anchor discovery, registry writes or markers.
	// Reaching this existing validation proves distinct root/thread IDs were not
	// rejected as an identity conflict.
	err := threadRegisterCmd.RunE(&cobra.Command{}, nil)
	if err == nil || !strings.Contains(err.Error(), "--agent is required") {
		t.Fatalf("distinct Codex IDs should reach required-agent validation, got %v", err)
	}
}
