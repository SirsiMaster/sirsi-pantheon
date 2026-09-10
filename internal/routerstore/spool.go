package routerstore

// spool.go — the filesystem relay for lanes that have no network (ADR-062 step
// 20a.1b). A codex workspace-write sandbox can neither open a socket nor resolve
// DNS, but it can write files under a declared writable root. So a lane talks to
// the router service through a per-host spool directory:
//
//   <spool>/<agent>/req/<id>.json   written by the lane (tmp + rename: atomic)
//   <spool>/<agent>/res/<id>.json   written by the relay (tmp + rename)
//
// The lane side is an http.RoundTripper, so RemoteStore's signing path (session,
// nonce, runtime, HMAC) is byte-for-byte unchanged: the file carries the request
// method path, the X-Sirsi-* headers and the JSON body. The relay — the ONLY
// process on the host that holds the host token — forwards the request to the
// service, replacing nothing but the Authorization header, and writes back the
// status code and body. It refuses the token-management methods by name. Least
// privilege, stated exactly: one token holder instead of one per lane; file modes
// do not isolate same-uid processes from each other.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	spoolMaxBody     = 4 << 20  // request bodies: the service limit
	spoolMaxResponse = 64 << 20 // response bodies: what RemoteStore itself accepts
	spoolMaxInFlight = 64       // per lane
	spoolStaleAfter  = 10 * time.Minute
	spoolPoll        = 150 * time.Millisecond
)

// spoolRefused lists the methods the relay never forwards: a lane must not be
// able to mint, list or revoke host tokens through the relay's own token.
var spoolRefused = map[string]bool{"MintHostToken": true, "RevokeHostToken": true, "ListHostTokens": true}

// spoolRequest is one file in <spool>/<agent>/req/.
type spoolRequest struct {
	Method  string            `json:"method"`
	Headers map[string]string `json:"headers"` // X-Sirsi-* only; never Authorization
	Body    []byte            `json:"body"`    // opaque bytes (base64 in the file): never re-validated as JSON
}

// spoolResponse is one file in <spool>/<agent>/res/.
type spoolResponse struct {
	Status int    `json:"status"`
	Body   []byte `json:"body"`
}

// SpoolDir returns the spool directory named by a spool:// URL, or "" when the
// URL is not a spool URL.
func SpoolDir(u string) string {
	u = strings.TrimSpace(u)
	if !strings.HasPrefix(u, "spool://") {
		return ""
	}
	return filepath.Clean(strings.TrimPrefix(u, "spool://"))
}

// spoolTransport is the lane side: an http.RoundTripper over files.
type spoolTransport struct {
	dir   string // <spool>/<agent>
	wait  time.Duration
	now   func() time.Time
	seqMu sync.Mutex
	seq   uint64
}

func newSpoolTransport(spool, agent string) *spoolTransport {
	return &spoolTransport{dir: filepath.Join(spool, agent), wait: 30 * time.Second, now: time.Now}
}

func (t *spoolTransport) nextID() string {
	t.seqMu.Lock()
	defer t.seqMu.Unlock()
	t.seq++
	r, _ := randomHex(4)
	return fmt.Sprintf("%d-%d-%s", t.now().UnixMilli(), t.seq, r)
}

