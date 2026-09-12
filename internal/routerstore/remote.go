package routerstore

// ADR-062 §2/§3 (rs-09, rs-10): the HTTP client is itself a Store, and it
// signs every request with a service-minted session key.
//
// Wire contract (serve.go is the other half):
//   POST {base}/v1/call/{Method}
//   Authorization:     Bearer <host token>
//   X-Sirsi-Session:   <session id>              (all methods but MintSession)
//   X-Sirsi-Nonce:     <unixms>.<random hex>     single use, ±60 s
//   X-Sirsi-Runtime:   <sha256 of this executable>
//   X-Sirsi-Signature: hex(HMAC-SHA256(secret, method \n nonce \n body))
//   body:   {"args":[...]}            positional, JSON-encoded Go values
//   200:    {"result":[...]}          every non-error return, in order
//   200:    {"error":{"name":"ErrNoWork","message":"..."}}  sentinel by NAME
//   4xx/5xx: {"error":{"message":"..."}}                  non-sentinel
//
// The session (id + secret) is minted once per (host, agent, runtime) and
// cached at ~/.sirsi/sessions/<agent>.json (0600). A runtime change (new
// binary) yields a different hash, so the cached session is dropped and a new
// one minted — the service revokes the old one on first mismatch anyway.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// sentinelErrors is the closed set that crosses the wire by name.
// TestSentinelsRoundTrip pins the list.
var sentinelErrors = map[string]error{
	"ErrNotComplete":          ErrNotComplete,
	"ErrBreakerOpen":          ErrBreakerOpen,
	"ErrOverQuota":            ErrOverQuota,
	"ErrIdentifierTaken":      ErrIdentifierTaken,
	"ErrNoWork":               ErrNoWork,
	"ErrNoClaimableTask":      ErrNoClaimableTask,
	"ErrLeaseInvalid":         ErrLeaseInvalid,
	"ErrTerminal":             ErrTerminal,
	"ErrBudgetExceeded":       ErrBudgetExceeded,
	"ErrReasonRequired":       ErrReasonRequired,
	"ErrIncompleteEvidence":   ErrIncompleteEvidence,
	"ErrNotFound":             ErrNotFound,
	"ErrAlreadyClosed":        ErrAlreadyClosed,
	"ErrConcurrentTaskUpdate": ErrConcurrentTaskUpdate,
	"ErrTaskExists":           ErrTaskExists,
	"ErrSessionUnknown":       ErrSessionUnknown,
	"ErrSessionRevoked":       ErrSessionRevoked,
	"ErrNotOwner":             ErrNotOwner,
	"ErrTokenUnknown":         ErrTokenUnknown,
	"ErrTokenRevoked":         ErrTokenRevoked,
	"ErrHostMismatch":         ErrHostMismatch,
	"ErrServiceUnavailable":   ErrServiceUnavailable,
	"ErrUnregistered":         ErrUnregistered,
	"ErrThreadUnknown":        ErrThreadUnknown,
	"ErrThreadAuthority":      ErrThreadAuthority,
}

// ErrServiceUnavailable is a transient service/database failure (HTTP 503):
// the caller should back off and retry; nothing about its credentials changed.
var ErrServiceUnavailable = errors.New("routerstore: router service unavailable (retry)")

func sentinelName(err error) string {
	for name, s := range sentinelErrors {
		if errors.Is(err, s) {
			return name
		}
	}
	return ""
}

type wireError struct {
	Name    string `json:"name,omitempty"`
	Message string `json:"message"`
}

type wireRequest struct {
	Args []json.RawMessage `json:"args"`
}

type wireResponse struct {
	Result []json.RawMessage `json:"result,omitempty"`
	Error  *wireError        `json:"error,omitempty"`
}

// RemoteStore is the Store implementation that speaks to `sirsi router serve`.
type RemoteStore struct {
	base    string
	token   string
	client  *http.Client
	perCall time.Duration

	host, agent, runtime string
	threadID             string // SIRSI_THREAD_ID: the registered thread this process works for (Rule of Ra)
	sessionDir           string // "" disables the on-disk cache (tests)

	mu      sync.Mutex
	session Session // ID + Secret once minted
	// now is injectable for the freshness tests.
	now func() time.Time
}

var _ Store = (*RemoteStore)(nil)

// NewRemoteStore builds a client for base with a host bearer token. The agent
// id comes from SIRSI_AGENT_ID (falls back to the hostname); the runtime hash
// is this executable's SHA-256. Like OpenPath/OpenPostgres it is for Resolve()
// and tests only.
// IdentityHook, when set, supplies the (agent, threadID) for a new store whenever
// SIRSI_AGENT_ID / SIRSI_THREAD_ID are absent from the environment. The CLI
// installs one that reads this session's markers so an interactive call carries
// its registered thread (the Rule of Ra, ADR-062 20b.2). It is nil in library
// and test use (env-only), and it must NEVER call os.Setenv — process-wide
// identity leaks into spawned children (PR #730). Each field falls back
// independently: the hook fills only what the environment left empty.
var IdentityHook func() (agent, threadID string)

