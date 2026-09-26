package maat

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileDecisionJournalAppendsAndReturnsNewestFirst(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "decisions.jsonl")
	j := &FileDecisionJournal{Path: path}
	for _, d := range []Decision{
		{Time: "2026-09-26T10:00:00Z", Host: "m5", Kind: "reservation", Requester: "alpha", Resource: "m5", Assessed: "free", Determination: "grant", Why: "no overlap"},
		{Time: "2026-09-26T10:01:00Z", Host: "m5", Kind: "reservation", Requester: "beta", Resource: "m5", Assessed: "held", Affected: "alpha", Determination: "refuse", Why: "active reservation"},
	} {
		if err := j.Append(d); err != nil {
			t.Fatal(err)
		}
	}
	got, err := j.Recent(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Requester != "beta" || got[0].Affected != "alpha" {
		t.Fatalf("Recent(1) = %+v, want newest refusal", got)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("journal mode = %o, want 0600", info.Mode().Perm())
	}
}

func TestFileDecisionJournalRejectsMalformedRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "decisions.jsonl")
	if err := os.WriteFile(path, []byte("{not json}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := (&FileDecisionJournal{Path: path}).Recent(10); err == nil {
		t.Fatal("Recent accepted malformed JSONL")
	}
}

func TestFileDecisionJournalReadsExistingMaatDecisionShape(t *testing.T) {
	path := filepath.Join(t.TempDir(), "decisions.jsonl")
	// This is the existing cross-lane JSONL shape. New Pantheon records must
	// join it rather than requiring a private schema wrapper.
	raw := `{"affected":"m9-smoke","assessed":"held 2026-09-26T10:44:12-04:00 → 10:49","determination":"released","evidence":"20260926T144413Z-m9-smoke-smoke-test","host":"Mac","kind":"reservation release","requester":"smoke-test","resource":"m9-smoke","time":"2026-09-26T14:44:22Z","why":"smoke test"}` + "\n"
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := (&FileDecisionJournal{Path: path}).Recent(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Assessed == "" || got[0].Why != "smoke test" || got[0].Affected != "m9-smoke" {
		t.Fatalf("existing decision shape = %+v", got)
	}
}
