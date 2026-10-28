package ledger

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fabricReadResult struct {
	generation *FabricGeneration
	err        error
}

func fabricAsyncRead(p *FabricGenerationProducer, ctx context.Context) <-chan fabricReadResult {
	done := make(chan fabricReadResult, 1)
	go func() { g, e := p.Read(ctx); done <- fabricReadResult{g, e} }()
	return done
}
func fabricTake(t *testing.T, done <-chan fabricReadResult) fabricReadResult {
	t.Helper()
	select {
	case v := <-done:
		return v
	case <-time.After(time.Second):
		t.Fatal("Fabric reader did not return within fixture deadline")
		return fabricReadResult{}
	}
}
func fabricSignal(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal("Fabric fixture barrier not reached")
	}
}
func fabricWaiterCount(t *testing.T, p *FabricGenerationProducer, n int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		p.mu.Lock()
		count := 0
		if p.active != nil {
			count = len(p.active.waiters)
		}
		p.mu.Unlock()
		if count == n {
			return
		}
		runtime.Gosched()
	}
	t.Fatalf("Fabric operation did not admit %d waiters", n)
}
func fabricRelease(ch chan struct{}) {
	select {
	case <-ch:
	default:
		close(ch)
	}
}
func fabricStatusRead(status string) FabricRead {
	snapshot := fabricTaskFixture(1)
	snapshot.Agents[0].Tasks[0].Status = status
	return FabricRead{Snapshot: snapshot}
}

type fabricObservedContext struct {
	context.Context
	observed chan struct{}
	once     sync.Once
}

func (c *fabricObservedContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.observed) })
	return c.Context.Done()
}

func TestFabricCoalescesOnlyOverlappingUnfilteredReads(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	defer fabricRelease(release)
	var calls atomic.Int32
	p := NewFabricGenerationProducerContext(context.Background(), func(ctx context.Context) (FabricRead, error) {
		if calls.Add(1) == 1 {
			close(entered)
			select {
			case <-release:
			case <-ctx.Done():
				return FabricRead{}, ctx.Err()
			}
		}
		return fabricStatusRead("pending"), nil
	}, "fixture")
	defer p.Close()
	first := fabricAsyncRead(p, context.Background())
	fabricSignal(t, entered)
	second := fabricAsyncRead(p, context.Background())
	fabricWaiterCount(t, p, 2)
	close(release)
	a, b := fabricTake(t, first), fabricTake(t, second)
	if a.err != nil || b.err != nil {
		t.Fatalf("coalesced reads: %v %v", a.err, b.err)
	}
	av, _ := a.generation.View()
	bv, _ := b.generation.View()
	if calls.Load() != 1 || av.ID != bv.ID || p.sequence != 1 {
		t.Fatalf("duplicate acquisition/history: calls=%d ids=%s/%s sequence=%d", calls.Load(), av.ID, bv.ID, p.sequence)
	}
	av.Board.Tasks.Rows[0].Subject = "caller-only"
	again, _ := b.generation.View()
	if again.Board.Tasks.Rows[0].Subject == "caller-only" {
		t.Fatal("coalesced consumers alias mutable views")
	}
	next, err := p.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	nv, _ := next.View()
	if calls.Load() != 2 || nv.ID == av.ID || p.sequence != 2 {
		t.Fatal("completed generation was reused as a cache")
	}
}

func TestFabricFirstCancellationKeepsSurvivingDemand(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	defer fabricRelease(release)
	var acquisition context.Context
	p := NewFabricGenerationProducerContext(context.Background(), func(ctx context.Context) (FabricRead, error) {
		acquisition = ctx
		close(entered)
		select {
		case <-release:
			return fabricStatusRead("pending"), nil
		case <-ctx.Done():
			return FabricRead{}, ctx.Err()
		}
	}, "fixture")
	defer p.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	first := fabricAsyncRead(p, ctx)
	fabricSignal(t, entered)
	second := fabricAsyncRead(p, context.Background())
	fabricWaiterCount(t, p, 2)
	cancel()
	if v := fabricTake(t, first); !errors.Is(v.err, context.Canceled) {
		t.Fatalf("first waiter: %v", v.err)
	}
	if acquisition.Err() != nil {
		t.Fatal("first waiter cancelled a surviving acquisition")
	}
	close(release)
	if v := fabricTake(t, second); v.err != nil {
		t.Fatal(v.err)
	}
	if p.sequence != 1 {
		t.Fatal("surviving acquisition did not commit once")
	}
}