func NewRemoteStore(base, token string) *RemoteStore {
	host, _ := os.Hostname()
	agent := strings.TrimSpace(os.Getenv("SIRSI_AGENT_ID"))
	threadID := strings.TrimSpace(os.Getenv("SIRSI_THREAD_ID"))
	if (agent == "" || threadID == "") && IdentityHook != nil {
		hookAgent, hookThread := IdentityHook()
		hookAgent, hookThread = strings.TrimSpace(hookAgent), strings.TrimSpace(hookThread)
		if agent == "" {
			agent = hookAgent
		}
		// Adopt the marker's thread only when it belongs to the agent we are
		// about to bind as. A thread is meaningful only paired with its own
		// agent, so an env-supplied foreign SIRSI_AGENT_ID must never inherit a
		// marker thread that names a different agent (SSA 2026-09-10, PR #731):
		// that would mint an incoherent (agentA, threadOfAgentB) audience. When
		// they disagree, the foreign agent stays threadless until it supplies
		// its own SIRSI_THREAD_ID.
		if threadID == "" && hookThread != "" && hookAgent != "" && hookAgent == agent {
			threadID = hookThread
		}
	}
	if agent == "" {
		agent = host
	}
	dir := ""
	if home, err := os.UserHomeDir(); err == nil {
		dir = filepath.Join(home, ".sirsi", "sessions")
	}
	client := &http.Client{}
	if d := SpoolDir(base); d != "" {
		// spool:// — the lane has no network (ADR-062 20a.1b). Requests travel
		// as files through the relay, which holds the host token; this side
		// carries none and signs exactly as over HTTP.
		client.Transport = newSpoolTransport(d, agent)
		base, token = "http://spool", ""
	}
	return &RemoteStore{
		base:       strings.TrimRight(base, "/"),
		token:      token,
		client:     client,
		perCall:    35 * time.Second, // CLIENT-side call budget. Must exceed the 30s spool wait, or it cancels a spool round-trip before the relay answers; the old 5s also canceled a warm ~3-4s full-ledger ListAll outright (SSA 2026-09-11, confirmed by a client negative control). ListAll's own QueryContext is now server-bound too (rs-26, serve.go's per-request timeout), so this ceiling and the server's deadline both apply. A LOCAL SQLiteStore's ExportMarkdown, by contrast, has no server injecting a deadline — its context.Background() bounds nothing.
		host:       host,
		agent:      agent,
		threadID:   threadID,
		runtime:    RuntimeHash(),
		sessionDir: dir,
		now:        func() time.Time { return time.Now().UTC() },
	}
}

var (
	runtimeOnce sync.Once
	runtimeHash string
)

// RuntimeHash is the SHA-256 of the running executable — the "runtime"
// identity claim bound into a session at mint (ADR-062 §3). Computed once.
func RuntimeHash() string {
	runtimeOnce.Do(func() {
		runtimeHash = "unknown"
		exe, err := os.Executable()
		if err != nil {
			return
		}
		f, err := os.Open(exe)
		if err != nil {
			return
		}
		defer func() { _ = f.Close() }()
		h := sha256.New()
		if _, err := io.Copy(h, f); err != nil {
			return
		}
		runtimeHash = hex.EncodeToString(h.Sum(nil))
	})
	return runtimeHash
}

// ensureSession returns the cached session or mints one, bounding the mint
// RPC by ctx (SSA review, PR #746) so a caller whose deadline already passed
// (or is about to) fails fast on the mint instead of racing an unrelated
// independent timeout. The cache lookup itself is in-memory/local-disk and not
// worth threading ctx through. rs.mu still serializes concurrent callers with
// a plain sync.Mutex — that acquisition is not itself ctx-cancellable, but the
// critical section it guards is now bounded by ctx, which is what SSA's
// reproduction actually exercised (a pre-canceled mint call running to its own
// full timeout, not lock contention). The on-disk cache is keyed by agent and
// validated against the current runtime hash.
func (rs *RemoteStore) ensureSession(ctx context.Context) (Session, error) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	if rs.session.ID != "" {
		return rs.session, nil
	}
	if rs.sessionDir != "" {
		if b, err := os.ReadFile(rs.sessionPath()); err == nil {
			var cached Session
			if json.Unmarshal(b, &cached) == nil && cached.ID != "" && cached.Secret != "" && cached.RuntimeHash == rs.runtime && cached.ThreadID == rs.threadID {
				rs.session = cached
				return cached, nil
			}
		}
	}
	var minted Session
	mintMethod, mintArgs := "MintSession", []any{rs.host, rs.agent, rs.runtime}
	if rs.threadID != "" {
		mintMethod, mintArgs = "MintSessionForThread", []any{rs.host, rs.agent, rs.runtime, rs.threadID}
	}
	if err := rs.callUnsignedCtx(ctx, mintMethod, mintArgs, &minted); err != nil {
		return Session{}, fmt.Errorf("routerstore: mint session: %w", err)
	}
	rs.session = minted
	if rs.sessionDir != "" {
		if err := os.MkdirAll(rs.sessionDir, 0o700); err == nil {
			b, _ := json.Marshal(minted)
			_ = os.WriteFile(rs.sessionPath(), b, 0o600)
		}
	}
	return minted, nil
}

