package reason

import (
	"context"
	"errors"
	"testing"
)

// Destructive must be refused under EVERY policy, including AutoRepair. There
// is deliberately no policy value that permits it, so no configuration mistake
// can enable it. ADR-035 exists because autonomy was once granted before the
// tiers were.
func TestDestructiveIsRefusedUnderEveryPolicy(t *testing.T) {
	for _, p := range []Policy{PolicyReadOnly, PolicyConfirmEach, PolicyAutoRepair} {
		if p.Allows(TierDestructive) {
			t.Errorf("policy %v permits destructive tools", p)
		}
	}
	// And observe must always be allowed — a diagnostic that needs permission
	// to LOOK is useless in the moment it matters.
	for _, p := range []Policy{PolicyReadOnly, PolicyConfirmEach, PolicyAutoRepair} {
		if !p.Allows(TierObserve) {
			t.Errorf("policy %v forbids observation", p)
		}
	}
}

// A repair tool with no Verify is refused at REGISTRATION, not discovered at
// 3am. "exit 0" is not evidence that the world changed.
func TestRepairToolWithoutVerifyIsRefused(t *testing.T) {
	r := NewRegistry()
	err := r.Register(Tool{
		Name: "bad.repair",
		Does: "change something and hope",
		Tier: TierRepair,
		Run:  func(context.Context) (Result, error) { return Result{}, nil },
	})
	if err == nil {
		t.Fatal("registered a repair tool with no Verify")
	}
	// The same tool as an observe tool is fine — the requirement is tied to
	// changing state, not to ceremony.
	if err := r.Register(Tool{
		Name: "ok.observe", Does: "look", Tier: TierObserve,
		Run: func(context.Context) (Result, error) { return Result{}, nil },
	}); err != nil {
		t.Fatalf("observe tool rejected: %v", err)
	}
}

// A repair that RAN but cannot be SEEN in the world must report failure. This
// inversion is the point: a fix that claims success and left no trace ends the
// investigation, which is worse than failing loudly.
func TestVerificationFailureOverridesRunSuccess(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(Tool{
		Name: "restart.thing", Does: "restart", Tier: TierRepair, Reversible: true,
		Run: func(context.Context) (Result, error) { return Result{Summary: "issued", Changed: true}, nil },
		Verify: func(context.Context, Result) (Result, error) {
			return Result{Summary: "still gone"}, errors.New("never came back")
		},
	}); err != nil {
		t.Fatal(err)
	}
	inv := Invoke(context.Background(), r, "restart.thing", PolicyAutoRepair)
	if inv.Err == nil {
		t.Fatal("Run succeeded and Verify failed, but the invocation reported success")
	}
	if inv.Verified == nil {
		t.Error("verification result was not carried — the evidence is the point")
	}
}

// Refusal must explain itself in terms the user can act on, and must NOT run
// the tool. A silent refusal is indistinguishable from a broken tool.
func TestRefusalExplainsAndDoesNotRun(t *testing.T) {
	ran := false
	r := NewRegistry()
	_ = r.Register(Tool{
		Name: "restart.thing", Does: "restart the local model broker", Tier: TierRepair,
		Run:    func(context.Context) (Result, error) { ran = true; return Result{}, nil },
		Verify: func(context.Context, Result) (Result, error) { return Result{}, nil },
	})
	inv := Invoke(context.Background(), r, "restart.thing", PolicyReadOnly)
	if ran {
		t.Fatal("a refused tool was executed anyway")
	}
	if inv.Allowed {
		t.Error("refused invocation reported Allowed")
	}
	if inv.Reason == "" {
		t.Error("refusal carried no reason")
	}
}

// The first version of the fork-storm scan reported 23 idle `distnoted` — the
// macOS notification daemon — as a fork storm. A check that cries wolf is how a
// surface earns the distrust that lets a REAL 358-process storm pass unread, so
// this pins both halves of the fix.
func TestForkStormIgnoresSystemDaemonsAndNormalNoise(t *testing.T) {
	systemPaths := []string{
		"/usr/sbin/distnoted",
		"/usr/libexec/secinitd",
		"/System/Library/CoreServices/Finder.app/Contents/MacOS/Finder",
		"/usr/bin/login",
	}
	for _, p := range systemPaths {
		if !isSystemDaemon(p) {
			t.Errorf("%q was not recognized as a system daemon — it will be reported as a storm", p)
		}
	}

	// The real 2026-07-27 offender lived under the user's home and must NOT be
	// excused by the same filter.
	userPath := "/Users/someone/Library/Application Support/Claude/claude-code/2.1.219/claude.app/Contents/MacOS/claude"
	if isSystemDaemon(userPath) {
		t.Error("the actual fork-storm binary was classified as a system daemon")
	}

	// The threshold must sit above observed normal noise (23 distnoted) and well
	// below the observed real storm (358).
	if forkStormThreshold <= 23 {
		t.Errorf("threshold %d is at or below observed system noise", forkStormThreshold)
	}
	if forkStormThreshold >= 358 {
		t.Errorf("threshold %d would have missed the real 358-process storm", forkStormThreshold)
	}
}
