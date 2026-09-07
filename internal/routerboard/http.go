package routerboard

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

// Handler serves the router board: the page, the stream, and the one lever.
type Handler struct {
	board              *Board
	dir                string // holds index.html
	controlToken       string
	requireControlAuth bool
	openControlStore   func() (routerstore.Store, bool, error)
}

func NewHandler(b *Board, dir string) *Handler {
	return NewHandlerWithControlAuth(b, dir, os.Getenv("SIRSI_CONTROL_TOKEN"), false)
}

// NewHandlerWithControlAuth configures whether read-only control snapshots
// require bearer authentication. Protected deployments use this when the
// surrounding listener is already bound to an authenticated private network;
// the default handler remains loopback-compatible.
func NewHandlerWithControlAuth(b *Board, dir, token string, requireAuth bool) *Handler {
	return &Handler{
		board: b, dir: dir, controlToken: token, requireControlAuth: requireAuth,
		openControlStore: func() (routerstore.Store, bool, error) {
			store, err := routerstore.Resolve()
			return store, true, err
		},
	}
}

// NewHandlerWithControlStore injects the canonical store for tests and
// embedded hosts. The handler does not close an injected store.
func NewHandlerWithControlStore(b *Board, dir string, store routerstore.Store, token string) *Handler {
	return &Handler{board: b, dir: dir, controlToken: token, openControlStore: func() (routerstore.Store, bool, error) {
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
	r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var request ControlActionRequest
	if err := decoder.Decode(&request); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, "invalid control action: "+err.Error()), http.StatusBadRequest)
		return
	}
	var trailing any
	if err := decoder.Decode(&trailing); err == nil {
		http.Error(w, `{"error":"invalid control action: multiple JSON values"}`, http.StatusBadRequest)
		return
	} else if err != io.EOF {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, "invalid control action: trailing data: "+err.Error()), http.StatusBadRequest)
		return
	}
	store, owned, err := h.openControlStore()
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, "control store unavailable: "+err.Error()), http.StatusServiceUnavailable)
		return
	}
	if owned {
		defer store.Close()
	}
	response, err := ApplyControlAction(store, request)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusConflict)
		return
	}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		return
	}
}

func (h *Handler) authorizeControl(w http.ResponseWriter, r *http.Request, required bool) bool {
	if !required && strings.TrimSpace(h.controlToken) == "" {
		return true
	}
	if strings.TrimSpace(h.controlToken) == "" {
		http.Error(w, `{"error":"control authorization is not configured"}`, http.StatusServiceUnavailable)
		return false
	}
	if strings.TrimSpace(r.Header.Get("Authorization")) != "Bearer "+h.controlToken {
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
