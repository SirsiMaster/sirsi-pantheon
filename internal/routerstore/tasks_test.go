package routerstore

import (
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestTaskLifecycle(t *testing.T) {
	s := newTestStore(t)
	if err := s.AddTask(Task{Agent: "claude-nexus", TaskID: "sne-01", Subject: "land backend"}); err != nil {
		t.Fatalf("AddTask: %v", err)
	}
	if err := s.AddTask(Task{Agent: "claude-nexus", TaskID: "sne-01", Subject: "duplicate"}); !errors.Is(err, ErrTaskExists) {
		t.Fatalf("duplicate = %v, want ErrTaskExists", err)
	}
	task, err := s.UpdateTask("claude-nexus", "sne-01", TaskUpdate{ResponsibleParty: "codex", BlockedBy: "sne-00", BlockedBySet: true})
	if err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}
	if task.Status != "pending" || task.ResponsibleParty != "codex" || task.BlockedBy != "sne-00" {
		t.Fatalf("updated task = %+v", task)
	}
	listed, err := s.ListTasks("claude-nexus")
	if err != nil || len(listed) != 1 {
		t.Fatalf("ListTasks = %+v, %v", listed, err)
	}
}

func TestTaskExecutionTransitionsRequireLease(t *testing.T) {
	s := newTestStore(t)
	if err := s.AddTask(Task{Agent: "codex-home", TaskID: "R1", Subject: "enforce work"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateTask("codex-home", "R1", TaskUpdate{Status: "in-progress"}); err == nil {
		t.Fatal("unfenced transition to in-progress was accepted")
	}
	if _, err := s.UpdateTask("codex-home", "R1", TaskUpdate{Status: "done"}); err == nil {
		t.Fatal("unfenced transition to done was accepted")
	}
	lease, err := s.ClaimNextTask("codex-home", "worker", "thread", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteTaskLease("codex-home", "R1", lease.Token, "proof://R1"); err != nil {
		t.Fatal(err)
	}
}

func TestTaskV4V5V6MigrateDirectlyToV7(t *testing.T) {
	for _, startingVersion := range []int{4, 5, 6} {
		t.Run(fmt.Sprintf("v%d", startingVersion), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "router.db")
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			for _, migration := range migrations {
				if migration.version > 4 {
					break
				}
				if _, err = db.Exec(migration.sql); err != nil {
					t.Fatalf("apply v%d: %v", migration.version, err)
				}
			}
			if _, err = db.Exec(fmt.Sprintf("PRAGMA user_version=%d", startingVersion)); err != nil {
				t.Fatal(err)
			}
			created := "2026-08-01T12:00:00Z"
			if _, err = db.Exec(`INSERT INTO tasks(agent,task_id,subject,status,phase,responsible_party,blocked_by,created,updated) VALUES(?,?,?,?,?,?,?,?,?)`, "a", "old", "legacy", "pending", "phase", "self", "", created, created); err != nil {
				t.Fatal(err)
			}
			if err = db.Close(); err != nil {
				t.Fatal(err)
			}

			s, err := Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			var version int
			if err = s.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
				t.Fatal(err)
			}
			// Assert the CURRENT max, not a literal: this test is about v4-v6
			// databases migrating forward with the v7 task columns populated,
			// not about 7 being the newest schema. Hardcoding 7 made every
			// future migration break an unrelated test (v8 did exactly that).
			wantVersion := migrations[len(migrations)-1].version
			if version != wantVersion {
				t.Fatalf("version=%d, want %d (current max)", version, wantVersion)
			}
			got, err := s.GetTask("a", "old")
			if err != nil {
				t.Fatal(err)
			}
			if got.CommissionedAt != created || got.CommissionedBy != "a" || got.Stage != "spec" || got.TestState != "untested" {
				t.Fatalf("migrated task = %+v", got)
			}
		})
	}
}

