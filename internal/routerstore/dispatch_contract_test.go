package routerstore

// §2b acceptance bar (PRD ROUTER_V2_DURABLE_DISPATCH; ADR-035 axiom 6):
// "Safety tests reproduce BOTH incidents and pass." The claude build-worker
// stays OFF until every test in this file is green.

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func openTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	s := openBackendStore(t, filepath.Join(t.TempDir(), "router.db"))
	t.Cleanup(func() { _ = s.Close() })
	s.notifyDir = t.TempDir()
	return s
}

// seedOpen inserts n open items for agent and returns their ids.
func seedOpen(t *testing.T, s *SQLiteStore, agent string, n int) []string {
	t.Helper()
	ids := make([]string, 0, n)
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("item-%03d", i)
		if err := s.Put(Item{ID: id, From: "seed", To: agent, Title: fmt.Sprintf("t%d", i), Status: "open"}); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	return ids
}

// --- Incident 1 (19,195 sessions / 0 closed): the loop must be structurally impossible ---

// TestFailingItemConvergesToDeadLetter: an item whose work always fails is
// re-claimable at most MaxRetriesPerItem times, then dead-letters TERMINALLY
// and is never claimable again — infinite reprocess cannot exist.
func TestFailingItemConvergesToDeadLetter(t *testing.T) {
	s := openTestStore(t)
	seedOpen(t, s, "worker", 1)

	claims := 0
	for {
		lease, err := s.ClaimNext("worker", time.Minute)
		if errors.Is(err, ErrNoWork) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		claims++
		if claims > MaxRetriesPerItem+1 {
			t.Fatalf("item still claimable after %d attempts — the 19,195-loop lives", claims)
		}
		if err := s.Fail(lease.ItemID, lease.Token, "rc=1", "build_error"); err != nil {
			t.Fatal(err)
		}
	}
	if claims != MaxRetriesPerItem {
		t.Fatalf("expected exactly %d claims before dead-letter, got %d", MaxRetriesPerItem, claims)
	}
	it, err := s.Get("item-000")
	if err != nil {
		t.Fatal(err)
	}
	if it.Status != StatusDeadLetter {
		t.Fatalf("expected dead_letter, got %q", it.Status)
	}
	// Terminal is terminal: a stale lifecycle op cannot resurrect it.
	if err := s.ForceOwner("item-000", "reopen", ""); !errors.Is(err, ErrReasonRequired) {
		t.Fatalf("force without reason must be refused, got %v", err)
	}
}

// TestDuplicateClaimRace: concurrent claimants over one inbox — every item is
// claimed exactly once; no double execution ownership.
func TestDuplicateClaimRace(t *testing.T) {
	s := openTestStore(t)
	old := MaxConcurrentClaimsPerTarget
	MaxConcurrentClaimsPerTarget = 100
	t.Cleanup(func() { MaxConcurrentClaimsPerTarget = old })

	const items = 20
	seedOpen(t, s, "worker", items)

	var mu sync.Mutex
	seen := map[string]int{}
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				lease, err := s.ClaimNext("worker", time.Minute)
				if errors.Is(err, ErrNoWork) {
					return
				}
				if err != nil {
					t.Errorf("claim: %v", err)
					return
				}
				mu.Lock()
				seen[lease.ItemID]++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if len(seen) != items {
		t.Fatalf("expected %d distinct items claimed, got %d", items, len(seen))
	}
	for id, n := range seen {
		if n != 1 {
			t.Fatalf("item %s claimed %d times — duplicate execution ownership", id, n)
		}
	}
}

// TestExpiredWorkerCannotCompleteNewerLeasedWork: w1's lease expires, w2
// reclaims — w1's stale token must be refused everywhere.
func TestExpiredWorkerCannotCompleteNewerLeasedWork(t *testing.T) {
	s := openTestStore(t)
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	seedOpen(t, s, "worker", 1)

	w1, err := s.ClaimNext("worker", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Minute) // w1's lease expires

	w2, err := s.ClaimNext("worker", time.Minute) // reclaim path returns it to open, then claims
	if err != nil {
		t.Fatal(err)
	}
	if w2.Token == w1.Token {
		t.Fatal("reclaim must mint a fresh token")
	}
	if err := s.Complete(w1.ItemID, w1.Token, "stale complete"); !errors.Is(err, ErrLeaseInvalid) {
		t.Fatalf("stale token must be ErrLeaseInvalid, got %v", err)
	}
	if err := s.RenewLease(w1.ItemID, w1.Token, time.Minute); !errors.Is(err, ErrLeaseInvalid) {
		t.Fatalf("stale renew must be ErrLeaseInvalid, got %v", err)
	}
	if err := s.Complete(w2.ItemID, w2.Token, "done"); err != nil {
		t.Fatalf("live lease must complete: %v", err)
	}
	it, _ := s.Get(w2.ItemID)
	if it.Status != StatusCompleted {
		t.Fatalf("expected completed, got %q", it.Status)
	}
	if err := s.Complete(w2.ItemID, w2.Token, "again"); !errors.Is(err, ErrTerminal) {
		t.Fatalf("terminal is terminal, got %v", err)
	}
}

