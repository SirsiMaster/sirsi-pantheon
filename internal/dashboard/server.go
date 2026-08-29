package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"sync"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/apprecovery"
	"github.com/SirsiMaster/sirsi-pantheon/internal/ledger"
	"github.com/SirsiMaster/sirsi-pantheon/internal/notify"
	"github.com/SirsiMaster/sirsi-pantheon/internal/platform"
)

// Config holds the dependencies for the dashboard server.
// All data sources are nil-safe — the server degrades gracefully.
type Config struct {
	Port     int
	NotifyDB *notify.Store
	// SNELocalAccessToken is the scoped capability required by inference and
	// state-changing SNE/recovery routes. Empty preserves migration-only legacy
	// behavior; production wiring must provide a durable generated token.
	SNELocalAccessToken string
	// SNELocalAccessTokenPath is the private durable file atomically replaced
	// by authenticated capability rotation. Empty disables the rotation API.
	SNELocalAccessTokenPath string
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
	// SNEInstall configures Pantheon's asynchronous, integrity-gated model
	// acquisition bridge. Nil keeps model install controls honestly disabled.
	SNEInstall *SNEInstallConfig
	// SNELifecycle configures exact installed-model to packaged-runtime
	// supervision. Nil keeps Start/Stop controls honestly disabled.
	SNELifecycle *SNELifecycleConfig
	// SNEPrefixCachePressureReader is an optional package-owned, validated
	// receipt reader. Nil projects honest unavailable evidence; it never causes
	// Pantheon to open an arbitrary path or operate SNE cache state.
	SNEPrefixCachePressureReader SNEPrefixCachePressureReceiptReader
	SNEPrefixCacheReceiptTool    string
	SNEPrefixCacheDataRoot       string
	SNEPrefixCacheIdentityJSON   string
	// AppRecovery is Pantheon's optional registry-bound application recovery
	// controller. Nil keeps recovery controls honestly unavailable.
	AppRecovery *apprecovery.Manager
}

// FleetProducer supplies the raw ledger snapshot the fleet board diffs into a
// transition feed.
type FleetProducer func() (ledger.Snapshot, error)

// Server is the Pantheon local dashboard HTTP server.
type Server struct {
	cfg               Config
	handler           http.Handler
	alt               []*http.Server
	srv               *http.Server
	unlock            func()
	mu                sync.RWMutex
	running           bool
	runner            *Runner
	confirm           *ConfirmGuard
	fleet             *FleetTracker
	sneJobs           *SNEInstallManager
	sneLifecycle      *SNELifecycleManager
	snePressure       *prefixCachePressureAuthorizationManager
	snePressureReader SNEPrefixCachePressureReceiptReader
	appRecovery       *apprecovery.Manager
	sneAccess         *sneLocalAccess
	sneAccessPath     string
}

