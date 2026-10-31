package router

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routercfg"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

func TestStoreOnlyG1PruneAndRegistrationPreserveSeededG2Fence(t *testing.T) {
	// Isolate identity, legacy registry and canonical store before any API use.
	t.Setenv("HOME", t.TempDir())
	t.Setenv(routercfg.StoreWakeEnv, "1")
	t.Setenv("SIRSI_ROUTER_URL", "")
	t.Setenv("SIRSI_ROUTER_TOKEN", "")
	t.Setenv("SIRSI_ALLOW_SCHEMA_MIGRATE", "1")
	path := filepath.Join(t.TempDir(), "router.db")
	t.Setenv("SIRSI_ROUTER_DB", path)
	root := t.TempDir()
	store, err := routerstore.OpenPath(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	host, err := routerstore.ThreadOwnerHost()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	old := now.Add(-TerminalRetention - time.Hour)
	initial := &ThreadRegistry{Threads: map[string]*Thread{
		"stale-writer": {ThreadID: "stale-writer", AgentID: "fixture-lane", Host: host, MachineID: "fixture-machine", Surface: "worker", SessionID: "stable-conversation", Status: ThreadStatusActive, LastSeenAt: old},
	}}
	if err := SaveThreadRegistry(root, initial); err != nil {
		t.Fatal(err)
	}
	stale, err := LoadThreadRegistry(root) // process B holds an active baseline
	if err != nil {
		t.Fatal(err)
	}
	terminal, err := LoadThreadRegistry(root) // process A closes, then prunes
	if err != nil {
		t.Fatal(err)
	}
	terminal.Threads["stale-writer"].Status = ThreadStatusClosed
	terminal.Threads["stale-writer"].LastSeenAt = old.Add(time.Minute)
	if err := SaveThreadRegistry(root, terminal); err != nil {
		t.Fatal(err)
	}
	pruning, err := LoadThreadRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := pruning.PruneClosed(now, TerminalRetention); got != 1 {
		t.Fatalf("fixture did not prune terminal: %d", got)
	}
	if err := SaveThreadRegistry(root, pruning); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ThreadBinding("stale-writer"); !errors.Is(err, routerstore.ErrThreadUnknown) {
		t.Fatalf("G1 ordinary prune was not physical: %v", err)
	}
	// Seed only this disposable store with the accepted G2 fence shape. G1
	// itself must never create this row; b1 remains the producer's source proof.
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	var initialFences int
	if err := raw.QueryRow(`SELECT count(*) FROM threads WHERE status='pruned'`).Scan(&initialFences); err != nil || initialFences != 0 {
		t.Fatalf("G1 ordinary prune produced fences: count=%d err=%v", initialFences, err)
	}
	if _, err := raw.Exec(`INSERT INTO threads(thread_id,agent,host,status,last_seen_at,payload) VALUES(?,?,?,?,?,?)`,
		"stale-writer", "fixture-lane", host, "pruned", old.Add(time.Minute).Format(routerstore.AudienceTS), []byte("{}")); err != nil {
		t.Fatal(err)
	}
	stale.Threads["stale-writer"].CurrentItem = "dirty-heartbeat-after-prune"
	stale.Threads["stale-writer"].LastSeenAt = now
	if err := SaveThreadRegistry(root, stale); !errors.Is(err, ErrLostLifecycleFence) {
		t.Fatalf("stale dirty baseline did not lose fence: %v", err)
	}
	visible, err := LoadThreadRegistry(root)
	if err != nil || len(visible.Threads) != 0 {
		t.Fatalf("stale baseline resurrected hidden thread: %+v err=%v", visible, err)
	}
	binding, err := store.ThreadBinding("stale-writer")
	if err != nil || binding.Status != "pruned" || binding.Agent != "fixture-lane" || binding.Host != host {
		t.Fatalf("retained authority fence: %+v err=%v", binding, err)
	}
	// Pinning the retired runtime ID must fail; the same conversation may
	// recover through ordinary registration with a newly minted runtime ID.
	if _, err := RegisterThread(root, &Thread{ThreadID: "stale-writer", AgentID: "fixture-lane", Host: host,
		MachineID: "fixture-machine", Surface: "worker", SessionID: "stable-conversation"}); !errors.Is(err, ErrLostLifecycleFence) {
		t.Fatalf("explicit retired ID pin did not fail closed: %v", err)
	}
	// Ordinary registration must still opportunistically prune another aged
	// terminal row physically while leaving the seeded fence intact.
	visible, err = LoadThreadRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	visible.Threads["ordinary-terminal"] = &Thread{ThreadID: "ordinary-terminal", AgentID: "fixture-lane", Host: host,
		Status: ThreadStatusClosed, LastSeenAt: old}
	if err := SaveThreadRegistry(root, visible); err != nil {
		t.Fatal(err)
	}
	fresh, err := RegisterThread(root, &Thread{AgentID: "fixture-lane", Host: host,
		MachineID: "fixture-machine", Surface: "worker", SessionID: "stable-conversation"})
	if err != nil {
		t.Fatalf("same conversation could not register fresh runtime: %v", err)
	}
	if fresh.ThreadID == "" || fresh.ThreadID == "stale-writer" || fresh.SessionID != "stable-conversation" || fresh.Status != ThreadStatusActive {
		t.Fatalf("fresh registration adopted retired identity: %+v", fresh)
	}
	visible, err = LoadThreadRegistry(root)
	if err != nil || len(visible.Threads) != 1 || visible.Threads[fresh.ThreadID] == nil {
		t.Fatalf("fresh runtime not exclusively visible: %+v err=%v", visible, err)
	}
	if binding, err = store.ThreadBinding("stale-writer"); err != nil || binding.Status != "pruned" {
		t.Fatalf("fresh registration erased old fence: %+v err=%v", binding, err)
	}
	if _, err := store.ThreadBinding("ordinary-terminal"); !errors.Is(err, routerstore.ErrThreadUnknown) {
		t.Fatalf("registration did not physically prune ordinary terminal: %v", err)
	}
	var fences int
	if err := raw.QueryRow(`SELECT count(*) FROM threads WHERE status='pruned'`).Scan(&fences); err != nil || fences != 1 {
		t.Fatalf("G1 produced or lost a fence: count=%d err=%v", fences, err)
	}
}
