package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
	"github.com/spf13/cobra"
)

// These controls execute the actual capacity Cobra/RunE and Resolve path over
// an explicitly private database. No network listener, live session, process,
// service, registration or production router state is needed. Empty private
// PATH prevents FindRepoRoot's optional git lookup from launching a child.
func capacityPrivateFixture(t *testing.T) (*sql.DB, string) {
	t.Helper()
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "SIRSI_") || strings.HasPrefix(key, "GIT_") || key == "CODEX_SESSION_ID" || key == "CODEX_THREAD_ID" {
			t.Setenv(key, "")
		}
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PATH", t.TempDir())
	repo := t.TempDir()
	root := filepath.Join(repo, ".agents", "idea-router")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(repo)
	path := filepath.Join(t.TempDir(), "capacity-owned.db")
	t.Setenv("SIRSI_ROUTER_DB", path)
	t.Setenv("SIRSI_ALLOW_SCHEMA_MIGRATE", "1")
	store, err := routerstore.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, root
}

func capacityExec(t *testing.T, args ...string) (string, error) {
	t.Helper()
	oldJSON := JsonOutput
	JsonOutput = false
	t.Cleanup(func() { JsonOutput = oldJSON })
	var out bytes.Buffer
	root := &cobra.Command{Use: "fixture-sirsi", SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().BoolVar(&JsonOutput, "json", false, "machine output")
	group := &cobra.Command{Use: "router"}
	group.AddCommand(newRouterCapacityCmd())
	root.AddCommand(group)
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs(append([]string{"router", "capacity"}, args...))
	err := root.Execute()
	return out.String(), err
}

func capacitySQL(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatal(err)
	}
}

