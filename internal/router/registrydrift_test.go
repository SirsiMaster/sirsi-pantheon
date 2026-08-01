package router

import "testing"

// The drift these tests describe is not hypothetical. On 2026-07-31 the live
// registry had lost claude-io's wake.launch_agent_label while origin/main still
// carried it — the third arming of the same landmine in six days, and the exact
// field whose earlier absence caused peers to be told a live agent was
// unreachable.
//
// Each case is written against BEHAVIOR rather than wording, so a copy edit
// does not fail them but deleting the detection does.

const upstream = `[
  {"id":"claude-io","type":"claude","cwd":"/x/sirsi-io","workstream":"io",
   "wake":{"mechanism":"launchagent","launch_agent_label":"ai.sirsi.router.wake.claude-io"}},
  {"id":"claude-home","type":"claude","cwd":"/x","workstream":"home",
   "wake":{"mechanism":"launchagent","launch_agent_label":"ai.sirsi.router.wake.claude-home"}}
]`

func TestDriftDetectsALostFieldNobodyEnumerated(t *testing.T) {
	// The live registry kept the agent and the wake block, and dropped one leaf.
	live := `[
      {"id":"claude-io","type":"claude","cwd":"/x/sirsi-io","workstream":"io",
       "wake":{"mechanism":"launchagent"}},
      {"id":"claude-home","type":"claude","cwd":"/x","workstream":"home",
       "wake":{"mechanism":"launchagent","launch_agent_label":"ai.sirsi.router.wake.claude-home"}}
    ]`
	d := diffRegistries([]byte(upstream), []byte(live))

	if !d.Drifted() {
		t.Fatal("a field present upstream and missing live MUST be drift")
	}
	if len(d.LostFields) != 1 {
		t.Fatalf("want exactly the one lost leaf, got %+v", d.LostFields)
	}
	got := d.LostFields[0]
	if got.AgentID != "claude-io" || got.Path != "wake.launch_agent_label" {
		t.Fatalf("must name the agent AND the path, got %+v", got)
	}
	if got.Upstream == "" {
		t.Fatal("must report the value the live registry is missing, so the fix is obvious")
	}
}

// The check must find a lost field it was never told about. This is the whole
// design constraint: enforcement must not share the bug's shape, or it is blind
// to the next field exactly as it was blind to the last one.
func TestDriftFindsAFieldTheCheckNeverHeardOf(t *testing.T) {
	up := `[{"id":"a","wake":{"mechanism":"launchagent"},"future_field":{"nested":{"deep":"value"}}}]`
	live := `[{"id":"a","wake":{"mechanism":"launchagent"}}]`
	d := diffRegistries([]byte(up), []byte(live))
	if !d.Drifted() {
		t.Fatal("a check that only knows today's fields is blind to tomorrow's")
	}
	if d.LostFields[0].Path != "future_field.nested.deep" {
		t.Fatalf("must walk to the leaf generically, got %q", d.LostFields[0].Path)
	}
}

func TestDriftReportsAMissingAgentSeparatelyFromALostField(t *testing.T) {
	live := `[{"id":"claude-io","wake":{"mechanism":"launchagent","launch_agent_label":"ai.sirsi.router.wake.claude-io"},
               "type":"claude","cwd":"/x/sirsi-io","workstream":"io"}]`
	d := diffRegistries([]byte(upstream), []byte(live))
	if len(d.MissingAgents) != 1 || d.MissingAgents[0] != "claude-home" {
		t.Fatalf("an agent the router cannot address at all is its own category, got %+v", d.MissingAgents)
	}
}

// Being AHEAD is normal. A check that cries wolf on unpushed registrations gets
// ignored, and then it is not there for the case that matters.
func TestAheadIsNotDrift(t *testing.T) {
	live := upstream[:len(upstream)-1] + `,
      {"id":"claude-inference","type":"claude","cwd":"/x/inf","workstream":"inference",
       "wake":{"mechanism":"launchagent","launch_agent_label":"ai.sirsi.router.wake.claude-inference"}}]`
	d := diffRegistries([]byte(upstream), []byte(live))
	if d.Drifted() {
		t.Fatalf("unpushed registrations are not drift, got %+v", d)
	}
	if len(d.AheadAgents) != 1 || d.AheadAgents[0] != "claude-inference" {
		t.Fatalf("ahead agents must still be VISIBLE, got %+v", d.AheadAgents)
	}
}

func TestIdenticalRegistriesAreClean(t *testing.T) {
	if d := diffRegistries([]byte(upstream), []byte(upstream)); d.Drifted() {
		t.Fatalf("identical registries must be clean, got %+v", d)
	}
}

// IO7a (sirsi-io ADR-002): a surface must be ABLE to say "unknown". If the
// comparison cannot be made, that is NOT a clean bill of health — and the
// caller can only render it honestly if the verdict distinguishes the two.
func TestUnknownIsNotClean(t *testing.T) {
	d := diffRegistries([]byte(`not json`), []byte(upstream))
	if d.Checked {
		t.Fatal("an unparseable input must not be reported as checked")
	}
	if d.Unknown == "" {
		t.Fatal("unknown must carry its reason — 'could not check' is useless without why")
	}
	if d.Drifted() {
		t.Fatal("unknown must not masquerade as drift either; it is a third state")
	}
}

// Both registry shapes in use must parse, because a check that only understands
// today's schema fails SILENTLY the day the schema changes — which is the same
// class of defect it exists to catch.
func TestBothRegistryShapesParse(t *testing.T) {
	wrapped := `{"agents":` + upstream + `}`
	if d := diffRegistries([]byte(wrapped), []byte(upstream)); !d.Checked || d.Drifted() {
		t.Fatalf("object-wrapped and bare-list registries must compare equal, got %+v", d)
	}
}

// A map-shaped entry with NO inner "id" is fully live to the router, because
// LoadRegistry injects the map key as the ID. A check that skipped it would be
// blind to an agent the router happily addresses — a facade of its own, which is
// the exact class this file exists to catch.
//
// Found by codex-pantheon reviewing #413. Zero of 23 live entries hit it that
// day, which is why it is a latent blind spot rather than a live defect: the
// worst kind to leave, because nothing will remind you.
func TestEntryWithoutInnerIDIsStillSeen(t *testing.T) {
	up := `{"agents":{"ghost":{"wake":{"mechanism":"launchagent","launch_agent_label":"ai.sirsi.router.wake.ghost"}}}}`
	live := `{"agents":{"ghost":{"wake":{"mechanism":"launchagent"}}}}`

	d := diffRegistries([]byte(up), []byte(live))
	if !d.Checked {
		t.Fatalf("id-less entries must not make the whole comparison unknown: %+v", d)
	}
	if !d.Drifted() {
		t.Fatal("an agent the router CAN address must not be invisible to the drift check")
	}
	if len(d.LostFields) != 1 || d.LostFields[0].AgentID != "ghost" {
		t.Fatalf("must attribute the loss to the map key, got %+v", d.LostFields)
	}
}
