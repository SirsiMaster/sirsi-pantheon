package ledger

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func newFinalizerFixture(read func(context.Context) (FabricRead, error)) (*FabricGenerationProducer, *fabricOperation, context.CancelFunc) {
	p := NewFabricGenerationProducerContext(context.Background(), read, "fixture")
	ctx, cancel := context.WithCancel(context.Background())
	trace := &fabricStageTrace{}
	ctx = context.WithValue(ctx, fabricStageTraceKey{}, trace)
	waiter := &fabricWaiter{ctx: context.Background()}
	op := &fabricOperation{ctx: ctx, cancel: cancel, done: make(chan struct{}), waiters: map[*fabricWaiter]struct{}{waiter: {}}, trace: trace, unfiltered: true}
	p.previous = map[string]string{"lane\x00task-000": "in-progress"}
	p.events = []FabricTransition{{At: "2026-10-25T12:00:00Z", Agent: "lane", TaskID: "prior", From: "pending", To: "blocked"}}
	p.completed, p.sequence, p.seeded, p.instance = 7, 11, true, "prior-instance"
	p.active = op
	return p, op, cancel
}

func assertFinalizerFailurePreservesBaseline(t *testing.T, p *FabricGenerationProducer, op *fabricOperation, wantStage FabricStage, wantCauses ...error) {
	t.Helper()
	if op.generation != nil || op.err == nil {
		t.Fatalf("failed finalization published a generation: generation=%v err=%v", op.generation, op.err)
	}
	var attributed *FabricStageError
	if !errors.As(op.err, &attributed) || attributed.Stage != wantStage {
		t.Fatalf("finalized error lost stage %q: %v", wantStage, op.err)
	}
	for _, cause := range wantCauses {
		if !errors.Is(op.err, cause) {
			t.Fatalf("finalized error lost cause %v: %v", cause, op.err)
		}
	}
	if !strings.HasPrefix(op.err.Error(), "fabric cancellation observed during stage "+string(wantStage)+" after ") {
		t.Fatalf("finalized public error is not fixed-stage text: %v", op.err)
	}
	if strings.Contains(op.err.Error(), "private-sentinel-payload") {
		t.Fatalf("finalized public error exposed source payload: %v", op.err)
	}
	if p.sequence != 11 || !p.seeded || p.completed != 7 || p.instance != "prior-instance" ||
		!reflect.DeepEqual(p.previous, map[string]string{"lane\x00task-000": "in-progress"}) ||
		!reflect.DeepEqual(p.events, []FabricTransition{{At: "2026-10-25T12:00:00Z", Agent: "lane", TaskID: "prior", From: "pending", To: "blocked"}}) {
		t.Fatalf("failed finalization changed prior history: sequence=%d seeded=%v completed=%d previous=%v events=%v", p.sequence, p.seeded, p.completed, p.previous, p.events)
	}
}

func TestFabricFinalizerAttributesBareCancellationAfterProjection(t *testing.T) {
	p, op, cancel := newFinalizerFixture(func(context.Context) (FabricRead, error) {
		return fabricStatusRead("done"), nil
	})
	defer p.Close()
	p.runFabricOperation(op, func(FabricRead) error {
		cancel()
		return nil
	})
	assertFinalizerFailurePreservesBaseline(t, p, op, FabricStageProjection, context.Canceled)
}

func TestFabricFinalizerPreservesContextAndSentinelRace(t *testing.T) {
	sentinel := errors.New("private-sentinel-payload")
	var cancel context.CancelFunc
	p, op, stop := newFinalizerFixture(func(context.Context) (FabricRead, error) {
		cancel()
		return FabricRead{}, sentinel
	})
	cancel = stop
	defer p.Close()
	p.runFabricOperation(op, nil)
	assertFinalizerFailurePreservesBaseline(t, p, op, FabricStageSnapshot, context.Canceled, sentinel)
}

