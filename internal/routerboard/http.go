package routerboard

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/rolereceipt"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

// Handler serves the router board: the page, the stream, and the one lever.
type Handler struct {
	board                 *Board
	dir                   string // holds index.html
	controlToken          string
	requireControlAuth    bool
	requireControlRole    bool
	controlRoleAuthorizer ControlRoleAuthorizer
	controlRolePolicy     ControlRolePolicy
	openControlStore      func() (*routerstore.Store, bool, error)
}

// NewHandlerWithControlAuthAndRole is the production-facing constructor for
// a receipt-bound worker plane. The authorizer is supplied by the separately
// governed issuer/keyring/revocation layer; nil is intentionally fail-closed.
// The existing constructors remain available for local presentation and
// explicit tests, where no remote role is claimed.
func NewHandlerWithControlAuthAndRole(b *Board, dir, token string, authorizer ControlRoleAuthorizer) *Handler {
	return NewHandlerWithControlAuthAndRolePolicy(b, dir, token, ControlRolePolicy{Role: rolereceipt.ConstrainedClient}, authorizer)
}

// NewHandlerWithControlAuthAndRolePolicy constructs the receipt-bound worker
// plane with an explicit consumer policy. The callback remains responsible
// for external signature/revocation verification; this policy prevents a
// valid receipt for another role, host, issuer, or key from crossing the M5
// control boundary.
func NewHandlerWithControlAuthAndRolePolicy(b *Board, dir, token string, policy ControlRolePolicy, authorizer ControlRoleAuthorizer) *Handler {
	h := NewHandlerWithControlAuth(b, dir, token, true)
	h.requireControlRole = true
	h.controlRolePolicy = policy
	h.controlRoleAuthorizer = authorizer
	return h
}

func NewHandler(b *Board, dir string) *Handler {
	token := os.Getenv("SIRSI_CONTROL_TOKEN")
	// The default constructor is the production-facing board path.  An absent
	// token must fail closed for read-only control inspection just as control
	// actions already do; callers that intentionally inject a store use the
	// explicitly named test/embedded constructor below.
	return NewHandlerWithControlAuth(b, dir, token, true)
}

// NewHandlerWithControlAuth configures whether read-only control snapshots
// require bearer authentication. Production callers must pass true; NewHandler
// does so unconditionally. The boolean remains for an explicitly controlled
// embedded/test boundary where the surrounding listener owns authentication.
func NewHandlerWithControlAuth(b *Board, dir, token string, requireAuth bool) *Handler {
	return &Handler{
		board: b, dir: dir, controlToken: token, requireControlAuth: requireAuth,
		openControlStore: func() (*routerstore.Store, bool, error) {
			path, err := routerstore.DefaultStorePath()
			if err != nil {
				return nil, false, err
			}
			if parent := filepath.Dir(path); parent != "." && parent != "" {
				if err := os.MkdirAll(parent, 0o755); err != nil {
					return nil, false, err
				}
			}
			store, err := routerstore.Open(path)
			return store, true, err
		},
	}
}

// NewHandlerWithInjectedControlStore is an explicit test/embedded seam. It is
// not production board wiring; the handler does not close an injected store.
func NewHandlerWithInjectedControlStore(b *Board, dir string, store *routerstore.Store, token string) *Handler {
	return &Handler{board: b, dir: dir, controlToken: token, requireControlAuth: strings.TrimSpace(token) != "", openControlStore: func() (*routerstore.Store, bool, error) {
		return store, false, nil
	}}
}

// BuildID fingerprints index.html so the page can display which UI it is.
// The owner was once served a CACHED page for hours while a fixed one was
// verified in a force-refreshed browser — a visible build id makes that
// mismatch self-evident instead of invisible.
func BuildID(dir string) string {
	b, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		return "unknown"
	}
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])[:8]
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/", h.index)
	mux.HandleFunc("/index.html", h.index)
	mux.HandleFunc("/api/control", h.control)
	mux.HandleFunc("/api/control/action", h.controlAction)
	mux.HandleFunc("/api/ledger", h.slice)
	mux.HandleFunc("/api/tasks", h.slice)
	mux.HandleFunc("/api/stream", h.stream)
	mux.HandleFunc("/api/arm", h.arm)
}

