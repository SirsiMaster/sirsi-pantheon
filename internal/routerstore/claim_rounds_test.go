package routerstore

import (
	"errors"
	"fmt"
	"math/rand"
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
	const rounds = 1000
	rng := rand.New(rand.NewSource(62))
	shared := newTestStore(t)
	for i := 0; i < rounds; i++ {
		id := fmt.Sprintf("r%04d", i)
		if err := shared.AddTask(Task{Agent: "codex-home", TaskID: id, Subject: id}); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < rounds; i++ {
		if w := contend(t, shared, shared, fmt.Sprintf("r%04d", i), rng); w != 1 {
			t.Fatalf("round %d: %d winners on one shared ledger, want exactly 1", i, w)
		}
	}

	left, right := newTestStore(t), newTestStore(t)
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
