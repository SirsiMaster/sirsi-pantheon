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
	"sync/atomic"
	"time"
)

const (
	spoolMaxBody     = 4 << 20                       // request BODY (decoded): the service limit
	spoolMaxEnvelope = spoolMaxBody/3*4 + 64<<10     // request FILE: base64 overhead + headers
	spoolMaxResponse = 64 << 20                      // response BODY (decoded): what RemoteStore accepts
	spoolMaxResFile  = spoolMaxResponse/3*4 + 64<<10 // response FILE
	spoolMaxInFlight = 64                            // per lane, enforced with exclusive slot files
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
	for _, d := range []string{reqDir, resDir, filepath.Join(t.dir, "slots")} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return nil, fmt.Errorf("spool: %w", err)
		}
	}
	// In-flight cap, atomic across processes: one of spoolMaxInFlight slot files
	// is created O_EXCL and removed when this call ends.
	slot, err := acquireSlot(filepath.Join(t.dir, "slots"))
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.Remove(slot) }()
	id := t.nextID()
	reqPath := filepath.Join(reqDir, id+".json")
	if err := writeAtomic(reqPath, req); err != nil {
		return nil, fmt.Errorf("spool: publish %s id %s: %w", method, id, err)
	}
	resPath := filepath.Join(resDir, id+".json")
	defer func() { _ = os.Remove(resPath) }()
	// uncertain classifies a timeout or cancellation: if the request file is still
	// in req/ nobody consumed it (withdraw it, outcome known: nothing happened);
	// otherwise a relay consumed it (rename into inflight/ happens BEFORE the
	// forward) and the outcome is unknown — say so, with method and id, and never
	// retry a mutation here.
	uncertain := func(cause string) error {
		if err := os.Remove(reqPath); err == nil {
			return fmt.Errorf("spool: %s id %s not picked up (%s); nothing was sent — is `sirsi router relay serve` running?", method, id, cause)
		}
		return fmt.Errorf("spool: %s id %s: OUTCOME UNKNOWN — a relay consumed the request but no response arrived (%s); re-query before retrying a mutation", method, id, cause)
	}
	ctx := r.Context()
	deadline := t.now().Add(t.wait)
	for {
		if f, oerr := os.Open(resPath); oerr == nil {
			// A response file exists, so the request was consumed and forwarded:
			// any defect in the file is an UNCERTAIN outcome for the caller.
			unknownResp := func(cause string) error {
				return fmt.Errorf("spool: %s id %s: OUTCOME UNKNOWN — response file unusable (%s); re-query before retrying a mutation", method, id, cause)
			}
			b, rerr := io.ReadAll(io.LimitReader(f, spoolMaxResFile+1))
			_ = f.Close()
			if rerr != nil {
				return nil, unknownResp("read: " + rerr.Error())
			}
			if len(b) > spoolMaxResFile {
				return nil, unknownResp(fmt.Sprintf("file over %d bytes", spoolMaxResFile))
			}
			var sr spoolResponse
			if uerr := json.Unmarshal(b, &sr); uerr != nil {
				return nil, unknownResp("malformed: " + uerr.Error())
			}
			if len(sr.Body) > spoolMaxResponse {
				return nil, unknownResp(fmt.Sprintf("body over %d bytes", spoolMaxResponse))
			}
			return &http.Response{StatusCode: sr.Status, Status: http.StatusText(sr.Status), Header: http.Header{"Content-Type": {"application/json"}},
				Body: io.NopCloser(bytes.NewReader(sr.Body)), ContentLength: int64(len(sr.Body)), Request: r}, nil
		}
		if t.now().After(deadline) {
			return nil, uncertain(fmt.Sprintf("no response within %s", t.wait))
		}
		select {
		case <-ctx.Done():
			return nil, uncertain(ctx.Err().Error())
		case <-time.After(spoolPoll):
		}
	}
}

