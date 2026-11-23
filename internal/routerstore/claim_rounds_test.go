package routerstore

import (
	"errors"
	"fmt"
	"math/rand"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// contend has two claimers race for task id on the given stores (the same
// store twice for a shared ledger, two different stores for the control) and
// returns how many believe they won.
func contend(t *testing.T, a, b *SQLiteStore, id string, rng *rand.Rand) int {
	t.Helper()
	jitterA := time.Duration(rng.Intn(1500)) * time.Microsecond
	jitterB := time.Duration(rng.Intn(1500)) * time.Microsecond
	var wg sync.WaitGroup
	var mu sync.Mutex
	wins := 0
	for i, s := range []*SQLiteStore{a, b} {
		jitter := []time.Duration{jitterA, jitterB}[i]
		wg.Add(1)
		go func() {
			defer wg.Done()
			time.Sleep(jitter) // injected latency, standing in for a slow service hop
			_, err := s.ClaimTask("codex-home", id, "worker", "thread", time.Minute)
			if err == nil {
				mu.Lock()
				wins++
				mu.Unlock()
			} else if !errors.Is(err, ErrNoClaimableTask) {
				t.Errorf("round %s: unexpected claim error: %v", id, err)
			}
		}()
	}
	wg.Wait()
	return wins
}

// TestClaimExactlyOnceAcrossManyRounds is G3's contention leg: 1,000 rounds,
// two claimers per round with random latency, exactly one winner every time.
// The negative control gives each claimer its OWN ledger (two separate SQLite
// files, the split-brain G3 names) and must see both "win", proving the
// counter can fail.
func TestClaimExactlyOnceAcrossManyRounds(t *testing.T) {
	// Own stores, own rng, no package state: safe to overlap the other parallel
	// tests (it runs after every serial test has finished). Not on Postgres,
	// where all tests share one schema.
	if pgTestDSN() == "" {
		t.Parallel()
	}
	// 1,000 rounds in total, as ever: four shards of 250, each on its own shared
	// ledger (both claimers of a round still race on ONE store). A claim scans the
	// agent's whole task set, so the cost of a round grows with the ledger; four
	// smaller ledgers run in parallel instead of one 1,000-row ledger in series.
	const shards, perShard = 4, 250
	for shard := 0; shard < shards; shard++ {
		t.Run(fmt.Sprintf("shard%d", shard), func(t *testing.T) {
			if pgTestDSN() == "" {
				t.Parallel()
			}
			rng := rand.New(rand.NewSource(int64(62 + shard)))
			shared := newTestStore(t)
			for i := 0; i < perShard; i++ {
				id := fmt.Sprintf("r%d-%04d", shard, i)
				if err := shared.AddTask(Task{Agent: "codex-home", TaskID: id, Subject: id}); err != nil {
					t.Fatal(err)
				}
			}
			for i := 0; i < perShard; i++ {
				if w := contend(t, shared, shared, fmt.Sprintf("r%d-%04d", shard, i), rng); w != 1 {
					t.Fatalf("round %d/%d: %d winners on one shared ledger, want exactly 1", shard, i, w)
				}
			}
		})
	}
	rng := rand.New(rand.NewSource(62))

	// Two explicit SQLite files, whatever driver the suite runs under: on the
	// Postgres leg every newTestStore shares one schema, which would make this
	// control the shared-ledger case again.
	left, right := openSplitStore(t, "left.db"), openSplitStore(t, "right.db")
	split := 0
	for i := 0; i < 50; i++ {
		id := fmt.Sprintf("c%02d", i)
		for _, s := range []*SQLiteStore{left, right} {
			if err := s.AddTask(Task{Agent: "codex-home", TaskID: id, Subject: id}); err != nil {
				t.Fatal(err)
			}
		}
		if contend(t, left, right, id, rng) == 2 {
			split++
		}
	}
	if split == 0 {
		t.Fatal("negative control: two separate ledgers never produced two winners, so the counter cannot detect split-brain")
	}
}

func openSplitStore(t *testing.T, name string) *SQLiteStore {
	t.Helper()
	s, err := OpenPath(seedFromTemplate(t, filepath.Join(t.TempDir(), name)))
	if err != nil {
		t.Fatalf("open %s: %v", name, err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}
