package router

// gate.go — the Honest Gate: the deterministic boundary between work the
// continuous loop may do on its own and work that genuinely needs the owner
// (ADR-039). This is the code embodiment of the owner directive "models in
// effort at all times except when there is an honest user gate."
//
// SAFETY POSTURE (mirrors internal/cleaner/safety.go): the dangerous classes —
// money movement, access-control/IAM, destructive deletion, credentials,
// irreversible/outward-facing actions — are matched by HARDCODED patterns, not
// by any model's judgment. A model can be talked out of a gate; this table
// cannot. The classifier is deliberately CONSERVATIVE: a false gate only costs
// the owner a glance, a missed gate could ship an irreversible action, so when
// a term is ambiguous it gates. The Tier-0 model may only ADD gates (flag fuzzy
// business/ambiguity items); it may NEVER clear a hard gate here.

import (
	"strings"

	"github.com/SirsiMaster/sirsi-pantheon/internal/work"
)

// GateClass is why an item is (or is not) owner-gated. The order matches the
// approved taxonomy: safety first, then founder/business, then
// irreversible/outward, then genuine escalation.
type GateClass int

const (
	GateNone         GateClass = iota // the loop may act on this itself
	GateSafety                        // money / access-control / IAM / destructive / credentials
	GateFounder                       // founder / business / terms / investor / brand / strategy
	GateIrreversible                  // publish / send / merge-to-prod / deploy / outward-facing
	GateEscalate                      // explicitly ESCALATE-class, or addressed to the owner
)

func (c GateClass) String() string {
	switch c {
	case GateSafety:
		return "safety"
	case GateFounder:
		return "founder"
	case GateIrreversible:
		return "irreversible"
	case GateEscalate:
		return "escalate"
	default:
		return "none"
	}
}

// GateDecision is the classifier's verdict for one item.
type GateDecision struct {
	Gated  bool      `json:"gated"`
	Class  GateClass `json:"class"`
	Reason string    `json:"reason,omitempty"` // the matched term / why it gates
}

// gatePattern is one hardcoded trigger: if `term` appears in the item's text,
// the item gates under `class`. Kept as auditable data (like safety.go's
// protected-path table), not scattered through control flow.
type gatePattern struct {
	term  string
	class GateClass
}

// hardGates is the non-negotiable table. It is intentionally broad: these terms
// gate even in benign-looking contexts, because the cost asymmetry favors
// over-gating. Terms are matched case-insensitively as substrings of the item's
// title + instructions.
var hardGates = []gatePattern{
	// ── Safety: money movement ──────────────────────────────────────────────
	{"wire transfer", GateSafety}, {"send money", GateSafety}, {"transfer funds", GateSafety},
	{"buy stock", GateSafety}, {"sell stock", GateSafety}, {"place an order", GateSafety},
	{"withdraw", GateSafety}, {"payout", GateSafety}, {"refund", GateSafety},
	// ── Safety: access-control / IAM ────────────────────────────────────────
	{"add-iam-policy-binding", GateSafety}, {"iam policy", GateSafety}, {"grant role", GateSafety},
	{"storage.admin", GateSafety}, {"roles/", GateSafety}, {"setiampolicy", GateSafety},
	{"chmod 777", GateSafety}, {"make public", GateSafety}, {"share with", GateSafety},
	{"branch protection", GateSafety}, {"add collaborator", GateSafety},
	// ── Safety: destructive ─────────────────────────────────────────────────
	{"rm -rf", GateSafety}, {"drop database", GateSafety}, {"force-push", GateSafety},
	{"force push", GateSafety}, {"delete the", GateSafety}, {"hard delete", GateSafety},
	{"empty the trash", GateSafety}, {"purge", GateSafety}, {"revoke", GateSafety},
	{"delete branch", GateSafety}, {"delete repo", GateSafety},
	// ── Safety: credentials ─────────────────────────────────────────────────
	{"api key", GateSafety}, {"secret key", GateSafety}, {"password", GateSafety},
	{"credential", GateSafety}, {"private key", GateSafety}, {"access token", GateSafety},
	{"service account key", GateSafety},
	// ── Founder / business ──────────────────────────────────────────────────
	{"term sheet", GateFounder}, {"investor", GateFounder}, {"valuation", GateFounder},
	{"cap table", GateFounder}, {"equity", GateFounder}, {"fundraise", GateFounder},
	{"pricing", GateFounder}, {"founder decision", GateFounder}, {"go-to-market", GateFounder},
	{"rebrand", GateFounder}, {"press release", GateFounder},
	// ── Irreversible / outward-facing ───────────────────────────────────────
	{"deploy to prod", GateIrreversible}, {"deploy to production", GateIrreversible},
	{"merge to main", GateIrreversible}, {"merge to prod", GateIrreversible},
	{"push to production", GateIrreversible}, {"go live", GateIrreversible},
	{"publish", GateIrreversible}, {"send email", GateIrreversible}, {"send the email", GateIrreversible},
	{"post to", GateIrreversible}, {"tweet", GateIrreversible}, {"release to", GateIrreversible},
	{"tag a release", GateIrreversible}, {"cut a release", GateIrreversible},
	{"production deploy", GateIrreversible}, {"ship to prod", GateIrreversible},
}

// ClassifyGate returns whether an item needs the owner and why. It is the single
// authority every executor (the continuous loop, the menubar, the MCP adapter)
// MUST consult before acting on an item in autonomous mode. Deterministic and
// side-effect free.
func ClassifyGate(item work.Item) GateDecision {
	// Addressed to the human is an explicit escalation — the owner IS the assignee.
	if to := strings.ToLower(strings.TrimSpace(item.To)); to == "user" || to == "owner" || to == "cylton" {
		return GateDecision{Gated: true, Class: GateEscalate, Reason: "item is addressed to the owner"}
	}

	hay := strings.ToLower(item.Title + "\n" + item.Instructions)

	// Explicit escalation marker anywhere in the item wins as an escalate gate.
	if strings.Contains(hay, "escalate") || strings.Contains(hay, "needs owner") ||
		strings.Contains(hay, "owner-gated") || strings.Contains(hay, "owner directive needed") {
		return GateDecision{Gated: true, Class: GateEscalate, Reason: "explicit escalate/needs-owner marker"}
	}

	// Hard-gate table: first match wins (safety terms are listed first, so a
	// money/IAM/destructive/credential hit is reported over a weaker class).
	for _, p := range hardGates {
		if strings.Contains(hay, p.term) {
			return GateDecision{Gated: true, Class: p.class, Reason: "matched \"" + p.term + "\""}
		}
	}

	return GateDecision{Gated: false, Class: GateNone}
}

// GatePatternCount reports how many hard-gate patterns are armed — used by
// `sirsi ... doctor`/tests to prove the table is non-empty (a silently-empty
// gate table would make everything auto-executable, a critical safety failure).
func GatePatternCount() int { return len(hardGates) }
