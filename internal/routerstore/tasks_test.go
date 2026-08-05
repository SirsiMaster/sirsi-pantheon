package routerstore

import (
	"errors"
	"testing"
)

func TestTaskLifecycle(t *testing.T) {
	s := newTestStore(t)
	if err := s.AddTask(Task{Agent: "claude-nexus", TaskID: "sne-01", Subject: "land backend"}); err != nil {
		t.Fatalf("AddTask: %v", err)
	}
	if err := s.AddTask(Task{Agent: "claude-nexus", TaskID: "sne-01", Subject: "duplicate"}); !errors.Is(err, ErrTaskExists) {
		t.Fatalf("duplicate = %v, want ErrTaskExists", err)
	}
	task, err := s.UpdateTask("claude-nexus", "sne-01", TaskUpdate{Status: "in-progress", ResponsibleParty: "codex", BlockedBy: "sne-00", BlockedBySet: true})
	if err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}
	if task.Status != "in-progress" || task.ResponsibleParty != "codex" || task.BlockedBy != "sne-00" {
		t.Fatalf("updated task = %+v", task)
	}
	listed, err := s.ListTasks("claude-nexus")
	if err != nil || len(listed) != 1 {
		t.Fatalf("ListTasks = %+v, %v", listed, err)
	}
}

func TestTaskValidation(t *testing.T) {
	s := newTestStore(t)
	if err := s.AddTask(Task{Agent: "a", TaskID: "t", Subject: "x", Status: "maybe", ResponsibleParty: "self"}); err == nil {
		t.Fatal("invalid status accepted")
	}
}