// New creates a dashboard server with all routes registered.
func New(cfg Config) *Server {
	if cfg.Port == 0 {
		cfg.Port = DashboardPort
	}

	s := &Server{cfg: cfg, confirm: NewConfirmGuard(), fleet: NewFleetTracker(cfg.Unroutable), appRecovery: cfg.AppRecovery, sneAccess: newSNELocalAccess(cfg.SNELocalAccessToken), sneAccessPath: cfg.SNELocalAccessTokenPath, snePressureReader: cfg.SNEPrefixCachePressureReader}
	if cfg.SNEInstall != nil {
		s.sneJobs = NewSNEInstallManager(*cfg.SNEInstall)
	}
	if cfg.SNELifecycle != nil {
		s.sneLifecycle = NewSNELifecycleManager(*cfg.SNELifecycle)
	}
	s.snePressure = newPrefixCachePressureAuthorizationManager(s.confirm)
	if s.snePressureReader == nil {
		s.snePressureReader = newSNEPrefixCacheToolReader(cfg.SNEPrefixCacheReceiptTool, cfg.SNEPrefixCacheDataRoot, cfg.SNEPrefixCacheIdentityJSON)
	}
	if s.sneLifecycle != nil {
		s.snePressure.containment = func() string { return s.sneLifecycle.Snapshot().State }
	}

	// Initialize runner if we have both an event buffer and a binary path.
	if cfg.Events != nil && cfg.SirsiBin != "" {
		s.runner = NewRunner(cfg.Events, cfg.SirsiBin, cfg.NotifyDB)
	}

	mux := http.NewServeMux()

	// HTML pages
	mux.HandleFunc("/", s.handleOverview)
	mux.HandleFunc("/scan", s.handleScan)
	mux.HandleFunc("/ghosts", s.handleGhosts)
	mux.HandleFunc("/guard", s.handleGuard)
	mux.HandleFunc("/notifications", s.handleNotifications)
	mux.HandleFunc("/horus", s.handleHorus)
	mux.HandleFunc("/vault", s.handleVault)
	mux.HandleFunc("/sne", s.handleOverview)

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
	mux.HandleFunc("/api/node-status", s.apiNodeStatus)                   // ADR-026 Horus ops-view read endpoint
	mux.HandleFunc("/api/fleet", s.apiFleet)                              // A32 owner-reporting board (replaces server.py)
	mux.HandleFunc("/api/ledger", s.apiLedger)                            // A26 Nexus board seam — ledger.BoardSummary
	mux.HandleFunc("/api/fabric", s.apiFabric)                            // unified work/message/lane contract
	mux.HandleFunc("/api/sne", s.secureSNERoute(false, s.apiSNE))         // local SNE catalog and runtime read-model
	mux.HandleFunc("/api/sne/chat", s.secureSNERoute(true, s.apiSNEChat)) // governed streaming bridge to the verified local runtime
	mux.HandleFunc("/api/sne/diagnostics", s.secureSNERoute(false, s.apiSNEDiagnostics))
	mux.HandleFunc("/api/sne/support-bundle", s.secureSNERoute(true, s.apiSNESupportBundle))
	mux.HandleFunc("/api/sne/access/rotate", s.secureSNERoute(true, s.apiSNEAccessRotate))
	mux.HandleFunc("/api/sne/nexus/open", s.secureSNERoute(true, s.apiSNENexusOpen))
	mux.HandleFunc("/api/sne/install", s.secureSNERoute(true, s.apiSNEInstall))
	mux.HandleFunc("/api/sne/install/status", s.secureSNERoute(false, s.apiSNEInstallStatus))
	mux.HandleFunc("/api/sne/prepared/discard", s.secureSNERoute(true, s.apiSNEDiscardPrepared))
	mux.HandleFunc("/api/sne/remove", s.secureSNERoute(true, s.apiSNERemove))
	mux.HandleFunc("/api/sne/start", s.secureSNERoute(true, s.apiSNEStart))
	mux.HandleFunc("/api/sne/stop", s.secureSNERoute(true, s.apiSNEStop))
	mux.HandleFunc("/api/sne/lifecycle", s.secureSNERoute(false, s.apiSNELifecycle))
	mux.HandleFunc("/api/sne/prefix-cache-pressure", s.secureSNERoute(true, s.apiSNEPrefixCachePressure))
	mux.HandleFunc("/api/sne/prefix-cache-pressure/receipts/", s.secureSNERoute(false, s.apiSNEPrefixCachePressureExecutionReceipt))
	mux.HandleFunc("/api/sne/prefix-cache-pressure/retention/", s.secureSNERoute(false, s.apiSNEPrefixCachePressureRetentionReceipt))
	mux.HandleFunc("/api/sne/catalog/rollback", s.secureSNERoute(true, s.apiSNECatalogRollback))
	mux.HandleFunc("/api/sne/catalog/remove", s.secureSNERoute(true, s.apiSNECatalogRemove))
	mux.HandleFunc("/api/sne/catalog/updates", s.secureSNERoute(false, s.apiSNECatalogUpdates))
	mux.HandleFunc("/api/sne/catalog/install", s.secureSNERoute(true, s.apiSNECatalogInstall))
	mux.HandleFunc("/api/recovery", s.secureSNERoute(false, s.apiRecovery))
	mux.HandleFunc("/api/recovery/restart", s.secureSNERoute(true, s.apiRecoveryRestart))
	mux.HandleFunc("/api/recovery/resume", s.secureSNERoute(true, s.apiRecoveryResume))
	mux.HandleFunc("/v1/models", s.secureSNERoute(false, s.apiSNEOpenAIModels))  // governed OpenAI-compatible model discovery
	mux.HandleFunc("/v1/chat/completions", s.secureSNERoute(true, s.apiSNEChat)) // governed OpenAI-compatible streaming/non-streaming bridge

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
	if s.appRecovery != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			for _, result := range s.appRecovery.ReconcilePending(ctx) {
				if result.Err != nil {
					fmt.Printf("dashboard: recovery reconciliation failed for %s (%s)\n", result.TargetID, result.Receipt.FailureCode)
				}
			}
		}()
	}
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
	if s.sneLifecycle != nil {
		_, _ = s.sneLifecycle.Stop(ctx)
	}

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
