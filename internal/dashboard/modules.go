package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/horus"
	"github.com/SirsiMaster/sirsi-pantheon/internal/jackal"
	"github.com/SirsiMaster/sirsi-pantheon/internal/ka"
	"github.com/SirsiMaster/sirsi-pantheon/internal/neith"
	"github.com/SirsiMaster/sirsi-pantheon/internal/ra"
	"github.com/SirsiMaster/sirsi-pantheon/internal/vault"
)

// ── Ka / Ghosts API ──────────────────────────────────────────────────

// apiGhosts runs a ghost scan and returns structured results.
// GET /api/ghosts — scans and returns ghost list.
func (s *Server) apiGhosts(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	scanner := ka.NewScanner()
	ghosts, err := scanner.Scan(ctx, false)
	if err != nil {
		writeError(w, fmt.Sprintf("ghost scan: %v", err), http.StatusInternalServerError)
		return
	}

	type ghostJSON struct {
		AppName     string `json:"app_name"`
		BundleID    string `json:"bundle_id"`
		TotalSize   int64  `json:"total_size"`
		TotalFiles  int    `json:"total_files"`
		SizeHuman   string `json:"size_human"`
		InLaunchSvc bool   `json:"in_launch_services"`
		Residuals   []struct {
			Path      string `json:"path"`
			Type      string `json:"type"`
			SizeBytes int64  `json:"size_bytes"`
			FileCount int    `json:"file_count"`
		} `json:"residuals"`
	}

	var result []ghostJSON
	for _, g := range ghosts {
		gj := ghostJSON{
			AppName:     g.AppName,
			BundleID:    g.BundleID,
			TotalSize:   g.TotalSize,
			TotalFiles:  g.TotalFiles,
			SizeHuman:   fmt.Sprintf("%.1f MB", float64(g.TotalSize)/1048576),
			InLaunchSvc: g.InLaunchServices,
		}
		for _, r := range g.Residuals {
			gj.Residuals = append(gj.Residuals, struct {
				Path      string `json:"path"`
				Type      string `json:"type"`
				SizeBytes int64  `json:"size_bytes"`
				FileCount int    `json:"file_count"`
			}{
				Path:      r.Path,
				Type:      string(r.Type),
				SizeBytes: r.SizeBytes,
				FileCount: r.FileCount,
			})
		}
		if gj.Residuals == nil {
			gj.Residuals = make([]struct {
				Path      string `json:"path"`
				Type      string `json:"type"`
				SizeBytes int64  `json:"size_bytes"`
				FileCount int    `json:"file_count"`
			}, 0)
		}
		result = append(result, gj)
	}
	if result == nil {
		result = []ghostJSON{}
	}
	writeJSON(w, result)
}

