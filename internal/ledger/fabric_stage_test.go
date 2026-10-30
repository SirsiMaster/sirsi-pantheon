package ledger

import (
	"context"
	"errors"
	"testing"
	"time"
)

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
