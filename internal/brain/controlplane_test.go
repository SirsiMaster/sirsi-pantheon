package brain

import (
	"errors"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
)

// fakeDispatcher is an in-memory Dispatcher for control-plane tests — no real
// router, no wake side effects (A16). It lets us drive the exact substrate
// conditions the brain must surface.
type fakeDispatcher struct {
	ns      *router.NodeStatus
	nsErr   error
	wake    []router.AgentWakeHealth
	wakeErr error
}

func (f fakeDispatcher) NodeStatus() (*router.NodeStatus, error) { return f.ns, f.nsErr }
func (f fakeDispatcher) WakeReadiness() ([]router.AgentWakeHealth, error) {
	return f.wake, f.wakeErr
}

// withNodeCapacity swaps the injectable sampler for a test and restores it.
func withNodeCapacity(t *testing.T, nc guard.NodeCapacity) {
	t.Helper()
	prev := sampleNodeCapacity
	sampleNodeCapacity = func() guard.NodeCapacity { return nc }
	t.Cleanup(func() { sampleNodeCapacity = prev })
}

func TestCollect_TiersAndLevel(t *testing.T) {
	withNodeCapacity(t, guard.NodeCapacity{TotalRAM: 64 << 30, FreeRAM: 40 << 30})
	cfg := Config{Roles: map[string]string{"triage": "local:qwen", "execution": "none"}}
	d := fakeDispatcher{
		ns: &router.NodeStatus{AgentCount: 3},
		wake: []router.AgentWakeHealth{
			{AgentID: "a", Ready: true},
			{AgentID: "b", Ready: false, Detail: "no /loop"},
		},
	}
	st := Collect(cfg, d)
	if st.Level != 1 {
		t.Errorf("Level = %d, want 1", st.Level)
	}
	if st.Registry.Registered != 3 || st.Registry.Wakeable != 1 || len(st.Registry.Unwakeable) != 1 {
		t.Errorf("registry health = %+v", st.Registry)
	}
	// triage drives Tier-1; execution (none) stays Tier-0.
	tiers := map[Role]string{}
	for _, r := range st.Roles {
		tiers[r.Role] = r.Tier
	}
	if tiers[RoleTriage] != "Tier-1" || tiers[RoleExecution] != "Tier-0" || tiers[RoleDispatch] != "Tier-0" {
		t.Errorf("tiers = %v", tiers)
	}
	if st.Node.TotalRAM != 64<<30 {
		t.Errorf("node headroom not read from capacity: %+v", st.Node)
	}
}

func TestCollect_DegradesOnDeadRouter(t *testing.T) {
	// A dead router must not blank the brain view — Level still derives from config.
	withNodeCapacity(t, guard.NodeCapacity{TotalRAM: 16 << 30, FreeRAM: 8 << 30})
	cfg := Config{Roles: map[string]string{"triage": "hosted:haiku"}}
	d := fakeDispatcher{nsErr: errors.New("router down"), wakeErr: errors.New("router down")}
	st := Collect(cfg, d)
	if st.Level != 3 {
		t.Errorf("Level should still derive to 3 from config, got %d", st.Level)
	}
	if st.Registry.Registered != 0 {
		t.Errorf("dead router should zero registry, got %+v", st.Registry)
	}
}

func TestDoctor_HealthyFloor(t *testing.T) {
	// Level 0, everything wakeable, no stranded, no model needed -> all ✓.
	withNodeCapacity(t, guard.NodeCapacity{TotalRAM: 64 << 30, FreeRAM: 60 << 30})
	cfg := DefaultConfig()
	d := fakeDispatcher{
		ns:   &router.NodeStatus{AgentCount: 2},
		wake: []router.AgentWakeHealth{{AgentID: "a", Ready: true}, {AgentID: "b", Ready: true}},
	}
	for _, dg := range Doctor(cfg, d) {
		if !dg.Ok {
			t.Errorf("expected all healthy, got problem: %s — %s", dg.Check, dg.Detail)
		}
	}
}