// control serves the canonical worker-control envelope. It is intentionally
// read-only; mutations stay on the constrained router CLI/task lease surface.
func (h *Handler) control(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "control endpoint is read-only", http.StatusMethodNotAllowed)
		return
	}
	if !h.authorizeControl(w, r, h.requireControlAuth) {
		return
	}
	roleReceipt, err := h.authorizeControlRole(r.Context(), "inspect")
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusForbidden)
		return
	}
	body, version, err := h.board.SnapshotControl()
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err != nil {
		http.Error(w, `{"error":"control snapshot unavailable"}`, http.StatusInternalServerError)
		return
	}
	if version == 0 || len(body) == 0 {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"no poll completed yet"}`))
		return
	}
	if roleReceipt.RawSHA256() != "" {
		body, err = BindControlEnvelopeRoleReceipt(body, roleReceipt)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, "control snapshot role receipt: "+err.Error()), http.StatusInternalServerError)
			return
		}
	}
	_, _ = w.Write(body)
}

func (h *Handler) controlAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "control action requires POST", http.StatusMethodNotAllowed)
		return
	}
	if !h.authorizeControl(w, r, true) {
		return
	}
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		http.Error(w, `{"error":"control action requires Content-Type: application/json"}`, http.StatusUnsupportedMediaType)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64*1024+1)
	raw, readErr := io.ReadAll(r.Body)
	if readErr != nil || len(raw) > 64*1024 {
		http.Error(w, `{"error":"invalid control action: request body too large or unreadable"}`, http.StatusBadRequest)
		return
	}
	if err := ValidateJSONNoDuplicateKeys(raw); err != nil {
		h.writeControlActionFailure(w, http.StatusBadRequest, raw, "", err)
		return
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var request ControlActionRequest
	if err := decoder.Decode(&request); err != nil {
		h.writeControlActionFailure(w, http.StatusBadRequest, raw, "", fmt.Errorf("invalid control action: %w", err))
		return
	}
	var trailing any
	if err := decoder.Decode(&trailing); err == nil {
		h.writeControlActionFailure(w, http.StatusBadRequest, raw, request.Verb, errors.New("invalid control action: multiple JSON values"))
		return
	} else if err != io.EOF {
		h.writeControlActionFailure(w, http.StatusBadRequest, raw, request.Verb, fmt.Errorf("invalid control action: trailing data: %w", err))
		return
	}
	roleReceipt, err := h.authorizeControlRole(r.Context(), request.Verb)
	if err != nil {
		h.writeControlActionFailure(w, http.StatusForbidden, raw, request.Verb, err)
		return
	}
	store, owned, err := h.openControlStore()
	if err != nil {
		h.writeControlActionFailure(w, http.StatusServiceUnavailable, raw, request.Verb, fmt.Errorf("control store unavailable: %w", err))
		return
	}
	if owned {
		defer store.Close()
	}
	response, err := ApplyControlAction(store, request)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err != nil {
		h.writeControlActionFailure(w, http.StatusConflict, raw, request.Verb, err)
		return
	}
	if roleReceipt.RawSHA256() != "" {
		response.RoleReceiptID = roleReceipt.Receipt().ReceiptID
		response.RoleReceiptSHA256 = roleReceipt.RawSHA256()
	}
	if err := response.SealControlActionResponse(raw); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, "control action receipt: "+err.Error()), http.StatusInternalServerError)
		return
	}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		return
	}
}

func (h *Handler) writeControlActionFailure(w http.ResponseWriter, status int, requestBody []byte, verb string, cause error) {
	failure := ControlActionFailure{
		Schema: ControlFailureSchema, Authority: "canonical-routerstore", Verb: strings.TrimSpace(verb), Error: cause.Error(),
	}
	if err := failure.SealControlActionFailure(requestBody); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(failure)
}

func (h *Handler) authorizeControl(w http.ResponseWriter, r *http.Request, required bool) bool {
	configured := strings.TrimSpace(h.controlToken)
	if !required && configured == "" {
		return true
	}
	if configured == "" {
		http.Error(w, `{"error":"control authorization is not configured"}`, http.StatusServiceUnavailable)
		return false
	}
	// A control request has exactly one bearer credential. Header.Get can hide
	// duplicate values by joining or selecting one of them, which would make
	// the authenticated boundary depend on intermediary behavior. Compare
	// fixed-size digests so the secret itself is never used in a
	// length-dependent equality check.
	authorizations := r.Header.Values("Authorization")
	want := sha256.Sum256([]byte("Bearer " + configured))
	valid := len(authorizations) == 1
	if valid {
		got := sha256.Sum256([]byte(authorizations[0]))
		valid = subtle.ConstantTimeCompare(got[:], want[:]) == 1
	}
	if !valid {
		w.Header().Set("WWW-Authenticate", "Bearer")
		http.Error(w, `{"error":"control authorization failed"}`, http.StatusUnauthorized)
		return false
	}
	return true
}

func (h *Handler) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && r.URL.Path != "/index.html" {
		http.NotFound(w, r)
		return
	}
	b, err := os.ReadFile(filepath.Join(h.dir, "index.html"))
	if err != nil {
		http.Error(w, "index.html missing", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	// A dashboard the browser may cache is a dashboard that can lie about the
	// fleet: the owner read a stale page for hours while the fix was verified
	// elsewhere. No-store is load-bearing, not hygiene.
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	_, _ = w.Write(b)
}

// slice serves board or tasks out of the current payload.
//
// Before the first successful poll it returns 503, never an all-zero board:
// zeros render as a DEAD FLEET, which is a worse lie than an error.
func (h *Handler) slice(w http.ResponseWriter, r *http.Request) {
	body, version := h.board.Snapshot()
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	if version == 0 || len(body) == 0 {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"no poll completed yet"}`))
		return
	}
	var p map[string]json.RawMessage
	if err := json.Unmarshal(body, &p); err != nil {
		_, _ = w.Write([]byte("null"))
		return
	}
	key := "board"
	if r.URL.Path == "/api/tasks" {
		key = "tasks"
	}
	if v, ok := p[key]; ok {
		_, _ = w.Write(v)
		return
	}
	_, _ = w.Write([]byte("null"))
}

