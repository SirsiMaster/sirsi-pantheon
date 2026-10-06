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
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// SIRSI_RELAY_TRUST_GROUP is read on the RELAY side only (via the CLI layer,
// into Relay.TrustGroup) to decide which owner it accepts besides itself —
// see cmd/sirsi/routerrelaycmd.go. It resolves the name to a real gid
// (user.LookupGroup) and fails closed if it does not resolve — SSA review, PR
// #753: an earlier version treated any non-empty string as license to widen
// permissions, unverified.
//
// A lane client does NOT read this env var (Rule A35, 2026-09-15): it used to
// carry its own copy of the same group name, which meant every lane needed
// its environment kept in sync with whatever the relay operator configured,
// and a lane whose env forgot the var — or carried a stale one — silently
// fell back to single-uid mode with no signal that anything was wrong.
// resolvedLaneModes derives the same decision from the spool root's actual
// on-disk state instead: the one place the operator's trust decision is
// really recorded (chgrp the root to the trust group — CheckSpoolDirTrusting
// Group's own convergence widens it to 0770 from there).

// laneModesFromRoot derives the lane's own directory/file modes and trust gid
// from st — the top-level spool directory's own stat — instead of a
// separately configured env var: if the root is group-writable, its actual
// gid IS the trust group, self-derived rather than named twice, matching
// exactly the gid+write-bit trust decision checkSpoolDir's own groupTrusted
// branch makes for the same directory. Pulled out as a pure function (over a
// caller-supplied FileInfo, not a path) so the derivation policy stays
// unit-testable without needing a live os.Root for every case — openLaneRoot
// is what actually binds this decision to the object it is read from.

// spoolTrustGroupEnv is the env var name, still read on the RELAY side (via
// the CLI layer, into Relay.TrustGroup — cmd/sirsi/routerrelaycmd.go) and
// quoted in refuseUntrustedClientOnTrustedSpool's error message so an
// operator knows what to export. The lane side no longer reads it itself
// (see the comment above).
const spoolTrustGroupEnv = "SIRSI_RELAY_TRUST_GROUP"

func laneModesFromRoot(st os.FileInfo) (dirMode, fileMode os.FileMode, trustGID int) {
	if st.Mode().Perm()&0o020 == 0 {
		return 0o700, 0o600, -1
	}
	sys, ok := st.Sys().(*syscall.Stat_t)
	if !ok {
		return 0o700, 0o600, -1
	}
	return 0o770, 0o640, int(sys.Gid)
}

// openLaneRoot binds the lane's spool root to a single descriptor (os.Root)
// and derives its trust decision from that SAME bound object, then returns
// the root for every descendant create/write the caller goes on to make.
//
// The previous design (resolvedLaneModes(root string), removed here) called
// os.Stat(root) once by PATH to derive trustGID, then RoundTrip walked the
// same path again through independent, unbound os.MkdirAll/os.Chmod/os.Open
// calls (SSA review, Rule A35, 2026-09-19: PR #762 source review). A same-uid
// actor able to repoint the spool namespace between those calls — e.g. swap
// the root for a symlink to a group-writable directory after the trust stat
// but before publication — could make the trust decision and the actual
// writes land on two different filesystem objects, publishing under an
// object never verified to carry that gid. os.Root closes the gap
// structurally rather than by convention: it opens a real descriptor once,
// and every Root-relative operation below (mkdirTrustedIn, acquireSlotIn,
// writeAtomicRoot, and the response read/remove in RoundTrip) resolves
// through that same descriptor — even a root moved or replaced on disk after
// this call leaves the already-open descriptor referencing what it actually
// opened (see the os.Root doc comment), and no descendant path can escape it
// to a different directory.
func openLaneRoot(root string) (*os.Root, os.FileMode, os.FileMode, int, error) {
	r, err := os.OpenRoot(root)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, 0, 0, 0, fmt.Errorf("spool: open root %s: %w", root, err)
		}
		// Nothing on disk yet to inherit trust from (the very first call,
		// before the relay has ever created or validated it): create it
		// single-owner, matching the previous single-uid fallback exactly,
		// then open it the same way every later call will.
		if merr := os.MkdirAll(root, 0o700); merr != nil {
			return nil, 0, 0, 0, fmt.Errorf("spool: create root %s: %w", root, merr)
		}
		if r, err = os.OpenRoot(root); err != nil {
			return nil, 0, 0, 0, fmt.Errorf("spool: open root %s: %w", root, err)
		}
	}
	st, err := r.Lstat(".")
	if err != nil {
		_ = r.Close()
		return nil, 0, 0, 0, fmt.Errorf("spool: stat root %s: %w", root, err)
	}
	dirMode, fileMode, trustGID := laneModesFromRoot(st)
	return r, dirMode, fileMode, trustGID, nil
}

