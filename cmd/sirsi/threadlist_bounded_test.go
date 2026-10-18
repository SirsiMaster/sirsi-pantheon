package main_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
)

// writeThreadListRegistry writes the legacy fixture shape deliberately: the
// subprocess forces SIRSI_ROUTER_STORE_WAKE=0, so this test never reads or
// writes the host's cutover router service.
func writeThreadListRegistry(t *testing.T, root string, threads map[string]*router.Thread) {
	t.Helper()
	dir := filepath.Join(root, ".agents", "idea-router")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(&router.ThreadRegistry{Threads: threads})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "threads.json"), b, 0o600); err != nil {
		t.Fatal(err)
	}
}

func readThreadListRegistry(t *testing.T, root string) *router.ThreadRegistry {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, ".agents", "idea-router", "threads.json"))
	if err != nil {
		t.Fatal(err)
	}
	var reg router.ThreadRegistry
	if err := json.Unmarshal(b, &reg); err != nil {
		t.Fatal(err)
	}
	return &reg
}

func TestThreadListExactSelectorsSkipReapAndIsolateRecord(t *testing.T) {
	// Keep the subprocess on a local fixture registry even when the developer
	// host has promoted the router store to sole authority.
	t.Setenv("SIRSI_ROUTER_STORE_WAKE", "0")
	root := t.TempDir()
	now := time.Now().UTC()
	writeThreadListRegistry(t, root, map[string]*router.Thread{
		"thr-target": {
			ThreadID: "thr-target", AgentID: "maat-agent", Surface: "worker",
			PID: 999991, Status: router.ThreadStatusActive, LastSeenAt: now,
		},
		"thr-other": {
			ThreadID: "thr-other", AgentID: "other-agent", Surface: "worker",
			PID: 999992, Status: router.ThreadStatusActive, LastSeenAt: now,
		},
	})

	stdout, stderr, err := runSirsiInDir(t, root, 8*time.Second,
		"thread", "list", "--json", "--agent", "maat-agent", "--thread", "thr-target")
	if err != nil {
		t.Fatalf("exact bounded lookup failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	var rows []struct {
		Thread router.Thread `json:"thread"`
	}
	if err := json.Unmarshal([]byte(stdout), &rows); err != nil {
		t.Fatalf("decode exact bounded lookup: %v\n%s", err, stdout)
	}
	if len(rows) != 1 || rows[0].Thread.AgentID != "maat-agent" || rows[0].Thread.ThreadID != "thr-target" {
		t.Fatalf("exact lookup returned %#v, want only maat-agent/thr-target", rows)
	}

	// The PIDs are intentionally impossible. If the selector path accidentally
	// invokes the sweep, either row changes to reaped. Exact lookup must be
	// read-only because the supervisor performs the PID/start-time proof itself.
	reg := readThreadListRegistry(t, root)
	for _, id := range []string{"thr-target", "thr-other"} {
		if got := reg.Threads[id].Status; got != router.ThreadStatusActive {
			t.Fatalf("bounded lookup reaped %s: status=%q", id, got)
		}
	}

	stdout, stderr, err = runSirsiInDir(t, root, 8*time.Second,
		"thread", "list", "--json", "--agent", "wrong-agent", "--thread", "thr-target")
	if err != nil {
		t.Fatalf("wrong-agent bounded lookup failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	if err := json.Unmarshal([]byte(stdout), &rows); err != nil {
		t.Fatalf("decode wrong-agent bounded lookup: %v\n%s", err, stdout)
	}
	if len(rows) != 0 {
		t.Fatalf("wrong agent must not disclose thr-target: %#v", rows)
	}
}

func TestThreadListPartialSelectorStillReaps(t *testing.T) {
	t.Setenv("SIRSI_ROUTER_STORE_WAKE", "0")
	root := t.TempDir()
	writeThreadListRegistry(t, root, map[string]*router.Thread{
		"thr-doomed": {
			ThreadID: "thr-doomed", AgentID: "maat-agent", Surface: "worker",
			PID: 999993, Status: router.ThreadStatusActive, LastSeenAt: time.Now().UTC(),
		},
	})

	stdout, stderr, err := runSirsiInDir(t, root, 8*time.Second,
		"thread", "list", "--json", "--agent", "maat-agent")
	if err != nil {
		t.Fatalf("partial selector list failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	reg := readThreadListRegistry(t, root)
	if got := reg.Threads["thr-doomed"].Status; got != router.ThreadStatusReaped {
		t.Fatalf("partial selector must retain OS-truth reaping, got status=%q", got)
	}
}
