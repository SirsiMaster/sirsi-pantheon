package main

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

func TestRouterTaskWithdrawWithFence(t *testing.T) {
	db := filepath.Join(t.TempDir(), "router.db")
	t.Setenv("SIRSI_ROUTER_DB", db)
	s, err := routerstore.OpenPath(db)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"old", "next"} {
		if err = s.AddTask(routerstore.Task{Agent: "codex-home", TaskID: id, Subject: id}); err != nil {
			t.Fatal(err)
		}
	}
	lease, err := s.ClaimTask("codex-home", "old", "worker", "thread", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	oldToken, oldNext, oldReason := taskLeaseToken, taskWithdrawSupersededBy, taskLeaseReason
	flag := routerTaskWithdrawCmd.Flags().Lookup("lease")
	oldChanged := flag.Changed
	t.Cleanup(func() {
		taskLeaseToken, taskWithdrawSupersededBy, taskLeaseReason = oldToken, oldNext, oldReason
		flag.Changed = oldChanged
	})
	taskWithdrawSupersededBy = "codex-home/next"
	taskLeaseReason = "duplicate"
	if err = routerTaskWithdrawCmd.Flags().Set("lease", ""); err != nil {
		t.Fatal(err)
	}
	if err = routerTaskWithdrawCmd.RunE(routerTaskWithdrawCmd, []string{"codex-home", "old"}); !errors.Is(err, routerstore.ErrLeaseInvalid) {
		t.Fatalf("empty explicit fence: %v", err)
	}
	if err = routerTaskWithdrawCmd.Flags().Set("lease", lease.Token); err != nil {
		t.Fatal(err)
	}
	if err = routerTaskWithdrawCmd.RunE(routerTaskWithdrawCmd, []string{"codex-home", "old"}); err != nil {
		t.Fatal(err)
	}
	s, err = routerstore.OpenPath(db)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	row, err := s.GetTask("codex-home", "old")
	if err != nil {
		t.Fatal(err)
	}
	if row.Status != "withdrawn" || row.ResultRef != "superseded:codex-home/next" {
		t.Fatalf("wrong result: %+v", row)
	}
}
