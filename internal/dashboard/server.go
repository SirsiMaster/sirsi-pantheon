package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/apollo"
	"github.com/SirsiMaster/sirsi-pantheon/internal/ledger"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/casebook"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/knowledge"
	"github.com/SirsiMaster/sirsi-pantheon/internal/notify"
	"github.com/SirsiMaster/sirsi-pantheon/internal/platform"
)

// Config holds the dependencies for the dashboard server.
// All data sources are nil-safe — the server degrades gracefully.
type Config struct {
	Port     int
	NotifyDB *notify.Store
	// StatsFn returns the current system stats as JSON bytes.
	// The menubar marshals its own StatsSnapshot; we pass it through.
	StatsFn func() ([]byte, error)
	// StelePath is the path to the Stele JSONL ledger.
	// If empty, defaults to ~/.config/ra/stele.jsonl.
	StelePath string
	// Events is the shared ring buffer for SSE streaming.
	// If nil, /api/events returns 503.
	Events *EventBuffer
	// SirsiBin is the path to the sirsi binary for command execution.
	// If empty, the runner is disabled.
	SirsiBin string
	// NodeStatusFn is the producer for GET /api/node-status (ADR-026 read
	// contract). Typically wired to a closure over router.CollectNodeStatus.
	// If nil, /api/node-status returns 503 (graceful degrade — same pattern as
	// StatsFn / NotifyDB).
	NodeStatusFn NodeStatusCollector
	// LedgerFn is the producer for GET /api/ledger (A26 Nexus seam).
	// Typically wired to a closure over ledger.Build + ledger.Summarize.
	// If nil, /api/ledger returns 503 (graceful degrade).
	LedgerFn LedgerSummarizer
	// FleetFn is the producer for GET /api/fleet (A32 owner-reporting board).
	// Typically wired to a closure over ledger.Build. If nil, /api/fleet
	// returns 503 (graceful degrade) rather than an empty board, which would
	// read as "the fleet has no work".
	FleetFn FleetProducer
	// Unroutable is the set of agent ids with no automated wake path, read from
	// the registry by the caller (which owns registry access; the dashboard
	// deliberately does not import it). Empty or nil means every lane is
	// treated as routable — honest only when routability is genuinely unknown.
	Unroutable map[string]bool
	// AltPorts are ADDITIONAL ports served by the SAME process and the SAME
	// handler. Not a second dashboard: a second door onto one producer.
	//
	// 8734 is here because it was a separate Python board computing its own
	// lane states, and on 2026-08-05 it reported nine lanes WORKING while every
	// one of them had zero live processes. Two producers cannot agree by
	// discipline — only by being one producer. Retiring the port would have
	// broken the owner's habit; sharing the handler keeps the address and makes
	// the disagreement structurally impossible.
	AltPorts []int
	// FabricFn is the canonical producer for GET /api/fabric — the single
	// cross-surface work/message/lane contract (dashboard + menubar derive
	// their state from this producer). If nil, the endpoint returns 503
	// rather than a misleading zero-valued payload.
	FabricFn FabricProducer
	// RouterFn produces GET /api/router: lane verdicts, queue, consumer cap,
	// registry pin, known-failure catalog, swap-hygiene receipt and what each
	// release added. Wired by the caller, which owns registry/store access.
	// If nil the endpoint returns 503 rather than an empty panel.
	RouterFn RouterProducer
	// SurfaceOrigins is the explicit browser-origin allowlist for the canonical
	// router surface consumed by Nexus. Empty uses the local development and
	// Sirsi production defaults; wildcard CORS is never used.
	SurfaceOrigins []string
	// MaatDecisionsFn supplies the shared, read-only decision projection. The
	// dashboard never recalculates a Ma'at determination from reservations or
	// host facts; it renders the producer's recorded assessment verbatim.
	MaatDecisionsFn MaatDecisionProducer
	// MaatCasebookFn provides a local System One projection over the shared
	// decision journal. It remains read-only: the dashboard cannot treat a
	// case as a new decision or authorization.
	MaatCasebookFn MaatCasebookProducer
	// MaatKnowledgeFn provides the same sensitivity-filtered local knowledge
	// view used by the CLI, native app, and MCP. The dashboard never reads the
	// compatibility cache itself.
	MaatKnowledgeFn MaatKnowledgeProducer
	// ApolloTelemetryFn is the sole producer for the latest SNE-owned local
	// session sample. The dashboard does not inspect SNE endpoints or calculate
	// derived throughput; it presents this typed read verbatim.
	ApolloTelemetryFn ApolloTelemetryProducer
}

