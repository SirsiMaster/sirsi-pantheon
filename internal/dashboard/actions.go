package dashboard

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// ActionSpec is one entry in the frozen action registry (E1). It maps a
// canonical action key to the CLI verb it runs and declares whether the action
// is destructive (requires the E2 two-phase confirm) and whether it accepts
// caller-supplied positional args. Every surface discovers the action set via
// GET /api/actions and invokes it via the typed POST /api/run — no surface
// hardcodes its own command semantics.
type ActionSpec struct {
	Key         string   `json:"key"`          // canonical action key, e.g. "scan", "ra/kill"
	Label       string   `json:"label"`        // human-readable label
	Glyph       string   `json:"glyph"`        // deity/brand glyph
	Args        []string `json:"-"`            // base CLI args (server-internal; never client-supplied)
	Destructive bool     `json:"destructive"`  // true => POST /api/run requires the confirm token flow
	AcceptsArgs bool     `json:"accepts_args"` // true => ActionRequest.Args is appended (e.g. dedup path)
}

// actionSpecs is the canonical action registry. It folds the legacy
// DefaultActions() set together with the E1 gap-list actions so the entire
// menubar/TUI command set is reachable through one typed contract.
//
// Destructive actions (network/fix, ra/deploy, ra/kill) are gated by the E2
// confirm engine; the remaining actions stream output via the runner+SSE path.
func actionSpecs() []ActionSpec {
	return []ActionSpec{
		// Read / scan / report (streamed via runner+SSE)
		{Key: "scan", Label: "Scan", Glyph: "𓁢", Args: []string{"scan"}},
		{Key: "ghosts", Label: "Ghost Hunt", Glyph: "𓂓", Args: []string{"ghosts"}},
		{Key: "doctor", Label: "System Diagnostics", Glyph: "𓁐", Args: []string{"diagnose"}},
		{Key: "router/doctor", Label: "Router Doctor", Glyph: "𓇶", Args: []string{"router", "doctor", "--fix"}},
		{Key: "guard", Label: "Guard Check", Glyph: "🛡", Args: []string{"guard"}},
		{Key: "quality", Label: "Quality Audit", Glyph: "𓆄", Args: []string{"audit"}},
		{Key: "audit", Label: "Audit", Glyph: "𓆄", Args: []string{"audit"}},
		{Key: "maat", Label: "Ma'at Feather", Glyph: "𓆄", Args: []string{"maat"}},
		{Key: "risk", Label: "Risk", Glyph: "⚖", Args: []string{"risk"}},
		{Key: "network", Label: "Network Audit", Glyph: "🌐", Args: []string{"network"}},
		{Key: "hardware", Label: "Hardware", Glyph: "⚡", Args: []string{"hardware"}},
		{Key: "compute", Label: "Compute Profile", Glyph: "⚡", Args: []string{"hardware"}},
		{Key: "status", Label: "System Status", Glyph: "𓂀", Args: []string{"status"}},
		{Key: "gemma/serve", Label: "Start / Check Gemma Broker", Glyph: "☥", Args: []string{"gemma", "serve"}},
		{Key: "gemma/status", Label: "SNE Status", Glyph: "☥", Args: []string{"gemma", "serve", "--status"}},
		{Key: "router/ledger", Label: "Fabric Ledger", Glyph: "𓂀", Args: []string{"router", "ledger"}},
		{Key: "dedup", Label: "Find Duplicates", Glyph: "🔍", Args: []string{"duplicates"}, AcceptsArgs: true},
		{Key: "thoth/sync", Label: "Thoth Sync", Glyph: "𓁟", Args: []string{"thoth", "sync"}},
		{Key: "seshat/ingest", Label: "Seshat Ingest", Glyph: "𓄿", Args: []string{"seshat", "ingest"}, AcceptsArgs: true},
		{Key: "net/align", Label: "Net Align", Glyph: "𓁯", Args: []string{"net", "align"}},
		{Key: "ra/collect", Label: "Ra Collect", Glyph: "𓇶", Args: []string{"ra", "collect"}},

		// Destructive / high-impact (E2 confirm token required)
		{Key: "network/fix", Label: "Network Fix", Glyph: "🌐", Args: []string{"network", "--fix"}, Destructive: true},
		{Key: "system/fix", Label: "Repair Diagnosed Issues", Glyph: "𓁐", Args: []string{"fix"}, Destructive: true},
		{Key: "update/app", Label: "Install Signed Pantheon Update", Glyph: "↻", Args: []string{"update", "--app"}, Destructive: true},
		{Key: "gemma/stop", Label: "Stop SNE", Glyph: "☥", Args: []string{"gemma", "serve", "--stop"}, Destructive: true},
		{Key: "gemma/restore", Label: "Restore SNE", Glyph: "☥", Args: []string{"gemma", "serve", "--restore"}, Destructive: true},
		{Key: "gemma/quarantine", Label: "Quarantine SNE", Glyph: "☥", Args: []string{"gemma", "serve", "--quarantine"}, Destructive: true},
		{Key: "ra/deploy", Label: "Ra Deploy", Glyph: "𓇶", Args: []string{"ra", "deploy"}, Destructive: true, AcceptsArgs: true},
		{Key: "ra/kill", Label: "Ra Kill", Glyph: "𓇶", Args: []string{"ra", "kill"}, Destructive: true, AcceptsArgs: true},
	}
}

