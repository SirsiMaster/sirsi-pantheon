// P2 (ADR-068 §5.2): the resident surface="mcp" thread (A27) + the MUTATE tools
// gated behind it. Read tools (main.go) need no registration; a developer can
// plug in and read the fabric with no identity. To ACT, the server registers a
// thread on startup (one agent per instance, ADR-068), heartbeats from its
// runloop on a bounded interval, and closes the thread on graceful shutdown.
//
// The mutate tools add NO authority: each is a thin translator over a facade
// verb that already enforces its own gate (recipient-only ack, actor-checked
// close, idempotent send). router_claim/router_close are additionally
// session-ownership bound HERE — this instance may only close work IT claimed.
package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/machineid"
	"github.com/SirsiMaster/sirsi-pantheon/internal/mcp"
	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

// heartbeatInterval is bounded and ≥60s (A27) — inside the 10-min Rule-of-Ra
// stale window, but never a frequent tick that floods the registry.
const heartbeatInterval = 90 * time.Second

// claimTTL bounds a claimed lease; a client that claims and then dies releases
// the item for another consumer after this.
const claimTTL = 15 * time.Minute

// claimRecord is one local instance's record of a claim: the durable lease
// token it was issued and that lease's own expiry (never a locally-invented
// TTL — it mirrors ClaimNext's Lease.Expires exactly).
type claimRecord struct {
	token   string
	expires time.Time
}

// registration is the resident surface="mcp" thread this server holds. Set once
// at startup, then read by the heartbeat loop and the mutate gate. Guarded by a
// mutex because the heartbeat goroutine reads it concurrently with startup
// (A21: an injectable/shared field crossing goroutines takes a lock).
type registration struct {
	mu       sync.RWMutex
	threadID string
	agent    string
	claimed  map[string]claimRecord // item ids THIS instance claimed (session-ownership) → lease token + expiry
	err      error                  // why registration failed; surfaced by mutate tools
}

func newRegistration() *registration { return &registration{claimed: map[string]claimRecord{}} }

var reg = newRegistration()

func (r *registration) set(threadID, agent string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.threadID, r.agent, r.err = threadID, agent, nil
}

func (r *registration) setErr(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.err = err
}

// gate returns (agent, nil) when this server holds a registered thread, else an
// actionable error. A mutate before registration is a clear error, never a
// silent drop (ADR-068 §3).
func (r *registration) gate() (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.threadID == "" {
		if r.err != nil {
			return "", fmt.Errorf("this MCP server holds no registered thread, so it cannot act: %w", r.err)
		}
		return "", fmt.Errorf("this MCP server holds no registered thread, so it cannot act (set SIRSI_AGENT_ID and restart)")
	}
	return r.agent, nil
}

func (r *registration) recordClaim(itemID, token string, expires time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.claimed[itemID] = claimRecord{token: token, expires: expires}
}

// ownsClaim reports whether this instance still holds a non-evicted LOCAL
// record of claiming itemID. An entry past its own recorded expiry is
// evicted here unconditionally (point 3 of the accepted proposal, router item
// 20261001-031502): the local clock already knows, independent of whatever a
// round trip to VerifyLease would additionally confirm. This is a necessary
// local check, not a sufficient liveness one — router_close separately calls
// VerifyLease against the durable store before acting, because a clean local
// clock cannot see that another instance already reclaimed the item.
func (r *registration) ownsClaim(itemID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	rec, ok := r.claimed[itemID]
	if !ok {
		return false
	}
	if time.Now().After(rec.expires) {
		delete(r.claimed, itemID)
		return false
	}
	return true
}

