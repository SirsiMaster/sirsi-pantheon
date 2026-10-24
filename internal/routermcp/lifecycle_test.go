package routermcp

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
)

func inertServer(t *testing.T, id string) (*Server, *atomic.Int32, *atomic.Int32, *atomic.Int32) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	s := newServer("owned-fixture", t.TempDir())
	s.ownedThread = id
	registered, closed, beats := new(atomic.Int32), new(atomic.Int32), new(atomic.Int32)
	s.deps = threadOps{
		identity: func() (string, string) { return "owned-host", "owned-machine" },
		register: func(_ string, tr *router.Thread) (*router.Thread, error) {
			registered.Add(1)
			if tr.AgentID != "owned-fixture" || tr.Surface != "mcp" || tr.WakeMechanism != "none" || tr.Host != "owned-host" || tr.MachineID != "owned-machine" {
				t.Errorf("registration changed: %+v", tr)
			}
			if tr.ThreadID != id {
				t.Error("registration did not pin the instance's thread")
			}
			return &router.Thread{ThreadID: id, AgentID: "owned-fixture"}, nil
		},
		heartbeat: func(_ string, thread string, _ router.HeartbeatUpdate) (*router.Thread, error) {
			beats.Add(1)
			if thread != id {
				t.Errorf("foreign heartbeat: %s", thread)
			}
			return nil, nil
		},
		close: func(_ string, thread string) (*router.Thread, error) {
			closed.Add(1)
			if thread != id {
				t.Errorf("foreign close: %s", thread)
			}
			return nil, nil
		},
		ticker: func() (<-chan time.Time, func()) { return nil, func() {} },
	}
	return s, registered, closed, beats
}

type countedInput struct {
	io.ReadCloser
	closes atomic.Int32
}

func (r *countedInput) Close() error { r.closes.Add(1); return r.ReadCloser.Close() }

func TestServerEOFCleansRegistrationAndInput(t *testing.T) {
	s, registered, closed, _ := inertServer(t, "owned-eof")
	in := &countedInput{ReadCloser: io.NopCloser(strings.NewReader(""))}
	if err := s.Run(context.Background(), in, io.Discard); err != nil {
		t.Fatal(err)
	}
	if registered.Load() != 1 || closed.Load() != 1 || in.closes.Load() != 1 {
		t.Fatalf("register/close/input = %d/%d/%d", registered.Load(), closed.Load(), in.closes.Load())
	}
	if _, err := s.reg.gate(); err == nil {
		t.Fatal("closed connection still authorized")
	}
	if err := s.Run(context.Background(), in, io.Discard); err == nil {
		t.Fatal("instance reused")
	}
	if closed.Load() != 1 {
		t.Fatal("repeated run closed twice")
	}
}

func TestServerRefusesMismatchedRegistrationResult(t *testing.T) {
	for _, which := range []string{"thread", "agent"} {
		t.Run(which, func(t *testing.T) {
			s, _, closed, _ := inertServer(t, "owned-requested-thread")
			s.deps.register = func(_ string, tr *router.Thread) (*router.Thread, error) {
				copy := *tr
				if which == "thread" {
					copy.ThreadID = "foreign-returned-thread"
				} else {
					copy.AgentID = "foreign-returned-agent"
				}
				return &copy, nil
			}
			if err := s.Run(context.Background(), io.NopCloser(strings.NewReader("")), io.Discard); err != nil {
				t.Fatal(err)
			}
			if _, err := s.reg.gate(); err == nil {
				t.Fatal("mismatched return enabled mutation")
			}
			if closed.Load() != 0 {
				t.Fatal("closed an unowned returned registration")
			}
		})
	}
}

