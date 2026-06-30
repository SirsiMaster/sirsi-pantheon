package prd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSummarize(t *testing.T) {
	p := &PRD{Tasks: []Task{
		{ID: "A", Status: "done"},
		{ID: "B", Status: "open"},
		{ID: "C", Status: "owner-gated"},
		{ID: "D", Status: "blocked", Note: "x"},
		{ID: "E", Status: "open"},
	}}
	s := Summarize(p)
	if s.Total != 5 || s.ByStatus["open"] != 2 || s.ByStatus["done"] != 1 {
		t.Errorf("counts wrong: %+v", s)
	}
	if s.Complete {
		t.Error("Complete should be false with open/blocked tasks")
	}
	if len(s.OpenIDs) != 3 || s.OpenIDs[0] != "B" { // open(B,E) + blocked(D), sorted
		t.Errorf("OpenIDs = %v, want [B D E]", s.OpenIDs)
	}
}

func TestSummarize_Complete(t *testing.T) {
	p := &PRD{Tasks: []Task{{ID: "A", Status: "done"}, {ID: "B", Status: "owner-gated"}}}
	if !Summarize(p).Complete {
		t.Error("all done/owner-gated should be Complete")
	}
}

func TestValidate_Clean(t *testing.T) {
	root := t.TempDir()
	// canon_ref must resolve — create it
	_ = os.WriteFile(filepath.Join(root, "ROADMAP.md"), []byte("x"), 0o644)
	p := &PRD{
		Agent:     "claude-x",
		CanonRefs: []string{"ROADMAP.md"},
		Tasks:     []Task{{ID: "P1", Desc: "do a thing", Status: "open"}},
	}
	if probs := Validate(p, root); len(probs) != 0 {
		t.Errorf("clean PRD reported problems: %v", probs)
	}
}

func TestValidate_CatchesProblems(t *testing.T) {
	root := t.TempDir()
	p := &PRD{
		Agent:     "", // missing agent
		CanonRefs: []string{"docs/MISSING.md"},
		Tasks: []Task{
			{ID: "P1", Desc: "ok", Status: "open"},
			{ID: "P1", Desc: "dup id", Status: "done"},             // duplicate id
			{ID: "P2", Desc: "", Status: "open"},                   // empty desc
			{ID: "P3", Desc: "bad status", Status: "wibble"},       // invalid status
			{ID: "P4", Desc: "blocked no note", Status: "blocked"}, // blocked w/o note
		},
	}
	probs := Validate(p, root)
	// expect: missing agent, dup id, empty desc, invalid status, blocked-no-note, missing canon_ref
	if len(probs) < 6 {
		t.Errorf("expected >=6 problems, got %d: %v", len(probs), probs)
	}
	joined := ""
	for _, s := range probs {
		joined += s + "\n"
	}
	for _, want := range []string{"agent", "duplicate id", "empty desc", "invalid status", "blocked without", "canon_ref not found"} {
		if !contains(joined, want) {
			t.Errorf("problems missing %q: %v", want, probs)
		}
	}
}

func TestLoadSaveRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "claude-x.json")
	orig := &PRD{Agent: "claude-x", App: "Sirsi", Tasks: []Task{{ID: "P1", Desc: "d", Status: "open"}}}
	if err := Save(path, orig); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Agent != "claude-x" || len(got.Tasks) != 1 || got.Tasks[0].ID != "P1" {
		t.Errorf("roundtrip lost data: %+v", got)
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle ||
		indexOf(haystack, needle) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