// claimToken returns the lease token this instance recorded for itemID, so
// router_close can pass it to Store.VerifyLease. Shares ownsClaim's eviction
// so a token is never returned for an entry that just expired.
func (r *registration) claimToken(itemID string) (string, bool) {
	if !r.ownsClaim(itemID) {
		return "", false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.claimed[itemID].token, true
}

// registerSurface registers this process as a surface="mcp" resident thread and
// starts the heartbeat loop. It NEVER crashes the server: reads must keep
// working even if registration fails (the failure is recorded and surfaced by
// mutate tools). Returns a shutdown func that closes the thread (A27).
func registerSurface(logger *log.Logger) func() {
	agent := strings.TrimSpace(os.Getenv("SIRSI_AGENT_ID"))
	if agent == "" {
		reg.setErr(fmt.Errorf("SIRSI_AGENT_ID is unset — mutate tools need a registered identity (one agent per instance, ADR-068)"))
		return func() {}
	}
	repoRoot, routerRoot, err := resolveRoots()
	if err != nil {
		reg.setErr(err)
		return func() {}
	}
	// Host + MachineID are the ADR-067 identity the service authority check reads:
	// a session may only register a thread on its OWN host, so the record must
	// carry this host's identity (empty is refused). On an adopted host these
	// resolve to the same identity the relay's token authenticates as.
	host, _ := os.Hostname()
	t, err := router.RegisterThread(routerRoot, &router.Thread{
		AgentID:       agent,
		Surface:       "mcp",
		PID:           os.Getpid(),
		Host:          host,
		MachineID:     machineid.MachineID(),
		Repo:          repoRoot,
		WakeMechanism: "none", // a stdio server is woken by its client, not the router
	})
	if err != nil {
		reg.setErr(fmt.Errorf("register surface=mcp thread: %w", err))
		return func() {}
	}
	reg.set(t.ThreadID, agent)
	logger.Printf("registered surface=mcp thread %s for %s", t.ThreadID, agent)

	stop := make(chan struct{})
	go heartbeatLoop(routerRoot, t.ThreadID, logger, stop)

	return func() {
		close(stop)
		if _, err := router.CloseThread(routerRoot, t.ThreadID); err != nil {
			logger.Printf("close thread %s: %v", t.ThreadID, err)
		}
	}
}

func heartbeatLoop(routerRoot, threadID string, logger *log.Logger, stop <-chan struct{}) {
	tk := time.NewTicker(heartbeatInterval)
	defer tk.Stop()
	for {
		select {
		case <-stop:
			return
		case <-tk.C:
			if _, err := router.Heartbeat(routerRoot, threadID, router.HeartbeatUpdate{Status: router.ThreadStatusActive}); err != nil {
				logger.Printf("heartbeat: %v", err)
			}
		}
	}
}

// installSignalClose wires SIGINT/SIGTERM to the shutdown func so the thread is
// closed on graceful exit (A27); a hard kill falls back to OS-truth reaping.
func installSignalClose(shutdown func(), logger *log.Logger) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-ch
		logger.Println("shutting down — closing surface=mcp thread")
		shutdown()
		os.Exit(0)
	}()
}

func registerMutateTools(srv *mcp.Server) {
	srv.RegisterTool(mcp.Tool{
		Name: "router_send",
		Description: "Send a router item to another agent. Idempotent (a rephrased duplicate returns the existing id). " +
			"Requires this server's registered thread.",
		InputSchema: mcp.InputSchema{
			Type: "object",
			Properties: map[string]mcp.SchemaField{
				"to":           {Type: "string", Description: "Recipient agent id."},
				"title":        {Type: "string", Description: "Short subject line."},
				"instructions": {Type: "string", Description: "The message body."},
				"type":         {Type: "string", Description: "Optional item type: proposal | review | decision (default: plain item)."},
			},
			Required: []string{"to", "title", "instructions"},
		},
	}, handleSend)

	srv.RegisterTool(mcp.Tool{
		Name:        "router_acknowledge",
		Description: "Acknowledge (mark read) an item addressed to you. Recipient-only; does not close it. Requires this server's registered thread.",
		InputSchema: mcp.InputSchema{
			Type:       "object",
			Properties: map[string]mcp.SchemaField{"id": {Type: "string", Description: "Item id to acknowledge."}},
			Required:   []string{"id"},
		},
	}, handleAcknowledge)

	srv.RegisterTool(mcp.Tool{
		Name:        "router_claim",
		Description: "Claim the oldest open item addressed to you for exclusive work (a lease). Requires this server's registered thread.",
		InputSchema: mcp.InputSchema{Type: "object", Properties: map[string]mcp.SchemaField{}},
	}, handleClaim)

	srv.RegisterTool(mcp.Tool{
		Name:        "router_close",
		Description: "Close an item you CLAIMED via router_claim in this session, recording a result. Requires this server's registered thread.",
		InputSchema: mcp.InputSchema{
			Type: "object",
			Properties: map[string]mcp.SchemaField{
				"id":     {Type: "string", Description: "Item id to close (must be one this session claimed)."},
				"result": {Type: "string", Description: "The outcome/result recorded on close."},
			},
			Required: []string{"id"},
		},
	}, handleClose)
}

