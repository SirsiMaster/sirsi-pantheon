package routerstore

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func registerThread(t *testing.T, backend Store, threadID, agent, host, payload string, seen time.Time) {
	t.Helper()
	if err := backend.UpsertThreads([]ThreadRecord{{ThreadID: threadID, Agent: agent, Status: "active",
		LastSeenAt: seen.UTC().Format(time.RFC3339Nano), Payload: []byte(payload), Host: host}}); err != nil {
		t.Fatal(err)
	}
}

// Through the service: a headless worker cannot close an item without its lane's answering role; a granted
// worker and a lead (attended) session can; acknowledging stays open to the ungranted worker.
func TestHeadlessWorkerAnswersOnlyWithItsLeadsGrant(t *testing.T) {
	var logs strings.Builder
	backend, client := ruleHarness(t, "enforce", &logs)
	host, _ := os.Hostname()
	now := time.Date(2026, 9, 10, 15, 0, 0, 0, time.UTC).Add(-time.Minute)

	registerThread(t, backend, "thr-worker", "lane-a", host, `{"surface":"worker"}`, now)
	registerThread(t, backend, "thr-granted", "lane-b", host, `{"surface":"worker","answer_role":true}`, now)
	registerThread(t, backend, "thr-lead", "lane-a", host, `{"surface":"claude"}`, now)
	send := func(to, title string) string {
		t.Helper()
		id, _, err := backend.SendGuarded(SendReq{From: "asker", To: to, Title: title, Type: "decision", Instructions: "a question?"})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}

	q := send("lane-a", "for lane a")
	if err := client("lane-a", "thr-worker").AckItem(q); err != nil {
		t.Fatalf("an ungranted worker must still be able to acknowledge: %v", err)
	}
	if err := client("lane-a", "thr-worker").CloseItem(q, "worker answer"); !errors.Is(err, ErrWorkerAnswers) {
		t.Fatalf("ungranted worker answering: err = %v, want ErrWorkerAnswers", err)
	}
	if it, _ := backend.Get(q); it.Status == "closed" {
		t.Fatal("the refused answer still closed the item")
	}
	if err := client("lane-a", "thr-lead").CloseItem(q, "lead answer"); err != nil {
		t.Fatalf("the lead lane must answer: %v", err)
	}
	if err := client("lane-b", "thr-granted").CloseItem(send("lane-b", "for lane b"), "granted worker answer"); err != nil {
		t.Fatalf("a worker with its lead's answering role must answer: %v", err)
	}
}

func TestWorkerMayAnswer(t *testing.T) {
	threads := []ThreadRecord{
		{ThreadID: "w", Payload: []byte(`{"surface":"worker"}`)},
		{ThreadID: "g", Payload: []byte(`{"surface":"worker","answer_role":true}`)},
		{ThreadID: "l", Payload: []byte(`{"surface":"claude"}`)},
	}
	for id, want := range map[string]bool{"w": false, "g": true, "l": true, "unknown": true} {
		if got := WorkerMayAnswer(threads, id); got != want {
			t.Errorf("WorkerMayAnswer(%q) = %v, want %v", id, got, want)
		}
	}
}
