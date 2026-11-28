package routerstore

import (
	"testing"
	"time"
)

// A later acknowledged wake resolves an older terminal wake failure; a failure
// newer than every ack stays unresolved (UNROUTABLE for a runnable lane).
func TestClassifyLaneLaterAckedWakeResolvesTerminalFailure(t *testing.T) {
	for _, tc := range []struct {
		name       string
		failedAgo  time.Duration
		ackedAgo   time.Duration // 0 = no acked wake
		ackedAgent string
		unresolved int
		class      string
	}{
		{"later-ack-resolves", 10 * time.Minute, 5 * time.Minute, "codex-home", 0, LaneIdleWithWork},
		{"failure-after-ack-unresolved", 5 * time.Minute, 10 * time.Minute, "codex-home", 1, LaneUnroutable},
		{"equal-timestamp-unresolved", 5 * time.Minute, 5 * time.Minute, "codex-home", 1, LaneUnroutable},
		{"no-ack-unresolved", 10 * time.Minute, 0, "", 1, LaneUnroutable},
		{"other-agent-ack-ignored", 10 * time.Minute, 5 * time.Minute, "someone-else", 1, LaneUnroutable},
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
			insertLaneFailure(t, s, "failed", now.Add(-tc.failedAgo).Format(time.RFC3339))
			if tc.ackedAgo > 0 {
				stamp := now.Add(-tc.ackedAgo).Format(time.RFC3339)
				if _, err := s.db.Exec(`INSERT INTO wake_events(event_id,event_key,agent,source_kind,source_id,reason,status,ack_ref,created,updated) VALUES('acked','acked',?,'lane','x','ok','acked','router-lease:i:t',?,?)`, tc.ackedAgent, stamp, stamp); err != nil {
					t.Fatal(err)
				}
			}
			state, err := s.ClassifyLane("codex-home", true, time.Minute)
			if err != nil || state.TerminalWakeFailures != 1 || state.UnresolvedWakeFailures != tc.unresolved || state.Classification != tc.class {
				t.Fatalf("got %+v err=%v, want unresolved=%d class=%s", state, err, tc.unresolved, tc.class)
			}
		})
	}
}