// TestRestartMidLease: a store reopened over the same DB (process restart)
// still enforces the live lease, and reclaims it after expiry.
func TestRestartMidLease(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "router.db")
	s1, err := OpenPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = s1.Put(Item{ID: "item-000", From: "seed", To: "worker", Title: "t", Status: "open"}); err != nil {
		t.Fatal(err)
	}
	lease, err := s1.ClaimNext("worker", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	_ = s1.Close() // "restart"

	s2, err := OpenPath(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s2.Close() }()

	// Lease still live across restart: item not claimable, token still works.
	if _, err := s2.ClaimNext("worker", time.Minute); !errors.Is(err, ErrNoWork) {
		t.Fatalf("live lease must survive restart, got %v", err)
	}
	if err := s2.Complete(lease.ItemID, lease.Token, "done after restart"); err != nil {
		t.Fatalf("token minted before restart must still fence-check: %v", err)
	}
}

// --- Incident 2 (11,564-item flood): floods must update singletons, never append ---

// TestSenderFloodRejected reproduces the flood: hundreds of sends from one
// runaway sender yield at most quota rows + ONE throttle singleton. The other
// 460+ sends leave a counter, not items.
func TestSenderFloodRejected(t *testing.T) {
	s := openTestStore(t)
	oldQ := MaxSendsPerSenderPerWindow
	MaxSendsPerSenderPerWindow = 10
	t.Cleanup(func() { MaxSendsPerSenderPerWindow = oldQ })

	quotaDrops, breakerBlocks := 0, 0
	for i := 0; i < 500; i++ {
		_, _, err := s.SendGuarded(SendReq{
			From: "runaway-worker", To: "claude-home",
			Title: fmt.Sprintf("WORKER GAVE UP #%d", i), // rephrased every time, like the real flood
		})
		switch {
		case errors.Is(err, ErrOverQuota):
			quotaDrops++ // quota refused it; throttle singleton updated
		case errors.Is(err, ErrBreakerOpen):
			breakerBlocks++ // defense-in-depth: repeated drops tripped the sender's breaker
		case err != nil:
			t.Fatal(err)
		}
	}
	if quotaDrops == 0 || breakerBlocks == 0 {
		t.Fatalf("flood must first throttle (%d) then trip the sender breaker (%d)", quotaDrops, breakerBlocks)
	}
	all, err := s.ListAll()
	if err != nil {
		t.Fatal(err)
	}
	// quota rows + one throttle singleton + one breaker operator item — NOT 500.
	if len(all) > MaxSendsPerSenderPerWindow+2 {
		t.Fatalf("flood appended %d items — the 11,564 flood lives", len(all))
	}
	var throttle, operator int
	for i := range all {
		if all[i].From != "routerstore" {
			continue
		}
		if all[i].Title == "throttled: runaway-worker over send quota" {
			throttle++
		} else {
			operator++
		}
	}
	if throttle != 1 {
		t.Fatalf("expected exactly 1 throttle singleton, got %d", throttle)
	}
	if operator > 1 {
		t.Fatalf("expected at most 1 breaker operator item, got %d", operator)
	}
	c, err := s.Counters()
	if err != nil {
		t.Fatal(err)
	}
	if int(c.RateLimitDrops) != quotaDrops {
		t.Fatalf("drops must be one red number: counter=%d actual=%d", c.RateLimitDrops, quotaDrops)
	}
}

