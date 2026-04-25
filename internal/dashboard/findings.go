package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/jackal"
	"github.com/SirsiMaster/sirsi-pantheon/internal/jackal/rules"
)

// apiFindings serves the latest persisted scan findings.
// GET /api/findings — returns the full PersistedScan JSON.
func (s *Server) apiFindings(w http.ResponseWriter, r *http.Request) {
	scan, err := jackal.LoadLatest()
	if err != nil {
		writeJSON(w, map[string]interface{}{
			"findings": []interface{}{},
			"error":    "No scan results. Run a scan first.",
		})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(scan)
}

// cleanRequest is the payload for POST /api/clean.
type cleanRequest struct {
	Indices []int `json:"indices"` // finding indices to clean
	DryRun  bool  `json:"dry_run"`
}

// apiClean handles POST /api/clean — cleans selected findings.
// Requires findings indices. Runs dry-run by default.
func (s *Server) apiClean(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, "POST required", http.StatusMethodNotAllowed)
		return
	}

	var req cleanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	// Load persisted findings.
	persisted, err := jackal.LoadLatest()
	if err != nil {
		writeError(w, "no scan results available", http.StatusNotFound)
		return
	}

	// Validate indices and convert to engine Findings.
	var findings []jackal.Finding
	for _, idx := range req.Indices {
		if idx < 0 || idx >= len(persisted.Findings) {
			writeError(w, fmt.Sprintf("invalid finding index: %d", idx), http.StatusBadRequest)
			return
		}
		pf := persisted.Findings[idx]
		f := jackal.Finding{
			RuleName:    pf.RuleName,
			Category:    pf.Category,
			Description: pf.Description,
			Path:        pf.Path,
			SizeBytes:   pf.SizeBytes,
			Severity:    pf.Severity,
			IsDir:       pf.IsDir,
			FileCount:   pf.FileCount,
		}
		if pf.LastModified != "" {
			f.LastModified, _ = time.Parse(time.RFC3339, pf.LastModified)
		}
		findings = append(findings, f)
	}

	if len(findings) == 0 {
		writeError(w, "no findings selected", http.StatusBadRequest)
		return
	}

	// Build engine with all rules.
	engine := jackal.DefaultEngine()
	engine.RegisterAll(rules.AllRules()...)

	opts := jackal.CleanOptions{
		DryRun:   req.DryRun,
		Confirm:  !req.DryRun,
		UseTrash: true,
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	result, err := engine.Clean(ctx, findings, opts)
	if err != nil {
		writeError(w, fmt.Sprintf("clean failed: %v", err), http.StatusInternalServerError)
		return
	}

	var errStrings []string
	for _, e := range result.Errors {
		errStrings = append(errStrings, e.Error())
	}

	writeJSON(w, map[string]interface{}{
		"dry_run":     req.DryRun,
		"cleaned":     result.Cleaned,
		"bytes_freed": result.BytesFreed,
		"freed_human": jackal.FormatSize(result.BytesFreed),
		"skipped":     result.Skipped,
		"errors":      errStrings,
	})
}
