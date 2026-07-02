// Package brain — controlplane.go is the Orchestration Brain's live control
// plane: the read-model behind `sirsi brain {status,doctor,levels}` and the
// Dispatcher seam the brain runs over.
//
// REUSE, NOT REINVENT (Rule 0 / the whole point of this partition): the wake +
// registry substrate ALREADY EXISTS and is mature in internal/router
// (WakePass / ProbeWakeReadiness / InstallWakeLaunchAgent / RunWakeLoop +
// wakemechanism.go). This file does NOT rebuild any of it. It SURFACES and
// ENFORCES the Tier-0 Registry/Wake invariant (A29 §Tier-0) by reading that
// existing API, and it reads guard.NodeCapacity for the RAM gate. The brain is a
// CONSUMER of the wake system, not a second copy of it.
package brain

import (
	"fmt"
	"path/filepath"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
)

// Dispatcher is the Tier-0 seam the brain runs over. It abstracts the router so
// the brain's control plane is built against a stable interface TODAY (the
// current file router) and Router v2 can swap in underneath the same interface
// later without touching the brain (plan Amendment 1 — decouple from Router v2).
//
// It is deliberately minimal: the brain reads liveness/wake-readiness to enforce
// the registry invariant and (later phases) route/close/heartbeat. Send/Close
// are NOT added until Tier-1 triage needs them (P2) — no speculative surface.
type Dispatcher interface {
	// NodeStatus returns the router's registry + liveness read-model (who is
	// registered, which threads are live/armed, which inboxes are stranded).
	NodeStatus() (*router.NodeStatus, error)
	// WakeReadiness reports, per registered agent, whether the router can wake
	// it and how — the existing ProbeWakeReadiness, surfaced for the brain.
	WakeReadiness() ([]router.AgentWakeHealth, error)
}

// UnreachableDispatcher is the degrade-don't-die Dispatcher for a machine with
// no reachable router fabric (fresh install, no .agents/idea-router anywhere up
// the tree). Every method returns the constructor error, so Collect/Doctor's
// existing degradation paths render config + level and a plain-English
// registry/wake diagnosis instead of the whole verb dying — "a dead router must
// not blank the brain view" (the contract above), now true from the constructor
// down.
type UnreachableDispatcher struct{ Err error }

func (u UnreachableDispatcher) NodeStatus() (*router.NodeStatus, error) { return nil, u.Err }
func (u UnreachableDispatcher) WakeReadiness() ([]router.AgentWakeHealth, error) {
	return nil, u.Err
}

// RouterDispatcher is the production Dispatcher backed by the existing file
// router. It holds only the repo root; every call reads live from disk so the
// brain never caches a stale registry.
type RouterDispatcher struct {
	repoRoot   string
	routerRoot string
}

// NewRouterDispatcher builds a Dispatcher over the real router, locating the
// canonical repo/router root via the existing router.FindRepoRoot (which already
// resolves to the main worktree, ADR-029 Amendment 1 — we do not re-derive it).
func NewRouterDispatcher() (*RouterDispatcher, error) {
	repoRoot, err := router.FindRepoRoot()
	if err != nil {
		return nil, fmt.Errorf("locate repo root: %w", err)
	}
	return &RouterDispatcher{
		repoRoot:   repoRoot,
		routerRoot: filepath.Join(repoRoot, ".agents", "idea-router"),
	}, nil
}

// NodeStatus reads the router's live registry + liveness model.
func (d *RouterDispatcher) NodeStatus() (*router.NodeStatus, error) {
	return router.CollectNodeStatus(d.repoRoot, nil)
}

// WakeReadiness probes every registered agent through the EXISTING
// ProbeWakeReadiness — no new wake logic, just surfaced for the brain.
func (d *RouterDispatcher) WakeReadiness() ([]router.AgentWakeHealth, error) {
	reg, err := router.LoadRegistry(d.routerRoot)
	if err != nil {
		return nil, fmt.Errorf("load agents: %w", err)
	}
	out := make([]router.AgentWakeHealth, 0, len(reg.Agents))
	for _, cfg := range reg.Agents {
		out = append(out, router.ProbeWakeReadiness(cfg))
	}
	return out, nil
}

// BrainStatus is the brain's operator read-model: the active tier/level per role
// plus the Tier-0 substrate health (registry + wake) and node RAM headroom.
// (Named BrainStatus, not Status, to avoid colliding with the model-download
// Status in downloader.go — same package, different concern.)
type BrainStatus struct {
	Level    int
	Roles    []RoleStatus
	Registry RegistryHealth
	Node     NodeHeadroom
}

// RoleStatus is one role's plugged provider + its derived tier.
type RoleStatus struct {
	Role     Role
	Provider Provider
	Tier     string // "Tier-0"/"Tier-1"/"Tier-2" — the tier this role drives
}