func TestTaskV7DefaultsAndDerivedLiveness(t *testing.T) {
	s := newTestStore(t)
	if err := s.AddTask(Task{Agent: "codex-inference", TaskID: "sne-52", Subject: "build ledger", Status: "in-progress"}); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetTask("codex-inference", "sne-52")
	if err != nil {
		t.Fatal(err)
	}
	if got.CommissionedAt != got.Created || got.CommissionedBy != got.Agent || got.TestState != "untested" || got.Stage != "spec" || got.Liveness != "active" {
		t.Fatalf("v7 defaults = %+v", got)
	}
	if got.Timeline == nil || got.Links == nil {
		t.Fatal("JSON arrays must not be null")
	}

	s.now = func() time.Time { return time.Date(2026, 7, 2, 19, 4, 5, 0, time.UTC) }
	got, _ = s.GetTask("codex-inference", "sne-52")
	if got.Liveness != "stalled" {
		t.Fatalf("at threshold liveness=%q", got.Liveness)
	}
	_, err = s.UpdateTask("codex-inference", "sne-52", TaskUpdate{BlockedBy: "owner", BlockedBySet: true})
	if err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetTask("codex-inference", "sne-52")
	if got.Liveness != "blocked" {
		t.Fatalf("blocked liveness=%q", got.Liveness)
	}
}

func TestTaskV7EvidenceLinksAndAccounting(t *testing.T) {
	s := newTestStore(t)
	if err := s.AddTask(Task{Agent: "a", TaskID: "t", Subject: "work"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateTask("a", "t", TaskUpdate{TestState: "passed"}); err == nil {
		t.Fatal("passed without evidence accepted")
	}
	evidence := TaskLink{Kind: "evidence", Label: "test", URL: "file:///proof.json"}
	got, err := s.UpdateTask("a", "t", TaskUpdate{Links: []TaskLink{evidence, evidence}, TestState: "passed", AddTokens: 10, AddSeconds: 20})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Links, []TaskLink{evidence}) || got.TokensConsumed != 10 || got.DurationSeconds != 20 {
		t.Fatalf("updated = %+v", got)
	}
	got, err = s.UpdateTask("a", "t", TaskUpdate{AddTokens: 5, AddSeconds: 7})
	if err != nil {
		t.Fatal(err)
	}
	if got.TokensConsumed != 15 || got.DurationSeconds != 27 {
		t.Fatalf("accounting = %+v", got)
	}
	if _, err := s.UpdateTask("a", "t", TaskUpdate{AddTokens: -1}); err == nil {
		t.Fatal("negative increment accepted")
	}
}

func TestTaskV7AccountingConcurrentIncrements(t *testing.T) {
	s := newTestStore(t)
	if err := s.AddTask(Task{Agent: "a", TaskID: "cost", Subject: "cost center"}); err != nil {
		t.Fatal(err)
	}
	const workers = 20
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.UpdateTask("a", "cost", TaskUpdate{AddTokens: 3, AddSeconds: 2})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.GetTask("a", "cost")
	if err != nil {
		t.Fatal(err)
	}
	if got.TokensConsumed != workers*3 || got.DurationSeconds != workers*2 {
		t.Fatalf("lost increment: %+v", got)
	}
}

func TestTaskV7StageRegressionAndCharterGovernance(t *testing.T) {
	s := newTestStore(t)
	charter := "ship it"
	if err := s.AddTask(Task{Agent: "a", TaskID: "t", Subject: "work", Charter: &charter, Stage: "verify"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateTask("a", "t", TaskUpdate{Stage: "build"}); err == nil {
		t.Fatal("unexplained stage regression accepted")
	}
	if _, err := s.UpdateTask("a", "t", TaskUpdate{Stage: "build", Subject: "return to build: benchmark defect"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateTask("a", "t", TaskUpdate{CharterSet: true, Charter: "new"}); err == nil {
		t.Fatal("charter amendment without instruction accepted")
	}
	_, err := s.UpdateTask("a", "t", TaskUpdate{CharterSet: true, Charter: "new", Links: []TaskLink{{Kind: "owner-instruction", Label: "directive", URL: "router://item/1"}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateTask("a", "t", TaskUpdate{CharterSet: true, Charter: "newer"}); err == nil {
		t.Fatal("historic instruction link incorrectly authorized a new charter amendment")
	}
}

func TestTaskValidation(t *testing.T) {
	s := newTestStore(t)
	if err := s.AddTask(Task{Agent: "a", TaskID: "t", Subject: "x", Status: "maybe", ResponsibleParty: "self"}); err == nil {
		t.Fatal("invalid status accepted")
	}
}
