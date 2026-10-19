package router

import (
	"encoding/json"
	"testing"
	"time"
)

// A save must not rewrite a thread it did not change just because the stored bytes came from another
// encoder: an older binary without answer_role, or the service's answer_role carry-over (sorted keys).
// 2026-10-19: every v0.24.117 heartbeat on the M1 rewrote unrelated records and lost the fence.
func TestUnchangedThreadIsNotDirtyAcrossEncodings(t *testing.T) {
	th := Thread{ThreadID: "thr-a", AgentID: "a", Surface: "worker", Status: ThreadStatusActive,
		PID: 4242, LastSeenAt: time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC)}
	current, err := json.Marshal(&th)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(current, &m); err != nil {
		t.Fatal(err)
	}
	delete(m, "answer_role")
	older, _ := json.Marshal(m) // an older writer: no answer_role key, sorted keys
	m["answer_role"] = false
	carried, _ := json.Marshal(m) // the service carry-over: key restored, sorted keys
	for name, stored := range map[string][]byte{"older writer": older, "service carry-over": carried, "same encoder": current} {
		if payloadChanged("thr-a", stored, current) {
			t.Errorf("%s: an unchanged thread was marked dirty", name)
		}
	}
	th.LastSeenAt = th.LastSeenAt.Add(time.Second)
	moved, _ := json.Marshal(&th)
	if !payloadChanged("thr-a", current, moved) {
		t.Error("a real change was not marked dirty")
	}
	if !payloadChanged("thr-a", []byte("{not json"), current) {
		t.Error("an unreadable stored payload must be dirty")
	}
}