// RoundTrip publishes the request atomically and waits for the response file.
func (t *spoolTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	method := strings.TrimPrefix(r.URL.Path, "/v1/call/")
	if method == "" || method == r.URL.Path {
		return nil, fmt.Errorf("spool: not a router call: %s", r.URL.Path)
	}
	var body []byte
	if r.Body != nil {
		b, err := io.ReadAll(io.LimitReader(r.Body, spoolMaxBody+1))
		if err != nil {
			return nil, err
		}
		if len(b) > spoolMaxBody {
			return nil, fmt.Errorf("spool: request body over %d bytes", spoolMaxBody)
		}
		body = b
	}
	req := spoolRequest{Method: method, Headers: map[string]string{}, Body: body}
	for k := range r.Header {
		if strings.HasPrefix(k, "X-Sirsi-") {
			req.Headers[k] = r.Header.Get(k)
		}
	}
	reqDir, resDir := filepath.Join(t.dir, "req"), filepath.Join(t.dir, "res")
	for _, d := range []string{reqDir, resDir} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return nil, fmt.Errorf("spool: %w", err)
		}
	}
	if n, _ := filepath.Glob(filepath.Join(reqDir, "*.json")); len(n) >= spoolMaxInFlight {
		return nil, fmt.Errorf("spool: %d requests in flight for this lane; relay stalled?", len(n))
	}
	id := t.nextID()
	if err := writeAtomic(filepath.Join(reqDir, id+".json"), req); err != nil {
		return nil, fmt.Errorf("spool: publish: %w", err)
	}
	resPath := filepath.Join(resDir, id+".json")
	defer func() { _ = os.Remove(resPath) }()
	ctx := r.Context()
	deadline := t.now().Add(t.wait)
	for {
		if b, err := os.ReadFile(resPath); err == nil {
			var sr spoolResponse
			if err := json.Unmarshal(b, &sr); err != nil {
				return nil, fmt.Errorf("spool: bad response file: %w", err)
			}
			return &http.Response{StatusCode: sr.Status, Status: http.StatusText(sr.Status), Header: http.Header{"Content-Type": {"application/json"}},
				Body: io.NopCloser(bytes.NewReader(sr.Body)), ContentLength: int64(len(sr.Body)), Request: r}, nil
		}
		if t.now().After(deadline) {
			// Outcome-unknown by contract (20a.1b): the relay may have forwarded
			// and the service may have committed. Withdraw the request file if it
			// is still unconsumed, name the method and id, and never retry a
			// mutation here — the caller re-queries and decides.
			_, unconsumed := os.Stat(filepath.Join(reqDir, id+".json"))
			_ = os.Remove(filepath.Join(reqDir, id+".json"))
			if unconsumed == nil {
				return nil, fmt.Errorf("spool: %s id %s not picked up within %s — outcome unknown only if a relay consumed it after this check; is `sirsi router relay serve` running?", method, id, t.wait)
			}
			return nil, fmt.Errorf("spool: %s id %s: OUTCOME UNKNOWN — the relay consumed the request but no response arrived within %s; re-query before retrying a mutation", method, id, t.wait)
		}
		select {
		case <-ctx.Done():
			_ = os.Remove(filepath.Join(reqDir, id+".json"))
			return nil, ctx.Err()
		case <-time.After(spoolPoll):
		}
	}
}

// writeAtomic marshals v to path via a same-directory temp file and rename(2),
// so a reader never observes a partial file.
func writeAtomic(path string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// Relay is the host side: it holds the host token and forwards spooled requests.
type Relay struct {
	Spool  string
	Base   string // service URL
	Token  string // host token — the only copy on the host outside the relay's plist
	Client *http.Client
	Log    *slog.Logger
	now    func() time.Time
}

// Serve polls the spool until ctx is done. Each request file is forwarded once;
// its response file is written atomically; stale files are swept with a log line.
func (rl *Relay) Serve(ctx context.Context) error {
	if rl.Client == nil {
		rl.Client = &http.Client{Timeout: 25 * time.Second}
	}
	if rl.Log == nil {
		rl.Log = slog.Default()
	}
	if rl.now == nil {
		rl.now = time.Now
	}
	if rl.Token == "" || rl.Base == "" {
		return errors.New("relay: SIRSI_ROUTER_URL and SIRSI_ROUTER_TOKEN are required in the relay's own environment")
	}
	if err := os.MkdirAll(rl.Spool, 0o700); err != nil {
		return err
	}
	rl.recoverInflight()
	lastSweep := rl.now()
	for {
		n := rl.serveOnce()
		if rl.now().Sub(lastSweep) > time.Minute {
			rl.sweep()
			lastSweep = rl.now()
		}
		if n == 0 {
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(spoolPoll):
			}
		}
	}
}

// serveOnce consumes and forwards every pending request file once and returns
// how many it handled. At-most-once (20a.1b): a request is CONSUMED by renaming
// it into <agent>/inflight/ BEFORE the HTTP forward; a failed rename means
// another relay owns it and it is not forwarded. The response is published, then
// the in-flight file is deleted last.
func (rl *Relay) serveOnce() int {
	files, _ := filepath.Glob(filepath.Join(rl.Spool, "*", "req", "*.json"))
	sort.Strings(files)
	n := 0
	for _, f := range files {
		agent := filepath.Base(filepath.Dir(filepath.Dir(f)))
		id := strings.TrimSuffix(filepath.Base(f), ".json")
		inflight := filepath.Join(rl.Spool, agent, "inflight", id+".json")
		if err := os.MkdirAll(filepath.Dir(inflight), 0o700); err != nil {
			continue
		}
		if err := os.Rename(f, inflight); err != nil {
			continue // consumed by someone else, or gone: never forward
		}
		n++
		rl.publish(agent, id, rl.forward(agent, inflight))
		_ = os.Remove(inflight)
	}
	return n
}

