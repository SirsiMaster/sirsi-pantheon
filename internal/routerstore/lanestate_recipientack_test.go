package routerstore

import (
	"testing"
	"time"
)

// A recipient acknowledgement (recipient-only, never delegable) after a terminal
// wake failure resolves it even with no wake_events ack; a close by anyone does
// not, because items record no closer.
func TestClassifyLaneRecipientAckResolvesTerminalFailure(t *testing.T) {
	for _, tc := range []struct {
		name       string
		to         string // recipient of the evidence item
		ackedAgo   time.Duration
		closedOnly bool // closed (e.g. by ra), never acked
		unresolved int
		class      string
	}{
		{"lane-acked-item-after-failure", "codex-home", 5 * time.Minute, false, 0, LaneIdleWithWork},
		{"ack-before-failure", "codex-home", 15 * time.Minute, false, 1, LaneUnroutable},
		{"item-closed-by-other-after-failure", "codex-home", 5 * time.Minute, true, 1, LaneUnroutable},
		{"ack-on-other-lanes-item", "someone-else", 5 * time.Minute, false, 1, LaneUnroutable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestStore(t)
			now := time.Now().UTC().Truncate(time.Second)
			s.now = func() time.Time { return now }
			if err := s.MarkRequirementAudit("codex-home", "audit://current"); err != nil {
				t.Fatal(err)
			}
			if err := s.AddTask(Task{Agent: "codex-home", TaskID: "work", Subject: "pending work"}); err != nil {
				t.Fatal(err)
			}
			insertLaneFailure(t, s, "failed", now.Add(-10*time.Minute).Format(time.RFC3339))
			stamp := now.Add(-tc.ackedAgo).Format(time.RFC3339)
			if err := s.Put(Item{ID: "i1", From: "ra", To: tc.to, Title: "t", Type: "task", Status: "open", Opened: stamp}); err != nil {
				t.Fatal(err)
			}
			if tc.closedOnly {
				if err := s.CloseItem("i1", "closed by ra"); err != nil {
					t.Fatal(err)
				}
			} else if _, err := s.db.Exec(`UPDATE items SET acked_at=? WHERE id='i1'`, stamp); err != nil {
				t.Fatal(err)
			}
			state, err := s.ClassifyLane("codex-home", true, time.Minute)
			if err != nil || state.TerminalWakeFailures != 1 || state.UnresolvedWakeFailures != tc.unresolved || state.Classification != tc.class {
				t.Fatalf("got %+v err=%v, want unresolved=%d class=%s", state, err, tc.unresolved, tc.class)
			}
		})
	}
}
