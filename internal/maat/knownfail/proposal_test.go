package knownfail

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProposeCreatesReadBackVerifiedLocalEvidence(t *testing.T) {
	dir := t.TempDir()
	proposal, path, err := Propose(dir, Entry{ID: "apollo-catalog-root", Title: "Catalog root", Signature: "catalog missing", Cause: "ambient checkout absent"})
	if err != nil {
		t.Fatal(err)
	}
	if proposal.Status != "proposed" || proposal.CatalogSHA256 != CatalogSHA256() {
		t.Fatalf("unexpected proposal: %#v", proposal)
	}
	if filepath.Dir(path) != dir {
		t.Fatalf("proposal path %q escaped %q", path, dir)
	}
	got, err := ReadProposals(dir)
	if err != nil || len(got) != 1 || got[0].ID != proposal.ID {
		t.Fatalf("ReadProposals = %#v, %v", got, err)
	}
	if _, _, err := Propose(dir, Entry{ID: proposal.ID, Signature: "duplicate", Cause: "duplicate"}); err == nil {
		t.Fatal("duplicate proposal was accepted")
	}
}

func TestReadProposalsRefusesTamperedOrUnexpectedEntries(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "forged.json"), []byte(`{"schema":"sirsi.maat.known-failure-proposal.v1","id":"forged","signature":"x","cause":"x","status":"proposed","created_at_utc":"now","catalog_sha256":"forged"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadProposals(dir); err == nil {
		t.Fatal("tampered proposal was accepted")
	}
}
