package routerstore

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fabricRoundTrip func(*http.Request) (*http.Response, error)

func (f fabricRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func fabricRemoteClient(transport http.RoundTripper) *RemoteStore {
	return &RemoteStore{base: "https://fixture.invalid", token: "fixture-token", client: &http.Client{Transport: transport}, perCall: time.Second, host: "fixture-host", agent: "fixture-agent", runtime: strings.Repeat("a", 64), now: time.Now}
}
func fabricRemoteBarrier(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal("remote fixture barrier not reached")
	}
}

type fabricMintContext struct {
	context.Context
	observed chan struct{}
	once     sync.Once
}

func (c *fabricMintContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.observed) })
	return c.Context.Done()
}

func TestFabricMintWaiterCancellationDoesNotWaitForNetworkMutex(t *testing.T) {
	entered, cancelObserved := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	var enterOnce, cancelOnce sync.Once
	rs := fabricRemoteClient(fabricRoundTrip(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		enterOnce.Do(func() { close(entered) })
		<-r.Context().Done()
		cancelOnce.Do(func() { close(cancelObserved) })
		return nil, r.Context().Err()
	}))
	rs.perCall = 10 * time.Second // fixture holds a fake mint; own one-second barriers stay finite
	firstCtx, cancelFirst := context.WithCancel(context.Background())
	defer cancelFirst()
	first := make(chan error, 1)
	go func() { _, e := rs.ensureSession(firstCtx); first <- e }()
	fabricRemoteBarrier(t, entered)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	waiting := &fabricMintContext{Context: ctx, observed: make(chan struct{})}
	second := make(chan error, 1)
	go func() { _, e := rs.ensureSession(waiting); second <- e }()
	select {
	case <-waiting.observed:
	case <-time.After(time.Second):
		cancel()
		t.Fatal("mint admission blocked on a non-cancellable session mutex")
	}
	cancel()
	select {
	case err := <-second:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled mint waiter waited for another caller's network mint")
	}
	if calls.Load() != 1 {
		t.Fatal("mint waiter started a second network mint")
	}
	cancelFirst()
	fabricRemoteBarrier(t, cancelObserved)
	select {
	case err := <-first:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("active fake mint did not cancel")
	}
}

func TestFabricContextualReadsPreserveWireAndCancelFakeTransport(t *testing.T) {
	for _, tc := range []struct {
		name string
		args string
		call func(context.Context, *RemoteStore) error
	}{
		{"ListTasks", `["fixture-agent"]`, func(c context.Context, s *RemoteStore) error {
			_, e := s.ListTasksContext(c, "fixture-agent")
			return e
		}},
		{"OperationalAgents", `[]`, func(c context.Context, s *RemoteStore) error { _, e := s.OperationalAgentsContext(c); return e }},
		{"ClassifyLane", `["fixture-agent",true,300000000000]`, func(c context.Context, s *RemoteStore) error {
			_, e := s.ClassifyLaneContext(c, "fixture-agent", true, 5*time.Minute)
			return e
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entered, observed := make(chan struct{}), make(chan struct{})
			rs := fabricRemoteClient(fabricRoundTrip(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path != "/v1/call/"+tc.name {
					t.Errorf("wire method changed: %s", r.URL.Path)
				}
				if r.Header.Get("X-Sirsi-Session") != "fixture-session" || r.Header.Get("X-Sirsi-Signature") == "" || r.Header.Get("Authorization") != "Bearer fixture-token" {
					t.Error("contextual read bypassed existing authenticated wire")
				}
				raw, _ := io.ReadAll(r.Body)
				var body struct {
					Args json.RawMessage `json:"args"`
				}
				if e := json.Unmarshal(raw, &body); e != nil || string(body.Args) != tc.args {
					t.Errorf("wire arguments changed: %s %v", raw, e)
				}
				close(entered)
				<-r.Context().Done()
				close(observed)
				return nil, r.Context().Err()
			}))
			rs.session = Session{ID: "fixture-session", Secret: strings.Repeat("b", 64)}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			result := make(chan error, 1)
			go func() { result <- tc.call(ctx, rs) }()
			fabricRemoteBarrier(t, entered)
			cancel()
			fabricRemoteBarrier(t, observed)
			select {
			case e := <-result:
				if !errors.Is(e, context.Canceled) {
					t.Fatal(e)
				}
			case <-time.After(time.Second):
				t.Fatal("contextual read outlived cancelled transport")
			}
		})
	}
}

