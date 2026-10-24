package routermcp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"sync"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/machineid"
	"github.com/SirsiMaster/sirsi-pantheon/internal/mcp"
	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

// Server owns one connection's registration, claims and heartbeat. There is no
// shared mutable registration or library signal handler. Post-registration tools
// and lifecycle bind the returned owned thread through the existing RPC policy.
type Server struct {
	protocol    *mcp.Server
	reg         *registration
	agent       string
	repo        string
	ownedThread string
	deps        threadOps
	runMu       sync.Mutex
	started     bool
}

// Instance-owned seams keep lifecycle tests entirely off the live registry.
type threadOps struct {
	identity  func() (host, machine string)
	register  func(string, *router.Thread) (*router.Thread, error)
	heartbeat func(string, string, router.HeartbeatUpdate) (*router.Thread, error)
	close     func(string, string) (*router.Thread, error)
	ticker    func() (<-chan time.Time, func())
	bind      func(string, string) (func(string, string, router.HeartbeatUpdate) (*router.Thread, error), func(string, string) (*router.Thread, error))
}

func defaultThreadOps() threadOps {
	return threadOps{
		identity: func() (string, string) { host, _ := routerstore.ThreadOwnerHost(); return host, machineid.MachineID() },
		register: router.RegisterThread, heartbeat: router.Heartbeat, close: router.CloseThread,
		bind: router.CallerThreadLifecycle,
		ticker: func() (<-chan time.Time, func()) {
			t := time.NewTicker(heartbeatInterval)
			return t.C, t.Stop
		},
	}
}

// Run is the single implementation used by sirsi router-mcp and the legacy
// launcher. It owns cancellation of in: closing it must unblock Read. Diagnostics
// use stderr; out is reserved for the unchanged MCP JSON-RPC framing.
func Run(ctx context.Context, in io.ReadCloser, out io.Writer) error {
	return New().Run(ctx, in, out)
}

// Run serves one connection until EOF/cancellation, joining its heartbeat before
// closing the registered thread. Existing synchronous tool calls finish through
// their existing store bounds; cancellation is not an atomic in-flight abort.
func (s *Server) Run(ctx context.Context, in io.ReadCloser, out io.Writer) (err error) {
	if ctx == nil || in == nil || out == nil {
		return fmt.Errorf("router MCP needs a context, closable input and output")
	}
	s.runMu.Lock()
	if s.started {
		s.runMu.Unlock()
		return fmt.Errorf("router MCP instance already served a connection")
	}
	s.started = true
	s.runMu.Unlock()
	var inputErr error
	closeInput := sync.OnceFunc(func() { inputErr = in.Close() })
	defer func() { closeInput(); err = errors.Join(err, inputErr) }()
	if ctx.Err() != nil {
		return nil
	}
	shutdown := s.registerSurface(log.New(os.Stderr, "[sirsi-router-mcp] ", log.LstdFlags))
	defer func() { err = errors.Join(err, shutdown()) }()
	stop, joined := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(joined)
		select {
		case <-ctx.Done():
			closeInput()
		case <-stop:
		}
	}()
	defer func() { close(stop); <-joined }()
	return s.protocol.RunWithIOContext(ctx, in, out)
}

func (s *Server) registerSurface(logger *log.Logger) func() error {
	noop := func() error { return nil }
	if s.agent == "" {
		s.reg.setErr(fmt.Errorf("SIRSI_AGENT_ID is unset — mutate tools need a registered identity (one agent per instance, ADR-068)"))
		return noop
	}
	repoRoot, routerRoot, err := s.resolveRoots()
	if err != nil {
		s.reg.setErr(err)
		return noop
	}
	host, machine := s.deps.identity()
	t, err := s.deps.register(routerRoot, &router.Thread{
		ThreadID: s.ownedThread, AgentID: s.agent, Surface: "mcp", PID: os.Getpid(), Host: host,
		MachineID: machine, Repo: repoRoot, WakeMechanism: "none",
	})
	if err != nil {
		s.reg.setErr(fmt.Errorf("register surface=mcp thread: %w", err))
		return noop
	}
	if t == nil || t.ThreadID != s.ownedThread || t.AgentID != s.agent {
		s.reg.setErr(fmt.Errorf("register surface=mcp did not return the owned agent/thread; registration outcome needs reconciliation"))
		return noop
	}
	heartbeat, closeThread := s.deps.heartbeat, s.deps.close
	if s.deps.bind != nil {
		heartbeat, closeThread = s.deps.bind(s.agent, t.ThreadID)
	}
	s.reg.set(t.ThreadID, s.agent)
	logger.Printf("registered surface=mcp thread %s for %s", t.ThreadID, s.agent)
	stop, joined := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(joined)
		ticks, stopTicker := s.deps.ticker()
		defer stopTicker()
		for {
			select {
			case <-stop:
				return
			case <-ticks:
				if _, err := heartbeat(routerRoot, t.ThreadID, router.HeartbeatUpdate{Status: router.ThreadStatusActive}); err != nil {
					logger.Printf("heartbeat: %v", err)
				}
			}
		}
	}()
	var once sync.Once
	var closeErr error
	return func() error {
		once.Do(func() {
			close(stop)
			<-joined
			_, closeErr = closeThread(routerRoot, t.ThreadID)
			s.reg.mu.Lock()
			s.reg.threadID, s.reg.agent = "", ""
			s.reg.claimed = map[string]claimRecord{}
			s.reg.err = fmt.Errorf("router MCP connection closed")
			s.reg.mu.Unlock()
		})
		return closeErr
	}
}