func (rs *RemoteStore) sessionPath() string {
	return filepath.Join(rs.sessionDir, strings.ReplaceAll(rs.agent, "/", "_")+".json")
}

// dropSession forgets the session so the next call mints a fresh one
// (used when the service reports it unknown or revoked).
func (rs *RemoteStore) dropSession() {
	rs.mu.Lock()
	rs.session = Session{}
	if rs.sessionDir != "" {
		_ = os.Remove(rs.sessionPath())
	}
	rs.mu.Unlock()
}

func (rs *RemoteStore) nonce() string {
	r, _ := randomHex(8)
	return strconv.FormatInt(rs.now().UnixMilli(), 10) + "." + r
}

// call performs one signed RPC. outs receive the non-error results in order.
func (rs *RemoteStore) call(method string, args []any, outs ...any) error {
	ctx, cancel := context.WithTimeout(context.Background(), rs.perCall)
	defer cancel()
	return rs.callCtx(ctx, method, args, outs...)
}

func (rs *RemoteStore) callCtx(ctx context.Context, method string, args []any, outs ...any) error {
	sess, err := rs.ensureSession(ctx)
	if err != nil {
		return err
	}
	err = rs.do(ctx, method, args, &sess, outs...)
	if errors.Is(err, ErrSessionUnknown) || errors.Is(err, ErrSessionRevoked) {
		// One re-mint, then surface the error: a revoked runtime keeps failing.
		rs.dropSession()
		if sess2, e := rs.ensureSession(ctx); e == nil {
			return rs.do(ctx, method, args, &sess2, outs...)
		}
	}
	return err
}

// callUnsigned is MintSession's path: host token only, no caller ctx (used
// directly by the context-free MintSessionForThread interface method — the
// Store interface predates ctx-taking methods). It gets its own perCall
// budget, same as call().
func (rs *RemoteStore) callUnsigned(method string, args []any, outs ...any) error {
	ctx, cancel := context.WithTimeout(context.Background(), rs.perCall)
	defer cancel()
	return rs.callUnsignedCtx(ctx, method, args, outs...)
}

// callUnsignedCtx is callUnsigned's ctx-aware twin, used by ensureSession so a
// session mint triggered by a ctx-taking call (ListAll) actually races the
// caller's deadline instead of an independent one (SSA review, PR #746: a
// pre-canceled ListAll still ran MintSession to completion on its own timeout
// because ensureSession/callUnsigned built context.Background() internally).
// rs.perCall is still applied as a ceiling, same reasoning as ListAll.
func (rs *RemoteStore) callUnsignedCtx(ctx context.Context, method string, args []any, outs ...any) error {
	ctx, cancel := context.WithTimeout(ctx, rs.perCall)
	defer cancel()
	return rs.do(ctx, method, args, nil, outs...)
}

