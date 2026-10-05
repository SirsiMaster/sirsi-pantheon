package dashboard

import "time"

// This file defines the FROZEN typed request/response contract for the
// dashboard action API (ADR-020 surface-ladder; codex freeze gate item 162436,
// scope E1/E2/E3/E5). Every surface — CLI, menubar, TUI, SwiftUI — consumes
// these types instead of inventing its own action semantics. Additive-only:
// existing untyped handlers keep working; new/typed handlers use these shapes.

// ActionRequest is the typed body for POST /api/run and the action endpoints.
// It replaces the legacy query-only `?cmd=<key>` form (E5).
//
// Destructive actions use a two-phase flow (E2):
//   - Phase 1 (prepare): omit ConfirmToken. The server executes a read-only
//     dry-run and returns a PreparedAction with a one-time ConfirmToken.
//   - Phase 2 (commit): resend the SAME Action/Target/Params plus the
//     ConfirmToken. The server validates the token before executing for real.
//
// DryRun is advisory for non-destructive actions. For destructive actions the
// ABSENCE of a valid ConfirmToken always means dry-run/prepare — an omitted
// DryRun can never trigger destructive execution (Rule A1).
type ActionRequest struct {
	Action       string            `json:"action"`                  // canonical action key, e.g. "scan", "ra/kill"
	Args         []string          `json:"args,omitempty"`          // extra positional args passed to the verb
	Params       map[string]string `json:"params,omitempty"`        // flag-style parameters
	Target       string            `json:"target,omitempty"`        // primary resource the action affects (destructive ops)
	DryRun       bool              `json:"dry_run,omitempty"`       // explicit dry-run request for non-destructive actions
	ConfirmToken string            `json:"confirm_token,omitempty"` // present only on the commit phase of a destructive action
	ActionHash   string            `json:"action_hash,omitempty"`   // echoed fingerprint from the prepare phase (integrity check)
}

// ActionResult is the typed response for a started/queued/completed action.
type ActionResult struct {
	Status  string `json:"status"`            // "started" | "done" | "error"
	Action  string `json:"action"`            // echoes the requested action key
	RunID   string `json:"run_id,omitempty"`  // correlation id for SSE run_output/run_complete events
	Message string `json:"message,omitempty"` // human-readable detail
	Error   string `json:"error,omitempty"`   // populated when Status == "error"
}

// PreparedAction is returned by the prepare/dry-run phase of a destructive
// action (E2). The client must resend Action/Target/Params plus ConfirmToken
// (and may echo ActionHash) to commit. The token is single-use and expires.
type PreparedAction struct {
	Action            string    `json:"action"`
	Target            string    `json:"target"`
	DryRun            bool      `json:"dry_run"`            // always true for a prepare response
	ConfirmToken      string    `json:"confirm_token"`      // opaque, single-use
	ActionHash        string    `json:"action_hash"`        // stable fingerprint of (action, target, sorted params)
	ExpiresAt         time.Time `json:"expires_at"`         // token validity deadline
	Preview           string    `json:"preview"`            // human-readable description of what commit would do
	AffectedResources []string  `json:"affected_resources"` // concrete resources (paths, PIDs, scopes) the commit touches
	EstimatedImpact   string    `json:"estimated_impact"`   // e.g. "12 files, ~340 MB"
}

// RaScopeStatus mirrors the menubar producer's per-scope status so StatsResponse
// can decode the StatsFn bytes without importing cmd/sirsi-menubar (which is
// forbidden pre-freeze). JSON tags MUST match cmd/sirsi-menubar/stats.go.
type RaScopeStatus struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Window string `json:"window,omitempty"`
}

// StatsResponse is the typed shape of GET /api/stats, replacing the opaque
// []byte passthrough (E3). The JSON tags are identical to the menubar's
// StatsSnapshot so the producer's bytes decode cleanly into this contract type
// at the HTTP boundary. Surfaces decode into StatsResponse rather than guessing
// at an untyped map.
type StatsResponse struct {
	// RAM
	TotalRAM    int64   `json:"total_ram"`
	UsedRAM     int64   `json:"used_ram"`
	FreeRAM     int64   `json:"free_ram"`
	RAMPercent  float64 `json:"ram_percent"`
	RAMPressure string  `json:"ram_pressure"` // "low" | "medium" | "high"
	RAMIcon     string  `json:"ram_icon"`

	// Git / Osiris
	UncommittedFiles int    `json:"uncommitted_files"`
	TimeSinceCommit  string `json:"time_since_commit"`
	GitBranch        string `json:"git_branch"`
	OsirisRisk       string `json:"osiris_risk"`
	OsirisIcon       string `json:"osiris_icon"`

	// Accelerator
	PrimaryAccelerator string `json:"primary_accelerator"`
	AccelIcon          string `json:"accel_icon"`

	// Active Deities
	ActiveDeities   []string `json:"active_deities"`
	DeityCount      int      `json:"deity_count"`
	Components      []string `json:"components"`
	ComponentCount  int      `json:"component_count"`
	ComponentsKnown bool     `json:"components_known"`

	// Ra Deployment
	RaDeployed bool            `json:"ra_deployed"`
	RaScopes   []RaScopeStatus `json:"ra_scopes"`
	RaIcon     string          `json:"ra_icon"`

	// Disk
	DiskWasteEstimate string `json:"disk_waste_estimate"`

	// Meta
	Timestamp   time.Time `json:"timestamp"`
	CollectedIn string    `json:"collected_in"`
}