// Preserve every raw column of every fixture item/task, including custody,
// leases, timestamps, charter, outline and status; not just derived counts.
func capacityRawSnapshot(t *testing.T, db *sql.DB, tables ...string) string {
	t.Helper()
	var snapshot []any
	for _, table := range tables { // table names are fixed test literals only.
		rows, err := db.Query("SELECT * FROM " + table + " ORDER BY 1, 2")
		if err != nil {
			t.Fatal(err)
		}
		columns, err := rows.Columns()
		if err != nil {
			_ = rows.Close()
			t.Fatal(err)
		}
		snapshot = append(snapshot, columns)
		for rows.Next() {
			values := make([]any, len(columns))
			pointers := make([]any, len(columns))
			for i := range values {
				pointers[i] = &values[i]
			}
			if err := rows.Scan(pointers...); err != nil {
				_ = rows.Close()
				t.Fatal(err)
			}
			// Drivers may reuse byte buffers after the next Scan.
			for i, value := range values {
				if b, ok := value.([]byte); ok {
					values[i] = string(b)
				}
			}
			snapshot = append(snapshot, values)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			t.Fatal(err)
		}
		if err := rows.Close(); err != nil {
			t.Fatal(err)
		}
	}
	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func capacitySeed(t *testing.T, db *sql.DB, unacked, stalled int) {
	t.Helper()
	old := time.Now().UTC().Add(-72 * time.Hour).Format(time.RFC3339)
	future := time.Now().UTC().Add(72 * time.Hour).Format(time.RFC3339)
	for i := 0; i < unacked; i++ {
		capacitySQL(t, db, `INSERT INTO items(id,from_agent,to_agent,title,status) VALUES(?,?,?,?,?)`,
			fmt.Sprintf("open-%d", i), "fixture-sender", "fixture-recipient", "owned", "open")
	}
	for i := 0; i < stalled; i++ {
		// Blocked is still included in recipient admission's stale backlog.
		capacitySQL(t, db, `INSERT INTO tasks(agent,task_id,subject,status,responsible_party,created,updated) VALUES(?,?,?,?,?,?,?)`,
			"fixture-recipient", fmt.Sprintf("stalled-%d", i), "owned", "blocked", "fixture-recipient", old, old)
	}
	// Discriminators: do not count acknowledged/closed/other-recipient items,
	// done/canceled/fresh/live-leased tasks, or another owner's stale task.
	capacitySQL(t, db, `INSERT INTO items(id,from_agent,to_agent,title,status,acked_at) VALUES
		('acked','fixture-sender','fixture-recipient','owned','open',?),
		('closed','fixture-sender','fixture-recipient','owned','closed',''),
		('other','fixture-sender','other-recipient','owned','open','')`, old)
	for _, task := range []struct {
		id, agent, status, updated, lease string
	}{
		{"done", "fixture-recipient", "done", old, ""},
		{"canceled", "fixture-recipient", "canceled", old, ""},
		{"fresh", "fixture-recipient", "pending", future, ""},
		{"leased", "fixture-recipient", "in-progress", old, future},
		{"other", "other-recipient", "pending", old, ""},
	} {
		capacitySQL(t, db, `INSERT INTO tasks(agent,task_id,subject,status,responsible_party,created,updated,lease_expires) VALUES(?,?,?,?,?,?,?,?)`,
			task.agent, task.id, "owned", task.status, task.agent, old, task.updated, task.lease)
	}
}

func TestRouterCapacityCanonicalDBCountsAndReadOnly(t *testing.T) {
	for _, tc := range []struct {
		name             string
		unacked, stalled int
		warn, halt       bool
	}{
		{"zero", 0, 0, false, false},
		{"below-warning", 49, 3, false, false},
		{"warning-unacked", 50, 0, true, false},
		{"warning-stalled", 0, 50, true, false},
		{"below-halt", 1, 98, true, false},
		{"halt-unacked", 99, 0, true, true},
		{"halt-stalled", 0, 99, true, true},
		{"not-clamped-to-threshold", 2, 600, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, root := capacityPrivateFixture(t)
			capacitySeed(t, db, tc.unacked, tc.stalled)
			before := capacityRawSnapshot(t, db, "items", "tasks", "wake_events")
			raw, err := capacityExec(t, "fixture-recipient", "--json")
			if err != nil {
				t.Fatal(err)
			}
			var got struct {
				Agent             string `json:"agent"`
				Unacked           int    `json:"unacked"`
				Stalled           int    `json:"stalled"`
				WarnAt            int    `json:"warn_at"`
				HaltAt            int    `json:"halt_at"`
				StallAfterSeconds int64  `json:"stall_after_seconds"`
				ThresholdSource   string `json:"threshold_source"`
				Warning           string `json:"warning"`
				AtHaltThreshold   bool   `json:"at_halt_threshold"`
				Scope             string `json:"scope"`
			}
			decoder := json.NewDecoder(strings.NewReader(raw))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&got); err != nil {
				t.Fatal(err)
			}
			var trailing any
			if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
				t.Fatalf("extra JSON/text: %q, %v", raw, err)
			}
			if got.Agent != "fixture-recipient" || got.Unacked != tc.unacked || got.Stalled != tc.stalled || got.AtHaltThreshold != tc.halt || (got.Warning != "") != tc.warn {
				t.Fatalf("counts/warning/threshold drift: %s", raw)
			}
			if got.WarnAt != 50 || got.HaltAt != 99 || got.StallAfterSeconds != 86400 || got.ThresholdSource != "client" || got.Scope != "recipient admission backlog only; not workability, routability or fleet counts" {
				t.Fatalf("threshold provenance/scope lost: %s", raw)
			}
			if tc.warn && !strings.Contains(got.Warning, fmt.Sprintf("%d unacknowledged items, %d stalled tasks", tc.unacked, tc.stalled)) {
				t.Fatalf("warning not based on exact counts: %s", raw)
			}
			text, err := capacityExec(t, "fixture-recipient")
			if err != nil {
				t.Fatal(err)
			}
			wantText := fmt.Sprintf("Recipient capacity: fixture-recipient\n  unacked: %d\n  stalled: %d\n  client thresholds: warn=50 halt=99 stall_after_seconds=86400 (server policy version not attested)\n  at client halt threshold: %t\n  warning: %q\n  scope: recipient admission backlog only; not workability, routability or fleet counts\n", tc.unacked, tc.stalled, tc.halt, got.Warning)
			if text != wantText {
				t.Fatalf("text contract drift: %q", text)
			}
			if after := capacityRawSnapshot(t, db, "items", "tasks", "wake_events"); after != before {
				t.Fatal("capacity changed raw item/task/wake state")
			}
			if _, err := os.Stat(filepath.Join(root, "items")); !os.IsNotExist(err) {
				t.Fatalf("read created/accessed unexpected items directory: %v", err)
			}
		})
	}
}