func (rs *RemoteStore) do(ctx context.Context, method string, args []any, sess *Session, outs ...any) error {
	req := wireRequest{Args: make([]json.RawMessage, 0, len(args))}
	for _, a := range args {
		b, err := json.Marshal(a)
		if err != nil {
			return fmt.Errorf("routerstore: remote %s: encode arg: %w", method, err)
		}
		req.Args = append(req.Args, b)
	}
	body, _ := json.Marshal(req)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, rs.base+"/v1/call/"+method, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("routerstore: remote %s: %w", method, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+rs.token)
	if sess != nil {
		n := rs.nonce()
		httpReq.Header.Set("X-Sirsi-Session", sess.ID)
		httpReq.Header.Set("X-Sirsi-Nonce", n)
		httpReq.Header.Set("X-Sirsi-Runtime", rs.runtime)
		httpReq.Header.Set("X-Sirsi-Signature", Sign(sess.Secret, method, n, body))
	}
	resp, err := rs.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("routerstore: remote %s: %w", method, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return fmt.Errorf("routerstore: remote %s: read: %w", method, err)
	}
	var wr wireResponse
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &wr); err != nil {
			return fmt.Errorf("routerstore: remote %s: HTTP %d, non-JSON body: %.200s", method, resp.StatusCode, raw)
		}
	}
	if wr.Error != nil {
		if s, ok := sentinelErrors[wr.Error.Name]; ok {
			return s
		}
		if resp.StatusCode == http.StatusServiceUnavailable {
			return fmt.Errorf("%w: %s", ErrServiceUnavailable, wr.Error.Message)
		}
		// Session errors arrive as 401 text; classify them so callCtx can re-mint.
		msg := wr.Error.Message
		switch {
		case strings.Contains(msg, ErrSessionUnknown.Error()):
			return ErrSessionUnknown
		case strings.Contains(msg, ErrSessionRevoked.Error()):
			return ErrSessionRevoked
		}
		return fmt.Errorf("routerstore: remote %s: HTTP %d: %s", method, resp.StatusCode, msg)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("routerstore: remote %s: HTTP %d", method, resp.StatusCode)
	}
	if len(wr.Result) != len(outs) {
		return fmt.Errorf("routerstore: remote %s: expected %d results, got %d", method, len(outs), len(wr.Result))
	}
	for i, out := range outs {
		if err := json.Unmarshal(wr.Result[i], out); err != nil {
			return fmt.Errorf("routerstore: remote %s: decode result %d: %w", method, i, err)
		}
	}
	return nil
}

// ── methods that cannot be generated ───────────────────────────────────────

// ListAll takes a context, so it is hand-written rather than generated (the
// stub generator only emits context-free methods). The caller's ctx is honored,
// but rs.perCall is always applied as a ceiling so the client-side per-call
// budget the generated call() gave this method is preserved (WithTimeout takes
// the earlier of the two deadlines). The server injects its own deadline on the
// read regardless (serve.go), which is the true server-side bound (rs-26).
func (rs *RemoteStore) ListAll(ctx context.Context) ([]Item, error) {
	ctx, cancel := context.WithTimeout(ctx, rs.perCall)
	defer cancel()
	var out []Item
	err := rs.callCtx(ctx, "ListAll", nil, &out)
	return out, err
}

// Wait is a long poll: the server blocks up to timeout on the store's own
// Wait and answers true when work landed.
func (rs *RemoteStore) Wait(ctx context.Context, agent string, timeout time.Duration) (bool, error) {
	if timeout <= 0 {
		timeout = time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout+10*time.Second)
	defer cancel()
	var woke bool
	err := rs.callCtx(ctx, "Wait", []any{agent, timeout}, &woke)
	return woke, err
}

// ListenNotify has no cross-process FIFO over HTTP; it is a goroutine that
// long-polls Wait and pulses the channel, backing off when the service is
// unreachable. Closing ctx stops it.
func (rs *RemoteStore) ListenNotify(ctx context.Context, agent string) (<-chan struct{}, error) {
	ch := make(chan struct{}, 1)
	go func() {
		defer close(ch)
		for {
			if ctx.Err() != nil {
				return
			}
			woke, err := rs.Wait(ctx, agent, 25*time.Second)
			if err != nil {
				select {
				case <-ctx.Done():
					return
				case <-time.After(2 * time.Second):
				}
				continue
			}
			if !woke {
				continue
			}
			select {
			case ch <- struct{}{}:
			default:
			}
		}
	}()
	return ch, nil
}

// NotifyAgent asks the service to wake in-process waiters for agent.
func (rs *RemoteStore) NotifyAgent(agent string) { _ = rs.call("NotifyAgent", []any{agent}) }

// NotifyPath names a local FIFO; a remote store has none.
func (rs *RemoteStore) NotifyPath(string) string { return "" }

// Close releases idle connections; the service owns the database handle.
func (rs *RemoteStore) Close() error {
	rs.client.CloseIdleConnections()
	return nil
}

// MintSessionForThread over the wire (host token only, like MintSession).
func (rs *RemoteStore) MintSessionForThread(host, agent, runtimeHash, threadID string) (Session, error) {
	var out Session
	err := rs.callUnsigned("MintSessionForThread", []any{host, agent, runtimeHash, threadID}, &out)
	return out, err
}

// ThreadBinding over the wire.
func (rs *RemoteStore) ThreadBinding(threadID string) (ThreadBinding, error) {
	var out ThreadBinding
	err := rs.call("ThreadBinding", []any{threadID}, &out)
	return out, err
}

// RecordAudience is server-side only; a lane never writes the audit log itself.
func (rs *RemoteStore) RecordAudience(AudienceEntry) error {
	return errors.New("routerstore: RecordAudience is not served over the wire")
}

// AudienceSince over the wire (read-only, exempt from the gate).
func (rs *RemoteStore) AudienceSince(since string) (AudienceReport, error) {
	var out AudienceReport
	err := rs.call("AudienceSince", []any{since}, &out)
	return out, err
}