// apiGhostClean cleans a specific ghost's residuals.
// POST /api/ghosts/clean — body: {"app_name":"...", "dry_run": true}
func (s *Server) apiGhostClean(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, "POST required", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		AppName string `json:"app_name"`
		DryRun  bool   `json:"dry_run"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	scanner := ka.NewScanner()
	ghosts, err := scanner.Scan(ctx, false)
	if err != nil {
		writeError(w, "scan failed", http.StatusInternalServerError)
		return
	}

	for _, g := range ghosts {
		if g.AppName == req.AppName {
			bytesFreed, filesRemoved, err := scanner.Clean(g, req.DryRun, true)
			if err != nil {
				writeError(w, fmt.Sprintf("clean failed: %v", err), http.StatusInternalServerError)
				return
			}
			writeJSON(w, map[string]interface{}{
				"dry_run":       req.DryRun,
				"bytes_freed":   bytesFreed,
				"freed_human":   fmt.Sprintf("%.1f MB", float64(bytesFreed)/1048576),
				"files_removed": filesRemoved,
			})
			return
		}
	}
	writeError(w, "ghost not found: "+req.AppName, http.StatusNotFound)
}

// ── Guard API ────────────────────────────────────────────────────────

// apiDoctor runs a full system health diagnostic.
// GET /api/doctor
func (s *Server) apiDoctor(w http.ResponseWriter, r *http.Request) {
	report, err := guard.Doctor()
	if err != nil {
		writeError(w, fmt.Sprintf("doctor: %v", err), http.StatusInternalServerError)
		return
	}
	writeJSON(w, report)
}

// apiSlay terminates process groups.
// POST /api/slay?target=node&dry_run=true
func (s *Server) apiSlay(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, "POST required", http.StatusMethodNotAllowed)
		return
	}

	target := r.URL.Query().Get("target")
	if !guard.IsValidTarget(target) {
		writeError(w, "invalid target: "+target, http.StatusBadRequest)
		return
	}

	dryRun := r.URL.Query().Get("dry_run") != "false"

	result, err := guard.Slay(guard.SlayTarget(target), dryRun)
	if err != nil {
		writeError(w, fmt.Sprintf("slay: %v", err), http.StatusInternalServerError)
		return
	}

	var errStrings []string
	for _, e := range result.Errors {
		errStrings = append(errStrings, e.Error())
	}

	writeJSON(w, map[string]interface{}{
		"target":  target,
		"dry_run": result.DryRun,
		"killed":  result.Killed,
		"failed":  result.Failed,
		"skipped": result.Skipped,
		"errors":  errStrings,
	})
}

// apiGuardStats returns current system resource stats.
// GET /api/guard/stats
func (s *Server) apiGuardStats(w http.ResponseWriter, r *http.Request) {
	stats, err := guard.GetStats()
	if err != nil {
		writeError(w, fmt.Sprintf("stats: %v", err), http.StatusInternalServerError)
		return
	}
	writeJSON(w, stats)
}

// apiRenice manually renices LSP/background processes.
// POST /api/guard/renice?target=lsp
func (s *Server) apiRenice(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, "POST required", http.StatusMethodNotAllowed)
		return
	}

	target := r.URL.Query().Get("target")
	if target == "" {
		target = "lsp"
	}
	if target != "lsp" && target != "all" {
		writeError(w, "target must be lsp or all", http.StatusBadRequest)
		return
	}

	result, err := guard.Renice(guard.ReniceTarget(target))
	if err != nil {
		writeError(w, fmt.Sprintf("renice: %v", err), http.StatusInternalServerError)
		return
	}

	writeJSON(w, result)
}

// ── Horus Workstation Report ──────────────────────────────────────────

// apiWorkstationReport returns the aggregated local workstation state.
// GET /api/horus/report — the complete picture of this machine.
func (s *Server) apiWorkstationReport(w http.ResponseWriter, r *http.Request) {
	report := horus.WorkstationReport{
		Timestamp: time.Now(),
	}

	// Hostname + platform
	hostname, _ := os.Hostname()
	report.Hostname = hostname
	report.OS = "darwin"  // runtime.GOOS
	report.Arch = "arm64" // runtime.GOARCH

	// Scan summary
	if ps, err := jackal.LoadLatest(); err == nil {
		report.ScanSummary(ps)
	}

	// Health
	if dr, err := guard.Doctor(); err == nil {
		report.HealthScore = dr.Score
	}

	// RAM
	if stats, err := guard.GetStats(); err == nil {
		report.RAMPressure = stats.PressureLevel
	}

	// Stats from dashboard
	if s.cfg.StatsFn != nil {
		if data, err := s.cfg.StatsFn(); err == nil {
			var snap map[string]interface{}
			if json.Unmarshal(data, &snap) == nil {
				if v, ok := snap["ram_percent"].(float64); ok {
					report.RAMPercent = v
				}
				if v, ok := snap["git_branch"].(string); ok {
					report.GitBranch = v
				}
				if v, ok := snap["uncommitted_files"].(float64); ok {
					report.UncommittedFiles = int(v)
				}
			}
		}
	}

	writeJSON(w, report)
}

// ── Horus API ────────────────────────────────────────────────────────

// apiHorusScan scans a directory and returns the symbol graph.
// Uses cache when available, falls back to fresh parse.
// GET /api/horus/scan?path=.
func (s *Server) apiHorusScan(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		path = "."
	}

	// Try cache first
	cache := horus.NewCache()
	if g, ok := cache.Get(path); ok {
		writeJSON(w, g)
		return
	}

	parser := horus.NewGoParser()
	graph, err := parser.ParseDir(path)
	if err != nil {
		writeError(w, fmt.Sprintf("horus scan: %v", err), http.StatusInternalServerError)
		return
	}

	// Cache for next time
	_ = cache.Put(path, graph)
	writeJSON(w, graph)
}

// apiHorusQuery queries symbols from a previously scanned graph.
// GET /api/horus/query?path=.&name=Foo or &kind=func or &filter=*Test*
func (s *Server) apiHorusQuery(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		path = "."
	}

	parser := horus.NewGoParser()
	graph, err := parser.ParseDir(path)
	if err != nil {
		writeError(w, fmt.Sprintf("parse: %v", err), http.StatusInternalServerError)
		return
	}

	q := horus.NewQuery(graph)

	name := r.URL.Query().Get("name")
	kind := r.URL.Query().Get("kind")
	filter := r.URL.Query().Get("filter")

	switch {
	case name != "":
		symbols, _ := q.Lookup(name)
		writeJSON(w, symbols)
	case kind != "":
		symbols := q.ByKind(horus.SymbolKind(kind))
		writeJSON(w, symbols)
	case filter != "":
		symbols := q.MatchSymbols(filter)
		writeJSON(w, symbols)
	default:
		writeJSON(w, graph.Stats)
	}
}

// ── Vault API ────────────────────────────────────────────────────────

// apiVaultSearch runs an FTS5 search on the vault.
// GET /api/vault/search?q=<query>&limit=20
func (s *Server) apiVaultSearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		writeError(w, "missing q parameter", http.StatusBadRequest)
		return
	}

	limit := parseIntParam(r, "limit", 20)

	store, err := vault.Open(vault.DefaultPath())
	if err != nil {
		writeError(w, "vault not available", http.StatusServiceUnavailable)
		return
	}
	defer store.Close()

	result, err := store.Search(query, limit)
	if err != nil {
		writeError(w, fmt.Sprintf("search: %v", err), http.StatusInternalServerError)
		return
	}
	writeJSON(w, result)
}

// apiVaultStats returns vault statistics.
// GET /api/vault/stats
func (s *Server) apiVaultStats(w http.ResponseWriter, r *http.Request) {
	store, err := vault.Open(vault.DefaultPath())
	if err != nil {
		writeJSON(w, map[string]interface{}{"totalEntries": 0, "error": "vault not available"})
		return
	}
	defer store.Close()

	stats, err := store.Stats()
	if err != nil {
		writeError(w, fmt.Sprintf("stats: %v", err), http.StatusInternalServerError)
		return
	}
	writeJSON(w, stats)
}

// apiVaultPrune removes old vault entries.
// POST /api/vault/prune?older_than=720h
func (s *Server) apiVaultPrune(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, "POST required", http.StatusMethodNotAllowed)
		return
	}

	olderThan := r.URL.Query().Get("older_than")
	if olderThan == "" {
		olderThan = "720h" // 30 days default
	}

	dur, err := time.ParseDuration(olderThan)
	if err != nil {
		writeError(w, "invalid duration: "+olderThan, http.StatusBadRequest)
		return
	}

	store, err := vault.Open(vault.DefaultPath())
	if err != nil {
		writeError(w, "vault not available", http.StatusServiceUnavailable)
		return
	}
	defer store.Close()

	removed, err := store.Prune(dur)
	if err != nil {
		writeError(w, fmt.Sprintf("prune: %v", err), http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]interface{}{
		"removed":    removed,
		"older_than": olderThan,
	})
}

// ── Ra API ───────────────────────────────────────────────────────────

// apiRaStatus returns the current Ra deployment state.
// GET /api/ra/status
func (s *Server) apiRaStatus(w http.ResponseWriter, r *http.Request) {
	raDir := ra.RADir()
	status, err := ra.Monitor(raDir)
	if err != nil {
		// No deployment — return empty state
		writeJSON(w, map[string]interface{}{
			"deployed": false,
			"windows":  []interface{}{},
		})
		return
	}

	type windowJSON struct {
		Name     string `json:"name"`
		PID      int    `json:"pid"`
		State    string `json:"state"`
		ExitCode int    `json:"exit_code"`
		LogTail  string `json:"log_tail"`
		Duration string `json:"duration"`
	}

	var windows []windowJSON
	for _, w := range status.Windows {
		windows = append(windows, windowJSON{
			Name:     w.Name,
			PID:      w.PID,
			State:    w.State,
			ExitCode: w.ExitCode,
			LogTail:  w.LogTail,
			Duration: w.Duration.Truncate(time.Second).String(),
		})
	}
	if windows == nil {
		windows = []windowJSON{}
	}

	writeJSON(w, map[string]interface{}{
		"deployed":   true,
		"started_at": status.StartedAt.Format(time.RFC3339),
		"all_done":   status.AllDone,
		"windows":    windows,
	})
}

// apiRaScopes returns available scope configurations.
// GET /api/ra/scopes
func (s *Server) apiRaScopes(w http.ResponseWriter, r *http.Request) {
	// Find scope config directory
	configDir := filepath.Join(findRepoRoot(), "configs", "scopes")
	loom := neith.NewLoom(configDir)
	scopes, err := loom.LoadScopes()
	if err != nil {
		writeJSON(w, []interface{}{})
		return
	}

	type scopeJSON struct {
		Name        string `json:"name"`
		DisplayName string `json:"display_name"`
		RepoPath    string `json:"repo_path"`
		Priority    string `json:"priority"`
		Deadline    string `json:"deadline"`
		Sprints     int    `json:"sprints"`
	}

	var result []scopeJSON
	for _, sc := range scopes {
		result = append(result, scopeJSON{
			Name:        sc.Name,
			DisplayName: sc.DisplayName,
			RepoPath:    sc.RepoPath,
			Priority:    sc.Priority,
			Deadline:    sc.Deadline,
			Sprints:     sc.Sprints,
		})
	}
	if result == nil {
		result = []scopeJSON{}
	}
	writeJSON(w, result)
}

// findRepoRoot finds the sirsi-sirsi repo root.
func findRepoRoot() string {
	// Try from binary location
	exe, _ := os.Executable()
	dir := filepath.Dir(exe)
	for d := dir; d != "/"; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			return d
		}
	}
	// Fallback
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Development", "sirsi-pantheon")
}
