package routerstore

import (
	"reflect"
	"testing"
	"time"
)

func TestOwnerCancelPreservesCustodyAndTerminalHistory(t *testing.T) {
	s := openTestStore(t)
	for _, id := range []string{"live", "expired", "done", "canceled", "withdrawn", "next"} {
		if err := s.AddTask(Task{Agent: "lane", TaskID: id, Subject: id}); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{"live", "expired"} {
		if _, err := s.ClaimTask("lane", id, "worker", "thread", time.Minute); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.exec(`UPDATE tasks SET lease_expires=? WHERE agent='lane' AND task_id='expired'`, time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteTaskByOwner("lane", "done", "board:fixture@sirsi.ai", "existing receipt"); err != nil {
		t.Fatal(err)
	}
	if err := s.CancelTask("lane", "canceled", "board:fixture@sirsi.ai", "original reason"); err != nil {
		t.Fatal(err)
	}
	if err := s.WithdrawTask("lane", "withdrawn", "lane/next", "lane@fixture", "existing successor"); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"live", "expired", "done", "canceled", "withdrawn"} {
		t.Run(id, func(t *testing.T) {
			before, err := s.GetTask("lane", id)
			if err != nil {
				t.Fatal(err)
			}
			token, expires, live, err := s.taskLeaseSnapshot("lane", id)
			if err != nil {
				t.Fatal(err)
			}
			if err := s.CancelTask("lane", id, "board:fixture@sirsi.ai", "retirement must not rewrite"); err == nil {
				t.Fatal("cancellation erased custody or terminal history")
			}
			after, err := s.GetTask("lane", id)
			if err != nil {
				t.Fatal(err)
			}
			afterToken, afterExpires, afterLive, err := s.taskLeaseSnapshot("lane", id)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, after) || token != afterToken || expires != afterExpires || live != afterLive {
				t.Fatal("refusal changed original record or custody")
			}
		})
	}
}

func TestOwnerCancelBeforeClaimLeavesTerminalUnclaimable(t *testing.T) {
	s := openTestStore(t)
	if err := s.AddTask(Task{Agent: "lane", TaskID: "idle", Subject: "reviewed"}); err != nil {
		t.Fatal(err)
	}
	if err := s.CancelTask("lane", "idle", "board:fixture@sirsi.ai", "owner retired, not completed"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ClaimTask("lane", "idle", "worker", "thread", time.Minute); err == nil {
		t.Fatal("claimed canceled task")
	}
	got, err := s.GetTask("lane", "idle")
	if err != nil || got.Status != "canceled" {
		t.Fatalf("status=%s err=%v", got.Status, err)
	}
	if _, exists, err := s.GetState(receiptKey("lane", "idle")); err != nil || exists {
		t.Fatal("cancel created a completion receipt")
	}
}
