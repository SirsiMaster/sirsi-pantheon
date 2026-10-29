package main

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestAttendedRegistrationRejectsAliasConflictBeforeRouterAccess(t *testing.T) {
	t.Setenv("CLAUDE_CODE_SESSION_ID", "")
	t.Setenv("CODEX_SESSION_ID", "parent-session")
	t.Setenv("CODEX_THREAD_ID", "helper-session")
	before := threadRegSurface
	threadRegSurface = "codex"
	t.Cleanup(func() { threadRegSurface = before })
	// No router fixture or network is provided: the conflict must be caught
	// before resolver access, native anchor discovery, registry writes or markers.
	err := threadRegisterCmd.RunE(&cobra.Command{}, nil)
	if err == nil || !strings.Contains(err.Error(), "conflicting native Codex session aliases") {
		t.Fatalf("expected early identity refusal, got %v", err)
	}
}
