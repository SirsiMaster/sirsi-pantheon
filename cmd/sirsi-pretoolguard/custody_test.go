package main

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/desktopcustody"
)

type fakeDisplay struct{ calls int }

func (a *fakeDisplay) DisruptDisplay(context.Context) error { a.calls++; return nil }

func TestCustodyStatesCannotBypassCommandHook(t *testing.T) {
	for _, state := range []string{"active", "stale", "revoked", "expired", "restarted"} {
		t.Run(state, func(t *testing.T) {
			now := time.Date(2026, 10, 11, 13, 0, 0, 0, time.UTC)
			root := t.TempDir()
			if err := os.Chmod(root, 0o700); err != nil {
				t.Fatal(err)
			}
			store, err := desktopcustody.NewFileStore(root)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = store.Close() }()
			svc := desktopcustody.New(store).WithClock(func() time.Time { return now })
			g, err := svc.Arm(desktopcustody.Grant{ID: "grant-1", Host: "m5", Owner: "owner", UnattendedStart: now.Add(-time.Minute), UnattendedEnd: now.Add(time.Minute), ExpiresAt: now.Add(2 * time.Minute)})
			if err != nil {
				t.Fatal(err)
			}
			control := &fakeDisplay{}
			if err := svc.AuthorizeAndAct(context.Background(), g.Host, g.Owner, g.ID, control); err != nil || control.calls != 1 {
				t.Fatalf("active negative control: err=%v calls=%d", err, control.calls)
			}
			switch state {
			case "stale":
				now = g.UnattendedEnd
			case "expired":
				now = g.ExpiresAt
			case "revoked", "restarted":
				if err := svc.Revoke(g.ID, "owner returned"); err != nil {
					t.Fatal(err)
				}
			}
			if state == "restarted" {
				if err := store.Close(); err != nil {
					t.Fatal(err)
				}
				store, err = desktopcustody.NewFileStore(root)
				if err != nil {
					t.Fatal(err)
				}
				svc = desktopcustody.New(store).WithClock(func() time.Time { return now })
			}
			if state != "active" {
				denied := &fakeDisplay{}
				if err := svc.AuthorizeAndAct(context.Background(), g.Host, g.Owner, g.ID, denied); err == nil || denied.calls != 0 {
					t.Fatalf("custody denial: err=%v calls=%d", err, denied.calls)
				}
			}
			// Supplying a grant as extra hook metadata never authorizes a shell.
			payload, err := json.Marshal(map[string]any{"tool_name": "exec_command", "tool_input": map[string]any{"cmd": "pmset displaysleepnow", "grant": g}})
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			if code := fakeDispatch(string(payload), &calls); code != 2 || calls != 0 {
				t.Fatalf("hook denial: exit=%d calls=%d", code, calls)
			}
		})
	}
}
