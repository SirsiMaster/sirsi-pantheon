package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// The port wait exists because a respawn that bound while the dying predecessor
// still held the port got Errno 48 inside the worker and wedged it as a zombie
// with the model loaded. Both directions matter: it must actually WAIT (not
// sail past a held port) and it must actually RETURN (not hang forever).
func TestGemmaWaitPortFree(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port

	// Held → must time out rather than let a worker launch into a bind failure.
	start := time.Now()
	if err := gemmaWaitPortFree("127.0.0.1", port, 1200*time.Millisecond); err == nil {
		t.Fatal("returned success while the port was still held — a worker would die on bind")
	} else if time.Since(start) < time.Second {
		t.Fatalf("gave up after %v; it did not actually wait", time.Since(start))
	}

	// Released → must return promptly, or every restart pays the full timeout.
	go func() { time.Sleep(200 * time.Millisecond); _ = l.Close() }()
	start = time.Now()
	if err := gemmaWaitPortFree("127.0.0.1", port, 5*time.Second); err != nil {
		t.Fatalf("port was released but wait still failed: %v", err)
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Fatalf("took %v to notice a freed port", d)
	}
}

// A pid file outlives the process it names, and a stale one reads exactly like a
// healthy one unless liveness is checked separately. That is the
// green-surface-over-a-dead-thing shape, and it has cost real outages here.
func TestGemmaReadPidDistinguishesStaleFromLive(t *testing.T) {
	dir := t.TempDir()

	live := filepath.Join(dir, "live.pid")
	if err := os.WriteFile(live, []byte(fmt.Sprintf("%d\n", os.Getpid())), 0o644); err != nil {
		t.Fatal(err)
	}
	if pid, alive := gemmaReadPid(live); !alive || pid != os.Getpid() {
		t.Fatalf("live pid read as pid=%d alive=%v", pid, alive)
	}

	// A pid that cannot be running. Trailing newline included deliberately: the
	// files are written with one, and an unparsed newline would make every real
	// read fail closed and look like "not running".
	stale := filepath.Join(dir, "stale.pid")
	if err := os.WriteFile(stale, []byte("999999\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, alive := gemmaReadPid(stale); alive {
		t.Fatal("stale pid reported alive — a dead broker would render as healthy")
	}

	if _, alive := gemmaReadPid(filepath.Join(dir, "nope.pid")); alive {
		t.Fatal("missing pid file reported alive")
	}
	if err := os.WriteFile(filepath.Join(dir, "junk.pid"), []byte("not-a-pid"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, alive := gemmaReadPid(filepath.Join(dir, "junk.pid")); alive {
		t.Fatal("unparseable pid file reported alive")
	}
}

// The pid-file contract is the thing that broke a reader, so pin the paths.
func TestGemmaPidPathsAreDistinct(t *testing.T) {
	home := "/tmp/x"
	if gemmaPidPath(home) == gemmaWorkerPidPath(home) {
		t.Fatal("supervisor and worker pid paths must differ (ADR-046 pid-file contract)")
	}
}

// The obvious name (gemma-worker.pid) was already owned by the launchd job
// ai.sirsi.gemma-worker. Pin the rename so nobody "tidies" it back.
func TestGemmaWorkerPidDoesNotCollideWithTaskWorker(t *testing.T) {
	got := gemmaWorkerPidPath("/h")
	if filepath.Base(got) == "gemma-worker.pid" {
		t.Fatal("gemma-worker.pid belongs to ai.sirsi.gemma-worker — writing it clobbers a live launchd job")
	}
}

// Overwriting a pidfile that names another live job silently transfers ownership
// of a running process to something that did not start it.
func TestGemmaWritePidExclusiveRefusesLiveForeignPid(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "taken.pid")
	if err := os.WriteFile(p, []byte(fmt.Sprintf("%d\n", os.Getpid())), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := gemmaWritePidExclusive(p, os.Getpid()+100000); err == nil {
		t.Fatal("overwrote a pidfile naming a live foreign process")
	}
	// Stale file is the ordinary restart case and must be reclaimed.
	stale := filepath.Join(dir, "stale.pid")
	if err := os.WriteFile(stale, []byte("999999\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := gemmaWritePidExclusive(stale, 4242); err != nil {
		t.Fatalf("refused to reclaim a stale pidfile: %v", err)
	}
}
