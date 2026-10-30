package ledger

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

func TestClassifyFabricAgentsBoundedAndComplete(t *testing.T) {
	agents := make([]Agent, 9)
	unavailable := map[string]bool{"agent-2": true, "agent-5": false}
	for i := range agents {
		agents[i].AgentID = fmt.Sprintf("agent-%d", i)
	}
	var active, maximum atomic.Int32
	var mu sync.Mutex
	calls := make(map[string]int, len(agents))
	started := make(chan struct{}, fabricClassificationWorkers)
	release := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	classify := func(_ context.Context, id string, wake bool, _ time.Duration) (routerstore.LaneOperationalState, error) {
		current := active.Add(1)
		for old := maximum.Load(); current > old && !maximum.CompareAndSwap(old, current); old = maximum.Load() {
		}
		started <- struct{}{}
		<-release
		active.Add(-1)
		mu.Lock()
		calls[id]++
		mu.Unlock()
		return routerstore.LaneOperationalState{Agent: id, Classification: routerstore.LaneComplete, WakeRoutable: wake}, nil
	}
	result := make(chan struct {
		states map[string]routerstore.LaneOperationalState
		err    error
	}, 1)
	go func() {
		states, err := classifyFabricAgents(context.Background(), agents, unavailable, classify)
		result <- struct {
			states map[string]routerstore.LaneOperationalState
			err    error
		}{states, err}
	}()
	for range fabricClassificationWorkers {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("classification did not start four workers")
		}
	}
	releaseOnce.Do(func() { close(release) })
	var got struct {
		states map[string]routerstore.LaneOperationalState
		err    error
	}
	select {
	case got = <-result:
	case <-time.After(time.Second):
		t.Fatal("classification did not finish after release")
	}
	if got.err != nil || len(got.states) != len(agents) {
		t.Fatalf("classification result = %v, %d states", got.err, len(got.states))
	}
	if maximum.Load() > fabricClassificationWorkers || maximum.Load() != fabricClassificationWorkers {
		t.Fatalf("maximum concurrent classifiers = %d", maximum.Load())
	}
	want := make(map[string]routerstore.LaneOperationalState, len(agents))
	for _, agent := range agents {
		blocked, declared := unavailable[agent.AgentID]
		wake := declared && !blocked
		want[agent.AgentID] = routerstore.LaneOperationalState{Agent: agent.AgentID, Classification: routerstore.LaneComplete, WakeRoutable: wake}
		if calls[agent.AgentID] != 1 {
			t.Errorf("classifier calls for %s = %d, want exactly one", agent.AgentID, calls[agent.AgentID])
		}
	}
	if !reflect.DeepEqual(got.states, want) {
		t.Fatalf("classification parity mismatch:\n got: %#v\nwant: %#v", got.states, want)
	}
}

func TestClassifyFabricAgentsReturnsFirstInputErrorWithoutPartialMap(t *testing.T) {
	agents := []Agent{{AgentID: "first"}, {AgentID: "second"}, {AgentID: "third"}}
	firstErr := errors.New("first source failure")
	secondErr := errors.New("second source failure")
	var calls atomic.Int32
	got, err := classifyFabricAgents(context.Background(), agents, nil, func(_ context.Context, id string, _ bool, _ time.Duration) (routerstore.LaneOperationalState, error) {
		calls.Add(1)
		if id == "first" {
			return routerstore.LaneOperationalState{}, firstErr
		}
		if id == "second" {
			return routerstore.LaneOperationalState{}, secondErr
		}
		return routerstore.LaneOperationalState{Agent: id}, nil
	})
	if got != nil || !errors.Is(err, firstErr) || errors.Is(err, secondErr) || calls.Load() != int32(len(agents)) {
		t.Fatalf("got states=%v err=%v calls=%d; want nil, first input error, all joined", got, err, calls.Load())
	}
}

func TestClassifyFabricAgentsCancellationJoinsLateClassifier(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	agents := []Agent{{AgentID: "agent-0"}, {AgentID: "agent-1"}}
	sourceErr := errors.New("late source failure")
	started := make(chan struct{}, len(agents))
	release := make(chan struct{})
	returned := make(chan struct {
		states map[string]routerstore.LaneOperationalState
		err    error
	}, 1)
	go func() {
		states, err := classifyFabricAgents(ctx, agents, nil, func(_ context.Context, id string, _ bool, _ time.Duration) (routerstore.LaneOperationalState, error) {
			started <- struct{}{}
			<-release // Deliberately emulate a late, non-contextual source call.
			return routerstore.LaneOperationalState{Agent: id}, sourceErr
		})
		returned <- struct {
			states map[string]routerstore.LaneOperationalState
			err    error
		}{states, err}
	}()
	for range len(agents) {
		select {
		case <-started:
		case <-time.After(time.Second):
			close(release)
			t.Fatal("expected classifiers did not start")
		}
	}
	cancel()
	select {
	case <-returned:
		close(release)
		t.Fatal("helper returned while late classifiers were still running")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	select {
	case got := <-returned:
		if got.states != nil || !errors.Is(got.err, context.Canceled) || !errors.Is(got.err, sourceErr) {
			t.Fatalf("canceled result = states %v, err %v", got.states, got.err)
		}
	case <-time.After(time.Second):
		t.Fatal("helper did not return after late classifiers joined")
	}
}

func TestClassifyFabricAgentsStopsSchedulingAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	agents := make([]Agent, 8)
	for i := range agents {
		agents[i].AgentID = fmt.Sprintf("agent-%d", i)
	}
	var calls atomic.Int32
	started := make(chan struct{}, len(agents))
	result := make(chan error, 1)
	go func() {
		_, err := classifyFabricAgents(ctx, agents, nil, func(callCtx context.Context, id string, _ bool, _ time.Duration) (routerstore.LaneOperationalState, error) {
			calls.Add(1)
			started <- struct{}{}
			<-callCtx.Done()
			return routerstore.LaneOperationalState{}, callCtx.Err()
		})
		result <- err
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("classifier did not start")
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancellation error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancellation did not join classifiers")
	}
	if calls.Load() > fabricClassificationWorkers {
		t.Fatalf("started %d classifiers after cancellation; limit %d", calls.Load(), fabricClassificationWorkers)
	}
}