// refuseUntrustedClientOnTrustedSpool fails FAST, before anything is
// published, when the spool root is group-trusted (setgid: the owner-run
// migration in CheckSpoolDirTrustingGroup's doc) but THIS client has no
// SIRSI_RELAY_TRUST_GROUP. Such a client would create its lane directory 0700
// under a relay that runs as another uid; the relay never sees the request and
// the client waits the whole spool timeout for nothing — the exact hang SHA
// measured 2026-09-14 in `ctr`/`node-status`/`doctor`. A bounded, actionable
// error is what A31 requires of CTR; a silent 30 s wait is not.
func refuseUntrustedClientOnTrustedSpool(spoolRoot string, trustGID int) error {
	if trustGID >= 0 {
		return nil
	}
	st, err := os.Stat(spoolRoot)
	if err != nil || st.Mode()&os.ModeSetgid == 0 {
		return nil // absent or single-uid spool: the existing path decides
	}
	group := "<gid>"
	if sys, ok := st.Sys().(*syscall.Stat_t); ok {
		group = strconv.Itoa(int(sys.Gid))
		if g, lerr := user.LookupGroupId(group); lerr == nil {
			group = g.Name
		}
	}
	return fmt.Errorf("spool: %s is group-trusted (setgid, group %s) but this process has no %s; "+
		"its lane directory would be unreadable by the relay and every call would wait unanswered — "+
		"export %s=%s (lanes get it from their plist)", spoolRoot, group, spoolTrustGroupEnv, spoolTrustGroupEnv, group)
}

// mkdirTrusted creates dir (with any needed parents) and converges it to
// mode. MkdirAll's requested mode alone is not enough: it is masked by the
// process umask exactly like a bare mkdir(2), so a umask of the common 022
// silently turns an intended 0770 into 0750 with no error to notice it by —
// the explicit os.Chmod after is what actually guarantees the final mode.
//
// When trustGID >= 0 and dir already exists, its ACTUAL group must already
// equal trustGID before this widens it — SSA review, PR #753: a directory
// that inherited some OTHER, unrelated group (predates the spool's setgid
// bit, or was created under a different configuration entirely) must never
// have its permissions widened just because trust mode is configured
// somewhere; that would grant group-write to whatever group the directory
// happens to carry, not the specific, verified trust group. A mismatch fails
// closed with an actionable message rather than silently doing the wrong
// thing in either direction. trustGID < 0 (the default, no-trust-group path)
// skips this check entirely.
//
// The chmod applies ONLY to a directory this call created. chmod(2) requires
// OWNERSHIP, not access: a lane client (uid 501) sharing a spool whose lane
// directory the relay's uid (_sirsipantheon) created can read and write it
// through the setgid group but can never chmod it — EPERM. The first version
// chmod'd unconditionally and, on 2026-09-13, crash-looped every wake loop on
// the M5 the moment the client was rolled (rs-38); the pre-#753 code never
// chmod'd at all, so "behaves exactly as before" was never true of it. A
// pre-existing directory keeps the mode its owner gave it — the owner (relay,
// CheckSpoolDirTrustingGroup) converges modes; this call only guarantees the
// mode of what it made, which is the umask-bypass the chmod exists for.
func mkdirTrusted(dir string, mode os.FileMode, trustGID int) error {
	_, statErr := os.Stat(dir)
	existed := statErr == nil
	if err := os.MkdirAll(dir, mode); err != nil {
		return err
	}
	if trustGID >= 0 {
		st, err := os.Stat(dir)
		if err != nil {
			return err
		}
		if sys, ok := st.Sys().(*syscall.Stat_t); ok && int(sys.Gid) != trustGID {
			return fmt.Errorf("spool: %s has group %d, not the configured trust group (gid %d); refusing to widen an unrelated group's access — verify the spool's setgid inheritance or chgrp this directory to the trust group first", dir, sys.Gid, trustGID)
		}
	}
	if !existed {
		return os.Chmod(dir, mode)
	}
	// A pre-existing directory THIS uid owns is converged too — chmod on one's
	// own directory never EPERMs (the rs-38 crash was a chmod on the relay's).
	// Without this a lane dir first created by a client that lacked
	// SIRSI_RELAY_TRUST_GROUP stays 0700 forever: the relay's uid cannot enter
	// it, every request waits the full spool timeout unseen, and `ctr` hangs
	// (SHA 2026-09-14, M5; reproduced on the M1 2026-09-15).
	if trustGID >= 0 {
		st, err := os.Stat(dir)
		if err != nil {
			return err
		}
		if sys, ok := st.Sys().(*syscall.Stat_t); ok && int(sys.Uid) == os.Getuid() && st.Mode().Perm()&0o070 != mode&0o070 {
			return os.Chmod(dir, mode)
		}
	}
	return nil
}