func handleSend(args map[string]interface{}) (*mcp.ToolResult, error) {
	agent, err := reg.gate()
	if err != nil {
		return errResult(err.Error()), nil
	}
	to := strings.TrimSpace(stringArg(args, "to"))
	title := strings.TrimSpace(stringArg(args, "title"))
	body := stringArg(args, "instructions")
	if to == "" || title == "" || strings.TrimSpace(body) == "" {
		return errResult("router_send: to, title, and instructions are all required"), nil
	}
	f, closeF, err := openFacade()
	if err != nil {
		return errResult(err.Error()), nil
	}
	defer closeF()
	res, err := f.Send(agent, to, title, stringArg(args, "type"), body)
	if err != nil {
		return errResult(fmt.Sprintf("send: %v", err)), nil
	}
	msg := fmt.Sprintf("Sent %s → %s: %s", agent, to, res.ID)
	if res.Deduped {
		msg = fmt.Sprintf("Idempotent duplicate — existing item returned: %s", res.ID)
	}
	return textResult(msg), nil
}

func handleAcknowledge(args map[string]interface{}) (*mcp.ToolResult, error) {
	agent, err := reg.gate()
	if err != nil {
		return errResult(err.Error()), nil
	}
	id := strings.TrimSpace(stringArg(args, "id"))
	if id == "" {
		return errResult("router_acknowledge: id is required"), nil
	}
	f, closeF, err := openFacade()
	if err != nil {
		return errResult(err.Error()), nil
	}
	defer closeF()
	if err := f.AckItem(agent, id); err != nil {
		return errResult(fmt.Sprintf("acknowledge: %v", err)), nil
	}
	return textResult(fmt.Sprintf("Acknowledged %s", id)), nil
}

func handleClaim(_ map[string]interface{}) (*mcp.ToolResult, error) {
	agent, err := reg.gate()
	if err != nil {
		return errResult(err.Error()), nil
	}
	f, closeF, err := openFacade()
	if err != nil {
		return errResult(err.Error()), nil
	}
	defer closeF()
	lease, err := f.Store().ClaimNext(agent, claimTTL)
	if err != nil {
		return errResult(fmt.Sprintf("claim: %v", err)), nil
	}
	if lease == nil {
		return textResult("No open item to claim. Inbox is clear."), nil
	}
	reg.recordClaim(lease.ItemID, lease.Token, lease.Expires)
	return textResult(fmt.Sprintf("Claimed %s (lease expires %s). Work it, then router_close it with a result.",
		lease.ItemID, lease.Expires.Format(time.RFC3339))), nil
}

func handleClose(args map[string]interface{}) (*mcp.ToolResult, error) {
	agent, err := reg.gate()
	if err != nil {
		return errResult(err.Error()), nil
	}
	id := strings.TrimSpace(stringArg(args, "id"))
	if id == "" {
		return errResult("router_close: id is required"), nil
	}
	// Session-ownership (ADR-068): this instance may only close work it claimed.
	token, ok := reg.claimToken(id)
	if !ok {
		return errResult(fmt.Sprintf("router_close: %s was not claimed by this session — claim it with router_claim first (a developer can only complete work its own instance claimed)", id)), nil
	}
	f, closeF, err := openFacade()
	if err != nil {
		return errResult(err.Error()), nil
	}
	defer closeF()
	// Complete against the DURABLE lease (router item 20261001-031502 Ra
	// ACCEPTED, corrected 20261001-131036 codex-pantheon CHANGES REQUIRED
	// PR944): the local claimToken above only proves this instance ONCE held
	// the claim and its own record hasn't locally expired — it cannot see
	// that the lease was reclaimed by another instance after expiry, and a
	// separate VerifyLease-then-CloseItem pair leaves a TOCTOU window where a
	// reclaim lands between the two calls AND CloseItem itself guards on
	// status='open', which a claimed item never is. CompleteItem fences
	// token+expiry+status in the SAME atomic UPDATE as the mutation. An error
	// here (including an old deployed service that predates Complete) is
	// treated as "cannot verify" and refuses — never falls back to an
	// unfenced close.
	if err := f.CompleteItem(agent, id, token, stringArg(args, "result")); err != nil {
		if strings.Contains(err.Error(), "no such store method") {
			return errResult(fmt.Sprintf("router_close: %s — the router service predates lease-fenced completion; refusing rather than closing unfenced", id)), nil
		}
		if errors.Is(err, routerstore.ErrLeaseInvalid) {
			return errResult(fmt.Sprintf("router_close: %s — your claim expired or moved to another instance; re-claim with router_claim before closing", id)), nil
		}
		return errResult(fmt.Sprintf("close: %v", err)), nil
	}
	return textResult(fmt.Sprintf("Closed %s", id)), nil
}