// Inspect admitted deadlines in fake transport, without waiting out any budget.
func TestFabricDeadlineCapPreservesWaitLongPollBudget(t *testing.T) {
	for _, tc := range []struct {
		name        string
		method      string
		budget      time.Duration
		callerBound bool
		call        func(context.Context, *RemoteStore) error
	}{
		{"default Wait", "Wait", 70 * time.Second, false, func(c context.Context, s *RemoteStore) error { _, e := s.Wait(c, "fixture-agent", 0); return e }},
		{"explicit Wait", "Wait", 100 * time.Second, false, func(c context.Context, s *RemoteStore) error {
			_, e := s.Wait(c, "fixture-agent", 90*time.Second)
			return e
		}},
		{"caller bounded Wait", "Wait", 20 * time.Second, true, func(c context.Context, s *RemoteStore) error { _, e := s.Wait(c, "fixture-agent", 0); return e }},
		{"Fabric tasks", "ListTasks", 35 * time.Second, false, func(c context.Context, s *RemoteStore) error {
			_, e := s.ListTasksContext(c, "fixture-agent")
			return e
		}},
		{"Fabric agents", "OperationalAgents", 35 * time.Second, false, func(c context.Context, s *RemoteStore) error { _, e := s.OperationalAgentsContext(c); return e }},
		{"Fabric classification", "ClassifyLane", 35 * time.Second, false, func(c context.Context, s *RemoteStore) error {
			_, e := s.ClassifyLaneContext(c, "fixture-agent", true, 5*time.Minute)
			return e
		}},
		{"caller bounded Fabric", "ListTasks", 20 * time.Second, true, func(c context.Context, s *RemoteStore) error {
			_, e := s.ListTasksContext(c, "fixture-agent")
			return e
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stop := errors.New("inert deadline observation complete")
			var deadline time.Time
			var called bool
			rs := fabricRemoteClient(fabricRoundTrip(func(r *http.Request) (*http.Response, error) {
				called = true
				if r.URL.Path != "/v1/call/"+tc.method {
					t.Errorf("unexpected method: %s", r.URL.Path)
				}
				var ok bool
				deadline, ok = r.Context().Deadline()
				if !ok {
					t.Error("RPC has no admitted deadline")
				}
				return nil, stop
			}))
			rs.perCall = 35 * time.Second
			// A warm session isolates the signed long-poll budget from the
			// existing separately bounded session-mint RPC.
			rs.session = Session{ID: "fixture-session", Secret: strings.Repeat("b", 64)}
			ctx := context.Background()
			var callerDeadline time.Time
			if tc.callerBound {
				callerDeadline = time.Now().Add(tc.budget)
				var cancel context.CancelFunc
				ctx, cancel = context.WithDeadline(ctx, callerDeadline)
				defer cancel()
			}
			before := time.Now()
			err := tc.call(ctx, rs)
			after := time.Now()
			if !called || !errors.Is(err, stop) {
				t.Fatalf("fake transport not reached: called=%v err=%v", called, err)
			}
			if tc.callerBound {
				if !deadline.Equal(callerDeadline) {
					t.Fatalf("caller deadline changed: got %v want %v", deadline, callerDeadline)
				}
			} else if deadline.Before(before.Add(tc.budget)) || deadline.After(after.Add(tc.budget)) {
				t.Fatalf("%s admitted deadline %v, want call bracket [%v, %v] plus %v", tc.name, deadline, before, after, tc.budget)
			}
		})
	}
}
