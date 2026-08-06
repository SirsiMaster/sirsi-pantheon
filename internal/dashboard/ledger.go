// Package dashboard — ledger.go
//
// Serves GET /api/ledger — the compact ledger.BoardSummary read-model
// consumed by the Nexus board renderer (A26 seam). Follows the same
// dependency-injection pattern as /api/node-status (ADR-026): the producer
// function is injected via Config.LedgerFn so the handler is nil-safe and
// testable without touching the filesystem.
package dashboard

import (
	"net/http"

	"github.com/SirsiMaster/sirsi-pantheon/internal/ledger"
)

// LedgerSummarizer is the producer hook for GET /api/ledger.
// Tests inject a deterministic ledger.BoardSummary without filesystem access.
// Production wiring (cmd/sirsi-menubar) passes a closure over ledger.Build + Summarize.
type LedgerSummarizer func() (ledger.BoardSummary, error)

// apiLedger serves GET /api/ledger (A26 Nexus seam).
// Returns the compact ledger.BoardSummary JSON expected by the Nexus board renderer.
// Read-only: no method-gating, no ConfirmGuard path, no side effects.
// CORS scoping matches /api/node-status (allowNexusOrigin, ADR-047).
func (s *Server) apiLedger(w http.ResponseWriter, r *http.Request) {
	allowNexusOrigin(w, r)
	if s.cfg.LedgerFn == nil {
		writeError(w, "ledger not available (summarizer not wired)", http.StatusServiceUnavailable)
		return
	}
	bs, err := s.cfg.LedgerFn()
	if err != nil {
		writeError(w, "ledger summary failed", http.StatusInternalServerError)
		return
	}
	writeJSON(w, bs)
}
