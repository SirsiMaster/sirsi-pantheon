package router

import (
	"context"
	"testing"
	"time"
)

// The wake loop stamps answer_role on its worker thread exactly when its lane grants it (registry
// worker_answers); the router service then lets that worker answer.
func TestWakeLoopStampsAnswerRoleFromTheLeadsGrant(t *testing.T) {
	root := wakeTestRoot(t,
		AgentConfig{ID: "granted", Type: "gemma", WorkerAnswers: true},
		AgentConfig{ID: "ungranted", Type: "gemma"},
	)
	for agent, want := range map[string]bool{"granted": true, "ungranted": false} {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // register + record once, then exit
		if err := RunWakeLoop(ctx, root, agent, time.Hour); err != nil {
			t.Fatalf("RunWakeLoop(%s): %v", agent, err)
		}
		reg, err := LoadThreadRegistry(root)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, th := range reg.Threads {
			if th != nil && th.AgentID == agent && th.Surface == surfaceWorker {
				found = true
				if th.AnswerRole != want {
					t.Errorf("%s worker thread answer_role = %v, want %v", agent, th.AnswerRole, want)
				}
			}
		}
		if !found {
			t.Fatalf("no worker thread recorded for %s", agent)
		}
	}
}
