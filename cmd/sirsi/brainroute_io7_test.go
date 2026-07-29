package main

import (
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/brain"
	"github.com/SirsiMaster/sirsi-pantheon/internal/localrouter"
)

// IO7 (sirsi-io ADR-002): "A surface that cannot reach its source says so, in
// place, at the value. It never renders stale data as live, never blanks a live
// field, and never substitutes a plausible default."
//
// Resolve() substitutes DefaultLocalProvider for an unconfigured role — a good
// default, so the call is not a naked model call. The defect was that the
// substitution was recorded on the Route and emitted in --json, while the human
// render printed only the provider name. The operator could not distinguish a
// role configured for local:gemma from an unconfigured role that had local:gemma
// chosen for it, which is exactly the "confident wrong number" IO7 forbids: the
// contract was honest and the surface was not.
//
// These tests assert the DISCLOSURE, not the wording, so a copy edit does not
// fail them but deleting the disclosure does.

func renderRoute(t *testing.T, cfg brain.Config, role brain.Role) string {
	t.Helper()
	route := localrouter.Resolve(cfg, role)
	var b strings.Builder
	writeRouteHuman(&b, route)
	return b.String()
}

func TestBrainRouteDisclosesASubstitutedDefault(t *testing.T) {
	// DefaultConfig sets every role to "none" EXPLICITLY — that is Level 0, "all
	// roles deterministic (no LLM)", a deliberate operator choice and not a gap.
	// Resolve still substitutes, so the disclosure is still owed: the route WILL
	// answer from local:gemma if called, which is exactly what an operator at
	// Level 0 needs told. What the render may not do is claim a REASON.
	out := renderRoute(t, brain.DefaultConfig(), brain.RoleTriage)

	if !strings.Contains(out, localrouter.DefaultLocalProvider) {
		t.Fatalf("render must still name the provider that will answer:\n%s", out)
	}
	// The disclosure itself. Without this the render is an IO7 violation.
	if !strings.Contains(out, "DEFAULT") || !strings.Contains(out, "has no provider selected") {
		t.Fatalf("a substituted default MUST be disclosed at the value (IO7); got:\n%s", out)
	}
	// And it must tell the operator how to resolve it, not just that it happened.
	if !strings.Contains(out, "sirsi brain use") {
		t.Fatalf("disclosure must name the command that selects a provider:\n%s", out)
	}
	// It must NOT assert a cause. Provider() collapses three states into
	// ProviderNone — absent, explicit "none", malformed — and this config is the
	// explicit-none one, so "not configured" would be a false diagnosis on a
	// stock install. State the effect; never guess why.
	if strings.Contains(out, "not configured") {
		t.Fatalf("render must state the EFFECT, not assert an unknowable cause:\n%s", out)
	}
}

func TestBrainRouteDoesNotCryWolfOnAConfiguredRole(t *testing.T) {
	cfg := brain.Config{Roles: map[string]string{"triage": "local:qwen2.5-7b-instruct"}}
	out := renderRoute(t, cfg, brain.RoleTriage)

	if !strings.Contains(out, "local:qwen2.5-7b-instruct") {
		t.Fatalf("render must name the configured provider:\n%s", out)
	}
	// A false substitution warning is its own defect: an operator who learns the
	// warning is noise stops reading it, which costs the real one.
	if strings.Contains(out, "DEFAULT") || strings.Contains(out, "has no provider selected") {
		t.Fatalf("configured role must NOT be reported as a default:\n%s", out)
	}
}
