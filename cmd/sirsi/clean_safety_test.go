package main

import (
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/jackal"
)

// TestSelectCleanTargets_PreviewEqualsApply is the A1 evidence that
// `sirsi clean` preview and `--confirm` apply target the IDENTICAL set: the
// selector takes no `confirm` parameter, so apply cannot widen scope. Scope is
// governed solely by include-caution; SeverityWarning is never auto-cleaned.
func TestSelectCleanTargets_PreviewEqualsApply(t *testing.T) {
	findings := []jackal.Finding{
		{Path: "/a/safe1", Severity: jackal.SeveritySafe},
		{Path: "/b/caution1", Severity: jackal.SeverityCaution},
		{Path: "/c/warn1", Severity: jackal.SeverityWarning},
		{Path: "/d/safe2", Severity: jackal.SeveritySafe},
		{Path: "/e/caution2", Severity: jackal.SeverityCaution},
	}

	// Default scope (safe only). Preview and apply run the same selector with the
	// same includeCaution → identical set; --confirm is not an input here, so it
	// can never change what is cleaned.
	safeOnly := selectCleanTargets(findings, false)
	if got := len(safeOnly); got != 2 {
		t.Fatalf("safe-only target = %d, want 2", got)
	}
	for _, f := range safeOnly {
		if f.Severity != jackal.SeveritySafe {
			t.Errorf("safe-only scope leaked a %v finding (%s)", f.Severity, f.Path)
		}
	}

	// include-caution widens scope to safe + caution — but NEVER warning.
	withCaution := selectCleanTargets(findings, true)
	if got := len(withCaution); got != 4 {
		t.Fatalf("include-caution target = %d, want 4 (2 safe + 2 caution)", got)
	}
	for _, f := range withCaution {
		if f.Severity == jackal.SeverityWarning {
			t.Errorf("warning-severity finding must never be auto-cleaned (%s)", f.Path)
		}
	}

	// Determinism: identical inputs yield an identical-size set every call — the
	// preview the user sees is exactly the apply set (Rule A1).
	if len(selectCleanTargets(findings, false)) != len(safeOnly) {
		t.Error("selectCleanTargets is non-deterministic for the same inputs")
	}
	if len(selectCleanTargets(findings, true)) != len(withCaution) {
		t.Error("selectCleanTargets is non-deterministic for the same inputs (caution)")
	}
}

// TestDecideCleanAction pins the --yes gate (TUI design proof gap V2): --yes
// suppresses ONLY the interactive [y/N] prompt; it never converts a dry-run
// into an apply and never widens scope (scope lives in selectCleanTargets,
// which --yes cannot reach).
func TestDecideCleanAction(t *testing.T) {
	cases := []struct {
		name                 string
		dryRun, confirm, yes bool
		want                 cleanAction
	}{
		{"default is a dry-run preview", true, false, false, cleanActionPreview},
		{"--yes without --confirm STAYS a dry-run", true, false, true, cleanActionPreview},
		{"--confirm alone prompts interactively", true, true, false, cleanActionPrompt},
		{"--confirm --yes applies without the prompt", true, true, true, cleanActionApply},
		{"--dry-run=false alone still prompts (unchanged)", false, false, false, cleanActionPrompt},
		{"--dry-run=false --yes skips the prompt", false, false, true, cleanActionApply},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := decideCleanAction(tc.dryRun, tc.confirm, tc.yes); got != tc.want {
				t.Errorf("decideCleanAction(dryRun=%v, confirm=%v, yes=%v) = %v, want %v",
					tc.dryRun, tc.confirm, tc.yes, got, tc.want)
			}
		})
	}
}

// TestSelectCleanTargets_Empty confirms an empty/clean machine yields no targets
// (no spurious deletions).
func TestSelectCleanTargets_Empty(t *testing.T) {
	if got := selectCleanTargets(nil, true); len(got) != 0 {
		t.Errorf("nil findings should yield 0 targets, got %d", len(got))
	}
	onlyWarnings := []jackal.Finding{
		{Path: "/x", Severity: jackal.SeverityWarning},
		{Path: "/y", Severity: jackal.SeverityWarning},
	}
	if got := selectCleanTargets(onlyWarnings, true); len(got) != 0 {
		t.Errorf("warnings-only should yield 0 clean targets even with caution, got %d", len(got))
	}
}