// publish writes the response file atomically.
func (rl *Relay) publish(agent, id string, sr spoolResponse) {
	resPath := filepath.Join(rl.Spool, agent, "res", id+".json")
	if err := os.MkdirAll(filepath.Dir(resPath), 0o700); err == nil {
		if err := writeAtomic(resPath, sr); err != nil {
			rl.Log.Error("relay: write response", "agent", agent, "id", id, "err", err)
		}
	}
}

// recoverInflight runs once at startup: anything under <agent>/inflight/ was
// consumed by a relay that died before deleting it. It is NEVER re-forwarded —
// the service may already have committed — the caller gets outcome-unknown.
func (rl *Relay) recoverInflight() int {
	files, _ := filepath.Glob(filepath.Join(rl.Spool, "*", "inflight", "*.json"))
	for _, f := range files {
		agent := filepath.Base(filepath.Dir(filepath.Dir(f)))
		id := strings.TrimSuffix(filepath.Base(f), ".json")
		method := "?"
		var req spoolRequest
		if b, err := os.ReadFile(f); err == nil && json.Unmarshal(b, &req) == nil {
			method = req.Method
		}
		body, _ := json.Marshal(wireResponse{Error: &wireError{Name: "relay", Message: "relay restarted after consuming " + method + " " + id + ": OUTCOME UNKNOWN — re-query before retrying a mutation"}})
		rl.publish(agent, id, spoolResponse{Status: http.StatusBadGateway, Body: body})
		_ = os.Remove(f)
		rl.Log.Warn("relay: in-flight request from a previous relay reported as outcome-unknown", "agent", agent, "method", method, "id", id)
	}
	return len(files)
}

func (rl *Relay) forward(agent, path string) spoolResponse {
	fail := func(status int, msg string) spoolResponse {
		b, _ := json.Marshal(wireResponse{Error: &wireError{Name: "relay", Message: msg}})
		return spoolResponse{Status: status, Body: b}
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return fail(http.StatusBadRequest, "relay: read request: "+err.Error())
	}
	if len(raw) > spoolMaxBody+4096 {
		return fail(http.StatusRequestEntityTooLarge, "relay: request over the 4 MiB limit")
	}
	var req spoolRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return fail(http.StatusBadRequest, "relay: bad request file: "+err.Error())
	}
	if spoolRefused[req.Method] {
		rl.Log.Warn("relay: refused token method", "agent", agent, "method", req.Method)
		return fail(http.StatusForbidden, "relay: "+req.Method+" is never forwarded (token management stays on the service host)")
	}
	httpReq, err := http.NewRequest(http.MethodPost, strings.TrimRight(rl.Base, "/")+"/v1/call/"+req.Method, bytes.NewReader(req.Body))
	if err != nil {
		return fail(http.StatusBadRequest, "relay: "+err.Error())
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+rl.Token)
	for k, v := range req.Headers {
		if strings.HasPrefix(k, "X-Sirsi-") {
			httpReq.Header.Set(k, v)
		}
	}
	resp, err := rl.Client.Do(httpReq)
	if err != nil {
		rl.Log.Error("relay: service unreachable", "agent", agent, "method", req.Method, "err", err)
		return fail(http.StatusServiceUnavailable, "relay: service unreachable: "+err.Error())
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, spoolMaxResponse))
	rl.Log.Info("relay: forwarded", "agent", agent, "method", req.Method, "status", resp.StatusCode)
	return spoolResponse{Status: resp.StatusCode, Body: body}
}

// sweep removes request/response files older than spoolStaleAfter.
func (rl *Relay) sweep() {
	for _, pat := range []string{"*/req/*.json", "*/res/*.json", "*/inflight/*.json", "*/req/*.tmp", "*/res/*.tmp"} {
		files, _ := filepath.Glob(filepath.Join(rl.Spool, pat))
		for _, f := range files {
			if st, err := os.Stat(f); err == nil && rl.now().Sub(st.ModTime()) > spoolStaleAfter {
				_ = os.Remove(f)
				rl.Log.Warn("relay: swept stale spool file", "file", filepath.Base(f))
			}
		}
	}
}