// RegistryHealth is the Tier-0 Registry/Wake invariant, surfaced: how many
// registered agents are wakeable vs unwakeable (the zombie condition A29 forbids).
type RegistryHealth struct {
	Registered int
	Wakeable   int      // agents with a live/ready wake channel (ProbeWakeReadiness.Ready)
	Unwakeable []string // agent IDs registered but with no ready channel — surfaced, never hidden
	Stranded   int      // agents with open items but no armed watcher
}

// NodeHeadroom is the RAM gate the brain consults before a model load — read
// straight from guard.NodeCapacity (the resource broker), not re-measured.
type NodeHeadroom struct {
	FreeRAM  int64
	TotalRAM int64
	Pressure string
}

// tierForRole maps a role's provider to the tier it drives, honoring the
// deterministic floor (a role on ProviderNone is Tier-0 regardless).
func tierForRole(role Role, p Provider) string {
	if p.Kind == ProviderNone {
		return "Tier-0"
	}
	switch role {
	case RoleTriage:
		return "Tier-1"
	case RoleExecution:
		return "Tier-2"
	default:
		return "Tier-0"
	}
}

// Collect builds the full brain Status from the config + the Dispatcher +
// NodeCapacity. It never fails on a partial substrate: registry/node problems
// are reported IN the Status (Unwakeable list, zeroed headroom), so `status`
// always renders and `doctor` can diagnose — a dead router must not blank the
// brain view.
func Collect(cfg Config, d Dispatcher) BrainStatus {
	st := BrainStatus{Level: cfg.Level()}
	for _, r := range Roles() {
		p := cfg.Provider(r)
		st.Roles = append(st.Roles, RoleStatus{Role: r, Provider: p, Tier: tierForRole(r, p)})
	}

	if ns, err := d.NodeStatus(); err == nil && ns != nil {
		st.Registry.Registered = ns.AgentCount
		st.Registry.Stranded = len(ns.StrandedInbox)
	}
	if hs, err := d.WakeReadiness(); err == nil {
		for _, h := range hs {
			if h.Ready {
				st.Registry.Wakeable++
			} else {
				st.Registry.Unwakeable = append(st.Registry.Unwakeable, h.AgentID)
			}
		}
	}

	nc := sampleNodeCapacity()
	st.Node = NodeHeadroom{FreeRAM: nc.FreeRAM, TotalRAM: nc.TotalRAM, Pressure: nc.Pressure.String()}
	return st
}

// sampleNodeCapacity is an injectable seam (A16) so doctor/status tests run
// without touching real hardware. Defaults to the live guard sampler.
var sampleNodeCapacity = guard.SampleNodeCapacity

// Diagnosis is one `sirsi brain doctor` finding: a plain-English problem + fix.
// Ok=true means the check passed (shown as a ✓, not an alarm) — per
// [[feedback_surfaces_current_actionable_only]], only real, currently-fixable
// conditions are flagged as problems.
type Diagnosis struct {
	Check  string
	Ok     bool
	Detail string
	Fix    string // the plain-English remedy; empty when Ok
}

