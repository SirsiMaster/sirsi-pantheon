package routerstore

import (
	"testing"
	"time"
)

func TestExactTaskClaimAcknowledgesOnlyItsSourceWake(t *testing.T) {
	s := newTestStore(t)
	_ = s.AddTask(Task{Agent: "codex-home", TaskID: "A", Subject: "a"})
	_ = s.AddTask(Task{Agent: "codex-home", TaskID: "B", Subject: "b"})
	events, _ := s.ListWakeEvents("codex-home")
	var eventA WakeEvent
	for _, event := range events {
		if event.SourceID == "A" {
			eventA = event
		}
	}
	_, _ = s.db.Exec(`UPDATE wake_events SET created='2020-01-01T00:00:00Z' WHERE event_id=?`, eventA.EventID)
	leased, err := s.ClaimWakeEventFor("codex-home", time.Minute)
	if err != nil || leased.SourceID != "A" {
		t.Fatalf("lease event A: %+v %v", leased, err)
	}
	b, err := s.ClaimTask("codex-home", "B", "worker-b", "thread-b", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	events, _ = s.ListWakeEvents("codex-home")
	for _, event := range events {
		if event.EventID == leased.EventID && event.Status != "leased" {
			t.Fatalf("exact claim B acknowledged source A: %+v", event)
		}
	}
	if err := s.ReleaseTaskLease(b.Agent, b.TaskID, b.Token, "yield"); err != nil {
		t.Fatal(err)
	}
	a, err := s.ClaimTask("codex-home", "A", "worker-a", "thread-a", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	events, _ = s.ListWakeEvents("codex-home")
	for _, event := range events {
		if event.EventID == leased.EventID {
			wantRef := "task-lease:codex-home:A:" + a.Token
			if event.Status != "acked" || event.AckRef != wantRef {
				t.Fatalf("exact source wake ack = %+v, want ref %q", event, wantRef)
			}
		}
	}
}
