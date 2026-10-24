//go:build !windows

package main

import (
	"io"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"golang.org/x/sys/unix"
)

func TestRouterRelayRepairRootPins(t *testing.T) {
	cmd := routerRelayRepairOutboxCmd
	flags := []string{"agent", "spool", "expected-root-device", "expected-root-inode"}
	for _, name := range flags {
		flag := cmd.Flags().Lookup(name)
		value, changed := flag.Value.String(), flag.Changed
		t.Cleanup(func() { _ = flag.Value.Set(value); flag.Changed = changed })
	}
	oldOut := cmd.OutOrStdout()
	cmd.SetOut(io.Discard)
	t.Cleanup(func() { cmd.SetOut(oldOut) })
	for _, mode := range []string{"legacy", "pinned", "wrong-inode", "partial", "zero-device"} {
		t.Run(mode, func(t *testing.T) {
			base, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			spool := filepath.Join(base, "relay")
			outbox := filepath.Join(spool, "agent-a", "outbox")
			if err := os.MkdirAll(outbox, 0o700); err != nil {
				t.Fatal(err)
			}
			var st unix.Stat_t
			if err := unix.Stat(spool, &st); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(outbox, 0); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(outbox, 0o700) })
			for _, name := range flags {
				cmd.Flags().Lookup(name).Changed = false
			}
			args := []string{"--agent", "agent-a", "--spool", spool}
			if mode != "legacy" {
				device, inode := uint64(st.Dev), uint64(st.Ino)
				if mode == "wrong-inode" {
					inode++
				}
				if mode == "zero-device" {
					device = 0
					inode++ // Explicit zero is present, never an unpinned fallback.
				}
				args = append(args, "--expected-root-device", strconv.FormatUint(device, 10))
				if mode != "partial" {
					args = append(args, "--expected-root-inode", strconv.FormatUint(inode, 10))
				}
			}
			if err := cmd.ParseFlags(args); err != nil {
				t.Fatal(err)
			}
			err = cmd.RunE(cmd, cmd.Flags().Args())
			refused := mode != "legacy" && mode != "pinned"
			if (err != nil) != refused {
				t.Fatalf("mode=%s error=%v", mode, err)
			}
			info, err := os.Stat(outbox)
			if err != nil {
				t.Fatal(err)
			}
			want := os.FileMode(0o700)
			if refused {
				want = 0
			}
			if info.Mode().Perm() != want {
				t.Fatalf("mode=%04o want=%04o", info.Mode().Perm(), want)
			}
		})
	}
}
