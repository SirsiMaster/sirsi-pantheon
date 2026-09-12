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

// spoolTrustGroupEnv is read on BOTH sides of the spool: the relay reads it
// (via the CLI layer, into Relay.TrustGroup) to decide which owner it accepts
// besides itself; a lane client reads it here to decide the mode it creates
// its own directories with. The group itself is never resolved on the client
// side — a lane cannot chgrp a directory it creates (only its own primary
// group applies), so the mechanism is the spool's OWN setgid bit: once the top
// spool directory is group-owned by the trust group with setgid set (set up
// once, out of band, when the relay is deployed under a service account),
// every directory MkdirAll creates beneath it inherits that group
// automatically. The client's only job is to grant that inherited group
// read/write instead of leaving it inherited-but-closed at 0700.
const spoolTrustGroupEnv = "SIRSI_RELAY_TRUST_GROUP"

// laneDirMode is the mode a lane creates its own req/res/slots directories
// with. 0700 (default, unchanged from before this existed) unless
// SIRSI_RELAY_TRUST_GROUP is set in THIS process's environment, in which case
// 0770 — group access is then whatever group the directory inherits via the
// spool's setgid bit, never chosen by the lane itself.
func laneDirMode() os.FileMode {
	if strings.TrimSpace(os.Getenv(spoolTrustGroupEnv)) != "" {
		return 0o770
	}
	return 0o700
}

// laneFileMode is the client-side counterpart to laneDirMode for the files
// themselves (request files): 0600 by default, 0640 (group-readable, never
// group-writable — nobody but its writer should overwrite a request or
// response body) when SIRSI_RELAY_TRUST_GROUP is set in this process's
// environment.
func laneFileMode() os.FileMode {
	if strings.TrimSpace(os.Getenv(spoolTrustGroupEnv)) != "" {
		return 0o640
	}
	return 0o600
}

