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
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/user"
	"path/filepath"
	"reflect"
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
var spoolRefused = map[string]bool{"MintHostToken": true, "MintScopedHostToken": true, "RevokeHostToken": true, "ListHostTokens": true}

var errSpoolMaintenancePending = errors.New("spool admission: retained root maintenance transaction")
var errRelaySettlementUnresolved = errors.New("relay: response settlement remains unresolved; retained inflight/outbox evidence requires recovery")

// spoolRequest is one file in <spool>/<agent>/req/.
type spoolRequest struct {
	Method  string            `json:"method"`
	Headers map[string]string `json:"headers"` // X-Sirsi-* only; never Authorization
	Body    []byte            `json:"body"`    // opaque bytes (base64 in the file): never re-validated as JSON
}

// spoolResponse is one file in <spool>/<agent>/res/.
type spoolResponse struct {
	Status         int    `json:"status"`
	Body           []byte `json:"body"`
	outcomeUnknown bool
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

type spoolResponseBody struct {
	root      *os.Root
	path      string
	reader    *bytes.Reader
	mu        sync.Mutex
	eof       bool
	closed    bool
	closeOnce sync.Once
	closeErr  error
}

func (b *spoolResponseBody) Read(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return 0, io.ErrClosedPipe
	}
	n, err := b.reader.Read(p)
	if errors.Is(err, io.EOF) {
		b.eof = true
	}
	return n, err
}

func (b *spoolResponseBody) Close() error {
	b.closeOnce.Do(func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if b.eof {
			b.closeErr = errors.Join(b.closeErr, b.root.Remove(b.path))
		}
		b.closeErr = errors.Join(b.closeErr, b.root.Close())
		b.closed = true
	})
	return b.closeErr
}