// MaatDecisionProducer supplies the most recent recorded Ma'at decisions.
type MaatDecisionProducer func(limit int) ([]maat.Decision, error)

// MaatCasebookProducer supplies a classified and evidence-linked decision
// projection for the Ma'at dashboard view.
type MaatCasebookProducer func(casebook.Query) (casebook.View, error)

// MaatKnowledgeProducer supplies Ma'at's read-only local knowledge view.
type MaatKnowledgeProducer func(query string) (knowledge.View, error)

// ApolloTelemetryProducer supplies the strict local Apollo telemetry read.
type ApolloTelemetryProducer func() (apollo.TelemetryRead, error)

// FleetProducer supplies the raw ledger snapshot the fleet board diffs into a
// transition feed.
type FleetProducer func() (ledger.Snapshot, error)

// Server is the Pantheon local dashboard HTTP server.
type Server struct {
	cfg            Config
	handler        http.Handler
	alt            []*http.Server
	srv            *http.Server
	unlock         func()
	mu             sync.RWMutex
	running        bool
	runner         *Runner
	confirm        *ConfirmGuard
	fleet          *FleetTracker
	surfaceOrigins map[string]struct{}
}

// New creates a dashboard server with all routes registered.
func New(cfg Config) *Server {
	if cfg.Port == 0 {
		cfg.Port = DashboardPort
	}

	origins := cfg.SurfaceOrigins
	if origins == nil {
		origins = []string{"http://127.0.0.1:5173", "http://localhost:5173", "https://sirsi.ai"}
	}
	allowed := make(map[string]struct{}, len(origins))
	for _, origin := range origins {
		origin = strings.TrimRight(strings.TrimSpace(origin), "/")
		if origin != "" {
			allowed[origin] = struct{}{}
		}
	}
	s := &Server{cfg: cfg, confirm: NewConfirmGuard(), fleet: NewFleetTracker(cfg.Unroutable), surfaceOrigins: allowed}

	// Initialize runner if we have both an event buffer and a binary path.
	if cfg.Events != nil && cfg.SirsiBin != "" {
		s.runner = NewRunner(cfg.Events, cfg.SirsiBin, cfg.NotifyDB)
	}

	mux := http.NewServeMux()

	// HTML pages
	mux.HandleFunc("/", s.handleHome)
	mux.HandleFunc("/assets/", s.handleUIAsset)
	mux.HandleFunc("/classic", s.handleOverview)
	mux.HandleFunc("/scan", s.handleScan)
	mux.HandleFunc("/ghosts", s.handleGhosts)
	mux.HandleFunc("/guard", s.handleGuard)
	mux.HandleFunc("/notifications", s.handleNotifications)
	mux.HandleFunc("/horus", s.handleHorus)
	mux.HandleFunc("/vault", s.handleVault)
	mux.HandleFunc("/router", s.handleRouterSurface)

	// JSON API endpoints
	mux.HandleFunc("/api/stats", s.apiStats)
	mux.HandleFunc("/api/notifications", s.apiNotifications)
	mux.HandleFunc("/api/stele", s.apiStele)
	mux.HandleFunc("/api/events", s.apiEvents)
	mux.HandleFunc("/api/run", s.apiRun)
	mux.HandleFunc("/api/run/status", s.apiRunStatus)
	mux.HandleFunc("/api/actions", s.apiActions)
	mux.HandleFunc("/api/findings", s.apiFindings)
	mux.HandleFunc("/api/clean", s.apiClean)

	// Module APIs
	mux.HandleFunc("/api/ghosts", s.apiGhosts)
	mux.HandleFunc("/api/ghosts/clean", s.apiGhostClean)
	mux.HandleFunc("/api/doctor", s.apiDoctor)
	mux.HandleFunc("/api/ask", s.apiAsk)
	mux.HandleFunc("/api/slay", s.apiSlay)
	mux.HandleFunc("/api/guard/stats", s.apiGuardStats)
	mux.HandleFunc("/api/guard/renice", s.apiRenice)
	mux.HandleFunc("/api/horus/report", s.apiWorkstationReport)
	mux.HandleFunc("/api/horus/scan", s.apiHorusScan)
	mux.HandleFunc("/api/horus/query", s.apiHorusQuery)
	mux.HandleFunc("/api/vault/search", s.apiVaultSearch)
	mux.HandleFunc("/api/vault/stats", s.apiVaultStats)
	mux.HandleFunc("/api/vault/prune", s.apiVaultPrune)
	mux.HandleFunc("/api/ra/status", s.apiRaStatus)
	mux.HandleFunc("/api/ra/scopes", s.apiRaScopes)
	mux.HandleFunc("/api/node-status", s.apiNodeStatus) // ADR-026 Horus ops-view read endpoint
	mux.HandleFunc("/api/fleet", s.apiFleet)            // A32 owner-reporting board (replaces server.py)
	mux.HandleFunc("/api/ledger", s.apiLedger)          // A26 Nexus board seam — ledger.BoardSummary
	mux.HandleFunc("/api/router", s.apiRouter)          // router panel: lanes, queue, known failures, swap, releases
	mux.HandleFunc("/api/router/v1/manifest", s.apiRouterSurfaceManifest)
	mux.HandleFunc("/api/router/v1/snapshot", s.apiRouterSurfaceSnapshot)
	mux.HandleFunc("/api/fabric", s.apiFabric) // unified work/message/lane contract
	mux.HandleFunc("/api/maat/decisions", s.apiMaatDecisions)
	mux.HandleFunc("/api/maat/casebook", s.apiMaatCasebook)
	mux.HandleFunc("/api/maat/knowledge", s.apiMaatKnowledge)
	mux.HandleFunc("/api/apollo/telemetry", s.apiApolloTelemetry)

	s.handler = mux
	s.srv = &http.Server{
		Addr:         fmt.Sprintf("127.0.0.1:%d", cfg.Port),
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 0, // SSE connections are long-lived
	}

	return s
}