func TestFabricAbandonmentCancelsAndJoinsBeforeReplacement(t *testing.T) {
	entered, cancelled, cleanup := make(chan struct{}), make(chan struct{}), make(chan struct{})
	defer fabricRelease(cleanup)
	var calls atomic.Int32
	p := NewFabricGenerationProducerContext(context.Background(), func(ctx context.Context) (FabricRead, error) {
		if calls.Add(1) == 1 {
			close(entered)
			<-ctx.Done()
			close(cancelled)
			<-cleanup
			return fabricStatusRead("done"), nil
		}
		return fabricStatusRead("pending"), nil
	}, "fixture")
	defer p.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	first := fabricAsyncRead(p, ctx)
	fabricSignal(t, entered)
	cancel()
	if v := fabricTake(t, first); !errors.Is(v.err, context.Canceled) {
		t.Fatal(v.err)
	}
	fabricSignal(t, cancelled)
	queued := &fabricObservedContext{Context: context.Background(), observed: make(chan struct{})}
	next := fabricAsyncRead(p, queued)
	fabricSignal(t, queued.observed)
	p.mu.Lock()
	held := p.active != nil && p.active.abandoned
	sequence := p.sequence
	p.mu.Unlock()
	if !held || calls.Load() != 1 || sequence != 0 {
		t.Fatal("abandoned worker released slot/history before actual cleanup")
	}
	close(cleanup)
	v := fabricTake(t, next)
	if v.err != nil {
		t.Fatal(v.err)
	}
	view, _ := v.generation.View()
	if calls.Load() != 2 || p.sequence != 1 || view.Board.ActivitySeeded || len(view.Events) != 0 || view.Completed != 0 {
		t.Fatal("abandoned completion polluted first baseline")
	}
}

func TestFabricCancelledQueuedSelectionAndRejectedPageNeverCommit(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	defer fabricRelease(release)
	var calls atomic.Int32
	p := NewFabricGenerationProducerContext(context.Background(), func(ctx context.Context) (FabricRead, error) {
		call := calls.Add(1)
		if call == 2 {
			close(entered)
			select {
			case <-release:
			case <-ctx.Done():
				return FabricRead{}, ctx.Err()
			}
		}
		status := "pending"
		if call == 3 {
			status = "done"
		}
		if call >= 4 {
			status = "blocked"
		}
		return fabricStatusRead(status), nil
	}, "fixture")
	defer p.Close()
	if _, err := p.Read(context.Background()); err != nil {
		t.Fatal(err)
	}
	active := fabricAsyncRead(p, context.Background())
	fabricSignal(t, entered)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	queued := &fabricObservedContext{Context: ctx, observed: make(chan struct{})}
	page := make(chan fabricReadResult, 1)
	go func() { g, e := p.ReadTaskSelection(queued, 0, "", "blocked"); page <- fabricReadResult{g, e} }()
	fabricSignal(t, queued.observed)
	cancel()
	if v := fabricTake(t, page); !errors.Is(v.err, context.Canceled) {
		t.Fatal(v.err)
	}
	if calls.Load() != 2 {
		t.Fatal("cancelled queued selection invoked acquisition callback")
	}
	close(release)
	if v := fabricTake(t, active); v.err != nil {
		t.Fatal(v.err)
	}
	if _, err := p.ReadPage(context.Background(), 64, "stale"); !errors.Is(err, ErrFabricPageRejected) {
		t.Fatalf("rejected page: %v", err)
	}
	if p.sequence != 2 {
		t.Fatal("page rejection advanced history")
	}
	g, err := p.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	v, _ := g.View()
	if len(v.Events) != 1 || v.Events[0].From != "pending" || v.Events[0].To != "blocked" || v.Completed != 0 {
		t.Fatal("rejected done page swallowed or committed a transition", v.Events)
	}
}

func TestFabricFirstFailureAndShutdownDoNotSeedHistory(t *testing.T) {
	calls := 0
	p := NewFabricGenerationProducerContext(context.Background(), func(context.Context) (FabricRead, error) {
		calls++
		if calls == 1 {
			return FabricRead{}, errors.New("inert first failure")
		}
		return fabricStatusRead("pending"), nil
	}, "fixture")
	defer p.Close()
	if _, err := p.Read(context.Background()); err == nil || p.sequence != 0 || p.seeded {
		t.Fatal("failed first read seeded history")
	}
	g, err := p.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	v, _ := g.View()
	if p.sequence != 1 || v.Board.ActivitySeeded || len(v.Events) != 0 {
		t.Fatal("first success fabricated history")
	}
	p.Close()
	if _, err := p.Read(context.Background()); !errors.Is(err, context.Canceled) {
		t.Fatalf("closed producer accepted work: %v", err)
	}
}

func TestFabricShutdownReturnsWaiterButRetainsWorkerUntilExit(t *testing.T) {
	entered, cancelled, cleanup := make(chan struct{}), make(chan struct{}), make(chan struct{})
	defer fabricRelease(cleanup)
	p := NewFabricGenerationProducerContext(context.Background(), func(ctx context.Context) (FabricRead, error) {
		if _, ok := ctx.Deadline(); !ok {
			return FabricRead{}, errors.New("acquisition lacks its own ceiling")
		}
		close(entered)
		<-ctx.Done()
		close(cancelled)
		<-cleanup
		return fabricStatusRead("done"), nil
	}, "fixture")
	defer p.Close()
	result := fabricAsyncRead(p, context.Background())
	fabricSignal(t, entered)
	p.Close()
	fabricSignal(t, cancelled)
	if got := fabricTake(t, result); !errors.Is(got.err, context.Canceled) {
		t.Fatalf("shutdown failed to release waiter: %v", got.err)
	}
	p.mu.Lock()
	active := p.active
	seq := p.sequence
	p.mu.Unlock()
	if active == nil || seq != 0 {
		t.Fatal("shutdown pretended worker cleanup/history completion")
	}
	close(cleanup)
	fabricSignal(t, active.done)
	if p.sequence != 0 || p.seeded {
		t.Fatal("cancelled worker committed after shutdown")
	}
}
