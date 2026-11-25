package main

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/schedule"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

// `reserve --queue --wait`: a free resource is granted at once, --wait without
// --queue is refused, and a held resource times out non-zero with reason_code
// wait-timeout and leaves no queued row behind.
func TestReserveCLIWait(t *testing.T) {
	oldLedger, oldJournal := maatLedger, newMaatDecisionJournal
	oldHolder, oldWork, oldRegime, oldStart, oldEnd := resHolder, resWork, resRegime, resStart, resEstEnd
	oldCores, oldQueue, oldJSON, oldNonce, oldWait := resCores, resQueue, maatJSON, maatOperationNonce, resWait
	t.Cleanup(func() {
		maatLedger, newMaatDecisionJournal = oldLedger, oldJournal
		resHolder, resWork, resRegime, resStart, resEstEnd = oldHolder, oldWork, oldRegime, oldStart, oldEnd
		resCores, resQueue, maatJSON, maatOperationNonce, resWait = oldCores, oldQueue, oldJSON, oldNonce, oldWait
	})
	store, err := routerstore.OpenPath(filepath.Join(t.TempDir(), "fixture.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ledger := schedule.NewLedger(store)
	maatLedger = func() (*schedule.Ledger, error) { return ledger, nil }
	newMaatDecisionJournal = func() (maat.DecisionJournal, error) { return &capturedDecisionJournal{}, nil }

	run := func(resource, holder string, queue bool, wait int) (map[string]any, error) {
		resHolder, resWork, resRegime, resStart = holder, "w", "build", ""
		resEstEnd = time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
		resCores, resQueue, maatJSON, maatOperationNonce, resWait = 0, queue, true, "", wait
		var runErr error
		out := captureStdout(t, func() { runErr = maatReserveCmd.RunE(maatReserveCmd, []string{resource}) })
		var got map[string]any
		if out != "" {
			if err := json.Unmarshal([]byte(out), &got); err != nil {
				t.Fatalf("stdout must be one JSON object: %q %v", out, err)
			}
		}
		return got, runErr
	}

	if _, err := run("m5", "a", false, 5); err == nil {
		t.Fatal("--wait without --queue must be refused")
	}
	got, err := run("m5", "a", true, 5)
	if err != nil || got["granted"] != true {
		t.Fatalf("a free resource is granted at once: %v %v", got, err)
	}
	got, err = run("m5", "b", true, 1)
	if err == nil || !errors.Is(err, schedule.ErrWaitTimeout) || got["reason_code"] != "wait-timeout" || got["granted"] != false || got["error"] == "" {
		t.Fatalf("held resource: want wait-timeout, got %v %v", got, err)
	}
	rows, _ := ledger.Status("m5")
	for _, r := range rows {
		if r.Holder == "b" && r.Status != schedule.StatusReleased {
			t.Fatalf("the timed-out waiter's row must be canceled: %+v", r)
		}
	}
}
