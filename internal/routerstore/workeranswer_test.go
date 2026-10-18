package routerstore

import (
	"encoding/json"
	"errors"
	"fmt"
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

// Through the service: a thread write from a binary that predates answer_role (no key at all) cannot erase a
// stored grant; a current client's explicit false revokes it. 2026-10-18: old-image Horus/dashboard
// heartbeats rewrote every same-host record and stripped every worker's grant.
func TestOldWriterCannotEraseAnswerRole(t *testing.T) {
	var logs strings.Builder
	backend, client := ruleHarness(t, "enforce", &logs)
	host, _ := os.Hostname()
	base := time.Date(2026, 9, 10, 15, 0, 0, 0, time.UTC).Add(-time.Minute)
	registerThread(t, backend, "thr-granted", "lane-b", host, `{"surface":"worker","answer_role":true}`, base)
	registerThread(t, backend, "thr-plain", "lane-c", host, `{"surface":"worker"}`, base)
	at := func(i int) string { return base.Add(time.Duration(i) * time.Second).UTC().Format(time.RFC3339Nano) }
	role := func(id string) string {
		t.Helper()
		threads, err := backend.ListThreads()
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range threads {
			if r.ThreadID == id {
				var p map[string]any
				_ = json.Unmarshal(r.Payload, &p)
				return fmt.Sprint(p["answer_role"])
			}
		}
		t.Fatalf("thread %s missing", id)
		return ""
	}
	c := client("lane-b", "thr-granted")
	rec := func(i int, payload string) ThreadRecord {
		return ThreadRecord{ThreadID: "thr-granted", Agent: "lane-b", Status: "active", LastSeenAt: at(i), Payload: []byte(payload), Host: host}
	}

	if ok, err := c.UpsertThreadCAS(rec(1, `{"surface":"worker","status":"active"}`)); err != nil || !ok {
		t.Fatalf("old-style heartbeat: ok=%v err=%v", ok, err)
	}
	if got := role("thr-granted"); got != "true" {
		t.Fatalf("a write with no answer_role key erased the grant: answer_role=%s", got)
	}
	if err := c.UpsertThreads([]ThreadRecord{rec(2, `{"surface":"worker"}`)}); err != nil {
		t.Fatal(err)
	}
	if got := role("thr-granted"); got != "true" {
		t.Fatalf("bulk write erased the grant: answer_role=%s", got)
	}
	if ok, err := c.UpsertThreadCAS(rec(3, `{"surface":"worker","answer_role":false}`)); err != nil || !ok {
		t.Fatalf("revoke: ok=%v err=%v", ok, err)
	}
	if got := role("thr-granted"); got != "false" {
		t.Fatalf("an explicit false must revoke: answer_role=%s", got)
	}
	plain := ThreadRecord{ThreadID: "thr-plain", Agent: "lane-c", Status: "active", LastSeenAt: at(4), Payload: []byte(`{"surface":"worker"}`), Host: host}
	if _, err := client("lane-c", "thr-plain").UpsertThreadCAS(plain); err != nil {
		t.Fatal(err)
	}
	if got := role("thr-plain"); got != "<nil>" {
		t.Fatalf("a thread with no stored grant must not gain one: answer_role=%s", got)
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
