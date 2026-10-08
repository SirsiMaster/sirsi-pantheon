package routerboard

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Handler serves the router board: the page, the stream, and the one lever.
type Handler struct {
	board          *Board
	dir            string // holds index.html
	allowedOrigins map[string]struct{}
}

// DefaultSurfaceOrigins are the browser surfaces allowed to read the shared
// read-only router contract. The board remains local-first; these are explicit
// integration origins, never a wildcard credential boundary.
var DefaultSurfaceOrigins = []string{
	"http://127.0.0.1:9119", // Pantheon/Horus
	"http://localhost:9119",
	"http://127.0.0.1:5173", // Nexus development surface
	"http://localhost:5173",
	"https://sirsi.ai", // Nexus production surface
}

func NewHandler(b *Board, dir string) *Handler {
	return NewHandlerWithOrigins(b, dir, DefaultSurfaceOrigins)
}

// NewHandlerWithOrigins creates the standalone Router Surface with an explicit
// browser-origin allowlist. An empty list disables cross-origin reads while
// preserving same-origin operation.
func NewHandlerWithOrigins(b *Board, dir string, origins []string) *Handler {
	allowed := make(map[string]struct{}, len(origins))
	for _, origin := range origins {
		origin = strings.TrimRight(strings.TrimSpace(origin), "/")
		if origin != "" {
			allowed[origin] = struct{}{}
		}
	}
	return &Handler{board: b, dir: dir, allowedOrigins: allowed}
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
	mux.HandleFunc("/router", h.index)
	mux.HandleFunc("/router/", h.index)
	mux.HandleFunc("/api/ledger", h.slice)
	mux.HandleFunc("/api/tasks", h.slice)
	mux.HandleFunc("/api/stream", h.stream)
	mux.HandleFunc("/api/arm", h.arm)
	// Versioned contract for Nexus and the Pantheon dashboard. These endpoints
	// are projections of this same Board producer; consumers must not rebuild
	// lane state from their own local files.
	mux.HandleFunc("/api/router/v1/manifest", h.manifest)
	mux.HandleFunc("/api/router/v1/snapshot", h.snapshot)
	mux.HandleFunc("/api/router/v1/ledger", h.slice)
	mux.HandleFunc("/api/router/v1/tasks", h.slice)
	mux.HandleFunc("/api/router/v1/stream", h.stream)
}

func (h *Handler) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && r.URL.Path != "/index.html" && r.URL.Path != "/router" && r.URL.Path != "/router/" {
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
	w.Header().Set("Content-Security-Policy", "frame-ancestors 'self' http://127.0.0.1:9119 http://localhost:9119 http://127.0.0.1:5173 http://localhost:5173 https://sirsi.ai")
	_, _ = w.Write(b)
}

func (h *Handler) surfaceHeaders(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Sirsi-Router-Schema", "router-surface.v1")
	origin := strings.TrimRight(strings.TrimSpace(r.Header.Get("Origin")), "/")
	if origin == "" {
		return
	}
	if _, ok := h.allowedOrigins[origin]; ok {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Vary", "Origin")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	}
}

// manifest is the stable discovery document for embedded consumers. It tells
// Nexus and Pantheon where the read model lives without making either surface
// guess a port, payload version, or authority owner.
func (h *Handler) manifest(w http.ResponseWriter, r *http.Request) {
	h.surfaceHeaders(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"schema":    "router-surface.v1",
		"authority": "ra",
		"producer":  "pantheon.routerboard",
		"build":     h.board.buildID,
		"surface":   "/router",
		"endpoints": map[string]string{
			"snapshot": "/api/router/v1/snapshot",
			"stream":   "/api/router/v1/stream",
			"ledger":   "/api/router/v1/ledger",
			"tasks":    "/api/router/v1/tasks",
		},
		"integrations": map[string]string{
			"pantheon": "read-only same-producer projection",
			"nexus":    "read-only embedded surface or contract consumer",
		},
	})
}

func (h *Handler) snapshot(w http.ResponseWriter, r *http.Request) {
	h.surfaceHeaders(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	body, version := h.board.Snapshot()
	w.Header().Set("Content-Type", "application/json")
	if version == 0 || len(body) == 0 {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"no poll completed yet"}`))
		return
	}
	_, _ = w.Write(body)
}

// slice serves board or tasks out of the current payload.
//
// Before the first successful poll it returns 503, never an all-zero board:
// zeros render as a DEAD FLEET, which is a worse lie than an error.
func (h *Handler) slice(w http.ResponseWriter, r *http.Request) {
	body, version := h.board.Snapshot()
	if strings.HasPrefix(r.URL.Path, "/api/router/v1/") {
		h.surfaceHeaders(w, r)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
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
	if r.URL.Path == "/api/tasks" || r.URL.Path == "/api/router/v1/tasks" {
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
	if strings.HasPrefix(r.URL.Path, "/api/router/v1/") {
		h.surfaceHeaders(w, r)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
	}
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
