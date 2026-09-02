package routerstore

// ADR-062 §2/§3 (rs-08, rs-10): the router service exposes a Store over HTTP,
// one route per method resolved by reflection against the Store interface.
//
// Every request passes an ordered chain, and the order is the security
// argument (SSA condition, ADR-062 rev 3):
//
//  1. host      — bearer token (constant-time compare)               → 401
//  2. session   — X-Sirsi-Session names a minted, unrevoked session  → 401
//  3. freshness — X-Sirsi-Nonce "unixms.random", |age| ≤ 60 s, unseen → 401
//  4. signature — X-Sirsi-Signature = HMAC-SHA256(secret, method \n nonce \n body) → 401
//  5. runtime   — X-Sirsi-Runtime equals the hash bound at MintSession → 403,
//                 and the session is revoked (a mismatch is not a retry)
//  6. ownership — lease-bearing mutations must come from the session that
//                 claimed the item/task → ErrNotOwner (sentinel)
//
// MintSession is the one method that needs only step 1: it is how a node
// obtains a session. GetSession/RevokeSession/Bind*/…Session are server-only
// and are never served over the wire.

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	ctxType   = reflect.TypeOf((*context.Context)(nil)).Elem()
	errType   = reflect.TypeOf((*error)(nil)).Elem()
	storeType = reflect.TypeOf((*Store)(nil)).Elem()
)

// notServed are Store methods that exist for the service's own use and must
// never be reachable from a node.
var notServed = map[string]bool{
	"GetSession": true, "RevokeSession": true, "TouchSession": true,
	"BindItemSession": true, "ItemSession": true, "BindTaskSession": true, "TaskSession": true,
	"MintHostToken": true, "LookupHostToken": true, "RevokeHostToken": true, "ListHostTokens": true,
	"Close": true,
}

// itemOwnership: method → index of the item id argument. The caller's session
// must equal the session bound at claim time.
var itemOwnership = map[string]int{
	"Complete": 0, "Fail": 0, "Block": 0, "StartWork": 0, "RenewLease": 0,
}

// taskOwnership: method → indexes of (agent, taskID).
var taskOwnership = map[string][2]int{
	"CompleteTaskLease": {0, 1}, "ReleaseTaskLease": {0, 1}, "RenewTaskLease": {0, 1},
}

const nonceWindow = 60 * time.Second

// ServerOptions configure Handler.
type ServerOptions struct {
	// Token is the host bearer token every request must present. Empty means
	// the handler refuses to serve at all.
	Token string
	// MaxWait caps a Wait long-poll regardless of what the client asked for.
	MaxWait time.Duration
	// CallTimeout bounds every non-Wait call server-side.
	CallTimeout time.Duration
	// now is injectable for the freshness tests (Rule A16).
	now func() time.Time
}

type server struct {
	store Store
	sv    reflect.Value
	opts  ServerOptions

	nonceMu sync.Mutex
	nonces  map[string]time.Time // "session|nonce" → expiry
}