// mkdirTrustedIn is mkdirTrusted's Root-bound twin for the lane side, where
// path-based operations are exactly the check/use gap Rule A35 flags (see
// openLaneRoot): relDir is resolved through root's own descriptor, so it can
// never land outside the directory that root's trust decision was derived
// from, regardless of what happens to the path on disk after root was
// opened. Logic is identical to mkdirTrusted otherwise; kept as a separate
// function rather than a shared one because the relay's mkdirTrusted call
// (publish, over an already-canonicalized, symlink-refused rl.Spool) sits in
// a different trust boundary than the lane's and does not need this bound.
func mkdirTrustedIn(root *os.Root, relDir string, mode os.FileMode, trustGID int) error {
	_, statErr := root.Lstat(relDir)
	existed := statErr == nil
	if err := root.MkdirAll(relDir, mode); err != nil {
		return err
	}
	if trustGID >= 0 {
		st, err := root.Lstat(relDir)
		if err != nil {
			return err
		}
		if sys, ok := st.Sys().(*syscall.Stat_t); ok && int(sys.Gid) != trustGID {
			return fmt.Errorf("spool: %s has group %d, not the configured trust group (gid %d); refusing to widen an unrelated group's access — verify the spool's setgid inheritance or chgrp this directory to the trust group first", relDir, sys.Gid, trustGID)
		}
	}
	if existed {
		return nil
	}
	return root.Chmod(relDir, mode)
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
	agent string // <spool>/<agent> — resolved Root-relative, never as a bare path
	root  string // <spool> — opened by openLaneRoot to self-derive and bind trust
	wait  time.Duration
	now   func() time.Time
	seqMu sync.Mutex
	seq   uint64
}

