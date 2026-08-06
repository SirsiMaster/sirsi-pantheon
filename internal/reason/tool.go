// Package reason is the agent loop Sirsi owns.
//
// The loop is scaffolded here, in Go, and the model does what a model is
// actually good at: ranking hypotheses against evidence and writing the
// explanation. That split is deliberate and is what makes this buildable now.
// A 12B local model will not invent a diagnostic method; it will happily
// produce a fluent, confident, unfalsifiable one.
//
// The loop's most important property is DISCARDING. The 2026-07-27 diagnosis
// that motivated this package reached the right answer only after two confident
// hypotheses — Jetsam, then the session reaper — were tested and falsified. A
// system that cannot be wrong out loud cannot diagnose.
package reason

import (
	"context"
	"fmt"
	"time"
)

// Tier is the permission class of a tool. This is the autonomy question
// answered ONCE, for everything, rather than re-litigated per feature — which
// is how ADR-035's runaway executor happened.
type Tier int

const (
	// TierObserve reads state and changes nothing. Always allowed, never gated:
	// a diagnostic tool that needs permission to look is useless in the moment
	// it matters.
	TierObserve Tier = iota
	// TierRepair changes state reversibly — restart a broker, reap an orphan,
	// evict a model. Gated by policy; every action names what it will do first.
	TierRepair
	// TierDestructive loses data or changes system configuration. NEVER
	// autonomous, under any policy. The tier exists so that "never autonomous"
	// is a property of the tool rather than a habit of the caller.
	TierDestructive
)

func (t Tier) String() string {
	switch t {
	case TierObserve:
		return "observe"
	case TierRepair:
		return "repair"
	case TierDestructive:
		return "destructive"
	}
	return "unknown"
}

// Policy is how much the loop may do without asking.
type Policy int

const (
	PolicyReadOnly    Policy = iota // observe only — the default
	PolicyConfirmEach               // repair allowed, each one confirmed
	PolicyAutoRepair                // repair allowed without asking; destructive never
)

// Allows reports whether a policy permits a tier. Destructive is refused under
// every policy, including PolicyAutoRepair — there is deliberately no policy
// value that permits it, so no configuration mistake can enable it.
func (p Policy) Allows(t Tier) bool {
	switch t {
	case TierObserve:
		return true
	case TierRepair:
		return p == PolicyConfirmEach || p == PolicyAutoRepair
	default:
		return false
	}
}

// Tool is one capability the loop can invoke.
type Tool struct {
	Name string
	// What it does, in the words a user should see BEFORE it runs. Not a
	// developer description — this is shown at the confirm prompt.
	Does string
	Tier Tier
	// Reversible records whether the effect can be undone. A broker restart is
	// reversible (the model reloads); deleting a ledger volume is not. Surfaces
	// use this to distinguish "safe to automate" from "ask first", and it is
	// recorded rather than inferred because inferring it is how irreversible
	// things get automated.
	Reversible bool
	// Applies lets the TOOL decide whether the current machine state warrants
	// it, and return the one-line reason a user should see. Optional: nil means
	// "only when asked for by name".
	//
	// Relevance lives here rather than in a caller-side table on purpose. A map
	// from finding to tool in cmd/ would be an enumeration that silently rots
	// every time a tool or a check is added, and the caller cannot know what a
	// tool actually addresses. The tool can. (PANTHEON_RULES.md A29: discover,
	// never enumerate.)
	Applies func(ctx context.Context) (bool, string)
	// Run performs the tool. Observe tools must have no side effects — that is
	// enforced by review, not by the type system, so the tier is a promise the
	// author makes explicitly.
	Run func(ctx context.Context) (Result, error)
	// Verify re-reads the world after Run to prove the effect landed. Required
	// for repair tools: "exit 0" is not evidence, and this fabric has recorded
	// the difference repeatedly. A repair tool without Verify is refused at
	// registration.
	// `ran` is Run's own Result, so verification state is INVOCATION-LOCAL.
	// Sharing a captured variable between Run and Verify let concurrent
	// invocations of the same registered tool overwrite each other's baseline and
	// produce false pass/fail verdicts (codex-pantheon, router item
	// 20260729-193639).
	Verify func(ctx context.Context, ran Result) (Result, error)
}

// Result is what a tool observed or did.
type Result struct {
	Summary string
	// Evidence is the raw material a human or a model can re-judge. Carrying it
	// is what lets a conclusion be overturned later instead of being taken on
	// trust.
	Evidence map[string]any
	Changed  bool
}

// Registry holds the tools available to the loop.
type Registry struct {
	tools map[string]Tool
	order []string
}

func NewRegistry() *Registry { return &Registry{tools: map[string]Tool{}} }

// Register adds a tool, refusing ones that cannot be held to the fabric's own
// rules. A repair tool with no Verify is rejected at registration rather than
// discovered at 3am: an action whose effect nobody checks is exactly the
// "merged is not deployed" class in a different costume.
func (r *Registry) Register(t Tool) error {
	if t.Name == "" || t.Run == nil {
		return fmt.Errorf("reason: tool needs a name and a Run")
	}
	if t.Tier >= TierRepair && t.Verify == nil {
		return fmt.Errorf("reason: %q changes state but has no Verify — exit 0 is not evidence", t.Name)
	}
	if _, dup := r.tools[t.Name]; dup {
		return fmt.Errorf("reason: tool %q already registered", t.Name)
	}
	r.tools[t.Name] = t
	r.order = append(r.order, t.Name)
	return nil
}

func (r *Registry) Get(name string) (Tool, bool) { t, ok := r.tools[name]; return t, ok }

func (r *Registry) Names() []string { return append([]string(nil), r.order...) }

// Invocation is one attempt to use a tool, and its outcome. The loop's
// transcript is a list of these — it IS the explanation, not a summary of one.
type Invocation struct {
	Tool     string
	Tier     Tier
	Allowed  bool
	Reason   string // why it was or was not allowed
	Result   Result
	Verified *Result // nil when not a repair, or when verification did not run
	Err      error
	Took     time.Duration
}

// Invoke runs a tool under a policy, then verifies it.
//
// Verification failure is reported as failure even when Run succeeded. That
// inversion is the point: a repair that claims success and cannot be seen in
// the world is worse than one that fails loudly, because it ends the
// investigation.
func Invoke(ctx context.Context, r *Registry, name string, p Policy) Invocation {
	inv := Invocation{Tool: name}
	t, ok := r.Get(name)
	if !ok {
		inv.Err = fmt.Errorf("reason: no such tool %q", name)
		return inv
	}
	inv.Tier = t.Tier

	if !p.Allows(t.Tier) {
		inv.Allowed = false
		inv.Reason = fmt.Sprintf("%s tools are not permitted under policy %v — %s", t.Tier, p, t.Does)
		return inv
	}
	inv.Allowed = true
	inv.Reason = t.Does

	start := time.Now()
	res, err := t.Run(ctx)
	inv.Took = time.Since(start)
	inv.Result = res
	if err != nil {
		inv.Err = err
		return inv
	}

	if t.Verify != nil {
		v, verr := t.Verify(ctx, res)
		inv.Verified = &v
		if verr != nil {
			inv.Err = fmt.Errorf("ran, but verification failed: %w", verr)
		}
	}
	return inv
}
