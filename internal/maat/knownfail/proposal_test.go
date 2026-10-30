package knownfail

import (
	"bytes"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
	projected := ProjectProposals(got)
	if len(projected) != 1 || projected[0].CatalogRelation != CatalogRelationCurrent {
		t.Fatalf("new proposal read projection = %#v", projected)
	}
	stored, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(stored, []byte("catalog_relation")) {
		t.Fatalf("read-time catalog classification was persisted: %s", stored)
	}
	if _, _, err := Propose(dir, Entry{ID: proposal.ID, Signature: "duplicate", Cause: "duplicate"}); err == nil {
		t.Fatal("duplicate proposal was accepted")
	}
}

func TestTrustedHistoricalCatalogSnapshotMatchesPinnedSourceEvidence(t *testing.T) {
	if len(trustedHistoricalCatalogs) != 1 {
		t.Fatalf("trusted historical catalog count = %d, want the reviewed bounded history", len(trustedHistoricalCatalogs))
	}
	evidence := trustedHistoricalCatalogs[0]
	if evidence.commit != "68b1ee17ac49d4c087c54bd4860eee597181fd45" ||
		evidence.path != "internal/maat/knownfail/catalog.json" ||
		evidence.blob != "325edb6a7017905209515529006fc908c126ce51" ||
		evidence.sha256 != "e8a1b1a1e295f3ff02eef4342d4bd315555960d561458687c8ee1be034b2b178" {
		t.Fatalf("historical source pin drifted: %#v", evidence)
	}
	sha := sha256.Sum256(evidence.bytes)
	if got := hex.EncodeToString(sha[:]); got != evidence.sha256 {
		t.Fatalf("historical snapshot SHA-256 = %s, want %s", got, evidence.sha256)
	}
	if got := gitBlobSHA1(evidence.bytes); got != evidence.blob {
		t.Fatalf("historical snapshot Git blob = %s, want %s", got, evidence.blob)
	}
}

