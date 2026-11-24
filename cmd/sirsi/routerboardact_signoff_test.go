package main

import (
	"path/filepath"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

func TestVerifiedBoardSignoffBindsOpeningOwnerAndPreservesTerminalRecord(t *testing.T) {
	st, err := routerstore.OpenPath(filepath.Join(t.TempDir(), "fixture.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.AddTask(routerstore.Task{Agent: "lane", TaskID: "verified", Subject: "Synthetic verified fixture"}); err != nil {
		t.Fatal(err)
	}
	before, _ := st.GetTask("lane", "verified")
	for _, body := range []string{
		`{"agent":"lane","task_id":"verified","note":"evidence"}`,
		`{"agent":"lane","task_id":"verified","note":"evidence","expected_owner":"board:other@sirsi.ai"}`,
	} {
		if _, err := runBoardAct(st, "board:owner@sirsi.ai", "sign-off-verified", []byte(body)); err == nil {
			t.Fatal("unbound owner was accepted")
		}
		after, _ := st.GetTask("lane", "verified")
		if after.Status != before.Status || after.ResultRef != before.ResultRef || after.Updated != before.Updated {
			t.Fatal("owner refusal changed task")
		}
	}
	body := []byte(`{"agent":"lane","task_id":"verified","note":"exact scope and evidence accepted","expected_owner":"board:owner@sirsi.ai"}`)
	if _, err := runBoardAct(st, "board:owner@sirsi.ai", "sign-off-verified", body); err != nil {
		t.Fatal(err)
	}
	done, _ := st.GetTask("lane", "verified")
	if done.Status != "done" || done.ResultRef != "owner:board:owner@sirsi.ai" {
		t.Fatalf("wrong owner receipt: status=%s ref=%s", done.Status, done.ResultRef)
	}
	if _, err := runBoardAct(st, "board:owner@sirsi.ai", "sign-off-verified", body); err == nil {
		t.Fatal("terminal replay accepted")
	}
	after, _ := st.GetTask("lane", "verified")
	if after.ResultRef != done.ResultRef || after.Updated != done.Updated {
		t.Fatal("terminal replay changed original receipt")
	}
}
