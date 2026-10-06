package router

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routercfg"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

func TestThreadTimestampKeyAdvancesAcrossZeroNanoseconds(t *testing.T) {
	store, err := routerstore.OpenPath(filepath.Join(t.TempDir(), "router.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	base := time.Date(2026, 8, 6, 9, 31, 0, 0, time.UTC)
	makeRecord := func(seen time.Time, item string) routerstore.ThreadRecord {
		thread := &Thread{ThreadID: "thr", AgentID: "codex-pantheon", Status: ThreadStatusActive, LastSeenAt: seen, CurrentItem: item}
		records, recordErr := threadRecords(&ThreadRegistry{Threads: map[string]*Thread{"thr": thread}})
		if recordErr != nil {
			t.Fatal(recordErr)
		}
		return records[0]
	}
	first := makeRecord(base, "old")
	later := makeRecord(base.Add(500*time.Millisecond), "new")
	if !(later.LastSeenAt > first.LastSeenAt) {
		t.Fatalf("fixed-width timestamp lost ordering: %q <= %q", later.LastSeenAt, first.LastSeenAt)
	}
	err = store.UpsertThreads([]routerstore.ThreadRecord{first})
	if err != nil {
		t.Fatal(err)
	}
	err = store.UpsertThreads([]routerstore.ThreadRecord{later})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := store.ListThreads()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].LastSeenAt != later.LastSeenAt {
		t.Fatalf("later sub-second heartbeat was not applied: %#v", rows)
	}
}

func TestStoreOnlyThreadLifecycleDoesNotWriteRegistryFile(t *testing.T) {
	home := t.TempDir()
	routerRoot := filepath.Join(t.TempDir(), ".agents", "idea-router")
	if err := os.MkdirAll(routerRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv(routercfg.StoreWakeEnv, "1")
	t.Setenv("SIRSI_ROUTER_DB", filepath.Join(home, ".sirsi", "router.db"))
	t.Setenv("SIRSI_ALLOW_SCHEMA_MIGRATE", "1")

	thread, err := RegisterThread(routerRoot, &Thread{ThreadID: "019f8fc4-96a4-7f00-b564-d91a64d0a4d1", AgentID: "codex-pantheon", Surface: "codex"})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	item := "adr057-thread-heartbeat-store-boundary"
	_, err = Heartbeat(routerRoot, thread.ThreadID, HeartbeatUpdate{CurrentItem: &item})
	if err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	reg, err := LoadThreadRegistry(routerRoot)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := reg.Threads[thread.ThreadID].CurrentItem; got != item {
		t.Fatalf("current item = %q, want %q", got, item)
	}
	_, err = CloseThread(routerRoot, thread.ThreadID)
	if err != nil {
		t.Fatalf("close: %v", err)
	}
	_, statErr := os.Stat(filepath.Join(routerRoot, "threads.json"))
	if !os.IsNotExist(statErr) {
		t.Fatalf("STORE-ONLY wrote threads.json: %v", statErr)
	}
}

func TestStoreOnlyConcurrentDistinctRegistrationsSurvive(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(t.TempDir(), ".agents", "idea-router")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv(routercfg.StoreWakeEnv, "1")
	t.Setenv("SIRSI_ROUTER_DB", filepath.Join(home, ".sirsi", "router.db"))
	t.Setenv("SIRSI_ALLOW_SCHEMA_MIGRATE", "1")
	done := make(chan error, 2)
	for _, id := range []string{"thread-a", "thread-b"} {
		go func(threadID string) {
			_, registerErr := RegisterThread(root, &Thread{ThreadID: threadID, AgentID: "codex-pantheon", Surface: "codex"})
			done <- registerErr
		}(id)
	}
	for range 2 {
		if chErr := <-done; chErr != nil {
			t.Fatal(chErr)
		}
	}
	reg, err := LoadThreadRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	if reg.Threads["thread-a"] == nil || reg.Threads["thread-b"] == nil {
		t.Fatalf("registrations lost: %#v", reg.Threads)
	}
}

func TestStoreOnlySuspendResumePersistsAndImportsLegacy(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(t.TempDir(), ".agents", "idea-router")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	legacy := &ThreadRegistry{Threads: map[string]*Thread{"legacy": {ThreadID: "legacy", AgentID: "codex-pantheon", Surface: "codex", Status: ThreadStatusActive, LastSeenAt: time.Now().Add(-time.Minute)}}}
	if err := SaveThreadRegistry(root, legacy); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv(routercfg.StoreWakeEnv, "1")
	t.Setenv("SIRSI_ROUTER_DB", filepath.Join(home, ".sirsi", "router.db"))
	t.Setenv("SIRSI_ALLOW_SCHEMA_MIGRATE", "1")
	reg, err := LoadThreadRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	if reg.Threads["legacy"] == nil {
		t.Fatal("legacy thread not imported")
	}
	_, err = SuspendThread(root, "legacy", &SuspendPayload{ResumePrompt: "continue"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = ResumeThread(root, "legacy")
	if err != nil {
		t.Fatal(err)
	}
	reg, err = LoadThreadRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := reg.Threads["legacy"].Status; got != ThreadStatusActive {
		t.Fatalf("persisted status=%q, want active", got)
	}
}

func TestStoreOnlyPruneDeletesOnlyObservedTerminal(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(t.TempDir(), ".agents", "idea-router")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv(routercfg.StoreWakeEnv, "1")
	t.Setenv("SIRSI_ROUTER_DB", filepath.Join(home, ".sirsi", "router.db"))
	t.Setenv("SIRSI_ALLOW_SCHEMA_MIGRATE", "1")
	old := time.Now().Add(-TerminalRetention - time.Hour)
	reg := &ThreadRegistry{Threads: map[string]*Thread{"old": {ThreadID: "old", AgentID: "a", Surface: "codex", Status: ThreadStatusClosed, LastSeenAt: old}, "live": {ThreadID: "live", AgentID: "a", Surface: "codex", Status: ThreadStatusActive, LastSeenAt: time.Now()}}}
	if err := SaveThreadRegistry(root, reg); err != nil {
		t.Fatal(err)
	}
	reg, err := LoadThreadRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	reg.PruneClosed(time.Now(), TerminalRetention)
	err = SaveThreadRegistry(root, reg)
	if err != nil {
		t.Fatal(err)
	}
	reg, err = LoadThreadRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	if reg.Threads["old"] != nil || reg.Threads["live"] == nil {
		t.Fatalf("prune result=%#v", reg.Threads)
	}
}

// TestSetThreadConsumerCapablePersistsAgainstRealStore is the end-to-end
// sanity check for lifecycle-fence-lost's fix: setThreadConsumerCapable now
// wraps its load-mutate-save in retryOnLostFenceErr (fenceretry_test.go pins
// that helper's retry/backoff/surface behavior against mocks). This test
// exercises the real call site against a real store-wake-mode store, which
// the mock tests alone do not — a mocked retryOnLostFenceErr call proves the
// helper works but not that setThreadConsumerCapable actually uses it.
func TestSetThreadConsumerCapablePersistsAgainstRealStore(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(t.TempDir(), ".agents", "idea-router")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv(routercfg.StoreWakeEnv, "1")
	t.Setenv("SIRSI_ROUTER_DB", filepath.Join(home, ".sirsi", "router.db"))
	t.Setenv("SIRSI_ALLOW_SCHEMA_MIGRATE", "1")

	if _, err := RegisterThread(root, &Thread{ThreadID: "thr-consumer", AgentID: "codex-pantheon", Surface: "codex"}); err != nil {
		t.Fatal(err)
	}
	if err := setThreadConsumerCapable(root, "thr-consumer", true); err != nil {
		t.Fatalf("setThreadConsumerCapable: %v", err)
	}
	reg, err := LoadThreadRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	if reg.Threads["thr-consumer"] == nil || !reg.Threads["thr-consumer"].ConsumerCapable {
		t.Fatalf("ConsumerCapable did not persist: %#v", reg.Threads["thr-consumer"])
	}
}

// TestHeartbeat_ConcurrentWritersDoNotSurfaceLostFence is the root-cause
// regression for m5-thread-registry-fence-contention: Heartbeat is the
// highest-frequency registry mutator (every registered wake loop calls it
// once per cycle, A27), and unlike setThreadConsumerCapable it had no
// retry-on-lost-fence wrapping — any wake loop whose heartbeat raced a
// concurrent writer touching the SAME shared registry save got "mutation
// lost lifecycle fence" as a hard failure instead of an absorbed retry,
// observed as a retry storm (65x in one lane's log on 2026-10-05). This pins
// real CAS contention (several goroutines heartbeating concurrently against
// the same store-wake registry row) and requires zero fence errors to
// surface. Verified as a negative control (A35): reverting the retry wrap
// reproduces this failure.
func TestHeartbeat_ConcurrentWritersDoNotSurfaceLostFence(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(t.TempDir(), ".agents", "idea-router")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv(routercfg.StoreWakeEnv, "1")
	t.Setenv("SIRSI_ROUTER_DB", filepath.Join(home, ".sirsi", "router.db"))
	t.Setenv("SIRSI_ALLOW_SCHEMA_MIGRATE", "1")

	thr, err := RegisterThread(root, &Thread{AgentID: "claude-test", Surface: "claude", PID: 9100, StartTime: "sig"})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	// A few goroutines heartbeat the SAME thread id concurrently — this is the
	// actual CAS race: each loads the registry, mutates its in-memory copy,
	// then saves, so whichever saves second finds the row changed since its
	// own load baseline. Without retry that surfaces as a hard failure on
	// every lost race, not just the first writer to land. Kept small (3
	// workers, 2 heartbeats each) so the 3-attempt/50ms retry budget — shared
	// infra, not tunable per-caller — reliably absorbs it rather than
	// modeling contention heavier than a real wake-loop fleet produces.
	const n = 3
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 2; j++ {
				item := "cycle"
				if _, err := Heartbeat(root, thr.ThreadID, HeartbeatUpdate{CurrentItem: &item}); err != nil {
					errs[i] = err
					return
				}
			}
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil && strings.Contains(err.Error(), "lost lifecycle fence") {
			t.Fatalf("heartbeat %d surfaced lost lifecycle fence instead of retrying: %v", i, err)
		}
		if err != nil {
			t.Fatalf("heartbeat %d: %v", i, err)
		}
	}
}