// TestIdempotentDuplicateSend: the same logical send twice is one row.
func TestIdempotentDuplicateSend(t *testing.T) {
	s := openTestStore(t)
	req := SendReq{From: "a", To: "b", Title: "deploy the fix", SubjectKey: "deploy-fix", SourceItem: "item-42"}

	id1, dup1, err := s.SendGuarded(req)
	if err != nil || dup1 {
		t.Fatalf("first send: id=%s dup=%v err=%v", id1, dup1, err)
	}
	id2, dup2, err := s.SendGuarded(req)
	if err != nil || !dup2 || id2 != id1 {
		t.Fatalf("second send must dedupe to same id: id=%s dup=%v err=%v", id2, dup2, err)
	}
	all, _ := s.ListAll()
	if len(all) != 1 {
		t.Fatalf("expected 1 row, got %d", len(all))
	}
}

// TestStuckItemProducesOneEscalation: a stuck item that dead-letters (and
// keeps being reported) yields exactly one terminal record and exactly one
// escalation row — occurrences grow, rows do not.
func TestStuckItemProducesOneEscalation(t *testing.T) {
	s := openTestStore(t)
	seedOpen(t, s, "worker", 1)

	for {
		lease, err := s.ClaimNext("worker", time.Minute)
		if errors.Is(err, ErrNoWork) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Fail(lease.ItemID, lease.Token, "stuck", "stuck_class"); err != nil {
			t.Fatal(err)
		}
	}
	all, _ := s.ListAll()
	var escalations, deadLetters int
	for _, it := range all {
		if it.Status == StatusDeadLetter {
			deadLetters++
		}
		if it.From == "routerstore" {
			escalations++
		}
	}
	if deadLetters != 1 || escalations != 1 {
		t.Fatalf("stuck item must yield exactly 1 dead_letter + 1 escalation, got %d + %d", deadLetters, escalations)
	}
}