// mkdirTrusted creates dir (with any needed parents) and then explicitly
// chmods it to mode. MkdirAll alone is not enough: the requested mode is
// masked by the process umask exactly like a bare mkdir(2), so a umask of the
// common 022 silently turns the intended 0770 into 0750 — defeating the whole
// point of laneDirMode's widened mode without any error to notice it by. The
// chmod runs even when the directory already existed (MkdirAll no-ops on an
// existing directory, indistinguishable from "just created"), so a lane's
// directories converge to the current mode on every call — a lane created
// under the old single-uid default self-heals to group-writable the first
// time it runs again after SIRSI_RELAY_TRUST_GROUP is configured, with no
// separate migration step.
func mkdirTrusted(dir string, mode os.FileMode) error {
	if err := os.MkdirAll(dir, mode); err != nil {
		return err
	}
	return os.Chmod(dir, mode)
}

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
	// t.dir itself (<spool>/<agent>) is included: the relay needs to traverse
	// INTO it, not just into its req/res/slots children, and MkdirAll creates
	// it as an intermediate directory with the same requested mode.
	for _, d := range []string{t.dir, reqDir, resDir, filepath.Join(t.dir, "slots")} {
		if err := mkdirTrusted(d, laneDirMode()); err != nil {
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
	if err := writeAtomic(reqPath, req, laneFileMode()); err != nil {
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
// so a reader never observes a partial file. mode is the file's final
// permission: 0600 (owner-only, the historical default) unless the caller
// opts into laneFileMode()'s widened 0640 — a directory being group-writable
// (laneDirMode/mkdirTrusted) does NOT make the FILES inside it group-readable;
// file permissions are independent of their containing directory's, so a
// request or response file written at 0600 is invisible to a relay or lane
// running as a different, even trust-group-configured, uid. mode is chmod'd
// explicitly after the rename (not just passed to WriteFile) because the
// requested mode is masked by the process umask exactly like a bare
// open(2)/creat(2) — the same reason mkdirTrusted exists for directories.
func writeAtomic(path string, v any, mode os.FileMode) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, mode); err != nil {
		return err
	}
	if err := os.Chmod(tmp, mode); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// newRelayHTTPClient builds the relay's forward client. A relay is a long-lived
// launchd process; it MUST NOT pool keep-alive connections to the service. The
// SUSPECTED failure (rs-30, 2026-09-11): a pooled HTTPS connection goes
// half-open (e.g. macOS idle/sleep) and the next forward then hangs to the
// client Timeout ("context deadline exceeded while awaiting headers") while a
// fresh dial from another process reaches the same healthy service instantly.
// This prevents pooled-connection reuse to mitigate that suspected wedge — it is
// not proven incident closure. Dial fresh per forward (the relay's volume is
// low, so a new handshake is cheap) and bound the header wait below the overall
// Timeout so a dead peer fails fast instead of stalling the lane's whole budget.
func newRelayHTTPClient() *http.Client {
	// Clone the default transport so proxy resolution (ProxyFromEnvironment /
	// HTTPS_PROXY) and the other stdlib defaults are PRESERVED — a zero-value
	// Transport has Proxy==nil and would break proxy-only egress (rs-30 review).
	// Only the pooling and header-wait are overridden: a long-lived launchd
	// relay must not reuse pooled keep-alive connections, because a pooled HTTPS
	// connection that goes half-open (suspected macOS idle/sleep) then hangs the
	// next forward to the timeout while a fresh dial succeeds. Dialing fresh per
	// forward prevents that reuse; the relay's volume is low.
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.DisableKeepAlives = true
	tr.ResponseHeaderTimeout = 20 * time.Second
	return &http.Client{Timeout: 25 * time.Second, Transport: tr}
}

// Relay is the host side: it holds the host token and forwards spooled requests.
type Relay struct {
	Spool  string
	Base   string // service URL
	Token  string // host token — the only copy on the host outside the relay's plist
	Client *http.Client
	Log    *slog.Logger
	now    func() time.Time

	// TrustGroup opts into CheckSpoolDirTrustingGroup instead of the default
	// single-UID CheckSpoolDir — for a relay deployed under a dedicated service
	// account that must still accept requests from lane clients running as a
	// different (interactive) uid. Empty (the default) preserves exact
	// single-UID behavior; this is additive, never a widening of the default.
	TrustGroup string

	// One long-lived worker per lane: a slow lane never delays discovery or
	// service of another lane (SSA 2026-09-10: a global wait barrier starved
	// newly arriving lanes past the client's 5 s deadline). Each worker drains
	// its lane's queue in order; consume-before-forward is unchanged.
	laneMu  sync.Mutex
	lanes   map[string]chan struct{}
	handled atomic.Int32
	scans   atomic.Int32 // discovery passes (tests assert no spin under backlog)
}

// Serve polls the spool until ctx is done. Each request file is forwarded once;
// its response file is written atomically; stale files are swept with a log line.
func (rl *Relay) Serve(ctx context.Context) error {
	if rl.Client == nil {
		rl.Client = newRelayHTTPClient()
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
	checkFn := CheckSpoolDir
	if rl.TrustGroup != "" {
		checkFn = func(spool string) (string, error) { return CheckSpoolDirTrustingGroup(spool, rl.TrustGroup) }
	}
	canon, err := checkFn(rl.Spool)
	if err != nil {
		return fmt.Errorf("relay: %w", err)
	}
	rl.Spool = canon
	rl.recoverInflight()
	lastSweep := rl.now()
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		woke := rl.discover(ctx)
		if rl.now().Sub(lastSweep) > time.Minute {
			rl.sweep()
			lastSweep = rl.now()
		}
		// Sleep unless this pass actually delivered a NEW wake: a lane whose
		// worker is busy with a backlog already has a pending signal, and
		// rescanning for it would spin (SSA 2026-09-10: 471 scans in 100 ms).
		if woke == 0 {
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(spoolPoll):
			}
		}
	}
}

// discover lists pending request files, wakes each lane's worker (starting it
// on first sight) and returns immediately — it never waits for any lane. The
// return value counts wakes actually DELIVERED (a worker that already holds a
// pending signal is not counted), so a backlog never turns the loop into a spin.
func (rl *Relay) discover(ctx context.Context) int {
	rl.scans.Add(1)
	files, _ := filepath.Glob(filepath.Join(rl.Spool, "*", "req", "*.json"))
	seen := map[string]bool{}
	for _, f := range files {
		seen[filepath.Base(filepath.Dir(filepath.Dir(f)))] = true
	}
	rl.laneMu.Lock()
	if rl.lanes == nil {
		rl.lanes = map[string]chan struct{}{}
	}
	delivered := 0
	for agent := range seen {
		ch, ok := rl.lanes[agent]
		if !ok {
			ch = make(chan struct{}, 1)
			rl.lanes[agent] = ch
			go rl.laneWorker(ctx, agent, ch)
		}
		select { // coalesce wakes: one pending signal is enough
		case ch <- struct{}{}:
			delivered++
		default:
		}
	}
	rl.laneMu.Unlock()
	return delivered
}

// laneWorker drains one lane's queue in file order whenever woken.
func (rl *Relay) laneWorker(ctx context.Context, agent string, wake <-chan struct{}) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-wake:
		}
		files, _ := filepath.Glob(filepath.Join(rl.Spool, agent, "req", "*.json"))
		sort.Strings(files)
		for _, f := range files {
			rl.handleOne(agent, f)
		}
	}
}

