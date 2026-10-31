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

func TestStoreOnlySeededAndN2ProducedPruneFencesSurviveRegistration(t *testing.T) {
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

	// Seed the consumer guard directly in this disposable store. This seeded
	// fixture is independent of the production PruneClosed/SaveThreadRegistry
	// path exercised below.
	seed := &Thread{ThreadID: "seeded-writer", AgentID: "fixture-lane", Host: host,
		MachineID: "fixture-machine", Surface: "worker", SessionID: "seeded-conversation", Status: ThreadStatusActive, LastSeenAt: old}
	produced := &Thread{ThreadID: "n2-terminal", AgentID: "fixture-lane", Host: host,
		MachineID: "fixture-machine", Surface: "worker", SessionID: "stable-conversation", Status: ThreadStatusActive, LastSeenAt: old}
	initial := &ThreadRegistry{Threads: map[string]*Thread{seed.ThreadID: seed, produced.ThreadID: produced}}
	if err := SaveThreadRegistry(root, initial); err != nil {
		t.Fatal(err)
	}
	stale, err := LoadThreadRegistry(root) // an older writer retains the active baseline
	if err != nil {
		t.Fatal(err)
	}

	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	result, err := raw.Exec(`UPDATE threads SET status='pruned',last_seen_at=?,payload=? WHERE thread_id=? AND status='active'`,
		old.Format(routerstore.AudienceTS), []byte("{}"), seed.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	if n, err := result.RowsAffected(); err != nil || n != 1 {
		t.Fatalf("direct seeded fence rows=%d err=%v, want one", n, err)
	}
	stale.Threads[seed.ThreadID].CurrentItem = "dirty-heartbeat-after-prune"
	stale.Threads[seed.ThreadID].LastSeenAt = now
	if err := SaveThreadRegistry(root, stale); !errors.Is(err, ErrLostLifecycleFence) {
		t.Fatalf("stale dirty baseline did not lose seeded fence: %v", err)
	}
	visible, err = LoadThreadRegistry(root)
	if err != nil || len(visible.Threads) != 1 || visible.Threads[produced.ThreadID] == nil {
		t.Fatalf("seeded stale writer became visible or hid unrelated row: %+v err=%v", visible, err)
	}
	if _, err := RegisterThread(root, &Thread{ThreadID: seed.ThreadID, AgentID: seed.AgentID, Host: host,
		MachineID: seed.MachineID, Surface: seed.Surface, SessionID: seed.SessionID}); !errors.Is(err, ErrLostLifecycleFence) {
		t.Fatalf("explicit seeded retired ID pin did not fail closed: %v", err)
	}

	// Exercise the current N2 producer through the real registry consumer path:
	// persist terminal state, prune it from the loaded registry, then save the
	// removal. This must create the second retained fence without a SQL insert.
	staleProduced, err := LoadThreadRegistry(root) // this writer retains the ACTIVE N2 baseline
	if err != nil {
		t.Fatal(err)
	}
	terminal, err := LoadThreadRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	terminal.Threads[produced.ThreadID].Status = ThreadStatusClosed
	if err := SaveThreadRegistry(root, terminal); err != nil {
		t.Fatal(err)
	}
	pruning, err := LoadThreadRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := pruning.PruneClosed(now, TerminalRetention); got != 1 {
		t.Fatalf("N2 fixture did not prune terminal row: %d", got)
	}
	if err := SaveThreadRegistry(root, pruning); err != nil {
		t.Fatal(err)
	}
	if binding, err := store.ThreadBinding(produced.ThreadID); err != nil || binding.Status != "pruned" || binding.Agent != produced.AgentID || binding.Host != host || binding.LastSeenAt != old.Format(routerstore.AudienceTS) {
		t.Fatalf("N2-produced retirement fence: %+v err=%v", binding, err)
	}
	staleProduced.Threads[produced.ThreadID].CurrentItem = "dirty-heartbeat-after-n2-prune"
	staleProduced.Threads[produced.ThreadID].LastSeenAt = now
	if err := SaveThreadRegistry(root, staleProduced); !errors.Is(err, ErrLostLifecycleFence) {
		t.Fatalf("stale active N2 baseline did not lose fence: %v", err)
	}
	if _, err := RegisterThread(root, &Thread{ThreadID: produced.ThreadID, AgentID: produced.AgentID, Host: host,
		MachineID: produced.MachineID, Surface: produced.Surface, SessionID: produced.SessionID}); !errors.Is(err, ErrLostLifecycleFence) {
		t.Fatalf("explicit N2-retired ID pin did not fail closed: %v", err)
	}

	// Leave a third aged terminal record for RegisterThread's own
	// PruneClosed/SaveThreadRegistry pass; do not pre-prune this consumer case.
	visible, err := LoadThreadRegistry(root)
	if err != nil || len(visible.Threads) != 0 {
		t.Fatalf("unexpected visible records before registration prune: %+v err=%v", visible, err)
	}
	visible.Threads["registration-pruned"] = &Thread{ThreadID: "registration-pruned", AgentID: "fixture-lane", Host: host,
		MachineID: "fixture-machine", Surface: "worker", SessionID: "registration-pruned-conversation", Status: ThreadStatusClosed, LastSeenAt: old}
	if err := SaveThreadRegistry(root, visible); err != nil {
		t.Fatal(err)
	}

	// An ordinary registration for the same stable conversation gets a fresh
	// runtime ID, prunes the third terminal itself, and retains both older keys.
	fresh, err := RegisterThread(root, &Thread{AgentID: produced.AgentID, Host: host,
		MachineID: produced.MachineID, Surface: produced.Surface, SessionID: produced.SessionID})
	if err != nil {
		t.Fatalf("same conversation could not register fresh runtime: %v", err)
	}
	if fresh.ThreadID == "" || fresh.ThreadID == seed.ThreadID || fresh.ThreadID == produced.ThreadID || fresh.SessionID != produced.SessionID || fresh.Status != ThreadStatusActive {
		t.Fatalf("fresh registration adopted retired identity: %+v", fresh)
	}
	visible, err = LoadThreadRegistry(root)
	if err != nil || len(visible.Threads) != 1 || visible.Threads[fresh.ThreadID] == nil {
		t.Fatalf("fresh runtime not exclusively visible: %+v err=%v", visible, err)
	}
	var fences int
	if err := raw.QueryRow(`SELECT count(*) FROM threads WHERE status='pruned'`).Scan(&fences); err != nil || fences != 3 {
		t.Fatalf("ordinary registration lost or created a retirement fence: count=%d err=%v", fences, err)
	}
	for _, want := range []struct {
		id, agent, lastSeen string
	}{
		{seed.ThreadID, seed.AgentID, old.Format(routerstore.AudienceTS)},
		{produced.ThreadID, produced.AgentID, old.Format(routerstore.AudienceTS)},
		{"registration-pruned", "fixture-lane", old.Format(routerstore.AudienceTS)},
	} {
		binding, err := store.ThreadBinding(want.id)
		if err != nil || binding.ThreadID != want.id || binding.Status != "pruned" || binding.Agent != want.agent || binding.Host != host || binding.LastSeenAt != want.lastSeen {
			t.Fatalf("retained identity for %s: %+v err=%v", want.id, binding, err)
		}
		var payload string
		if err := raw.QueryRow(`SELECT payload FROM threads WHERE thread_id=?`, want.id).Scan(&payload); err != nil || payload != "{}" {
			t.Fatalf("retirement payload for %s = %q err=%v, want empty object", want.id, payload, err)
		}
	}
}
