package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

func TestTaskResumeCLIActualRemoteInspectClaimAndComplete(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, key := range []string{"SIRSI_ROUTER_URL", "SIRSI_ROUTER_TOKEN", "SIRSI_ROUTER_DB", "SIRSI_ROUTER_CLOUD_HOST", "SIRSI_ROUTER_USE_MACHINE_ID"} {
		t.Setenv(key, "")
	}
	const agent, thread = "owned-resume-lane", "owned-resume-thread"
	s, err := routerstore.OpenPath(filepath.Join(t.TempDir(), "owned-fixture.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertThreads([]routerstore.ThreadRecord{{ThreadID: thread, Agent: agent, Host: host, Status: "active", LastSeenAt: time.Now().UTC().Format(time.RFC3339Nano), Payload: json.RawMessage(`{"surface":"api"}`)}}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddTask(routerstore.Task{Agent: agent, TaskID: "original", Subject: "owned fixture"}); err != nil {
		t.Fatal(err)
	}
	// Three real failed attempts, not direct SQL setting status/ceiling.
	for i := 0; i < routerstore.MaxRetriesPerItem; i++ {
		lease, err := s.ClaimTask(agent, "original", agent, thread, time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.ReleaseTaskLease(agent, "original", lease.Token, "prior retained failure"); err != nil {
			t.Fatal(err)
		}
	}
	h, err := routerstore.Handler(s, routerstore.ServerOptions{Token: "owned-fixture-token", RuleOfRa: "enforce"})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	t.Setenv("SIRSI_ROUTER_URL", srv.URL)
	t.Setenv("SIRSI_ROUTER_TOKEN", "owned-fixture-token")
	resolve := func() (routerstore.Store, error) { return routerstore.ResolveForCaller(agent, thread) }
	execute := func(args ...string) (routerstore.TaskResumeResult, error) {
		cmd := newTaskResumeCommand(resolve)
		cmd.SilenceUsage, cmd.SilenceErrors = true, true
		var stdout bytes.Buffer
		cmd.SetOut(&stdout)
		cmd.SetArgs(args)
		err := cmd.Execute()
		var out routerstore.TaskResumeResult
		if err == nil {
			err = json.Unmarshal(stdout.Bytes(), &out)
		}
		return out, err
	}
	before, err := execute(agent, "original")
	if err != nil || before.Claimed || before.Lease != nil || before.Attempts != 3 || before.RowVersion == "" {
		t.Fatalf("actual CLI default inspection: %+v %v", before, err)
	}
	out, err := execute(agent, "original", "--apply", "--expected-version", before.RowVersion, "--evidence-ref", "evidence://owned/cause-fixed", "--evidence-sha256", strings.Repeat("b", 64), "--reason", "specific old cause fixed", "--ttl", "30m")
	if err != nil || !out.Claimed || out.Lease == nil || out.Lease.Attempt != 4 {
		t.Fatalf("CLI_RESUME_NORMAL_FENCE_MISSING: %+v %v", out, err)
	}
	// A separate authenticated client retains ordinary completion authority.
	client, err := resolve()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Close() }()
	if err := client.CompleteTaskLease(agent, "original", out.Lease.Token, "evidence://owned/normal-complete"); err != nil {
		t.Fatal(err)
	}
	final, err := s.TaskEligibility(agent, "original")
	if err != nil || final.Status != "done" || final.Attempts != 4 || !strings.Contains(final.FailureReason, "prior retained failure") {
		t.Fatal("CLI path erased failure history or skipped normal complete", err)
	}
	if _, err := client.ResumeTaskClaim(context.Background(), routerstore.TaskResumeRequest{Version: 1, Action: "inspect", Agent: "foreign", TaskID: "original"}); err == nil {
		t.Fatal("foreign CLI identity accepted")
	}
}

func TestTaskResumeCLIRefusesBeforeAuthorityAndLocalFallback(t *testing.T) {
	for _, args := range [][]string{
		{"lane"},
		{"lane", "task", "extra"},
		{"lane", "task", "--apply"},
		{"lane", "task", "--expected-version", strings.Repeat("a", 64)},
		{"lane", "task", "--ttl", "1m"},
		{"lane", "task", "--apply", "--expected-version", strings.Repeat("a", 64), "--evidence-ref", "evidence://proof", "--evidence-sha256", strings.Repeat("b", 64), "--reason", "cause fixed", "--ttl", "45m"},
	} {
		calls := 0
		cmd := newTaskResumeCommand(func() (routerstore.Store, error) { calls++; return nil, nil })
		cmd.SilenceUsage, cmd.SilenceErrors = true, true
		cmd.SetArgs(args)
		if err := cmd.Execute(); err == nil || calls != 0 {
			t.Fatal("invalid flags reached authority", args, calls, err)
		}
	}
	t.Setenv("HOME", t.TempDir())
	s, err := routerstore.OpenPath(filepath.Join(t.TempDir(), "owned-local.db"))
	if err != nil {
		t.Fatal(err)
	}
	cmd := newTaskResumeCommand(func() (routerstore.Store, error) { return s, nil })
	cmd.SilenceUsage, cmd.SilenceErrors = true, true
	cmd.SetArgs([]string{"lane", "task"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "not a local store") {
		t.Fatal("local fallback accepted", err)
	}
}
