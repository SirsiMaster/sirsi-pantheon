package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
)

func TestReadThreadListIsReadOnlyAndPreservesIdentity(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 8, 1, 3, 0, 0, 0, time.UTC)
	reg := &router.ThreadRegistry{Threads: map[string]*router.Thread{
		"thr-dead": {
			ThreadID: "thr-dead", AgentID: "codex-pantheon", Surface: "codex",
			Repo: "/repo/pantheon", Workstream: "observer-truthfulness",
			Status: router.ThreadStatusActive, PID: 999999, LastSeenAt: now.Add(-time.Hour),
		},
	}}
	if err := router.SaveThreadRegistry(root, reg); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "threads.json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	rows, err := readThreadList(root, time.Minute, false, now)
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("thread list observer mutated the registry")
	}
	if len(rows) != 1 || rows[0].thr.Repo != "/repo/pantheon" || rows[0].thr.Workstream != "observer-truthfulness" {
		t.Fatalf("identity fields lost: %+v", rows)
	}
	if rows[0].thr.Status != router.ThreadStatusActive {
		t.Fatalf("observer reaped dead PID: status=%s", rows[0].thr.Status)
	}
}

func TestDisplayUnknown(t *testing.T) {
	if got := displayUnknown(""); got != "unknown" {
		t.Fatalf("empty identity = %q", got)
	}
	if got := displayUnknown("pantheon"); got != "pantheon" {
		t.Fatalf("identity = %q", got)
	}
}
