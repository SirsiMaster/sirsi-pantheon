package router

// executor.go — the tiered executor's PLANNER (ADR-039 P2). Given one open item
// and the autonomous switch, it decides what the continuous loop does with it —
// and it does so as a PURE function (no side effects), so the decision is fully
// testable and the safety invariant (never act on a gated item, P6) is provable.
//
// The side-effectful step (actually dispatching / acking) is P3 and lives behind
// this planner: P3 MUST refuse to perform any plan whose Action is ActGate or
// ActPropose. This split — decide purely, act separately — is what lets Ma'at
// test-enforce "the loop never auto-acts on owner-gated work."

import (
	"github.com/SirsiMaster/sirsi-pantheon/internal/work"
)

// ExecAction is what the loop should do with one item this tick.
type ExecAction string

const (
	// ActGate — owner-gated (ClassifyGate said so). Park in the owner-queue and
	// wait; the loop MUST NOT act on it regardless of the autonomous switch.
	ActGate ExecAction = "gate"
	// ActPropose — autonomous is OFF: surface a proposal for the owner, do not act.
	ActPropose ExecAction = "propose"
	// ActDispatch — autonomous ON, not gated: wake the target agent to work it
	// (Tier-2 — a real agent does code/review/bind). Reuses the existing wake path.
	ActDispatch ExecAction = "dispatch"
)

// ExecPlan is the planner's decision for one item. Pure data.
type ExecPlan struct {
	ItemID string       `json:"item_id"`
	To     string       `json:"to"`
	Action ExecAction   `json:"action"`
	Tier   int          `json:"tier"` // 0 dispatch-none, 2 dispatch-agent; owner-gated = -1
	Gate   GateDecision `json:"gate"`
	Reason string       `json:"reason"`

	// authorized is an UNFORGEABLE dispatch token: only PlanExecution sets it, and
	// only on a non-gated ActDispatch. It is unexported, so no caller in another
	// package can build an ExecPlan that Actionable() will act on by setting
	// Action=ActDispatch directly, and it does not survive JSON (a decoded plan is
	// never actionable — the caller must re-plan). This makes the safety rail a
	// type property, not just a convention in a comment (review finding (a)).
	authorized bool
}

// Authorized reports whether this plan carries a genuine dispatch authorization
// minted by PlanExecution — the executor (P3) MUST gate every side effect on this.
func (p ExecPlan) Authorized() bool { return p.authorized }

// PlanExecution decides what the loop does with one item. It ALWAYS classifies
// the gate first, so a gated item can never fall through to an action — the
// single most important property of the whole system.
func PlanExecution(item work.Item, autonomous bool) ExecPlan {
	g := ClassifyGate(item)
	if g.Gated {
		return ExecPlan{
			ItemID: item.ID, To: item.To, Action: ActGate, Tier: -1, Gate: g,
			Reason: "owner-gated (" + g.Class.String() + "): " + g.Reason,
		}
	}
	if !autonomous {
		return ExecPlan{
			ItemID: item.ID, To: item.To, Action: ActPropose, Tier: -1, Gate: g,
			Reason: "autonomous off — propose, do not act",
		}
	}
	// Autonomous ON + not gated: hand it to the target agent to work (Tier-2).
	// authorized:true is the only place this token is minted.
	return ExecPlan{
		ItemID: item.ID, To: item.To, Action: ActDispatch, Tier: 2, Gate: g,
		Reason: "non-gated — dispatch to " + item.To, authorized: true,
	}
}

// PlanAll plans an entire open queue. Deterministic, side-effect free.
func PlanAll(items []work.Item, autonomous bool) []ExecPlan {
	plans := make([]ExecPlan, 0, len(items))
	for _, it := range items {
		plans = append(plans, PlanExecution(it, autonomous))
	}
	return plans
}

// OwnerQueue returns just the gated plans — the "these are waiting on you and
// nothing else is stuck" view (ADR-039 P5 seed). The honest idle is reached when
// EVERY remaining plan is in this set.
func OwnerQueue(plans []ExecPlan) []ExecPlan {
	var q []ExecPlan
	for _, p := range plans {
		if p.Action == ActGate {
			q = append(q, p)
		}
	}
	return q
}

// Actionable returns the plans the loop may act on THIS tick (dispatch). Empty
// when autonomous is off (everything is a proposal) or when all remaining work is
// owner-gated (honest idle). A gated plan can never appear here — enforced by
// construction and by TestExecutorNeverActsOnGated (P6).
func Actionable(plans []ExecPlan) []ExecPlan {
	var a []ExecPlan
	for _, p := range plans {
		// BOTH the action AND the unforgeable authorization — a plan whose Action
		// was set to ActDispatch by hand (or decoded from JSON) has authorized=false
		// and can never be acted on.
		if p.Action == ActDispatch && p.authorized {
			a = append(a, p)
		}
	}
	return a
}