func TestFabricFinalizerRetainsExistingStageError(t *testing.T) {
	sentinel := errors.New("private-sentinel-payload")
	var cancel context.CancelFunc
	want := &FabricStageError{Stage: FabricStageNodeStatus, Elapsed: 17 * time.Millisecond, Cause: sentinel}
	p, op, stop := newFinalizerFixture(func(context.Context) (FabricRead, error) {
		cancel()
		return FabricRead{}, want
	})
	cancel = stop
	defer p.Close()
	p.runFabricOperation(op, nil)
	assertFinalizerFailurePreservesBaseline(t, p, op, FabricStageNodeStatus, context.Canceled, sentinel)
	var got *FabricStageError
	if !errors.As(op.err, &got) || got.Elapsed != want.Elapsed {
		t.Fatalf("finalizer replaced the existing stage timing: got=%+v want=%+v", got, want)
	}
}

func TestFabricReadStageDeadlineAttribution(t *testing.T) {
	for _, stage := range []FabricStage{
		FabricStageSnapshot,
		FabricStageNodeStatus,
		FabricStageOperationalAgents,
		FabricStageRuntimeClassification,
		FabricStageHostHealth,
		FabricStageProjection,
	} {
		t.Run(string(stage), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
			defer cancel()
			err := FabricReadStage(ctx, stage, func() error {
				<-ctx.Done()
				return ctx.Err()
			})
			var attributed *FabricStageError
			if !errors.Is(err, context.DeadlineExceeded) || !errors.As(err, &attributed) {
				t.Fatalf("deadline cause or stage attribution lost: %v", err)
			}
			if attributed.Stage != stage || attributed.Elapsed <= 0 {
				t.Fatalf("deadline attribution = %+v", attributed)
			}
		})
	}
}

func TestFabricProducerDeadlineRacePreservesObservedStage(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	started := make(chan struct{})
	p := NewFabricGenerationProducerContext(context.Background(), func(readCtx context.Context) (FabricRead, error) {
		err := FabricReadStage(readCtx, FabricStageRuntimeClassification, func() error {
			close(started)
			<-readCtx.Done()
			return readCtx.Err()
		})
		return FabricRead{}, err
	}, "fixture")
	defer p.Close()
	result := make(chan error, 1)
	go func() { _, err := p.Read(ctx); result <- err }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("reader did not enter the injected stage")
	}
	select {
	case err := <-result:
		var attributed *FabricStageError
		if !errors.Is(err, context.DeadlineExceeded) || !errors.As(err, &attributed) || attributed.Stage != FabricStageRuntimeClassification {
			t.Fatalf("waiter deadline lost the active stage: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("deadline did not release the waiter")
	}
	p.mu.Lock()
	active := p.active
	p.mu.Unlock()
	if active != nil {
		<-active.done
	}
	if p.sequence != 0 || p.seeded {
		t.Fatal("late cancelled acquisition committed generation history")
	}
}

func TestFabricCoalescedWaiterDeadlineKeepsSurvivingReadFresh(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	defer fabricRelease(release)
	var calls int
	p := NewFabricGenerationProducerContext(context.Background(), func(ctx context.Context) (FabricRead, error) {
		calls++
		if calls == 1 {
			err := FabricReadStage(ctx, FabricStageRuntimeClassification, func() error {
				close(entered)
				select {
				case <-release:
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			})
			return fabricStatusRead("pending"), err
		}
		return fabricStatusRead("pending"), nil
	}, "fixture")
	defer p.Close()

	waiterCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	first := fabricAsyncRead(p, waiterCtx)
	fabricSignal(t, entered)
	second := fabricAsyncRead(p, context.Background())
	fabricWaiterCount(t, p, 2)
	cancel()
	firstResult := fabricTake(t, first)
	var attributed *FabricStageError
	if !errors.Is(firstResult.err, context.Canceled) || !errors.As(firstResult.err, &attributed) || attributed.Stage != FabricStageRuntimeClassification {
		t.Fatalf("coalesced waiter lost cancellation stage: %v", firstResult.err)
	}
	close(release)
	secondResult := fabricTake(t, second)
	if secondResult.err != nil || p.sequence != 1 {
		t.Fatalf("surviving reader did not commit exactly once: err=%v sequence=%d", secondResult.err, p.sequence)
	}
	next, err := p.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	firstView, _ := secondResult.generation.View()
	nextView, _ := next.View()
	if calls != 2 || firstView.ID == nextView.ID || p.sequence != 2 {
		t.Fatal("coalesced completion was cached or did not preserve fresh reads")
	}
}