// Doctor troubleshoots the brain in plain English (A29 §doctor). It layers on
// top of the EXISTING substrate: config hygiene (Validate), the Tier-0
// Registry/Wake invariant (via the surfaced wake readiness), the RAM gate (via
// NodeCapacity.Fits), and — for model providers — whether the node can hold the
// model. It NEVER wakes or mutates anything; `sirsi router doctor --fix` remains
// the actor (it runs WakePass). doctor here is the brain's READ over that.
func Doctor(cfg Config, d Dispatcher) []Diagnosis {
	var out []Diagnosis

	// 1. Config hygiene + the deterministic-floor invariant.
	if verrs := cfg.Validate(); len(verrs) > 0 {
		for _, e := range verrs {
			out = append(out, Diagnosis{
				Check:  "config",
				Ok:     false,
				Detail: e.Error(),
				Fix:    "edit ~/.sirsi/brain.yaml (or `sirsi brain use <role> <provider>`) to a valid value",
			})
		}
	} else {
		out = append(out, Diagnosis{Check: "config", Ok: true, Detail: "brain.yaml valid; Tier-0 dispatch stays deterministic"})
	}

	// 2. Tier-0 Registry/Wake invariant — SURFACE + ENFORCE the existing wake API.
	// "registration binds a persistent wake-channel; no live channel ⇒ surface"
	// (A29 §Tier-0). We read ProbeWakeReadiness (the existing probe); we do NOT
	// re-implement the wake decision.
	if hs, err := d.WakeReadiness(); err != nil {
		out = append(out, Diagnosis{Check: "registry/wake", Ok: false, Detail: "cannot read the router registry: " + err.Error(), Fix: "run `sirsi router doctor` — the router fabric may be unreachable"})
	} else {
		var unwakeable []string
		for _, h := range hs {
			if !h.Ready {
				unwakeable = append(unwakeable, fmt.Sprintf("%s (%s)", h.AgentID, h.Detail))
			}
		}
		if len(unwakeable) == 0 {
			out = append(out, Diagnosis{Check: "registry/wake", Ok: true, Detail: fmt.Sprintf("all %d registered agent(s) have a live wake-channel", len(hs))})
		} else {
			out = append(out, Diagnosis{
				Check:  "registry/wake",
				Ok:     false,
				Detail: fmt.Sprintf("%d registered agent(s) have no live wake-channel: %v", len(unwakeable), unwakeable),
				Fix:    "arm the channel: `sirsi router wake-install <agent>` (headless) or re-arm the agent's /loop watcher (interactive); then `sirsi router doctor --fix` runs the wake pass",
			})
		}
	}

	// 3. Stranded inboxes (open items, no armed watcher) — the concrete symptom
	// the invariant prevents.
	if ns, err := d.NodeStatus(); err == nil && ns != nil && len(ns.StrandedInbox) > 0 {
		var names []string
		for _, s := range ns.StrandedInbox {
			names = append(names, fmt.Sprintf("%s (%d item[s])", s.AgentID, s.OpenItems))
		}
		out = append(out, Diagnosis{
			Check:  "stranded-inbox",
			Ok:     false,
			Detail: fmt.Sprintf("%d agent(s) have unread items and no armed watcher: %v", len(names), names),
			Fix:    "`sirsi router doctor --fix` runs the wake-or-declare-unavailable pass (never blind-spawns interactive agents)",
		})
	} else {
		out = append(out, Diagnosis{Check: "stranded-inbox", Ok: true, Detail: "no stranded inboxes"})
	}

	// 4. RAM gate — before any local/hosted model role, can the node hold a
	// model? Reuses guard.NodeCapacity.Fits (the resource-broker refuse gate);
	// reports "won't fit — N GB short" instead of letting it OOM (Amendment 3).
	nc := sampleNodeCapacity()
	needsModel := false
	for _, r := range Roles() {
		if cfg.Provider(r).Kind == ProviderLocal {
			needsModel = true
		}
	}
	switch {
	case !needsModel:
		out = append(out, Diagnosis{Check: "ram", Ok: true, Detail: "Level 0 or hosted-only — no local model to fit"})
	case nc.Fits(defaultModelBytes):
		out = append(out, Diagnosis{Check: "ram", Ok: true, Detail: fmt.Sprintf("node can hold a local model (%s free)", humanBytes(nc.FreeRAM))})
	default:
		short := (2*defaultModelBytes + nc.DynamicReserve()) - nc.FreeRAM
		out = append(out, Diagnosis{
			Check:  "ram",
			Ok:     false,
			Detail: fmt.Sprintf("a local triage/execution model won't fit — about %s short (%s free)", humanBytes(short), humanBytes(nc.FreeRAM)),
			Fix:    "free RAM (`sirsi relieve` / close heavy apps) or drop the role to a smaller model / `sirsi brain use <role> none`",
		})
	}

	return out
}

// defaultModelBytes is the conservative model size the RAM gate assumes when the
// concrete model is not yet resolved (a 7B-class instruct model ≈ 14 GB). Kept
// in sync with guard's own unknown-model default so Fits behaves consistently.
const defaultModelBytes int64 = 14 << 30

func humanBytes(b int64) string {
	const gb = 1 << 30
	if b < 0 {
		b = -b
	}
	return fmt.Sprintf("%.1f GB", float64(b)/float64(gb))
}

// LevelInfo describes one rung of the LLM spectrum for `sirsi brain levels`.
type LevelInfo struct {
	Level   int
	Name    string
	Unlocks string
	Cost    string
}

// Spectrum returns the four LLM levels (A29 §spectrum) for display. Static
// canon — the user-facing map of what each rung adds and what it costs.
func Spectrum() []LevelInfo {
	return []LevelInfo{
		{0, "Deterministic", "Rules dispatch/route/heartbeat/ack. Ships ON — works with zero AI.", "free"},
		{1, "+ Local triage", "A local model screens ambiguous items. Zero-token.", "free (local)"},
		{2, "+ Agentic execution", "A local agent handles escalated build/review/bind work.", "free (local)"},
		{3, "+ Hosted", "Opt-in API keys (Haiku/Sonnet/Opus or OpenAI-compatible) per role.", "per-token (opt-in)"},
	}
}
