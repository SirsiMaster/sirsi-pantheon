package main

import (
	"context"
	"io"
	"testing"

	"github.com/spf13/cobra"
)

func TestManagedDesktopSleepRejectsMalformedUnknownAndLocalAuthority(t *testing.T) {
	if err := provisionManagedDesktopSleep(nil, ""); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{`{`, `{"UnknownOwnerBypass":true}`, `{} {}`, `{}`} {
		if err := provisionManagedDesktopSleep(nil, raw); err == nil {
			t.Fatal("invalid management input accepted", raw)
		}
	}
}

func TestDesktopSleepDispatchCannotUseLocalStore(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SIRSI_ROUTER_URL", "")
	t.Setenv("SIRSI_ROUTER_DB", t.TempDir()+"/local.db")
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	cmd.SetOut(io.Discard)
	if err := runDesktopSleep(cmd, "dispatch", "absent"); err == nil {
		t.Fatal("local store became native authority")
	}
}
