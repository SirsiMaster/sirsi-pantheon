package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
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
	Indices      []int  `json:"indices"`                 // finding indices to clean
	DryRun       bool   `json:"dry_run"`                 // legacy/explicit dry-run (preview only)
	ConfirmToken string `json:"confirm_token,omitempty"` // E2: present to commit a real deletion
	ActionHash   string `json:"action_hash,omitempty"`   // E2: echoed fingerprint from the prepare phase
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

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	// E2 confirm contract: a real deletion requires a valid confirm token.
	// Without one we ALWAYS run a dry-run preview and hand back a token — an
	// omitted dry_run can never delete (Rule A1). The action is keyed on the
	// concrete finding paths so the token can authorize only this exact set.
	var affected []string
	var previewBytes int64
	for _, f := range findings {
		affected = append(affected, f.Path)
		previewBytes += f.SizeBytes
	}
	target := strings.Join(affected, "\n")
	params := map[string]string{"count": strconv.Itoa(len(findings))}

	if req.ConfirmToken == "" && !req.DryRun {
		preview := fmt.Sprintf("Would delete %d item(s), ~%s", len(findings), jackal.FormatSize(previewBytes))
		if !s.requireConfirm(w, "clean", target, params, "", "", preview, affected, jackal.FormatSize(previewBytes)) {
			return // requireConfirm wrote the PreparedAction (token) — caller stops
		}
	} else if req.ConfirmToken != "" {
		if vErr := s.confirm.Validate(req.ConfirmToken, "clean", target, params, req.ActionHash); vErr != nil {
			writeError(w, vErr.Error(), http.StatusForbidden)
			return
		}
	}

	// Real deletion only when a token validated; otherwise this is an explicit
	// dry_run:true preview request.
	commit := req.ConfirmToken != ""
	opts := jackal.CleanOptions{
		DryRun:   !commit,
		Confirm:  commit,
		UseTrash: true,
	}

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
		"dry_run":     !commit,
		"cleaned":     result.Cleaned,
		"bytes_freed": result.BytesFreed,
		"freed_human": jackal.FormatSize(result.BytesFreed),
		"skipped":     result.Skipped,
		"errors":      errStrings,
	})
}
