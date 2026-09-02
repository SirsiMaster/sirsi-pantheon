package main

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

func TestRouterTaskClaimIDCommandClaimsExactTask(t *testing.T) {
	db := filepath.Join(t.TempDir(), "router.db")
	t.Setenv("SIRSI_ROUTER_DB", db)
	s, err := routerstore.OpenPath(db)
	if err != nil {
		t.Fatal(err)
	}
	if addErr := s.AddTask(routerstore.Task{Agent: "codex-home", TaskID: "older", Subject: "older"}); addErr != nil {
		t.Fatal(addErr)
	}
	if addErr := s.AddTask(routerstore.Task{Agent: "codex-home", TaskID: "exact", Subject: "exact"}); addErr != nil {
		t.Fatal(addErr)
	}
	if closeErr := s.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}

	oldWorker, oldThread, oldTTL := taskLeaseWorker, taskLeaseThread, taskLeaseTTL
	t.Cleanup(func() { taskLeaseWorker, taskLeaseThread, taskLeaseTTL = oldWorker, oldThread, oldTTL })
	taskLeaseWorker, taskLeaseThread, taskLeaseTTL = "worker", "thread", time.Minute
	if runErr := routerTaskClaimIDCmd.RunE(routerTaskClaimIDCmd, []string{"codex-home", "exact"}); runErr != nil {
		t.Fatal(runErr)
	}

	s, err = routerstore.OpenPath(db)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	exact, err := s.GetTask("codex-home", "exact")
	if err != nil || exact.Status != "in-progress" {
		t.Fatalf("exact task not claimed through CLI: %+v err=%v", exact, err)
	}
	older, err := s.GetTask("codex-home", "older")
	if err != nil || older.Status != "pending" {
		t.Fatalf("CLI claimed oldest rather than exact task: %+v err=%v", older, err)
	}
}