func TestServerCancellationJoinsAndClosesOnce(t *testing.T) {
	s, _, closed, _ := inertServer(t, "owned-cancel")
	ready := make(chan struct{})
	original := s.deps.register
	s.deps.register = func(root string, tr *router.Thread) (*router.Thread, error) {
		out, err := original(root, tr)
		close(ready)
		return out, err
	}
	reader, writer := io.Pipe()
	t.Cleanup(func() { _ = writer.Close() })
	in := &countedInput{ReadCloser: reader}
	ctx, cancel := context.WithCancel(context.Background())
	var runErr error
	exited := make(chan struct{})
	go func() { defer close(exited); runErr = s.Run(ctx, in, io.Discard) }()
	t.Cleanup(func() {
		cancel()
		_ = reader.Close()
		select {
		case <-exited:
		case <-time.After(2 * time.Second):
			t.Error("server did not join")
		}
	})
	select {
	case <-ready:
	case <-time.After(2 * time.Second):
		t.Fatal("no registration")
	}
	cancel()
	select {
	case <-exited:
		if runErr != nil {
			t.Fatal(runErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancellation did not unblock decoder")
	}
	if closed.Load() != 1 || in.closes.Load() != 1 {
		t.Fatal("cancellation cleanup not exact once")
	}
}

func TestServerFailedRegistrationKeepsReadsAndRefusesMutation(t *testing.T) {
	s, _, closed, _ := inertServer(t, "unused")
	s.deps.register = func(string, *router.Thread) (*router.Thread, error) {
		return nil, errors.New("owned registration refused")
	}
	input := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}` + "\n"
	var out bytes.Buffer
	if err := s.Run(context.Background(), io.NopCloser(strings.NewReader(input)), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "router_inbox") || closed.Load() != 0 {
		t.Fatal("failed registration broke reads or closed unowned thread")
	}
	result, err := s.handleClaim(nil)
	if err != nil || result == nil || !result.IsError || !strings.Contains(result.Content[0].Text, "owned registration refused") {
		t.Fatalf("mutate did not fail closed: %+v %v", result, err)
	}
}

func TestTwoInstancesHaveIndependentClaimsAndLifecycle(t *testing.T) {
	a, _, closeA, _ := inertServer(t, "thread-a")
	b, _, closeB, _ := inertServer(t, "thread-b")
	shutdownA := a.registerSurface(log.New(io.Discard, "", 0))
	shutdownB := b.registerSurface(log.New(io.Discard, "", 0))
	t.Cleanup(func() { _ = shutdownA(); _ = shutdownB() })
	a.reg.recordClaim("item-a", "token-a", time.Now().Add(time.Minute))
	b.reg.recordClaim("item-b", "token-b", time.Now().Add(time.Minute))
	if b.reg.ownsClaim("item-a") || a.reg.ownsClaim("item-b") {
		t.Fatal("claim leaked between instances")
	}
	result, err := b.handleClose(map[string]interface{}{"id": "item-a", "ack": true, "result": "owned coordination"})
	if err != nil || result == nil || !result.IsError || !strings.Contains(result.Content[0].Text, "not claimed by this session") {
		t.Fatalf("foreign instance reached close: %+v %v", result, err)
	}
	if err := shutdownA(); err != nil {
		t.Fatal(err)
	}
	if err := shutdownA(); err != nil {
		t.Fatal(err)
	}
	if closeA.Load() != 1 || closeB.Load() != 0 || !b.reg.ownsClaim("item-b") {
		t.Fatal("shutdown crossed instances")
	}
	if _, err := b.reg.gate(); err != nil {
		t.Fatal("sibling registration lost", err)
	}
	if err := shutdownB(); err != nil {
		t.Fatal(err)
	}
	if closeB.Load() != 1 {
		t.Fatal("sibling cleanup missing")
	}
}

func TestServerEOFSurfacesCloseFailure(t *testing.T) {
	s, _, _, _ := inertServer(t, "owned-close-error")
	want := errors.New("owned thread close refused")
	s.deps.close = func(string, string) (*router.Thread, error) { return nil, want }
	if err := s.Run(context.Background(), io.NopCloser(strings.NewReader("")), io.Discard); !errors.Is(err, want) {
		t.Fatalf("cleanup failure hidden: %v", err)
	}
	if _, err := s.reg.gate(); err == nil {
		t.Fatal("failed cleanup left local mutation gate open")
	}
}

func TestHeartbeatJoinedBeforeThreadClose(t *testing.T) {
	s, _, closed, beats := inertServer(t, "owned-heartbeat")
	ticks, entered, release := make(chan time.Time, 1), make(chan struct{}), make(chan struct{})
	s.deps.ticker = func() (<-chan time.Time, func()) { return ticks, func() {} }
	s.deps.heartbeat = func(string, string, router.HeartbeatUpdate) (*router.Thread, error) {
		beats.Add(1)
		close(entered)
		<-release
		return nil, nil
	}
	shutdown := s.registerSurface(log.New(io.Discard, "", 0))
	defer func() { _ = shutdown() }() // release/join below precedes idempotent fallback
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	ticks <- time.Now()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("heartbeat not called")
	}
	done := make(chan error, 1)
	go func() { done <- shutdown() }()
	select {
	case err := <-done:
		t.Fatalf("shutdown returned during blocked heartbeat: %v", err)
	case <-time.After(25 * time.Millisecond):
	}
	if closed.Load() != 0 {
		t.Fatal("closed during active heartbeat")
	}
	close(release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("shutdown not joined")
	}
	if beats.Load() != 1 || closed.Load() != 1 {
		t.Fatal("heartbeat/close counts")
	}
}