func TestDoctor_SurfacesUnwakeableInvariant(t *testing.T) {
	// The Registry/Wake invariant (A29): a registered agent with no live channel
	// must surface as a problem with a fix that points at the EXISTING wake verbs.
	withNodeCapacity(t, guard.NodeCapacity{TotalRAM: 64 << 30, FreeRAM: 60 << 30})
	cfg := DefaultConfig()
	d := fakeDispatcher{
		ns:   &router.NodeStatus{AgentCount: 2},
		wake: []router.AgentWakeHealth{{AgentID: "a", Ready: true}, {AgentID: "zombie", Ready: false, Detail: "no /loop watcher"}},
	}
	var found *Diagnosis
	for i := range Doctor(cfg, d) {
		if Doctor(cfg, d)[i].Check == "registry/wake" {
			dg := Doctor(cfg, d)[i]
			found = &dg
		}
	}
	if found == nil || found.Ok {
		t.Fatalf("registry/wake invariant not surfaced as a problem: %+v", found)
	}
	if found.Fix == "" {
		t.Error("unwakeable finding must carry a plain-English fix")
	}
}

func TestDoctor_StrandedInbox(t *testing.T) {
	withNodeCapacity(t, guard.NodeCapacity{TotalRAM: 64 << 30, FreeRAM: 60 << 30})
	cfg := DefaultConfig()
	d := fakeDispatcher{
		ns: &router.NodeStatus{
			AgentCount:    1,
			StrandedInbox: []router.StrandedAgent{{AgentID: "claude-x", OpenItems: 3}},
		},
		wake: []router.AgentWakeHealth{{AgentID: "claude-x", Ready: true}},
	}
	ok := false
	for _, dg := range Doctor(cfg, d) {
		if dg.Check == "stranded-inbox" && !dg.Ok {
			ok = true
		}
	}
	if !ok {
		t.Error("stranded inbox not surfaced")
	}
}

func TestDoctor_RAMGate(t *testing.T) {
	// A local model role on a tight node must report "won't fit" via Fits.
	// 8 GB free cannot hold a 14 GB model (2×14 + reserve >> 8).
	withNodeCapacity(t, guard.NodeCapacity{TotalRAM: 16 << 30, FreeRAM: 8 << 30})
	cfg := Config{Roles: map[string]string{"triage": "local:qwen"}}
	d := fakeDispatcher{ns: &router.NodeStatus{}, wake: nil}
	var ram *Diagnosis
	for _, dg := range Doctor(cfg, d) {
		if dg.Check == "ram" {
			dg := dg
			ram = &dg
		}
	}
	if ram == nil || ram.Ok {
		t.Fatalf("RAM gate should report won't-fit on a tight node: %+v", ram)
	}
}

func TestDoctor_RAMGate_NoModelNoAlarm(t *testing.T) {
	// Level 0 (no local model) must NOT raise a RAM alarm — only real, current,
	// fixable conditions alarm (feedback_surfaces_current_actionable_only).
	withNodeCapacity(t, guard.NodeCapacity{TotalRAM: 8 << 30, FreeRAM: 1 << 30})
	cfg := DefaultConfig()
	d := fakeDispatcher{ns: &router.NodeStatus{}}
	for _, dg := range Doctor(cfg, d) {
		if dg.Check == "ram" && !dg.Ok {
			t.Errorf("Level 0 must not raise a RAM alarm: %s", dg.Detail)
		}
	}
}

func TestSpectrum_FourRungs(t *testing.T) {
	s := Spectrum()
	if len(s) != 4 {
		t.Fatalf("spectrum has %d rungs, want 4", len(s))
	}
	for i, l := range s {
		if l.Level != i {
			t.Errorf("rung %d has level %d", i, l.Level)
		}
	}
}
