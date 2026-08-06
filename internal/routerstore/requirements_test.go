package routerstore

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRequirementRegistryPersistsAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "router.db")
	now := time.Date(2026, 8, 6, 2, 0, 0, 0, time.UTC)
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	s.now = func() time.Time { return now }
	r := Requirement{RequirementID: "R1", Agent: "codex-pantheon", SourcePath: "docs/ADR-054.md", SourceAnchor: "R1", Statement: "one runnable predicate"}
	if addErr := s.AddRequirement(r); addErr != nil {
		t.Fatal(addErr)
	}
	if closeErr := s.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}

	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	got, err := s.GetRequirement("R1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "unmet" || got.Agent != r.Agent || got.SourceAnchor != "R1" || len(got.Evidence) != 0 {
		t.Fatalf("requirement after restart = %+v", got)
	}
	if err := s.AddRequirement(r); !errors.Is(err, ErrRequirementExists) {
		t.Fatalf("duplicate error = %v", err)
	}
}

func TestVerifiedRequirementRequiresCompletionEvidence(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "router.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if addErr := s.AddRequirement(Requirement{RequirementID: "R6", Agent: "codex-pantheon", SourcePath: "canon", Statement: "evidence gate"}); addErr != nil {
		t.Fatal(addErr)
	}
	partial := []RequirementEvidence{{Kind: "implementation", Label: "commit", Ref: "abc"}}
	if _, updateErr := s.UpdateRequirement("R6", "verified", "task-r6", partial); updateErr == nil || !strings.Contains(updateErr.Error(), "test evidence") {
		t.Fatalf("partial verified evidence error = %v", updateErr)
	}
	complete := []RequirementEvidence{
		{Kind: "implementation", Label: "commit", Ref: "abc"},
		{Kind: "test", Label: "race", Ref: "ci/1"},
		{Kind: "deployment", Label: "binary", Ref: "v1"},
		{Kind: "production", Label: "autonomous wake", Ref: "wake-1"},
	}
	got, err := s.UpdateRequirement("R6", "verified", "task-r6", complete)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "verified" || got.TaskID != "task-r6" || len(got.Evidence) != 4 {
		t.Fatalf("verified requirement = %+v", got)
	}
}

func TestRequirementListIsScopedAndDeterministic(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "router.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, r := range []Requirement{
		{RequirementID: "z", Agent: "b", SourcePath: "canon", Statement: "z"},
		{RequirementID: "b", Agent: "a", SourcePath: "canon", Statement: "b"},
		{RequirementID: "a", Agent: "a", SourcePath: "canon", Statement: "a"},
	} {
		if addErr := s.AddRequirement(r); addErr != nil {
			t.Fatal(addErr)
		}
	}
	got, err := s.ListRequirements("a")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].RequirementID != "a" || got[1].RequirementID != "b" {
		t.Fatalf("scoped requirements = %+v", got)
	}
}