// Handler serves store over HTTP at /v1/call/{Method}.
func Handler(store Store, opts ServerOptions) (http.Handler, error) {
	if strings.TrimSpace(opts.Token) == "" {
		return nil, errors.New("routerstore: serve: a bearer token is required (ADR-062 §3); refusing to serve an unauthenticated ledger")
	}
	if opts.MaxWait <= 0 {
		opts.MaxWait = 60 * time.Second
	}
	if opts.CallTimeout <= 0 {
		opts.CallTimeout = 5 * time.Second
	}
	if opts.now == nil {
		opts.now = func() time.Time { return time.Now().UTC() }
	}
	s := &server{store: store, sv: reflect.ValueOf(store), opts: opts, nonces: map[string]time.Time{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("/v1/call/", s.call)
	return mux, nil
}

func (s *server) call(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "", "POST only")
		return
	}
	// 1. host — the bootstrap token (ServerOptions.Token) or a minted per-host
	// token (rs-11). The resolved host constrains what a session may claim.
	tokenHost, ok := s.hostForBearer(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "", "missing or invalid bearer token")
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/v1/call/")
	m, ok := storeType.MethodByName(name)
	if !ok || notServed[name] {
		writeErr(w, http.StatusNotFound, "", "no such store method: "+name)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64<<20))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "", "read body: "+err.Error())
		return
	}

	// 2–5. session, freshness, signature, runtime — everything but MintSession.
	var sess Session
	if name != "MintSession" {
		var code int
		sess, code, err = s.authenticate(r, name, body)
		if err != nil {
			writeErr(w, code, "", err.Error())
			return
		}
	}

	var req wireRequest
	if len(bytes.TrimSpace(body)) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			writeErr(w, http.StatusBadRequest, "", "bad JSON: "+err.Error())
			return
		}
	}
	// A per-host token may only mint sessions for its own host: a node cannot
	// claim to be another machine. The bootstrap token is unconstrained.
	if name == "MintSession" && tokenHost != bootstrapHost && len(req.Args) > 0 {
		var claimed string
		if json.Unmarshal(req.Args[0], &claimed) == nil && claimed != tokenHost {
			writeErr(w, http.StatusForbidden, "", ErrHostMismatch.Error()+": token is for "+tokenHost)
			return
		}
	}

	mt := m.Type
	in := make([]reflect.Value, 0, mt.NumIn())
	ai := 0
	timeout := s.opts.CallTimeout
	if name == "Wait" {
		timeout = s.opts.MaxWait
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	for i := 0; i < mt.NumIn(); i++ {
		pt := mt.In(i)
		if pt == ctxType {
			in = append(in, reflect.ValueOf(ctx))
			continue
		}
		if ai >= len(req.Args) {
			writeErr(w, http.StatusBadRequest, "", fmt.Sprintf("%s: expected %d args, got %d", name, mt.NumIn(), len(req.Args)))
			return
		}
		pv := reflect.New(pt)
		if err := json.Unmarshal(req.Args[ai], pv.Interface()); err != nil {
			writeErr(w, http.StatusBadRequest, "", fmt.Sprintf("%s: arg %d: %v", name, ai, err))
			return
		}
		in = append(in, pv.Elem())
		ai++
	}
	if name == "Wait" && len(in) == 3 {
		if d, ok := in[2].Interface().(time.Duration); ok && (d <= 0 || d > s.opts.MaxWait) {
			in[2] = reflect.ValueOf(s.opts.MaxWait)
		}
	}

	// 6. ownership — before the call, against the session bound at claim time.
	if idx, ok := itemOwnership[name]; ok && idx < len(in) {
		if err := s.checkItemOwner(in[idx].String(), sess.ID); err != nil {
			s.writeCallErr(w, name, err)
			return
		}
	}
	if idx, ok := taskOwnership[name]; ok && idx[1] < len(in) {
		if err := s.checkTaskOwner(in[idx[0]].String(), in[idx[1]].String(), sess.ID); err != nil {
			s.writeCallErr(w, name, err)
			return
		}
	}

	outs := s.sv.MethodByName(name).Call(in)

	var callErr error
	results := outs
	if mt.NumOut() > 0 && mt.Out(mt.NumOut()-1) == errType {
		if e, _ := outs[len(outs)-1].Interface().(error); e != nil {
			callErr = e
		}
		results = outs[:len(outs)-1]
	}
	if callErr != nil {
		s.writeCallErr(w, name, callErr)
		return
	}

	// Bind the session to what was just claimed, so ownership holds from here.
	s.bindAfterClaim(name, results, sess.ID)
	if sess.ID != "" {
		_ = s.store.TouchSession(sess.ID)
	}

	resp := wireResponse{Result: make([]json.RawMessage, 0, len(results))}
	for _, rv := range results {
		b, err := json.Marshal(rv.Interface())
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "", "encode result: "+err.Error())
			return
		}
		resp.Result = append(resp.Result, b)
	}
	writeJSON(w, http.StatusOK, resp)
}

// authenticate runs steps 2–5. Returns the session on success.
func (s *server) authenticate(r *http.Request, method string, body []byte) (Session, int, error) {
	sid := strings.TrimSpace(r.Header.Get("X-Sirsi-Session"))
	nonce := strings.TrimSpace(r.Header.Get("X-Sirsi-Nonce"))
	sig := strings.TrimSpace(r.Header.Get("X-Sirsi-Signature"))
	runtime := strings.TrimSpace(r.Header.Get("X-Sirsi-Runtime"))
	if sid == "" || nonce == "" || sig == "" || runtime == "" {
		return Session{}, http.StatusUnauthorized, errors.New("session headers required: X-Sirsi-Session, X-Sirsi-Nonce, X-Sirsi-Signature, X-Sirsi-Runtime (MintSession first)")
	}
	// 2. session
	sess, err := s.store.GetSession(sid)
	if err != nil {
		return Session{}, http.StatusUnauthorized, err
	}
	// 3. freshness
	if err := s.checkNonce(sid, nonce); err != nil {
		return Session{}, http.StatusUnauthorized, err
	}
	// 4. signature
	if !hmac.Equal([]byte(sig), []byte(Sign(sess.Secret, method, nonce, body))) {
		return Session{}, http.StatusUnauthorized, errors.New("bad request signature")
	}
	// 5. runtime — bound at mint; a mismatch invalidates the session.
	if subtle.ConstantTimeCompare([]byte(runtime), []byte(sess.RuntimeHash)) != 1 {
		_ = s.store.RevokeSession(sid)
		return Session{}, http.StatusForbidden, errors.New("runtime hash does not match the session's bound runtime; session revoked")
	}
	return sess, 0, nil
}

