package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"sync"
	"time"

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
}

// Server is the Pantheon local dashboard HTTP server.
type Server struct {
	cfg     Config
	srv     *http.Server
	unlock  func()
	mu      sync.RWMutex
	running bool
	runner  *Runner
}

// New creates a dashboard server with all routes registered.
func New(cfg Config) *Server {
	if cfg.Port == 0 {
		cfg.Port = DashboardPort
	}

	s := &Server{cfg: cfg}

	// Initialize runner if we have both an event buffer and a binary path.
	if cfg.Events != nil && cfg.SirsiBin != "" {
		s.runner = NewRunner(cfg.Events, cfg.SirsiBin)
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

	// JSON API endpoints
	mux.HandleFunc("/api/stats", s.apiStats)
	mux.HandleFunc("/api/notifications", s.apiNotifications)
	mux.HandleFunc("/api/stele", s.apiStele)
	mux.HandleFunc("/api/events", s.apiEvents)
	mux.HandleFunc("/api/run", s.apiRun)
	mux.HandleFunc("/api/run/status", s.apiRunStatus)
	mux.HandleFunc("/api/findings", s.apiFindings)
	mux.HandleFunc("/api/clean", s.apiClean)

	// Module APIs
	mux.HandleFunc("/api/ghosts", s.apiGhosts)
	mux.HandleFunc("/api/ghosts/clean", s.apiGhostClean)
	mux.HandleFunc("/api/doctor", s.apiDoctor)
	mux.HandleFunc("/api/slay", s.apiSlay)
	mux.HandleFunc("/api/guard/stats", s.apiGuardStats)
	mux.HandleFunc("/api/horus/scan", s.apiHorusScan)
	mux.HandleFunc("/api/horus/query", s.apiHorusQuery)
	mux.HandleFunc("/api/vault/search", s.apiVaultSearch)
	mux.HandleFunc("/api/vault/stats", s.apiVaultStats)
	mux.HandleFunc("/api/vault/prune", s.apiVaultPrune)
	mux.HandleFunc("/api/ra/status", s.apiRaStatus)
	mux.HandleFunc("/api/ra/scopes", s.apiRaScopes)

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
