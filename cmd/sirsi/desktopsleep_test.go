package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
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
		if err := revokeManagedDesktopSleep(nil, raw); err == nil {
			t.Fatal("invalid revocation accepted", raw)
		}
	}
}

func TestDesktopSleepLocalStatusDoesNotCreateJournal(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if events, err := localDesktopSleepEvents("not-started"); err != nil || len(events) != 0 {
		t.Fatalf("events=%v err=%v", events, err)
	}
	if _, err := os.Stat(filepath.Join(home, ".sirsi", "desktop-custody")); !os.IsNotExist(err) {
		t.Fatal("status created journal")
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