// acquireSlot creates one of spoolMaxInFlight exclusive slot files; O_EXCL makes
// the cap atomic across processes and lanes sharing a directory. Slots older
// than spoolStaleAfter belong to dead callers and are reclaimed.
func acquireSlot(dir string) (string, error) {
	for i := 0; i < spoolMaxInFlight; i++ {
		p := filepath.Join(dir, fmt.Sprintf("%02d", i))
		f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			_ = f.Close()
			return p, nil
		}
		if st, serr := os.Stat(p); serr == nil && time.Since(st.ModTime()) > spoolStaleAfter {
			_ = os.Remove(p)
			i-- // retry this slot once
		}
	}
	return "", fmt.Errorf("spool: %d requests in flight for this lane; relay stalled?", spoolMaxInFlight)
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
	canon, err := CheckSpoolDir(rl.Spool)
	if err != nil {
		return fmt.Errorf("relay: %w", err)
	}
	rl.Spool = canon
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
	// Lanes are served CONCURRENTLY (one goroutine per lane per pass) and each
	// lane's requests in order: a full-ledger ListAll from one lane must not
	// hold five other lanes past their 5 s per-call deadline (fleet activation
	// 2026-09-10 showed exactly that with one sequential loop).
	byLane := map[string][]string{}
	var lanes []string
	for _, f := range files {
		agent := filepath.Base(filepath.Dir(filepath.Dir(f)))
		if _, ok := byLane[agent]; !ok {
			lanes = append(lanes, agent)
		}
		byLane[agent] = append(byLane[agent], f)
	}
	var wg sync.WaitGroup
	var handled atomic.Int32
	for _, agent := range lanes {
		wg.Add(1)
		go func(agent string, queue []string) {
			defer wg.Done()
			for _, f := range queue {
				id := strings.TrimSuffix(filepath.Base(f), ".json")
				inflight := filepath.Join(rl.Spool, agent, "inflight", id+".json")
				if err := os.MkdirAll(filepath.Dir(inflight), 0o700); err != nil {
					continue
				}
				if err := os.Rename(f, inflight); err != nil {
					continue // consumed by someone else, or gone: never forward
				}
				handled.Add(1)
				rl.publish(agent, id, rl.forward(agent, id, inflight))
				_ = os.Remove(inflight)
			}
		}(agent, byLane[agent])
	}
	wg.Wait()
	return int(handled.Load())
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
		if fh, oerr := os.Open(f); oerr == nil {
			b, rerr := io.ReadAll(io.LimitReader(fh, spoolMaxEnvelope+1))
			_ = fh.Close()
			var req spoolRequest
			if rerr == nil && len(b) <= spoolMaxEnvelope && json.Unmarshal(b, &req) == nil && req.Method != "" {
				method = req.Method
			}
		}
		body, _ := json.Marshal(wireResponse{Error: &wireError{Name: "relay", Message: "relay restarted after consuming " + method + " " + id + ": OUTCOME UNKNOWN — re-query before retrying a mutation"}})
		rl.publish(agent, id, spoolResponse{Status: http.StatusBadGateway, Body: body})
		_ = os.Remove(f)
		rl.Log.Warn("relay: in-flight request from a previous relay reported as outcome-unknown", "agent", agent, "method", method, "id", id)
	}
	return len(files)
}

func (rl *Relay) forward(agent, id, path string) spoolResponse {
	fail := func(status int, msg string) spoolResponse {
		b, _ := json.Marshal(wireResponse{Error: &wireError{Name: "relay", Message: msg}})
		return spoolResponse{Status: status, Body: b}
	}
	f, err := os.Open(path)
	if err != nil {
		return fail(http.StatusBadRequest, "relay: read request: "+err.Error())
	}
	raw, err := io.ReadAll(io.LimitReader(f, spoolMaxEnvelope+1))
	_ = f.Close()
	if err != nil {
		return fail(http.StatusBadRequest, "relay: read request: "+err.Error())
	}
	if len(raw) > spoolMaxEnvelope {
		rl.Log.Warn("relay: request file over the envelope bound", "agent", agent, "id", id, "bytes", len(raw))
		return fail(http.StatusRequestEntityTooLarge, "relay: request file over the envelope bound")
	}
	var req spoolRequest
	if uerr := json.Unmarshal(raw, &req); uerr != nil {
		return fail(http.StatusBadRequest, "relay: bad request file: "+uerr.Error())
	}
	if len(req.Body) > spoolMaxBody {
		rl.Log.Warn("relay: request body over the limit", "agent", agent, "method", req.Method, "id", id, "bytes", len(req.Body))
		return fail(http.StatusRequestEntityTooLarge, fmt.Sprintf("relay: request body %d bytes over the %d limit", len(req.Body), spoolMaxBody))
	}
	if spoolRefused[req.Method] {
		rl.Log.Warn("relay: refused token method", "agent", agent, "method", req.Method, "id", id)
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
	// From here on the request has left this process: any failure is an
	// UNCERTAIN outcome for the caller (the service may have committed), and is
	// reported as such with method and id — never as a plain transport error.
	unknown := func(cause string) spoolResponse {
		rl.Log.Error("relay: outcome unknown after forward", "agent", agent, "method", req.Method, "id", id, "cause", cause)
		return fail(http.StatusBadGateway, fmt.Sprintf("relay: %s id %s: OUTCOME UNKNOWN — %s; re-query before retrying a mutation", req.Method, id, cause))
	}
	resp, err := rl.Client.Do(httpReq)
	if err != nil {
		return unknown("service unreachable or connection lost: " + err.Error())
	}
	defer func() { _ = resp.Body.Close() }()
	body, rerr := io.ReadAll(io.LimitReader(resp.Body, spoolMaxResponse+1))
	if rerr != nil {
		return unknown("service response could not be read: " + rerr.Error())
	}
	if len(body) > spoolMaxResponse {
		return unknown(fmt.Sprintf("service response over %d bytes", spoolMaxResponse))
	}
	rl.Log.Info("relay: forwarded", "agent", agent, "method", req.Method, "id", id, "status", resp.StatusCode)
	return spoolResponse{Status: resp.StatusCode, Body: body}
}

// sweep removes request/response files older than spoolStaleAfter.
func (rl *Relay) sweep() {
	for _, pat := range []string{"*/req/*.json", "*/res/*.json", "*/inflight/*.json", "*/req/*.tmp", "*/res/*.tmp", "*/slots/*"} {
		files, _ := filepath.Glob(filepath.Join(rl.Spool, pat))
		for _, f := range files {
			if st, err := os.Stat(f); err == nil && rl.now().Sub(st.ModTime()) > spoolStaleAfter {
				_ = os.Remove(f)
				rl.Log.Warn("relay: swept stale spool file", "file", filepath.Base(f))
			}
		}
	}
}
