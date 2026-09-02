package routerstore

// ADR-062 §2 (rs-09): the HTTP client is itself a Store. dispatch.Facade and
// every verb above it cannot tell whether the ledger is a local file or the
// router service; Resolve() hands back this type when SIRSI_ROUTER_URL is set.
//
// Wire contract (rs-08 serve.go is the other half):
//   POST {base}/v1/call/{Method}
//   Authorization: Bearer <token>
//   body:   {"args":[...]}            positional, JSON-encoded Go values
//   200:    {"result":[...]}          every non-error return, in order
//   200:    {"error":{"name":"ErrNoWork","message":"..."}}  sentinel by NAME
//   4xx/5xx: {"error":{"message":"..."}}                  non-sentinel
// Sentinels round-trip by name so errors.Is(err, ErrNoWork) holds on the
// client exactly as it does against a local store.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// sentinelErrors is the closed set that crosses the wire by name. Adding a
// sentinel to the package without adding it here makes it arrive as a plain
// error (still an error, just not errors.Is-able) — TestSentinelsRoundTrip
// pins the list.
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
}

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
	base   string
	token  string
	client *http.Client
	// perCall bounds every request (ADR-062 §2: default 5 s; Wait sets its own).
	perCall time.Duration
}

var _ Store = (*RemoteStore)(nil)

// NewRemoteStore builds a client for base (scheme://host[:port]) with a bearer
// token. Like OpenPath/OpenPostgres it is for Resolve() and tests only.
func NewRemoteStore(base, token string) *RemoteStore {
	return &RemoteStore{
		base:    strings.TrimRight(base, "/"),
		token:   token,
		client:  &http.Client{Timeout: 0}, // per-request contexts bound the calls
		perCall: 5 * time.Second,
	}
}

// call performs one RPC. outs receive the non-error results in order.
func (rs *RemoteStore) call(method string, args []any, outs ...any) error {
	ctx, cancel := context.WithTimeout(context.Background(), rs.perCall)
	defer cancel()
	return rs.callCtx(ctx, method, args, outs...)
}

func (rs *RemoteStore) callCtx(ctx context.Context, method string, args []any, outs ...any) error {
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
	if rs.token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+rs.token)
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
		return fmt.Errorf("routerstore: remote %s: %s", method, wr.Error.Message)
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

// Wait is a long poll: the server blocks up to timeout on the store's own
// Wait and answers true when work landed. The client context is bounded to
// timeout plus slack so a hung server cannot hold a lane forever.
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
// long-polls Wait and pulses the channel. Closing ctx stops it.
func (rs *RemoteStore) ListenNotify(ctx context.Context, agent string) (<-chan struct{}, error) {
	ch := make(chan struct{}, 1)
	go func() {
		defer close(ch)
		for {
			if ctx.Err() != nil {
				return
			}
			woke, err := rs.Wait(ctx, agent, 25*time.Second)
			if err != nil || !woke {
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