func newSpoolTransport(spool, agent string) *spoolTransport {
	// wait was 30s until rs-hw-20261006: M1 relay evidence (IDs
	// 1791249411390-1-c3e1aed8, 1791249524791-1-e0a45774) showed the relay
	// itself forwarding and answering GetState 34.040s/48.001s after the
	// request — a consumed-but-not-yet-answered round trip the old wait gave
	// up on before the relay could ever win. 60s gives headroom above the
	// worst measured leg (48.001s) without the transport ignoring the server
	// having genuinely failed.
	return &spoolTransport{agent: agent, root: spool, wait: 60 * time.Second, now: time.Now}
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
	root, dirMode, fileMode, trustGID, rerr := openLaneRoot(t.root)
	if rerr != nil {
		return nil, rerr
	}
	defer func() { _ = root.Close() }()
	if err := refuseUntrustedClientOnTrustedSpool(t.root, trustGID); err != nil {
		return nil, err
	}
	reqDir, resDir := filepath.Join(t.agent, "req"), filepath.Join(t.agent, "res")
	// t.agent itself (<spool>/<agent>) is included: the relay needs to traverse
	// INTO it, not just into its req/res/slots children, and MkdirAll creates
	// it as an intermediate directory with the same requested mode.
	for _, d := range []string{t.agent, reqDir, resDir, filepath.Join(t.agent, "slots")} {
		if err := mkdirTrustedIn(root, d, dirMode, trustGID); err != nil {
			return nil, fmt.Errorf("spool: %w", err)
		}
	}
	// In-flight cap, atomic across processes: one of spoolMaxInFlight slot files
	// is created O_EXCL and removed when this call ends.
	slot, err := acquireSlotIn(root, filepath.Join(t.agent, "slots"))
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Remove(slot) }()
	id := t.nextID()
	reqPath := filepath.Join(reqDir, id+".json")
	if err := writeAtomicRoot(root, reqPath, req, fileMode); err != nil {
		return nil, fmt.Errorf("spool: publish %s id %s: %w", method, id, err)
	}
	resPath := filepath.Join(resDir, id+".json")
	defer func() { _ = root.Remove(resPath) }()
	// uncertain classifies a timeout or cancellation: if the request file is still
	// in req/ nobody consumed it (withdraw it, outcome known: nothing happened);
	// otherwise a relay consumed it (rename into inflight/ happens BEFORE the
	// forward) and the outcome is unknown — say so, with method and id, and never
	// retry a mutation here.
	uncertain := func(cause string) error {
		if err := root.Remove(reqPath); err == nil {
			return fmt.Errorf("spool: %s id %s not picked up (%s); nothing was sent — is `sirsi router relay serve` running?", method, id, cause)
		}
		return fmt.Errorf("spool: %s id %s: OUTCOME UNKNOWN — a relay consumed the request but no response arrived (%s); re-query before retrying a mutation", method, id, cause)
	}
	ctx := r.Context()
	deadline := t.now().Add(t.wait)
	for {
		if f, oerr := root.Open(resPath); oerr == nil {
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

// acquireSlotIn creates one of spoolMaxInFlight exclusive slot files; O_EXCL
// makes the cap atomic across processes and lanes sharing a directory. Slots
// older than spoolStaleAfter belong to dead callers and are reclaimed. Bound
// through root, same as mkdirTrustedIn (see openLaneRoot): relDir is resolved
// through root's own descriptor, so a slot can never be created outside the
// directory root's trust decision was derived from.
func acquireSlotIn(root *os.Root, relDir string) (string, error) {
	for i := 0; i < spoolMaxInFlight; i++ {
		p := filepath.Join(relDir, fmt.Sprintf("%02d", i))
		f, err := root.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			_ = f.Close()
			return p, nil
		}
		if st, serr := root.Stat(p); serr == nil && time.Since(st.ModTime()) > spoolStaleAfter {
			_ = root.Remove(p)
			i-- // retry this slot once
		}
	}
	return "", fmt.Errorf("spool: %d requests in flight for this lane; relay stalled?", spoolMaxInFlight)
}

// writeAtomic marshals v to path via a same-directory temp file and rename(2),
// so a reader never observes a partial file. mode is the file's final
// permission: 0600 (owner-only, the historical default) unless the caller
// opts into the widened 0640 (openLaneRoot / CheckSpoolDirTrustingGroup) — a
// directory being group-writable (mkdirTrusted) does NOT make the FILES inside it
// group-readable; file permissions are independent of their containing
// directory's, so a request or response file written at 0600 is invisible to
// a relay or lane running as a different, even trust-group-configured, uid.
// mode is chmod'd explicitly on the TEMP file, before the rename into place
// (not just passed to WriteFile) because the requested mode is masked by the
// process umask exactly like a bare open(2)/creat(2) — the same reason
// mkdirTrusted exists for directories. Chmod-then-rename (rather than
// rename-then-chmod) means a reader can never observe the file at its
// pre-chmod, too-narrow mode: rename(2) is what makes it visible under its
// final name at all, so by the time it appears its permissions are already
// final.
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

// writeAtomicRoot is writeAtomic's Root-bound twin, used only by the lane
// side (see openLaneRoot): path is resolved through root's own descriptor
// for every step (temp write, chmod, rename), so the file that ends up
// visible under its final name can never land outside the directory root's
// trust decision was derived from.
func writeAtomicRoot(root *os.Root, path string, v any, mode os.FileMode) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := root.WriteFile(tmp, b, mode); err != nil {
		return err
	}
	if err := root.Chmod(tmp, mode); err != nil {
		_ = root.Remove(tmp)
		return err
	}
	if err := root.Rename(tmp, path); err != nil {
		_ = root.Remove(tmp)
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
	// PRIMARY guarantee against the half-open wedge: do not pool at all. With
	// keep-alives disabled there is no idle connection to survive a macOS
	// idle/sleep and go half-open, so the "hang awaiting response headers" case
	// (rs-30) cannot arise from a reused connection — every forward dials fresh.
	// The relay's volume is low, so a handshake per forward is cheap; correctness
	// beats reuse here.
	tr.DisableKeepAlives = true
	// Belt-and-suspenders in case a future change re-enables keep-alives: bound
	// how long an idle pooled connection may linger before it is a half-open
	// risk, and keep the pool per-host tiny. These are moot while
	// DisableKeepAlives is true (no idle conns exist), but they make the intent
	// explicit and keep the transport safe if the flag above is ever flipped.
	tr.IdleConnTimeout = 30 * time.Second
	tr.MaxIdleConnsPerHost = 2
	// ResponseHeaderTimeout bounds the wait for the first response byte below the
	// overall Client.Timeout, so a dead peer fails fast instead of stalling the
	// lane's whole budget. It stays UNDER Timeout deliberately.
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
	laneMu     sync.Mutex
	lanes      map[string]chan struct{}
	unreadable map[string]bool // lane dirs already reported as not readable by this uid
	handled    atomic.Int32
	scans      atomic.Int32 // discovery passes (tests assert no spin under backlog)
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
		rl.unreadable = map[string]bool{}
	}
	// Glob silently skips a lane directory this uid cannot enter (a client
	// without the trust group created it 0700). Say so ONCE per lane in the
	// relay's own log, so a hung client has a daemon-side trace to find.
	if lanes, _ := os.ReadDir(rl.Spool); lanes != nil {
		for _, l := range lanes {
			if !l.IsDir() || rl.unreadable[l.Name()] {
				continue
			}
			if _, err := os.ReadDir(filepath.Join(rl.Spool, l.Name(), "req")); err != nil && errors.Is(err, fs.ErrPermission) {
				rl.unreadable[l.Name()] = true
				rl.Log.Warn("relay: lane directory not readable by this uid; its requests will never be seen — "+
					"chmod g+rwx it (owner) or start the client with SIRSI_RELAY_TRUST_GROUP", "lane", l.Name(), "err", err)
			}
		}
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
		// ADR-069: re-forward any messages held during a prior outage, in order.
		// If some are still unreachable, schedule a backed-off re-wake so they
		// drain when the cloud returns — without hot-looping.
		if rl.drainOutbox(agent) > 0 {
			rl.scheduleOutboxRewake(ctx, agent)
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
	sr := rl.forward(agent, id, inflight)
	if sr.Status == statusHoldForRetry {
		// ADR-069: the service was unreachable — HOLD the request in the
		// relay-owned outbox/ (which the client never touches, so no ownership
		// race) and tell the client it is durably queued. drainOutbox re-forwards
		// it in id order when the cloud returns. Never dropped.
		outbox := filepath.Join(rl.Spool, agent, "outbox", id+".json")
		if mkErr := os.MkdirAll(filepath.Dir(outbox), 0o700); mkErr == nil {
			if mvErr := os.Rename(inflight, outbox); mvErr == nil {
				rl.publish(agent, id, queuedForRetryResponse(id))
				return false
			}
		}
		// Could not stage the outbox — do not lose it silently; report unknown.
		b, _ := json.Marshal(wireResponse{Error: &wireError{Name: "relay", Message: "relay: could not stage " + id + " for retry: OUTCOME UNKNOWN — re-query before retrying a mutation"}})
		rl.publish(agent, id, spoolResponse{Status: http.StatusBadGateway, Body: b})
		_ = os.Remove(inflight)
		return false
	}
	rl.publish(agent, id, sr)
	_ = os.Remove(inflight)
	return true
}

// outboxOutcome is the disposition of one retryOutboxOne attempt.
type outboxOutcome int

const (
	// outboxDelivered: the service positively answered (a real HTTP status was
	// read). The held record is removed — the ONLY case that may remove it.
	outboxDelivered outboxOutcome = iota
	// outboxHeld: the service was provably never reached (statusHoldForRetry).
	// The record is kept in the outbox, in order, for the next wake.
	outboxHeld
	// outboxUnknown: a post-send failure (OUTCOME UNKNOWN — connection lost
	// after the request left, or an unreadable response). The service MAY have
	// committed, so the record is neither re-forwarded (double-commit) nor
	// dropped: it is parked in failed/ for audit and re-query.
	outboxUnknown
)

// retryOutboxOne re-forwards one held request (ADR-069). Three outcomes (SSA
// #794, 2026-09-27): a held request is removed ONLY on a positively confirmed
// delivery. A never-reached result is re-held in order for the next wake; a
// post-send OUTCOME-UNKNOWN result is parked for audit — never auto-deleted and
// never auto-re-forwarded, because the service may already have committed it.
// The client already received QUEUED_FOR_RETRY, so no response is re-published.
func (rl *Relay) retryOutboxOne(agent, f string) outboxOutcome {
	id := strings.TrimSuffix(filepath.Base(f), ".json")
	inflight := filepath.Join(rl.Spool, agent, "inflight", id+".json")
	if err := os.MkdirAll(filepath.Dir(inflight), 0o700); err != nil {
		return outboxHeld // could not stage — leave it in the outbox, stop the drain
	}
	if err := os.Rename(f, inflight); err != nil {
		return outboxHeld // lost the race / gone — treat as still held, stop the drain
	}
	sr := rl.forward(agent, id, inflight)
	switch {
	case sr.Status == statusHoldForRetry:
		_ = os.Rename(inflight, f) // provably never reached — keep held in order
		return outboxHeld
	case sr.Status == http.StatusBadGateway:
		// forward's OUTCOME-UNKNOWN sentinel (post-send failure). The request may
		// have crossed the service boundary and committed. Park for audit; do NOT
		// re-forward and do NOT delete. A genuine service 502 is equally uncertain,
		// so parking it too is the safe (conservative) direction — no data loss.
		failed := filepath.Join(rl.Spool, agent, "failed", id+".json")
		if os.MkdirAll(filepath.Dir(failed), 0o700) == nil && os.Rename(inflight, failed) == nil {
			rl.Log.Error("relay: held request OUTCOME UNKNOWN on retry — parked for audit, not re-forwarded", "agent", agent, "id", id)
		} else {
			_ = os.Rename(inflight, f) // could not park — leave held rather than lose it
		}
		return outboxUnknown
	default:
		_ = os.Remove(inflight) // positively delivered (service answered)
		rl.Log.Info("relay: held request delivered on retry", "agent", agent, "id", id, "status", sr.Status)
		return outboxDelivered
	}
}

// drainOutbox re-forwards held items for agent in id order and returns how many
// remain unresolved. It STOPS at the first non-delivery (SSA #794, 2026-09-27):
// if an earlier held request is still unreachable while a later one is
// reachable, forwarding the later one first would violate ADR-069's
// ordered-release guarantee. The frontier (the first non-delivered item and
// everything after it) is left for the next wake.
func (rl *Relay) drainOutbox(agent string) int {
	held, _ := filepath.Glob(filepath.Join(rl.Spool, agent, "outbox", "*.json"))
	sort.Strings(held)
	for i, f := range held {
		if rl.retryOutboxOne(agent, f) != outboxDelivered {
			// Stop at the frontier. Count the rest as unresolved so the caller
			// schedules a backed-off re-wake to drain from here next time.
			return len(held) - i
		}
	}
	return 0
}

// scheduleOutboxRewake re-wakes a lane after a backoff so a persistent outage
// keeps retrying the outbox without hot-looping.
func (rl *Relay) scheduleOutboxRewake(ctx context.Context, agent string) {
	time.AfterFunc(outboxRetryBackoff, func() {
		if ctx.Err() != nil {
			return
		}
		rl.laneMu.Lock()
		ch := rl.lanes[agent]
		rl.laneMu.Unlock()
		if ch != nil {
			select {
			case ch <- struct{}{}:
			default:
			}
		}
	})
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
	// ADR-069: retry held outbox items too, per lane in id order, via drainOutbox
	// so each lane STOPS at its first non-delivery and never releases a later
	// held request before an earlier one (ordered-release guarantee, SSA #794).
	held, _ := filepath.Glob(filepath.Join(rl.Spool, "*", "outbox", "*.json"))
	laneSet := map[string]struct{}{}
	for _, f := range held {
		laneSet[filepath.Base(filepath.Dir(filepath.Dir(f)))] = struct{}{}
	}
	lanes := make([]string, 0, len(laneSet))
	for a := range laneSet {
		lanes = append(lanes, a)
	}
	sort.Strings(lanes) // deterministic: lanes in name order
	for _, agent := range lanes {
		before, _ := filepath.Glob(filepath.Join(rl.Spool, agent, "outbox", "*.json"))
		remaining := rl.drainOutbox(agent)
		n += len(before) - remaining // items delivered before this lane's frontier
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
	// mkdirTrusted's chmod is safe to use HERE (unlike the earlier bug where it
	// ran on an already-existing directory the relay might not own): this
	// branch only runs when os.Stat just reported the directory absent, so a
	// successful MkdirAll here means THIS process created it and therefore
	// owns it — chmod on your own freshly created directory cannot hit the
	// ownership-required EPERM that broke the pre-existing case (SSA review,
	// PR #753: the first version of this fix over-corrected and hardcoded
	// 0700 even under a configured trust group, leaving the fallback path
	// itself inaccessible to the client — this restores the widening, scoped
	// to only the fresh-creation branch where it is actually safe).
	resDir := filepath.Dir(resPath)
	dirMode, trustGID := os.FileMode(0o700), -1
	if rl.TrustGroup != "" {
		dirMode = 0o770
		g, gerr := user.LookupGroup(rl.TrustGroup)
		if gerr != nil {
			rl.Log.Error("relay: resolve trust group", "agent", agent, "id", id, "group", rl.TrustGroup, "err", gerr)
			return
		}
		if gid, aerr := strconv.Atoi(g.Gid); aerr == nil {
			trustGID = gid
		}
	}
	mkErr := error(nil)
	if _, statErr := os.Stat(resDir); os.IsNotExist(statErr) {
		mkErr = mkdirTrusted(resDir, dirMode, trustGID)
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

// neverReachedService reports whether a forward error means the request
// PROVABLY never reached the service — a connection could not be established
// (dial refused, no route, DNS failure, dial/TLS-handshake timeout). Such a
// request is NOT committed, so it is safe to HOLD in the outbox and re-forward
// in order once the cloud is reachable (ADR-069: a message that cannot reach
// the cloud is stored for future release, never dropped). A failure AFTER the
// request was sent (response lost mid-flight) is deliberately NOT this: the
// service may have committed, so that path stays OUTCOME UNKNOWN and is never
// auto-retried. This is the safety hinge of the durable-outbox design — it is
// method-agnostic because "never sent" means "never committed" for any verb.
func neverReachedService(err error) bool {
	if err == nil {
		return false
	}
	// DNS resolution failed → never connected (no socket was ever opened).
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return true
	}
	// A network op error is "never reached" ONLY when it occurred during the
	// dial/connection-establishment phase (SSA #794, 2026-09-27). The phase is
	// the safety hinge: a read/write-phase op error — EVEN one carrying
	// ECONNREFUSED/ECONNRESET (a reset arriving mid-write after the socket was
	// established) — happened after the request may already have crossed the
	// service boundary, so it must stay OUTCOME UNKNOWN and is never auto-retried.
	// The Go dialer wraps connect-time refusals as a dial-phase *net.OpError, so
	// a genuine "connection refused before we sent anything" is caught here.
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return opErr.Op == "dial"
	}
	// A bare ECONNREFUSED with no net.OpError carries no phase information of its
	// own, so we cannot prove it was pre-send. Classify conservatively as
	// UNKNOWN (not never-reached) rather than risk auto-retrying a mutation that
	// may have committed. In practice the stdlib always wraps a connect-time
	// refusal in the dial-phase *net.OpError handled above.
	return false
}

// statusHoldForRetry is an internal sentinel (never a real HTTP status): forward
// could not REACH the service, so the request is provably not committed and
// handleOne holds it in the relay-owned outbox for ordered retry instead of
// dropping it (ADR-069). handleOne translates it to a client-facing
// QUEUED_FOR_RETRY response; it is never written to a response file as-is.
const statusHoldForRetry = -1

// outboxRetryBackoff paces held-item retries so a persistent outage does not
// hot-loop the lane worker (ponytail: fixed backoff; exponential only if a real
// outage shows it matters).
var outboxRetryBackoff = 30 * time.Second

// queuedForRetryResponse is what a client sees when its message was durably held
// because the service was unreachable: not an error, not OUTCOME UNKNOWN —
// accepted, will deliver in order when the cloud is reachable (ADR-069 §2.3).
func queuedForRetryResponse(id string) spoolResponse {
	b, _ := json.Marshal(wireResponse{Error: &wireError{
		Name:    "queued",
		Message: "relay: " + id + " QUEUED_FOR_RETRY — service unreachable; held durably and will deliver in order when the cloud is reachable",
	}})
	return spoolResponse{Status: http.StatusAccepted, Body: b}
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
	// buildReq is called once per attempt: each attempt needs its own body
	// reader (a bytes.Reader is consumed once) and its own *http.Request, so a
	// safe re-dial below re-sends the identical bytes on a fresh request.
	buildReq := func() (*http.Request, error) {
		hr, herr := http.NewRequest(http.MethodPost, strings.TrimRight(rl.Base, "/")+"/v1/call/"+req.Method, bytes.NewReader(req.Body))
		if herr != nil {
			return nil, herr
		}
		hr.Header.Set("Content-Type", "application/json")
		hr.Header.Set("Authorization", "Bearer "+rl.Token)
		for k, v := range req.Headers {
			if strings.HasPrefix(k, "X-Sirsi-") {
				hr.Header.Set(k, v)
			}
		}
		return hr, nil
	}
	httpReq, err := buildReq()
	if err != nil {
		return fail(http.StatusBadRequest, "relay: "+err.Error())
	}
	// From here on the request has left this process: any failure is an
	// UNCERTAIN outcome for the caller (the service may have committed), and is
	// reported as such with method and id — never as a plain transport error.
	unknown := func(cause string) spoolResponse {
		rl.Log.Error("relay: outcome unknown after forward", "agent", agent, "method", req.Method, "id", id, "cause", cause)
		return fail(http.StatusBadGateway, fmt.Sprintf("relay: %s id %s: OUTCOME UNKNOWN — %s; re-query before retrying a mutation", req.Method, id, cause))
	}
	resp, err := rl.Client.Do(httpReq)
	// rs-30 SAFE re-dial (ADR-062 relay trust boundary): if the first attempt
	// PROVABLY never reached the service (neverReachedService — a dial/DNS-phase
	// failure, the shape a dropped half-open connection surfaces once it is
	// re-dialed), the request was not committed, so ONE retry on a fresh
	// connection cannot double-commit. Drop any idle pooled connection first
	// (CloseIdleConnections) so the retry never reuses a possibly half-open one,
	// then re-send once. This turns a transient dial blip (Cloud Run instance
	// rotation, a momentarily half-open peer) into an immediate success instead
	// of a 30s+ outbox round-trip that would strand a network-less codex sandbox.
	// CRITICAL: the retry is gated on neverReachedService — a post-send failure
	// (response lost mid-flight) is NEVER retried here; it falls through to the
	// OUTCOME-UNKNOWN branch below, the exact discipline the durable outbox uses.
	if err != nil && neverReachedService(err) {
		rl.Client.CloseIdleConnections()
		if hr2, berr := buildReq(); berr == nil {
			rl.Log.Warn("relay: forward never reached service, re-dialing once on a fresh connection", "agent", agent, "method", req.Method, "id", id, "err", err.Error())
			resp, err = rl.Client.Do(hr2)
		}
	}
	if err != nil {
		if neverReachedService(err) {
			// Provably never reached the service → NOT committed → hold for
			// ordered retry rather than drop (ADR-069). The sentinel tells
			// handleOne to move it to the outbox and tell the client it is queued.
			rl.Log.Warn("relay: service unreachable, holding request for ordered retry", "agent", agent, "method", req.Method, "id", id, "err", err.Error())
			return spoolResponse{Status: statusHoldForRetry}
		}
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
