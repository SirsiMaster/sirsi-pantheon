package router

import (
	"testing"
	"time"
)

// A worker loop whose lead lane has not granted it the answering role holds dispatch (the lead
// answers) and reports the no_grant hold; a granted lane dispatches; an unreadable registry holds.
func TestWorkerWithoutAnswerGrantHoldsDispatch(t *testing.T) {
	setAttendedLiveFn(func(string, string) bool { return false })
	t.Cleanup(func() { setAttendedLiveFn(nil) })
	setLaneGrantFn(nil) // the real registry probe
	t.Cleanup(func() { setLaneGrantFn(func(string, string) bool { return true }) })
	root := writeReg(t, `{"agents":{
	  "granted":{"id":"granted","type":"claude","worker_answers":true,"wake":{"mechanism":"launchagent"}},
	  "led":{"id":"led","type":"claude","wake":{"mechanism":"launchagent"}}}}`)

	if !workerLacksAnswerGrant(root, "led", 3) {
		t.Fatal("a lane with no worker_answers grant was not held")
	}
	if workerLacksAnswerGrant(root, "granted", 3) {
		t.Fatal("a granted lane was held")
	}
	if hold, _ := currentHold(root, "led", 0, time.Time{}); hold != HoldNoGrant {
		t.Fatalf("lane-state hold = %q, want %q", hold, HoldNoGrant)
	}
	if hold, _ := currentHold(root, "granted", 0, time.Time{}); hold == HoldNoGrant {
		t.Fatal("a granted lane reported the no_grant hold")
	}
	if !workerLacksAnswerGrant(writeReg(t, `{not json`), "granted", 3) {
		t.Fatal("an unreadable registry must hold (the service would refuse the answer)")
	}
}