// stream pushes the whole payload on every version change, with a keepalive so
// intermediaries do not reap an idle connection.
func (h *Handler) stream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	var lastSent uint64
	lastPing := time.Now()
	tick := time.NewTicker(300 * time.Millisecond)
	defer tick.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-tick.C:
			body, v := h.board.Snapshot()
			if v != lastSent && v != 0 {
				fmt.Fprintf(w, "data: %s\n\n", body)
				flusher.Flush()
				lastSent, lastPing = v, time.Now()
			} else if time.Since(lastPing) > 15*time.Second {
				fmt.Fprint(w, ": keepalive\n\n")
				flusher.Flush()
				lastPing = time.Now()
			}
		}
	}
}

// arm runs the SAME command the menubar's "Arm wake channel" runs, so the two
// surfaces cannot disagree about what arming means. A board that can only
// REPORT a stranded lane makes you go elsewhere to fix it.
func (h *Handler) arm(w http.ResponseWriter, r *http.Request) {
	agent := r.URL.Query().Get("agent")
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")

	// Whitelist against the live registry: never pass an arbitrary query-param
	// string into a subprocess argument list.
	var errs []string
	known := h.board.registeredAgents(&errs)
	if _, ok := known[agent]; !ok || agent == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "error": "unknown agent"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, h.board.sirsiBin, "router", "wake-install", agent).CombinedOutput()
	detail := string(out)
	if len(detail) > 400 {
		detail = detail[:400]
	}
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"ok": err == nil, "agent": agent, "detail": detail,
	})
}

// Run polls until the context is canceled.
func (b *Board) Run(ctx context.Context, every time.Duration) {
	b.Poll(ctx)
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			b.Poll(ctx)
		}
	}
}