// Start begins serving the dashboard in a background goroutine.
// Acquires a singleton lock so only one dashboard runs at a time.
// Non-blocking — returns immediately.
func (s *Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return nil
	}

	unlock, err := platform.TryLock("dashboard")
	if err != nil {
		return fmt.Errorf("dashboard: %w", err)
	}
	s.unlock = unlock

	go func() {
		if err := s.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("dashboard: server error: %v\n", err)
		}
	}()

	// Additional doors onto the same handler. A failure here is reported, never
	// fatal: losing the alias port must not take down the primary board.
	for _, p := range s.cfg.AltPorts {
		if p == 0 || p == s.cfg.Port {
			continue
		}
		alt := &http.Server{
			Addr:         fmt.Sprintf("127.0.0.1:%d", p),
			Handler:      s.handler,
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 0,
		}
		s.alt = append(s.alt, alt)
		go func(a *http.Server) {
			if err := a.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				fmt.Printf("dashboard: alt listener %s: %v\n", a.Addr, err)
			}
		}(alt)
	}

	s.running = true
	return nil
}

// Stop gracefully shuts down the dashboard server.
func (s *Server) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for _, a := range s.alt {
		_ = a.Shutdown(ctx)
	}
	s.alt = nil
	err := s.srv.Shutdown(ctx)
	if s.unlock != nil {
		s.unlock()
	}
	s.running = false
	return err
}

// URL returns the dashboard base URL.
func (s *Server) URL() string {
	return fmt.Sprintf("http://127.0.0.1:%d", s.cfg.Port)
}

// IsRunning reports whether the server is active.
func (s *Server) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// openBrowserMu and openBrowserFn implement injectable side effects (Rule A16/A21).
var (
	openBrowserMu sync.RWMutex
	openBrowserFn = defaultOpenBrowser
)

func getOpenBrowserFn() func(string) error {
	openBrowserMu.RLock()
	defer openBrowserMu.RUnlock()
	return openBrowserFn
}

// SetOpenBrowserFn allows tests to inject a mock browser opener.
func SetOpenBrowserFn(fn func(string) error) {
	openBrowserMu.Lock()
	defer openBrowserMu.Unlock()
	openBrowserFn = fn
}

func defaultOpenBrowser(url string) error {
	return exec.Command("open", url).Start()
}

// OpenPage opens the given dashboard page in the default browser.
func (s *Server) OpenPage(path string) error {
	if !s.IsRunning() {
		if err := s.Start(); err != nil {
			return err
		}
		// Give the server a moment to bind.
		time.Sleep(50 * time.Millisecond)
	}
	return getOpenBrowserFn()(s.URL() + path)
}

// writeJSON is a helper for API handlers.
func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		http.Error(w, `{"error":"encode failed"}`, http.StatusInternalServerError)
	}
}

// writeError sends a JSON error response.
func writeError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
