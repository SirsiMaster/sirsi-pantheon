package main

import (
	"os"
	"path/filepath"
	"testing"
)

func writeADRs(t *testing.T, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("# "+n), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// Lettered sub-parts (ADR-031-A/B/C) legitimately share the base number — each
// one amends ADR-031. Reporting them as collisions made the gate cry wolf on 4
// of its first 5 findings, and a gate that is mostly wrong gets ignored.
func TestScanADRDocsTreatsLetteredSubPartsAsLegitimate(t *testing.T) {
	dir := writeADRs(t,
		"ADR-031-LOCAL-MODELS-THROUGH-PANTHEON.md",
		"ADR-031-A-NEVER-EXHAUST-THE-HOST.md",
		"ADR-031-B-DYNAMIC-PER-NODE-ENFORCEMENT.md",
		"ADR-031-C-BROKER-ENFORCEMENT-UNIVERSAL.md",
	)
	base, subs, err := scanADRDocs(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(base[31]) != 1 {
		t.Fatalf("want exactly 1 base document for 031, got %v", base[31])
	}
	if len(subs[31]) != 3 {
		t.Fatalf("want 3 sub-parts for 031, got %v", subs[31])
	}
}

// The real shape on origin/main: two unrelated documents both numbered 054.
func TestScanADRDocsDetectsGenuineCollision(t *testing.T) {
	dir := writeADRs(t,
		"ADR-054-CONTRACTS-IDENTITY-AND-LEDGER-V7.md",
		"ADR-054-ONE-HORUS-UNIFIED-AGENT-FABRIC.md",
	)
	base, _, err := scanADRDocs(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(base[54]) != 2 {
		t.Fatalf("a genuine collision must be reported, got %v", base[54])
	}
}

func TestScanADRDocsIgnoresNonADRFiles(t *testing.T) {
	dir := writeADRs(t, "ADR-INDEX.md", "README.md", "ADR-007-REAL.md")
	base, _, err := scanADRDocs(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(base) != 1 || len(base[7]) != 1 {
		t.Fatalf("only ADR-NNN-*.md files count, got %v", base)
	}
}

// The gate must FAIL the build on a collision. A gate that reports and exits 0
// is decoration.
func TestADRAuditExitsNonZeroOnCollision(t *testing.T) {
	dir := writeADRs(t,
		"ADR-054-CONTRACTS.md",
		"ADR-054-ONE-HORUS.md",
	)
	t.Setenv("SIRSI_ROUTER_DB", filepath.Join(t.TempDir(), "router.db"))

	cmd := newADRAuditCmd()
	cmd.SetArgs([]string{"--docs", dir})
	cmd.SetOut(os.Stderr)
	if err := cmd.Execute(); err == nil {
		t.Fatal("audit must return an error when two documents claim one number")
	}
}

func TestADRAuditPassesOnCleanTree(t *testing.T) {
	dir := writeADRs(t, "ADR-001-ONE.md", "ADR-002-TWO.md", "ADR-002-A-AMENDS-TWO.md")
	t.Setenv("SIRSI_ROUTER_DB", filepath.Join(t.TempDir(), "router.db"))

	cmd := newADRAuditCmd()
	cmd.SetArgs([]string{"--docs", dir})
	cmd.SetOut(os.Stderr)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("clean tree (incl. a sub-part) must pass, got: %v", err)
	}
}

// The ratchet: pre-existing collisions are grandfathered so the gate can land
// on a dirty tree, but a NEW collision must still fail. A gate that only ever
// passes is decoration; a gate that fails on day-one debt gets switched off.
func TestADRAuditRatchet(t *testing.T) {
	dir := writeADRs(t,
		"ADR-013-OLD-DEBT-A.md", "ADR-013-OLD-DEBT-B.md", // known
		"ADR-050-NEW-A.md", "ADR-050-NEW-B.md", // new
	)
	t.Setenv("SIRSI_ROUTER_DB", filepath.Join(t.TempDir(), "router.db"))

	// Known collision alone must pass.
	only13 := writeADRs(t, "ADR-013-OLD-DEBT-A.md", "ADR-013-OLD-DEBT-B.md")
	cmd := newADRAuditCmd()
	cmd.SetArgs([]string{"--docs", only13, "--allow", "13"})
	cmd.SetOut(os.Stderr)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("grandfathered collision must not fail the build: %v", err)
	}

	// A new collision alongside it must still fail.
	cmd2 := newADRAuditCmd()
	cmd2.SetArgs([]string{"--docs", dir, "--allow", "13"})
	cmd2.SetOut(os.Stderr)
	if err := cmd2.Execute(); err == nil {
		t.Fatal("a NEW collision must fail even when other numbers are grandfathered")
	}
}