func TestReadProposalsProjectsCurrentAndHistoricalPinsWithoutMutation(t *testing.T) {
	dir := t.TempDir()
	beforeDigest := CatalogSHA256()
	beforeMatches := Match("another golangci-lint instance is running")
	historical := proposalFixture("apollo-inline-zsh-path-binding", historicalCatalogSHA25668b1ee17)
	current := proposalFixture("current-catalog-example", CatalogSHA256())
	historicalRaw := proposalFixtureBytes(t, historical)
	var withForgedRelation map[string]any
	if err := json.Unmarshal(historicalRaw, &withForgedRelation); err != nil {
		t.Fatal(err)
	}
	withForgedRelation["catalog_relation"] = CatalogRelationCurrent
	historicalRaw, err := json.MarshalIndent(withForgedRelation, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	historicalRaw = append(historicalRaw, '\n')
	currentRaw := proposalFixtureBytes(t, current)
	writeProposalBytes(t, dir, historical.ID+".json", historicalRaw)
	writeProposalBytes(t, dir, current.ID+".json", currentRaw)

	before := map[string][]byte{
		historical.ID + ".json": append([]byte(nil), historicalRaw...),
		current.ID + ".json":    append([]byte(nil), currentRaw...),
	}
	proposals, err := ReadProposals(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(proposals) != 2 || proposals[0].ID != historical.ID || proposals[1].ID != current.ID {
		t.Fatalf("mixed proposal queue was not deterministically sorted: %#v", proposals)
	}
	projected := ProjectProposals(proposals)
	if len(projected) != 2 ||
		projected[0].CatalogRelation != CatalogRelationHistorical ||
		projected[1].CatalogRelation != CatalogRelationCurrent {
		t.Fatalf("read-time catalog relation ignored the record hash: %#v", projected)
	}
	assertProposalBytesUnchanged(t, dir, before)

	_ = ProjectProposals(proposals)
	if got := CatalogSHA256(); got != beforeDigest {
		t.Fatalf("proposal projection changed catalog digest: %s -> %s", beforeDigest, got)
	}
	if got := Match("another golangci-lint instance is running"); len(got) != len(beforeMatches) ||
		(len(got) > 0 && got[0].ID != beforeMatches[0].ID) {
		t.Fatalf("proposal read changed fabric Match: before=%#v after=%#v", beforeMatches, got)
	}
}

func TestReadProposalsRejectsInvalidMixedQueueWithoutMutation(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		proposal *Proposal
		raw      []byte
	}{
		{name: "unknown pin", filename: "unknown-pin.json", proposal: proposalPtr(proposalFixture("unknown-pin", strings.Repeat("0", 64)))},
		{name: "altered historical digest", filename: "altered-pin.json", proposal: proposalPtr(proposalFixture("altered-pin", "f"+historicalCatalogSHA25668b1ee17[1:]))},
		{name: "invalid timestamp", filename: "invalid-time.json", proposal: proposalPtr(withProposal(proposalFixture("invalid-time", CatalogSHA256()), func(p *Proposal) { p.CreatedAtUTC = "not-a-timestamp" }))},
		{name: "non-UTC timestamp", filename: "non-utc.json", proposal: proposalPtr(withProposal(proposalFixture("non-utc", CatalogSHA256()), func(p *Proposal) { p.CreatedAtUTC = "2026-10-05T20:00:00-04:00" }))},
		{name: "invalid regex", filename: "invalid-regex.json", proposal: proposalPtr(withProposal(proposalFixture("invalid-regex", CatalogSHA256()), func(p *Proposal) { p.Signature = "[" }))},
		{name: "wrong schema", filename: "wrong-schema.json", proposal: proposalPtr(withProposal(proposalFixture("wrong-schema", CatalogSHA256()), func(p *Proposal) { p.Schema = "other" }))},
		{name: "wrong status", filename: "wrong-status.json", proposal: proposalPtr(withProposal(proposalFixture("wrong-status", CatalogSHA256()), func(p *Proposal) { p.Status = "reviewed" }))},
		{name: "filename mismatch", filename: "z-filename.json", proposal: proposalPtr(proposalFixture("filename-bound-id", CatalogSHA256()))},
		{name: "malformed JSON", filename: "z-malformed.json", raw: []byte("{not-json\n")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			current := proposalFixture("a-valid-current", CatalogSHA256())
			currentRaw := proposalFixtureBytes(t, current)
			writeProposalBytes(t, dir, current.ID+".json", currentRaw)
			invalidRaw := tt.raw
			if tt.proposal != nil {
				invalidRaw = proposalFixtureBytes(t, *tt.proposal)
			}
			writeProposalBytes(t, dir, tt.filename, invalidRaw)
			before := map[string][]byte{
				current.ID + ".json": append([]byte(nil), currentRaw...),
				tt.filename:          append([]byte(nil), invalidRaw...),
			}
			if _, err := ReadProposals(dir); err == nil {
				t.Fatal("invalid member of a mixed queue was accepted")
			}
			assertProposalBytesUnchanged(t, dir, before)
		})
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

func proposalFixture(id, catalogPin string) Proposal {
	return Proposal{
		Schema:        proposalSchema,
		ID:            id,
		Signature:     "path binding failed",
		Cause:         "the configured shell path could not be bound",
		Status:        "proposed",
		CreatedAtUTC:  "2026-10-05T20:00:00Z",
		CatalogSHA256: catalogPin,
	}
}

func proposalFixtureBytes(t *testing.T, proposal Proposal) []byte {
	t.Helper()
	data, err := json.MarshalIndent(proposal, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return append(data, '\n')
}

func writeProposalBytes(t *testing.T, dir, filename string, data []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, filename), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertProposalBytesUnchanged(t *testing.T, dir string, before map[string][]byte) {
	t.Helper()
	for name, want := range before {
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("read preserved proposal %q: %v", name, err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("proposal %q bytes changed during read", name)
		}
	}
}

func proposalPtr(proposal Proposal) *Proposal { return &proposal }

func withProposal(proposal Proposal, update func(*Proposal)) Proposal {
	update(&proposal)
	return proposal
}

func gitBlobSHA1(data []byte) string {
	h := sha1.New()
	_, _ = fmt.Fprintf(h, "blob %s\x00", strconv.Itoa(len(data)))
	_, _ = h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}
