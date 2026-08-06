package routerstore

import (
	"path/filepath"
	"testing"
)

func seedRunnableItem(t *testing.T, s *Store, item Item) {
	t.Helper()
	_, err := s.db.Exec(`INSERT INTO items(id,from_agent,to_agent,title,status) VALUES(?,?,?,?,?)`, item.ID, item.From, item.To, item.Title, item.Status)
	if err != nil {
		t.Fatal(err)
	}
}

func TestRunnableThreeSourcePredicate(t *testing.T) {
	tests := []struct {
		name         string
		seed         func(*testing.T, *Store)
		wantRunnable bool
		want         RunnableState
	}{
		{name: "empty lane may park", want: RunnableState{Agent: "a"}},
		{name: "open message", wantRunnable: true, want: RunnableState{Agent: "a", OpenItems: 1}, seed: func(t *testing.T, s *Store) {
			seedRunnableItem(t, s, Item{ID: "item", From: "b", To: "a", Title: "work", Status: "open"})
		}},
		{name: "actionable task", wantRunnable: true, want: RunnableState{Agent: "a", ActionableTasks: 1}, seed: func(t *testing.T, s *Store) {
			if err := s.AddTask(Task{Agent: "a", TaskID: "task", Subject: "work", Status: "pending", ResponsibleParty: "a"}); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "unmet canon", wantRunnable: true, want: RunnableState{Agent: "a", UnmetRequirements: 1}, seed: func(t *testing.T, s *Store) {
			if err := s.AddRequirement(Requirement{RequirementID: "R", Agent: "a", SourcePath: "canon", Statement: "work"}); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "blocked and terminal sources may park", want: RunnableState{Agent: "a"}, seed: func(t *testing.T, s *Store) {
			if err := s.AddTask(Task{Agent: "a", TaskID: "blocked", Subject: "wait", Status: "blocked", ResponsibleParty: "a", BlockedBy: "dep"}); err != nil {
				t.Fatal(err)
			}
			if err := s.AddRequirement(Requirement{RequirementID: "R", Agent: "a", SourcePath: "canon", Statement: "wait", Status: "blocked"}); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := Open(filepath.Join(t.TempDir(), "router.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			if tt.seed != nil {
				tt.seed(t, s)
			}
			got, err := s.Runnable("a")
			if err != nil {
				t.Fatal(err)
			}
			tt.want.Runnable = tt.wantRunnable
			if got != tt.want {
				t.Fatalf("Runnable = %+v, want %+v", got, tt.want)
			}
		})
	}
}
