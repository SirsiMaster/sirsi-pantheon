package main

import (
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/setup"
)

func TestParseInstallableSurfaceArgIsCaretakerOnly(t *testing.T) {
	for _, token := range []string{"gui", "menubar"} {
		got, err := parseInstallableSurfaceArg(token)
		if err != nil || got != setup.SurfaceMenubar {
			t.Fatalf("parseInstallableSurfaceArg(%q) = %q, %v", token, got, err)
		}
	}
	for _, token := range []string{"cli", "tui", "ide", "mcp", "unknown"} {
		if got, err := parseInstallableSurfaceArg(token); err == nil || got != "" {
			t.Fatalf("parseInstallableSurfaceArg(%q) = %q, %v; want rejection", token, got, err)
		}
	}
}

func TestSurfaceCommandExposesNarrowInstallSubcommand(t *testing.T) {
	command, _, err := surfaceCmd.Find([]string{"install"})
	if err != nil || command != surfaceInstallCmd {
		t.Fatalf("surface install command missing: command=%v err=%v", command, err)
	}
}