// TestBreakerTripsOnceAndGates: systemic failure trips the domain breaker,
// writes ONE operator item, and pauses dispatch until an explicit reset.
func TestBreakerTripsOnceAndGates(t *testing.T) {
	s := openTestStore(t)
	oldT := BreakerThreshold
	BreakerThreshold = 3
	t.Cleanup(func() { BreakerThreshold = oldT })
	oldR := MaxRetriesPerItem
	MaxRetriesPerItem = 1 // every Fail dead-letters → records breaker failures
	t.Cleanup(func() { MaxRetriesPerItem = oldR })

	seedOpen(t, s, "worker", 5)
	for i := 0; i < 3; i++ { // 3 dead-letters → class/target/sender domains hit threshold
		lease, err := s.ClaimNext("worker", time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Fail(lease.ItemID, lease.Token, "systemic", "same_class"); err != nil {
			t.Fatal(err)
		}
	}
	// Target breaker now tripped: dispatch through it pauses.
	if _, err := s.ClaimNext("worker", time.Minute); !errors.Is(err, ErrBreakerOpen) {
		t.Fatalf("tripped target breaker must pause claims, got %v", err)
	}
	// Exactly one operator item for the tripped domain, no matter how many failures followed.
	all, _ := s.ListAll()
	operator := 0
	for _, it := range all {
		if it.From == "routerstore" && it.Title == "breaker tripped: target:worker" {
			operator++
		}
	}
	if operator != 1 {
		t.Fatalf("expected exactly 1 operator item for the tripped breaker, got %d", operator)
	}
	if err := s.ResetBreaker("target:worker"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ClaimNext("worker", time.Minute); errors.Is(err, ErrBreakerOpen) {
		t.Fatal("reset breaker must re-open dispatch")
	}
}

// TestBudgetsRefuse: per-target concurrent claims cap at MaxConcurrentClaimsPerTarget.
func TestBudgetsRefuse(t *testing.T) {
	s := openTestStore(t)
	seedOpen(t, s, "worker", MaxConcurrentClaimsPerTarget+2)

	for i := 0; i < MaxConcurrentClaimsPerTarget; i++ {
		if _, err := s.ClaimNext("worker", time.Minute); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.ClaimNext("worker", time.Minute); !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("claim over budget must refuse, got %v", err)
	}
}

// --- Event-driven dispatch (PRD /goal #1) ---

// TestWaitWakesUnder250ms: a live send wakes a blocked waiter in <250ms.
func TestWaitWakesUnder250ms(t *testing.T) {
	s := openTestStore(t)
	woke := make(chan time.Duration, 1)
	ready := make(chan struct{})

	go func() {
		close(ready)
		start := time.Now()
		ok, err := s.Wait(context.Background(), "sleepy", 5*time.Second)
		if err != nil || !ok {
			t.Errorf("wait: ok=%v err=%v", ok, err)
		}
		woke <- time.Since(start)
	}()
	<-ready
	time.Sleep(50 * time.Millisecond) // let the waiter block
	sendAt := time.Now()
	if _, _, err := s.SendGuarded(SendReq{From: "a", To: "sleepy", Title: "wake up"}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-woke:
		if lat := time.Since(sendAt); lat >= 250*time.Millisecond {
			t.Fatalf("wake took %v — poll floor lives (goal is <250ms)", lat)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("waiter never woke")
	}
}

// TestWaitTimeoutReturnsCleanly: no work → (false, nil) at the ceiling.
func TestWaitTimeoutReturnsCleanly(t *testing.T) {
	s := openTestStore(t)
	ok, err := s.Wait(context.Background(), "nobody", 100*time.Millisecond)
	if err != nil || ok {
		t.Fatalf("timeout must be (false, nil), got (%v, %v)", ok, err)
	}
}

// TestWaitMissedSignalFallback: an item written around the facade (raw Put —
// no notify) is still delivered by the safety re-check. A missed signal never
// strands a waiter.
func TestWaitMissedSignalFallback(t *testing.T) {
	s := openTestStore(t)
	oldTick := WaitSafetyRecheck
	WaitSafetyRecheck = 50 * time.Millisecond
	t.Cleanup(func() { WaitSafetyRecheck = oldTick })

	done := make(chan bool, 1)
	go func() {
		ok, err := s.Wait(context.Background(), "worker", 5*time.Second)
		if err != nil {
			t.Errorf("wait: %v", err)
		}
		done <- ok
	}()
	time.Sleep(30 * time.Millisecond)
	if err := s.Put(Item{ID: "sneaky", From: "x", To: "worker", Title: "no notify", Status: "open"}); err != nil {
		t.Fatal(err)
	}
	select {
	case ok := <-done:
		if !ok {
			t.Fatal("fallback re-check must deliver the item")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("missed signal stranded the waiter — fallback tick broken")
	}
}

// TestCrossProcessFIFONotify: a poke through the notify FIFO reaches a listener.
func TestCrossProcessFIFONotify(t *testing.T) {
	s := openTestStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events, err := s.ListenNotify(ctx, "worker")
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond) // listener opens the FIFO
	s.NotifyAgent("worker")
	select {
	case <-events:
		// delivered
	case <-time.After(2 * time.Second):
		t.Fatal("FIFO poke never reached the listener")
	}
}

// --- Fenced override + observability ---

// TestForceOwnerAuditsAndReopens: the human override needs a reason, writes an
// audit record, and a reopened item is claimable again.
func TestForceOwnerAuditsAndReopens(t *testing.T) {
	s := openTestStore(t)
	seedOpen(t, s, "worker", 1)
	lease, err := s.ClaimNext("worker", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Fail(lease.ItemID, lease.Token, "x", "c"); err != nil {
		t.Fatal(err)
	}
	// Owner reopens WITH a reason (fresh attempts).
	if err = s.ForceOwner("item-000", "reopen", "verified fixed upstream"); err != nil {
		t.Fatal(err)
	}
	var audits int
	rows, err := s.db.Query(`SELECT key FROM state WHERE key LIKE 'audit:force-owner:%';`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		audits++
	}
	_ = rows.Close()
	if audits != 1 {
		t.Fatalf("expected 1 audit record, got %d", audits)
	}
	if _, err := s.ClaimNext("worker", time.Minute); err != nil {
		t.Fatalf("reopened item must be claimable: %v", err)
	}
}

// TestCountersAggregate: the §2b axiom-9 aggregate reflects real activity —
// the next incident is one red number.
func TestCountersAggregate(t *testing.T) {
	s := openTestStore(t)
	seedOpen(t, s, "worker", 2)
	lease, err := s.ClaimNext("worker", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Fail(lease.ItemID, lease.Token, "x", "c"); err != nil {
		t.Fatal(err)
	}
	c, err := s.Counters()
	if err != nil {
		t.Fatal(err)
	}
	if c.Claims != 1 || c.Retries != 1 || c.OpenItems != 2 {
		t.Fatalf("counters lie: %+v", c)
	}
}