func newSpoolTransport(spool, agent string) *spoolTransport {
	return &spoolTransport{agent: agent, root: spool, wait: 30 * time.Second, now: time.Now}
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
	callCtx, callCancel := context.WithTimeout(r.Context(), t.wait)
	defer callCancel()
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
	rootTransferred := false
	defer func() {
		if !rootTransferred {
			_ = root.Close()
		}
	}()
	if err := refuseUntrustedClientOnTrustedSpool(t.root, trustGID); err != nil {
		return nil, err
	}
	// A canceled/paused call has not published anything; the existing uncertainty
	// classification still governs requests already consumed by the relay.
	var admission *os.File
	for {
		admission, rerr = lockSpoolAdmission(callCtx, root, false)
		if rerr == nil {
			break
		}
		if !errors.Is(rerr, errSpoolMaintenancePending) {
			if errors.Is(rerr, context.DeadlineExceeded) || errors.Is(rerr, context.Canceled) {
				return nil, fmt.Errorf("spool admission: router refresh is still pending; request not published, retry after the retained maintenance transaction completes: %w", rerr)
			}
			return nil, fmt.Errorf("spool admission: request not published: %w", rerr)
		}
		timer := time.NewTimer(spoolPoll)
		select {
		case <-callCtx.Done():
			timer.Stop()
			return nil, fmt.Errorf("spool admission: router refresh is still pending; request not published, retry after the retained maintenance transaction completes: %w", callCtx.Err())
		case <-timer.C:
		}
	}
	defer admission.Close()
	if err := callCtx.Err(); err != nil {
		return nil, fmt.Errorf("spool admission: request not published: %w", err)
	}
	reqDir, resDir := filepath.Join(t.agent, "req"), filepath.Join(t.agent, "res")
	// t.agent itself (<spool>/<agent>) is included: the relay needs to traverse
	// INTO it, not just into its req/res/slots children, and MkdirAll creates
	// it as an intermediate directory with the same requested mode.
	for _, d := range []string{t.agent, reqDir, resDir, filepath.Join(t.agent, "slots")} {
		if err := callCtx.Err(); err != nil {
			return nil, fmt.Errorf("spool admission: request not published: %w", err)
		}
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
	if err := callCtx.Err(); err != nil {
		return nil, fmt.Errorf("spool admission: request not published: %w", err)
	}
	id := t.nextID()
	reqPath := filepath.Join(reqDir, id+".json")
	if err := callCtx.Err(); err != nil {
		return nil, fmt.Errorf("spool admission: request not published: %w", err)
	}
	if err := writeAtomicRoot(root, reqPath, req, fileMode); err != nil {
		return nil, fmt.Errorf("spool: publish %s id %s: %w", method, id, err)
	}
	resPath := filepath.Join(resDir, id+".json")
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
	for {
		if err := callCtx.Err(); err != nil {
			return nil, uncertain(err.Error())
		}
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
			rootTransferred = true
			return &http.Response{StatusCode: sr.Status, Status: http.StatusText(sr.Status), Header: http.Header{"Content-Type": {"application/json"}},
				Body: &spoolResponseBody{root: root, path: resPath, reader: bytes.NewReader(sr.Body)}, ContentLength: int64(len(sr.Body)), Request: r}, nil
		}
		select {
		case <-callCtx.Done():
			return nil, uncertain(callCtx.Err().Error())
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
		// Only EEXIST proves occupancy. Sandbox/permission/I/O errors must not
		// masquerade as 64 occupied slots or an upstream relay outage.
		if !errors.Is(err, fs.ErrExist) {
			return "", fmt.Errorf("spool: cannot acquire request slot %s (request not published; verify sandbox writable roots and relay-directory access): %w", p, err)
		}
		st, serr := root.Stat(p)
		if serr != nil {
			if errors.Is(serr, fs.ErrNotExist) {
				continue // another caller released the slot
			}
			return "", fmt.Errorf("spool: inspect occupied request slot %s: %w", p, serr)
		}
		if time.Since(st.ModTime()) > spoolStaleAfter {
			if err := root.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return "", fmt.Errorf("spool: reclaim stale request slot %s: %w", p, err)
			}
			// One bounded retry, rather than decrementing i indefinitely when
			// reclamation is denied or another caller repeatedly wins the slot.
			f, err = root.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
			if err == nil {
				_ = f.Close()
				return p, nil
			}
			if !errors.Is(err, fs.ErrExist) {
				return "", fmt.Errorf("spool: reacquire request slot %s: %w", p, err)
			}
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

// newRelayHTTPClientFor is the relay's forward client for base with the TLS pin from the
// environment applied (remote_pin.go). The relay holds the host token, so it is the process
// where an unpinned connection matters most. A malformed pin is an error: the relay refuses to
// start rather than forward the host token unpinned.
func newRelayHTTPClientFor(base string) (*http.Client, error) {
	return relayClient(base, &tls.Config{MinVersion: tls.VersionTLS12})
}

// relayClient is newRelayHTTPClientFor with the base TLS config injectable (tests trust their
// own certificate authority through it).
func relayClient(base string, baseTLS *tls.Config) (*http.Client, error) {
	c := newRelayHTTPClient()
	cfg, err := pinConfig(os.Getenv(EnvSPKIPin), base, baseTLS)
	if err != nil {
		return nil, err
	}
	if cfg != nil {
		c.Transport.(*http.Transport).TLSClientConfig = cfg
	}
	return c, nil
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
	laneMu           sync.Mutex
	lanes            map[string]chan struct{}
	workers          sync.WaitGroup
	settlementFailed atomic.Bool
	dutyMu           sync.Mutex
	dutyCount        int
	dutyIdle         chan struct{}
	drainPaused      bool
	unreadable       map[string]bool // lane dirs already reported as not readable by this uid
	handled          atomic.Int32
	scans            atomic.Int32 // discovery passes (tests assert no spin under backlog)
	// Per-instance deterministic scheduler probe; nil uses real discovery.
	discoverFn func(context.Context) int
	// Instance-local fixture seam; nil always uses the retained native marker.
	maintenanceStartup func(context.Context) (io.Closer, error)
	// Instance-local filesystem seam; nil uses os.Remove.
	removeInflightFn func(string) error
	// ServeDrained is a one-shot lifecycle boundary for each relay instance.
	drained atomic.Bool
}

// Serve polls the spool until ctx is done. Each request file is forwarded once;
// its response file is written atomically; stale files are swept with a log line.
func (rl *Relay) Serve(ctx context.Context) (result error) {
	return rl.serve(ctx, nil, true, nil)
}

// ErrRelayDrainIncomplete means the serving loop stopped but an admitted duty
// has not acknowledged completion. It never authorizes retirement or replay.
var ErrRelayDrainIncomplete = errors.New("relay: admitted duties have not drained")

// ErrRelaySettlementIncomplete refuses retirement when completed duties could
// not record their response or release already-settled in-flight custody.
var ErrRelaySettlementIncomplete = errors.New("relay: response or outcome custody remains unsettled")

// ServeDrained uses the existing serving path but replaces Serve's unbounded
// worker join with a bounded result. It does not cancel an HTTP request whose
// mutation outcome may be unknown. Timed-out duties remain owned by this
// process; this is not producer quiescence, remote authorization, or proof that
// an already-loaded legacy binary cooperates.
func (rl *Relay) ServeDrained(ctx context.Context, drainTimeout time.Duration) error {
	if drainTimeout <= 0 || drainTimeout > 2*time.Minute {
		return errors.New("relay: drain timeout must be positive and at most two minutes")
	}
	if !rl.drained.CompareAndSwap(false, true) {
		return errors.New("relay: use a new instance for each drained serving lifetime")
	}
	serving, cancel := context.WithCancel(ctx)
	var maintenance io.Closer
	err := rl.serve(serving, nil, false, func(closer io.Closer) { maintenance = closer })
	cancel()
	done := make(chan struct{})
	go func() { rl.workers.Wait(); close(done) }()
	timer := time.NewTimer(drainTimeout)
	defer timer.Stop()
	closeMaintenance := func() error {
		if maintenance == nil {
			return nil
		}
		return maintenance.Close()
	}
	select {
	case <-done:
		maintenanceErr := closeMaintenance()
		if settlementErr := rl.settlementError(); settlementErr != nil || maintenanceErr != nil {
			return errors.Join(err, ErrRelaySettlementIncomplete, settlementErr, maintenanceErr)
		}
		return errors.Join(err, maintenanceErr)
	case <-timer.C:
		// A timed-out drain retains the endpoint owner until the existing
		// workers finish. Timeout does not retire the endpoint or cancel duties.
		go func() {
			<-done
			if closeErr := closeMaintenance(); closeErr != nil {
				rl.settlementFailed.Store(true)
				rl.Log.Error("relay: maintenance endpoint cleanup remains unsettled after worker drain", "err", closeErr)
			}
		}()
		return errors.Join(err, ErrRelayDrainIncomplete)
	}
}

func (rl *Relay) serve(ctx context.Context, duties *sync.WaitGroup, waitWorkers bool, transferMaintenance func(io.Closer)) (result error) {
	if rl.Client == nil {
		c, err := newRelayHTTPClientFor(rl.Base)
		if err != nil {
			return fmt.Errorf("relay: %w", err)
		}
		rl.Client = c
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
	startup := rl.awaitMaintenanceStartup
	if rl.maintenanceStartup != nil {
		startup = rl.maintenanceStartup
	}
	maintenance, err := startup(ctx)
	if err != nil {
		return fmt.Errorf("relay: startup maintenance remains closed: %w", err)
	}
	if transferMaintenance != nil {
		transferMaintenance(maintenance)
	} else {
		defer func() { result = errors.Join(result, maintenance.Close()) }()
	}
	// Optional names/counts projection belongs to this existing trusted relay
	// loop, never a second daemon. Failure cannot stop forwarding or alter any
	// existing queue permissions. One failed publication disables this instance's
	// projection (bounded new-file cleanup debt); prior observations age out.
	observation, observationErr := openRelayObservationRoot(canon, rl.TrustGroup)
	if observationErr != nil {
		rl.Log.Warn("relay outbox observation unavailable", "reason", observationErr.Error())
	}
	defer func() {
		if observation != nil {
			observation.close()
		}
	}()
	nextObservation := rl.now()
	rl.recoverInflight()
	discover := func(ctx context.Context) int { return rl.discoverTracked(ctx, duties) }
	if rl.discoverFn != nil {
		discover = rl.discoverFn
	}
	lastSweep := rl.now()
	for {
		select {
		case <-ctx.Done():
			if waitWorkers {
				rl.workers.Wait()
			}
			return rl.settlementError()
		default:
		}
		discover(ctx)
		if observation != nil && !rl.now().Before(nextObservation) {
			if err := observation.publish(observation.scan(rl.now())); err != nil {
				rl.Log.Warn("relay outbox observation publication stopped; transport continues", "reason", err.Error())
				observation.close()
				observation = nil
			}
			nextObservation = rl.now().Add(5 * time.Second)
		}
		if rl.now().Sub(lastSweep) > time.Minute {
			rl.sweep()
			lastSweep = rl.now()
		}
		// A delivered wake is not proof the request file was claimed yet:
		// its worker can consume the wake before the forward goroutine runs.
		// Bound EVERY discovery pass, including continuous fresh wakes, while
		// independent lane workers keep draining without this polling delay.
		select {
		case <-ctx.Done():
			if waitWorkers {
				rl.workers.Wait()
			}
			return rl.settlementError()
		case <-time.After(spoolPoll):
		}
	}
}

func (rl *Relay) settlementError() error {
	if rl.settlementFailed.Load() {
		return errRelaySettlementUnresolved
	}
	return nil
}

// PrepareDrain closes relay duty admission and waits for every duty that already
// began. It leaves the serving process and maintenance endpoint alive. A
// timeout or local settlement failure reopens admission; request contexts are
// never canceled by this operation.
func (rl *Relay) prepareDrain(ctx context.Context) error {
	if _, ok := ctx.Deadline(); !ok {
		return errors.New("relay: bounded drain context required")
	}
	rl.dutyMu.Lock()
	if rl.drainPaused {
		rl.dutyMu.Unlock()
		return errors.New("relay: drain already prepared")
	}
	rl.drainPaused = true
	idle := rl.dutyIdle
	if idle == nil {
		idle = make(chan struct{})
		close(idle)
	}
	rl.dutyMu.Unlock()
	select {
	case <-idle:
		if err := ctx.Err(); err != nil {
			rl.resumeAfterDrain()
			return err
		}
		if err := rl.settlementError(); err != nil {
			rl.resumeAfterDrain()
			return errors.Join(ErrRelaySettlementIncomplete, err)
		}
		return nil
	case <-ctx.Done():
		rl.resumeAfterDrain()
		return errors.Join(ErrRelayDrainIncomplete, ctx.Err())
	}
}

// resumeAfterDrain reopens this same relay after a failed pre-retirement check.
func (rl *Relay) resumeAfterDrain() {
	rl.dutyMu.Lock()
	rl.drainPaused = false
	rl.dutyMu.Unlock()
}

func (rl *Relay) beginDuty() bool {
	rl.dutyMu.Lock()
	defer rl.dutyMu.Unlock()
	if rl.drainPaused {
		return false
	}
	if rl.dutyCount == 0 {
		rl.dutyIdle = make(chan struct{})
	}
	rl.dutyCount++
	return true
}

func (rl *Relay) endDuty() {
	rl.dutyMu.Lock()
	defer rl.dutyMu.Unlock()
	rl.dutyCount--
	if rl.dutyCount == 0 && rl.dutyIdle != nil {
		close(rl.dutyIdle)
	}
}

// discover lists pending request files, wakes each lane's worker (starting it
// on first sight) and returns immediately — it never waits for any lane. The
// return value counts wakes actually DELIVERED (a worker that already holds a
// pending signal is not counted), so a backlog never turns the loop into a spin.
func (rl *Relay) discoverTracked(ctx context.Context, duties *sync.WaitGroup) int {
	rl.dutyMu.Lock()
	paused := rl.drainPaused
	rl.dutyMu.Unlock()
	if paused {
		return 0
	}
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
			rl.workers.Add(1)
			if duties != nil {
				duties.Add(1)
			}
			go func() {
				defer rl.workers.Done()
				if duties != nil {
					defer duties.Done()
				}
				rl.laneWorkerTracked(ctx, agent, ch, duties)
			}()
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

// relayLaneParallel bounds concurrent forwards within ONE lane. A lane is an agent id, and every
// wake loop on a host without SIRSI_AGENT_ID shares the hostname lane: measured on the M5
// 2026-10-16, that lane forwarded serially at ~1 req/s (each forward is a Cloud Run round trip)
// with queue delay p50 13.5 s, p90 29.3 s, and 8% past the client's 30 s pickup deadline.
// A var so tests can change it.
var relayLaneParallel = 8

// laneWorker drains one lane's queue whenever woken, keeping up to relayLaneParallel forwards in
// flight continuously; the ADR-069 outbox is still drained serially, in id order, after each pass.
func (rl *Relay) laneWorker(ctx context.Context, agent string, wake <-chan struct{}) {
	rl.laneWorkerTracked(ctx, agent, wake, nil)
}

func (rl *Relay) laneWorkerTracked(ctx context.Context, agent string, wake <-chan struct{}, duties *sync.WaitGroup) {
	// The slots are the lane's, not a pass's: a request arriving while a slow forward is in flight
	// starts as soon as any slot is free instead of waiting for the earlier batch to finish.
	// Each client process waits for its own response, so per-client order is kept; consumption
	// stays atomic (rename into inflight/), and started guards against re-launching a file the
	// next pass still sees before its goroutine has claimed it.
	sem := make(chan struct{}, relayLaneParallel)
	var mu sync.Mutex
	started := map[string]bool{}
	for {
		select {
		case <-ctx.Done():
			return
		case <-wake:
		}
		files, _ := filepath.Glob(filepath.Join(rl.Spool, agent, "req", "*.json"))
		sort.Strings(files)
		for _, f := range files {
			if ctx.Err() != nil {
				return
			}
			mu.Lock()
			dup := started[f]
			mu.Unlock()
			if dup {
				continue
			}
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			// A ready semaphore can win select even when cancellation is also
			// ready. Recheck before claiming this queued file or starting work.
			if ctx.Err() != nil {
				<-sem
				return
			}
			mu.Lock()
			started[f] = true
			mu.Unlock()
			rl.workers.Add(1)
			if duties != nil {
				duties.Add(1)
			}
			go func(f string) {
				defer func() {
					rl.workers.Done()
					if duties != nil {
						duties.Done()
					}
					mu.Lock()
					delete(started, f)
					mu.Unlock()
					<-sem
				}()
				rl.handleOne(ctx, agent, f)
			}(f)
		}
		if ctx.Err() != nil {
			return
		}
		// ADR-069: re-forward any messages held during a prior outage, in order.
		// If some are still unreachable, schedule a backed-off re-wake so they
		// drain when the cloud returns — without hot-looping.
		if rl.drainOutbox(ctx, agent) > 0 && ctx.Err() == nil {
			rl.scheduleOutboxRewake(ctx, agent)
		}
	}
}

// handleOne consumes one request atomically (rename into inflight/ BEFORE the
// HTTP forward; a failed rename means another relay owns it), publishes the
// response, then deletes the in-flight file last. Returns whether it forwarded.
func (rl *Relay) handleOne(ctx context.Context, agent, f string) bool {
	if ctx.Err() != nil {
		return false
	}
	if !rl.beginDuty() {
		return false
	}
	defer rl.endDuty()
	id := strings.TrimSuffix(filepath.Base(f), ".json")
	inflight := filepath.Join(rl.Spool, agent, "inflight", id+".json")
	if err := os.MkdirAll(filepath.Dir(inflight), 0o700); err != nil {
		return false
	}
	if ctx.Err() != nil {
		return false
	}
	if err := os.Rename(f, inflight); err != nil {
		return false
	}
	// If cancellation won after the pre-rename check, restore the untouched
	// request instead of starting another forward during shutdown.
	if ctx.Err() != nil {
		if err := os.Rename(inflight, f); err != nil {
			rl.settlementFailed.Store(true)
			rl.Log.Error("relay: canceled request could not be restored; retained inflight", "agent", agent, "id", id, "err", err)
		}
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
		stageErr := os.MkdirAll(filepath.Dir(outbox), 0o700)
		if stageErr == nil {
			stageErr = os.Rename(inflight, outbox)
		}
		if stageErr == nil {
			if err := rl.publish(agent, id, queuedForRetryResponse(id)); err != nil {
				rl.settlementFailed.Store(true)
				rl.Log.Error("relay: queued response publication failed; retained outbox", "agent", agent, "id", id, "err", err)
			}
			return false
		}
		// The service was provably not reached, but the ordered retry destination
		// could not be created. Preserve the envelope in failed/ for explicit
		// handling; if that transfer also fails, retain inflight and sticky debt.
		_, parkErr := rl.parkInflightInFailed(agent, id)
		message := "relay: " + id + " was not sent; ordered retry could not be staged, and its envelope was retained in non-retryable failed custody"
		if parkErr != nil {
			rl.settlementFailed.Store(true)
			message = "relay: " + id + " was not sent; ordered retry staging and failed custody transfer failed, so the original envelope remains inflight"
			rl.Log.Error("relay: could not stage or park not-sent envelope; retained inflight and settlement debt", "agent", agent, "id", id, "stage_err", stageErr, "park_err", parkErr)
		}
		b, _ := json.Marshal(wireResponse{Error: &wireError{Name: "relay", Message: message}})
		if err := rl.publish(agent, id, spoolResponse{Status: http.StatusBadGateway, Body: b}); err != nil {
			rl.settlementFailed.Store(true)
			rl.Log.Error("relay: not-sent response publication failed; retained custody and settlement debt", "agent", agent, "id", id, "stage_err", stageErr, "park_err", parkErr, "err", err)
			return false
		}
		rl.Log.Error("relay: ordered retry could not be staged; no request was sent", "agent", agent, "id", id, "stage_err", stageErr, "park_err", parkErr)
		return false
	}
	if sr.outcomeUnknown || sr.Status == http.StatusBadGateway {
		// Preserve non-retryable custody before publishing an OUTCOME UNKNOWN
		// response. Consuming its error bytes cannot settle the remote mutation.
		_, parkErr := rl.parkInflightInFailed(agent, id)
		if parkErr != nil {
			rl.settlementFailed.Store(true)
			rl.Log.Error("relay: could not park first-attempt unknown outcome; retained inflight and refused settlement", "agent", agent, "id", id, "err", parkErr)
		}
		if err := rl.publish(agent, id, sr); err != nil {
			rl.settlementFailed.Store(true)
			rl.Log.Error("relay: unknown response publication failed; retained non-retryable custody", "agent", agent, "id", id, "err", err)
			return false
		}
		return false
	}
	if err := rl.publish(agent, id, sr); err != nil {
		rl.settlementFailed.Store(true)
		rl.Log.Error("relay: response publication failed; retained inflight", "agent", agent, "id", id, "err", err)
		return false
	}
	return rl.removeSettledInflight(inflight)
}

func (rl *Relay) removeSettledInflight(path string) bool {
	remove := rl.removeInflightFn
	if remove == nil {
		remove = os.Remove
	}
	if err := remove(path); err != nil {
		rl.settlementFailed.Store(true)
		rl.Log.Error("relay: retain unsettled in-flight custody", "path", path, "err", err)
		return false
	}
	return true
}

func (rl *Relay) parkInflightInFailed(agent, id string) (string, error) {
	if !validSpoolAgentName(agent) || id == "" || strings.ContainsAny(id, `/\\`) {
		return "", errors.New("relay: invalid inflight custody identity")
	}
	root, err := os.OpenRoot(rl.Spool)
	if err != nil {
		return "", err
	}
	defer root.Close()
	lane, err := root.OpenRoot(agent)
	if err != nil {
		return "", err
	}
	defer lane.Close()
	source, err := lane.Open("inflight")
	if err != nil {
		return "", err
	}
	defer source.Close()
	if err := lane.Mkdir("failed", 0o700); err != nil && !errors.Is(err, fs.ErrExist) {
		return "", err
	}
	destination, err := lane.Open("failed")
	if err != nil {
		return "", err
	}
	defer destination.Close()
	for _, dir := range []*os.File{source, destination} {
		info, err := dir.Stat()
		if err != nil || !info.IsDir() {
			return "", errors.Join(errors.New("relay: custody directory is not a directory"), err)
		}
	}
	name := id + ".json"
	// The platform helper uses an atomic exclusive rename; an existing failed
	// envelope is never replaced by a probabilistically unique request ID.
	if err := renameSpoolNoReplace(source, name, destination, name); err != nil {
		return "", err
	}
	return filepath.Join(rl.Spool, agent, "failed", name), nil
}

func validSpoolAgentName(agent string) bool {
	if agent == "" || len(agent) > 128 || filepath.Base(agent) != agent || agent == "." || agent == ".." {
		return false
	}
	for _, r := range agent {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			continue
		}
		return false
	}
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
func (rl *Relay) retryOutboxOne(ctx context.Context, agent, f string) outboxOutcome {
	if ctx.Err() != nil {
		return outboxHeld
	}
	if !rl.beginDuty() {
		return outboxHeld
	}
	defer rl.endDuty()
	id := strings.TrimSuffix(filepath.Base(f), ".json")
	inflight := filepath.Join(rl.Spool, agent, "inflight", id+".json")
	if err := os.MkdirAll(filepath.Dir(inflight), 0o700); err != nil {
		return outboxHeld // could not stage — leave it in the outbox, stop the drain
	}
	if ctx.Err() != nil {
		return outboxHeld
	}
	if err := os.Rename(f, inflight); err != nil {
		return outboxHeld // lost the race / gone — treat as still held, stop the drain
	}
	if ctx.Err() != nil {
		if err := os.Rename(inflight, f); err != nil {
			rl.settlementFailed.Store(true)
			rl.Log.Error("relay: canceled outbox item could not be restored; retained inflight", "agent", agent, "id", id, "err", err)
		}
		return outboxHeld
	}
	sr := rl.forward(agent, id, inflight)
	switch {
	case sr.Status == statusHoldForRetry:
		if err := os.Rename(inflight, f); err != nil { // provably never reached
			rl.settlementFailed.Store(true)
			rl.Log.Error("relay: could not restore never-reached request to ordered outbox; retained inflight", "agent", agent, "id", id, "err", err)
		}
		return outboxHeld
	case sr.Status == http.StatusBadGateway:
		// forward's OUTCOME-UNKNOWN sentinel (post-send failure). The request may
		// have crossed the service boundary and committed. Park for audit; do NOT
		// re-forward and do NOT delete. A genuine service 502 is equally uncertain,
		// so parking it too is the safe (conservative) direction — no data loss.
		_, parkErr := rl.parkInflightInFailed(agent, id)
		if parkErr == nil {
			rl.Log.Error("relay: held request OUTCOME UNKNOWN on retry — parked for audit, not re-forwarded", "agent", agent, "id", id)
		} else {
			rl.settlementFailed.Store(true)
			rl.Log.Error("relay: could not park unknown outbox outcome; retained inflight and refused retry", "agent", agent, "id", id, "err", parkErr)
		}
		return outboxUnknown
	default:
		if !rl.removeSettledInflight(inflight) {
			return outboxUnknown
		}
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
func (rl *Relay) drainOutbox(ctx context.Context, agent string) int {
	held, _ := filepath.Glob(filepath.Join(rl.Spool, agent, "outbox", "*.json"))
	sort.Strings(held)
	for i, f := range held {
		if ctx.Err() != nil || rl.retryOutboxOne(ctx, agent, f) != outboxDelivered {
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
		if rl.handleOne(context.Background(), filepath.Base(filepath.Dir(filepath.Dir(f))), f) {
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
		remaining := rl.drainOutbox(context.Background(), agent)
		n += len(before) - remaining // items delivered before this lane's frontier
	}
	return n
}

// publish writes the response file atomically.
func (rl *Relay) publish(agent, id string, sr spoolResponse) error {
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
			return fmt.Errorf("relay: resolve trust group %q: %w", rl.TrustGroup, gerr)
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
		return fmt.Errorf("relay: create response directory: %w", mkErr)
	}
	if err := writeAtomic(resPath, sr, fileMode); err != nil {
		return fmt.Errorf("relay: write response: %w", err)
	}
	return nil
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
		_, parkErr := rl.parkInflightInFailed(agent, id)
		if parkErr != nil {
			rl.settlementFailed.Store(true)
			rl.Log.Error("relay: could not park recovered unknown outcome; retained inflight", "agent", agent, "id", id, "err", parkErr)
		}
		body, _ := json.Marshal(wireResponse{Error: &wireError{Name: "relay", Message: "relay restarted after consuming " + method + " " + id + ": OUTCOME UNKNOWN — re-query before retrying a mutation"}})
		if err := rl.publish(agent, id, spoolResponse{Status: http.StatusBadGateway, Body: body}); err != nil {
			rl.settlementFailed.Store(true)
			rl.Log.Error("relay: outcome-unknown response publication failed; retained custody", "agent", agent, "id", id, "err", err)
			continue
		}
		if parkErr != nil {
			continue
		}
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
	// A TLS handshake timeout is documented above as never-reached but was never
	// detected: net/http returns its own unexported tlsHandshakeTimeoutError (not a
	// *net.OpError), so it fell through to OUTCOME UNKNOWN. The handshake finishes
	// BEFORE any request byte is written, so a timeout there means the request
	// never left this process. On a lossy link (a phone hotspot, 1 connection in 5
	// stalls) this made every stalled handshake an unknown-outcome failure instead
	// of the one safe re-dial and, failing that, the durable outbox.
	if isTLSHandshakeTimeout(err) {
		return true
	}
	// A bare ECONNREFUSED with no net.OpError carries no phase information of its
	// own, so we cannot prove it was pre-send. Classify conservatively as
	// UNKNOWN (not never-reached) rather than risk auto-retrying a mutation that
	// may have committed. In practice the stdlib always wraps a connect-time
	// refusal in the dial-phase *net.OpError handled above.
	return false
}

// isTLSHandshakeTimeout reports whether err is exactly what http.Client.Do
// returns for a stalled TLS handshake: a *url.Error whose Err is net/http's own
// unexported tlsHandshakeTimeoutError leaf. It is matched by the leaf's type
// identity, never by text: a post-send error whose wrapper, URL or longer
// message merely mentions the phrase, or a joined error with several causes,
// must stay OUTCOME UNKNOWN (codex-pantheon review of #1206). Timeout() is not
// used either, because the post-send response-header timeout also reports it.
// This is safe for forward only because its scoped client forbids redirects:
// a later redirected handshake says nothing about an earlier received POST.
func isTLSHandshakeTimeout(err error) bool {
	ue, ok := err.(*url.Error)
	if !ok {
		return false
	}
	t := reflect.TypeOf(ue.Err)
	return t != nil && t.PkgPath() == "net/http" && t.Name() == "tlsHandshakeTimeoutError"
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
		sr := fail(http.StatusBadGateway, fmt.Sprintf("relay: %s id %s: OUTCOME UNKNOWN — %s; re-query before retrying a mutation", req.Method, id, cause))
		sr.outcomeUnknown = true
		return sr
	}
	// A redirect can occur after the canonical service already received a POST.
	// A later redirected dial/TLS failure must never make that original request
	// look never-reached and eligible for replay. Refuse redirects for this
	// forward only; keep the shared timeout, transport/TLS pins and other client
	// state intact, including its redirect policy for unrelated operations.
	forwardClient := *rl.Client
	forwardClient.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	resp, err := forwardClient.Do(httpReq)
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
		forwardClient.CloseIdleConnections()
		if hr2, berr := buildReq(); berr == nil {
			rl.Log.Warn("relay: forward never reached service, re-dialing once on a fresh connection", "agent", agent, "method", req.Method, "id", id, "err", err.Error())
			resp, err = forwardClient.Do(hr2)
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
	return spoolResponse{Status: resp.StatusCode, Body: body, outcomeUnknown: resp.StatusCode == http.StatusBadGateway}
}

// sweep removes abandoned request/temporary files older than spoolStaleAfter.
// Response and inflight files are outcome custody: age cannot establish
// consumption or settlement, so only their typed lifecycle paths remove them.
func (rl *Relay) sweep() {
	for _, pat := range []string{"*/req/*.json", "*/req/*.tmp", "*/res/*.tmp", "*/slots/*"} {
		files, _ := filepath.Glob(filepath.Join(rl.Spool, pat))
		for _, f := range files {
			if st, err := os.Stat(f); err == nil && rl.now().Sub(st.ModTime()) > spoolStaleAfter {
				_ = os.Remove(f)
				rl.Log.Warn("relay: swept stale spool file", "file", filepath.Base(f))
			}
		}
	}
}