func TestRouterCapacityReadFailureNeverPublishesZero(t *testing.T) {
	db, root := capacityPrivateFixture(t)
	capacitySeed(t, db, 3, 4)
	capacitySQL(t, db, "ALTER TABLE tasks RENAME TO capacity_tasks_unavailable")
	before := capacityRawSnapshot(t, db, "items", "capacity_tasks_unavailable", "wake_events")
	for _, args := range [][]string{{"fixture-recipient"}, {"fixture-recipient", "--json"}} {
		out, err := capacityExec(t, args...)
		if err == nil || !strings.Contains(err.Error(), "capacity: read recipient load") || out != "" {
			t.Fatalf("failed read must not publish zero/success: %q, %v", out, err)
		}
	}
	if after := capacityRawSnapshot(t, db, "items", "capacity_tasks_unavailable", "wake_events"); after != before {
		t.Fatal("failed read changed raw state")
	}
	if _, err := os.Stat(filepath.Join(root, "items")); !os.IsNotExist(err) {
		t.Fatalf("failed read created items directory: %v", err)
	}
	// Opening a broken canonical target must also refuse; never use file counts
	// or the healthy fixture database as an alternate authority.
	t.Setenv("SIRSI_ROUTER_DB", t.TempDir()) // a directory is not a database.
	out, err := capacityExec(t, "fixture-recipient", "--json")
	if err == nil || !strings.Contains(err.Error(), "capacity: open canonical router") || out != "" {
		t.Fatalf("failed open fell back/published success: %q, %v", out, err)
	}
}

func TestRouterCapacityExactArgumentsBeforeStoreOpen(t *testing.T) {
	registered, _, err := rootCmd.Find([]string{"router", "capacity"})
	if err != nil || registered != routerCapacityCmd {
		t.Fatalf("capacity is not registered on the production router command: %v", err)
	}
	db, root := capacityPrivateFixture(t)
	capacitySeed(t, db, 1, 1)
	before := capacityRawSnapshot(t, db, "items", "tasks", "wake_events")
	t.Setenv("SIRSI_ROUTER_DB", t.TempDir()) // proves rejection precedes Open.
	for _, args := range [][]string{
		{}, {""}, {" "}, {"\t"}, {"\u2003"}, {" fixture-recipient"},
		{"fixture-recipient "}, {"fixture\nrecipient"}, {"fixture\x00recipient"},
		{"fixture-recipient", "extra"}, {"--unknown"},
	} {
		out, err := capacityExec(t, args...)
		if err == nil || strings.Contains(err.Error(), "capacity: open canonical router") || out != "" {
			t.Fatalf("argument case %q reached store/published success: %q, %v", args, out, err)
		}
	}
	if after := capacityRawSnapshot(t, db, "items", "tasks", "wake_events"); after != before {
		t.Fatal("argument refusal changed raw state")
	}
	if _, err := os.Stat(filepath.Join(root, "items")); !os.IsNotExist(err) {
		t.Fatalf("argument refusal created items directory: %v", err)
	}
}