// checkNonce enforces the ±60 s window and single use per session.
func (s *server) checkNonce(sid, nonce string) error {
	ms, _, ok := strings.Cut(nonce, ".")
	if !ok {
		return errors.New("nonce must be unixms.random")
	}
	t, err := strconv.ParseInt(ms, 10, 64)
	if err != nil {
		return errors.New("nonce timestamp invalid")
	}
	now := s.opts.now()
	issued := time.UnixMilli(t)
	if d := now.Sub(issued); d > nonceWindow || d < -nonceWindow {
		return fmt.Errorf("nonce outside the %s window", nonceWindow)
	}
	key := sid + "|" + nonce
	s.nonceMu.Lock()
	defer s.nonceMu.Unlock()
	// Evict expired entries opportunistically; the map is bounded by rate×window.
	for k, exp := range s.nonces {
		if now.After(exp) {
			delete(s.nonces, k)
		}
	}
	if _, seen := s.nonces[key]; seen {
		return errors.New("nonce replayed")
	}
	s.nonces[key] = now.Add(nonceWindow)
	return nil
}

func (s *server) checkItemOwner(id, sid string) error {
	owner, err := s.store.ItemSession(id)
	if err != nil {
		return err
	}
	if owner != "" && owner != sid {
		return ErrNotOwner
	}
	return nil
}

func (s *server) checkTaskOwner(agent, taskID, sid string) error {
	owner, err := s.store.TaskSession(agent, taskID)
	if err != nil {
		return err
	}
	if owner != "" && owner != sid {
		return ErrNotOwner
	}
	return nil
}

func (s *server) bindAfterClaim(name string, results []reflect.Value, sid string) {
	if sid == "" || len(results) == 0 {
		return
	}
	switch name {
	case "ClaimNext":
		if l, ok := results[0].Interface().(*Lease); ok && l != nil {
			_ = s.store.BindItemSession(l.ItemID, sid)
		}
	case "ClaimTask", "ClaimNextTask":
		if l, ok := results[0].Interface().(*TaskLease); ok && l != nil {
			_ = s.store.BindTaskSession(l.Agent, l.TaskID, sid)
		}
	}
}

func (s *server) writeCallErr(w http.ResponseWriter, method string, err error) {
	if sn := sentinelName(err); sn != "" {
		writeJSON(w, http.StatusOK, wireResponse{Error: &wireError{Name: sn, Message: err.Error()}})
		return
	}
	writeErr(w, http.StatusInternalServerError, "", method+": "+err.Error())
}

// Sign is the request signature both sides compute:
// hex(HMAC-SHA256(secret, method \n nonce \n body)).
func Sign(secret, method, nonce string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(method))
	mac.Write([]byte{'\n'})
	mac.Write([]byte(nonce))
	mac.Write([]byte{'\n'})
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// bootstrapHost is the host name recorded for the ServerOptions.Token path.
const bootstrapHost = "*"

// hostForBearer resolves the bearer to a host: the bootstrap token → "*",
// a minted per-host token → its host, anything else → not authorized.
func (s *server) hostForBearer(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	const p = "Bearer "
	if !strings.HasPrefix(h, p) {
		return "", false
	}
	got := strings.TrimSpace(strings.TrimPrefix(h, p))
	if subtle.ConstantTimeCompare([]byte(got), []byte(s.opts.Token)) == 1 {
		return bootstrapHost, true
	}
	t, err := s.store.LookupHostToken(got)
	if err != nil {
		return "", false
	}
	return t.Host, true
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, name, msg string) {
	writeJSON(w, code, wireResponse{Error: &wireError{Name: name, Message: msg}})
}