// handleOne consumes one request atomically (rename into inflight/ BEFORE the
// HTTP forward; a failed rename means another relay owns it), publishes the
// response, then deletes the in-flight file last. Returns whether it forwarded.
func (rl *Relay) handleOne(agent, f string) bool {
	id := strings.TrimSuffix(filepath.Base(f), ".json")
	inflight := filepath.Join(rl.Spool, agent, "inflight", id+".json")
	if err := os.MkdirAll(filepath.Dir(inflight), 0o700); err != nil {
		return false
	}
	if err := os.Rename(f, inflight); err != nil {
		return false
	}
	rl.handled.Add(1)
	rl.publish(agent, id, rl.forward(agent, id, inflight))
	_ = os.Remove(inflight)
	return true
}

// serveOnce drains every pending request synchronously (lanes in name order,
// files in order) and returns how many it forwarded. Tests use it; Serve uses
// the per-lane workers above with the same handleOne.
func (rl *Relay) serveOnce() int {
	files, _ := filepath.Glob(filepath.Join(rl.Spool, "*", "req", "*.json"))
	sort.Strings(files)
	n := 0
	for _, f := range files {
		if rl.handleOne(filepath.Base(filepath.Dir(filepath.Dir(f))), f) {
			n++
		}
	}
	return n
}

// publish writes the response file atomically.
func (rl *Relay) publish(agent, id string, sr spoolResponse) {
	resPath := filepath.Join(rl.Spool, agent, "res", id+".json")
	fileMode := os.FileMode(0o600)
	if rl.TrustGroup != "" {
		fileMode = 0o640
	}
	// res/ almost always already exists — the lane client creates req/, res/
	// and slots/ together, up front, before ever writing anything (RoundTrip).
	// This is only a defensive fallback for a lane that somehow never did.
	// mkdirTrusted's unconditional chmod is deliberately NOT used here: it
	// would try to chmod a directory the RELAY does not own whenever res/
	// already exists (the normal case, created by the client) — chmod
	// requires ownership, so that call would fail with EPERM every time,
	// which the original "if err == nil" structure then swallowed with zero
	// log output. Widening res/'s mode is the CREATING side's job (the
	// client's laneDirMode()); the relay only creates it, at the plain
	// default mode, on the rare path where it must.
	resDir := filepath.Dir(resPath)
	mkErr := error(nil)
	if _, statErr := os.Stat(resDir); os.IsNotExist(statErr) {
		mkErr = os.MkdirAll(resDir, 0o700)
	}
	if mkErr != nil {
		rl.Log.Error("relay: create response dir", "agent", agent, "id", id, "err", mkErr)
		return
	}
	if err := writeAtomic(resPath, sr, fileMode); err != nil {
		rl.Log.Error("relay: write response", "agent", agent, "id", id, "err", err)
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