// ActionSpecs returns a defensive copy of the canonical cross-surface action
// registry. Menubar, TUI, MCP, and future native clients must consume this
// identity rather than hardcoding their own command semantics.
func ActionSpecs() []ActionSpec {
	specs := actionSpecs()
	out := make([]ActionSpec, len(specs))
	for i, spec := range specs {
		out[i] = spec
		out[i].Args = append([]string(nil), spec.Args...)
	}
	return out
}

// LookupAction resolves one canonical action for trusted local surfaces.
func LookupAction(key string) (ActionSpec, bool) { return lookupAction(key) }

// lookupAction returns the spec for a canonical action key.
func lookupAction(key string) (ActionSpec, bool) {
	for _, sp := range actionSpecs() {
		if sp.Key == key {
			return sp, true
		}
	}
	return ActionSpec{}, false
}

// apiActions handles GET /api/actions — the action discovery endpoint. Surfaces
// call it to learn the available actions and which require confirmation.
func (s *Server) apiActions(w http.ResponseWriter, r *http.Request) {
	specs := ActionSpecs()
	if specs == nil {
		specs = []ActionSpec{}
	}
	writeJSON(w, specs)
}

// dispatchRun implements the typed POST /api/run contract (E5), backing-compatible
// with the legacy ?cmd=<key> query form. Destructive actions are routed through
// the E2 confirm engine: a request without a confirm token returns a
// PreparedAction (dry-run/prepare); a request with a valid token commits.
func (s *Server) dispatchRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	if s.runner == nil {
		writeError(w, "runner not available", http.StatusServiceUnavailable)
		return
	}

	// Legacy query form: ?cmd=<key>. Back-compat for existing callers, but it
	// can never execute a destructive action (no confirm channel).
	if !strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		key := r.URL.Query().Get("cmd")
		if key == "" {
			writeError(w, "missing cmd parameter (or send a JSON ActionRequest body)", http.StatusBadRequest)
			return
		}
		if sp, ok := lookupAction(key); ok && sp.Destructive {
			writeError(w, "destructive action requires a JSON ActionRequest with the confirm flow", http.StatusBadRequest)
			return
		}
		if err := s.runner.Run(key); err != nil {
			writeError(w, err.Error(), http.StatusConflict)
			return
		}
		writeJSON(w, ActionResult{Status: "started", Action: key})
		return
	}

	var req ActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "invalid ActionRequest body", http.StatusBadRequest)
		return
	}
	spec, ok := lookupAction(req.Action)
	if !ok {
		writeError(w, fmt.Sprintf("unknown action: %q", req.Action), http.StatusBadRequest)
		return
	}

	// Compose the command: server-defined base args, plus caller positional args
	// only when the spec opts in (prevents arbitrary arg injection).
	args := append([]string{}, spec.Args...)
	if spec.AcceptsArgs {
		args = append(args, req.Args...)
	}

	if spec.Destructive {
		if s.confirm == nil {
			writeError(w, "confirm guard not available", http.StatusServiceUnavailable)
			return
		}
		// Bind the executable positional args into the confirm fingerprint so a
		// token prepared for one arg set can never commit a different one (codex
		// P1). Args ARE executable input for AcceptsArgs destructive actions
		// (ra/deploy, ra/kill), so they must be part of what the token authorizes.
		confirmParams := req.Params
		if spec.AcceptsArgs && len(req.Args) > 0 {
			confirmParams = make(map[string]string, len(req.Params)+1)
			for k, v := range req.Params {
				confirmParams[k] = v
			}
			confirmParams["__args"] = strings.Join(req.Args, "\x00")
		}
		// Phase 1 — no token: return a prepared (dry-run) action with a token.
		if req.ConfirmToken == "" {
			preview := fmt.Sprintf("Would run: sirsi %s", strings.Join(args, " "))
			prep, err := s.confirm.Prepare(req.Action, req.Target, confirmParams, preview, nil, "")
			if err != nil {
				writeError(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, prep)
			return
		}
		// Phase 2 — token present: validate before executing for real.
		if err := s.confirm.Validate(req.ConfirmToken, req.Action, req.Target, confirmParams, req.ActionHash); err != nil {
			writeError(w, err.Error(), http.StatusForbidden)
			return
		}
	}

	if err := s.runner.RunArgs(spec.Key, spec.Label, spec.Glyph, args); err != nil {
		writeError(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, ActionResult{Status: "started", Action: req.Action})
}
