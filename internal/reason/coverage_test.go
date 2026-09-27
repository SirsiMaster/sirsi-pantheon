package reason

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestTierPolicyAndRegistryCoverage(t *testing.T) {
	if TierObserve.String() != "observe" || TierRepair.String() != "repair" || TierDestructive.String() != "destructive" {
		t.Fatal("known tier names changed")
	}
	if Tier(99).String() != "unknown" || Policy(99).Allows(TierRepair) {
		t.Fatal("unknown tier/policy behavior changed")
	}
	r := NewRegistry()
	if err := r.Register(Tool{}); err == nil {
		t.Fatal("empty tool registered")
	}
	observe := Tool{Name: "observe", Tier: TierObserve, Run: func(context.Context) (Result, error) {
		return Result{Summary: "observed"}, nil
	}}
	if err := r.Register(observe); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(observe); err == nil {
		t.Fatal("duplicate tool registered")
	}
	if _, ok := r.Get("missing"); ok || len(r.Names()) != 1 {
		t.Fatal("registry lookup/order incorrect")
	}
	inv := Invoke(context.Background(), r, "observe", PolicyReadOnly)
	if inv.Err != nil || !inv.Allowed || inv.Result.Summary != "observed" {
		t.Fatalf("successful observe invocation = %+v", inv)
	}
	unknown := Invoke(context.Background(), r, "missing", PolicyReadOnly)
	if unknown.Err == nil || !strings.Contains(unknown.Err.Error(), "no such tool") {
		t.Fatalf("unknown invocation = %+v", unknown)
	}
	if err := r.Register(Tool{
		Name: "verified", Tier: TierRepair,
		Run:    func(context.Context) (Result, error) { return Result{Changed: true}, nil },
		Verify: func(context.Context, Result) (Result, error) { return Result{Summary: "seen"}, nil },
	}); err != nil {
		t.Fatal(err)
	}
	verified := Invoke(context.Background(), r, "verified", PolicyAutoRepair)
	if verified.Err != nil || verified.Verified == nil {
		t.Fatalf("verified invocation = %+v", verified)
	}
}

func TestMachineToolsRegistrationAndPureHelpers(t *testing.T) {
	r := NewRegistry()
	if err := MachineTools(r); err != nil {
		t.Fatal(err)
	}
	if got := r.Names(); len(got) != 4 {
		t.Fatalf("MachineTools registered %d tools, want 4: %v", len(got), got)
	}
	for _, tc := range []struct {
		in string
		ok bool
	}{
		{"00:59", false}, {"10:00", true}, {"01:10:00", true}, {"2-03:00:00", true}, {"bad", false},
	} {
		if got := oldEnough(tc.in); got != tc.ok {
			t.Errorf("oldEnough(%q) = %v, want %v", tc.in, got, tc.ok)
		}
	}
	for _, tc := range []struct {
		in string
		ok bool
	}{
		{"0:00", true}, {"0:19", true}, {"0:20", false}, {"1:00", false}, {"", false},
	} {
		if got := nearlyZeroCPU(tc.in); got != tc.ok {
			t.Errorf("nearlyZeroCPU(%q) = %v, want %v", tc.in, got, tc.ok)
		}
	}
	if ok, _ := restartVerdict(0, 12, "", nil); ok {
		t.Fatal("empty model accepted")
	}
	if ok, _ := restartVerdict(12, 12, "model", nil); ok {
		t.Fatal("unchanged pid accepted")
	}
	if ok, _ := restartVerdict(12, 13, "model", errors.New("down")); ok {
		t.Fatal("dead endpoint accepted")
	}
	if ok, reason := restartVerdict(12, 13, "model", nil); !ok || reason != "" {
		t.Fatalf("healthy restart verdict = %v, %q", ok, reason)
	}
	if summary, changed := restartSummary(1, 2, "old", "new", 1<<30); !changed || !strings.Contains(summary, "MODEL CHANGED") {
		t.Fatalf("model-change summary = %q, %v", summary, changed)
	}
	if _, changed := restartSummary(1, 2, "same", "same", 0); changed {
		t.Fatal("unchanged model reported as changed")
	}
}

func TestMachineObservationToolsRun(t *testing.T) {
	r := NewRegistry()
	if err := MachineTools(r); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, name := range []string{"process.census", "memory.pressure", "process.forkstorm"} {
		inv := Invoke(ctx, r, name, PolicyReadOnly)
		if inv.Err != nil {
			t.Fatalf("%s failed: %v", name, inv.Err)
		}
		if inv.Result.Evidence == nil {
			t.Fatalf("%s returned no evidence", name)
		}
	}
}
