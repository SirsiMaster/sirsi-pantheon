package main_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

// The subprocess reads only a synthetic legacy registry in a private directory.
func threadSweepFixture(t *testing.T) (string, []byte, func(...string) (string, error)) {
	t.Helper()
	root, home := setupTempRouter(t), t.TempDir()
	registry := filepath.Join(root, ".agents", "idea-router", "threads.json")
	old := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339)
	rows := map[string]any{"threads": map[string]any{
		"thr-a": map[string]any{"thread_id": "thr-a", "agent_id": "fixture-a", "surface": "worker", "status": "active", "pid": 0, "started_at": old, "last_seen_at": old},
		"thr-b": map[string]any{"thread_id": "thr-b", "agent_id": "fixture-b", "surface": "worker", "status": "active", "pid": 0, "started_at": old, "last_seen_at": old},
	}}
	before, err := json.Marshal(rows)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(registry, before, 0600); err != nil {
		t.Fatal(err)
	}
	invoke := func(args ...string) (string, error) {
		t.Helper()
		cmd := exec.Command(testBinary, args...)
		cmd.Dir = root
		cmd.Env = sirsiTestEnv(root, filepath.Join(root, "fixture.db"), "HOME="+home, "SIRSI_ROUTER_STORE_WAKE=0")
		cmd.Stdin = nil
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	return registry, before, invoke
}

func TestThreadListReadOnly(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"plain", []string{"thread", "list"}},
		{"agent", []string{"thread", "list", "--agent", "fixture-a"}},
		{"json", []string{"thread", "list", "--json"}},
		{"agent-json", []string{"thread", "list", "--agent", "fixture-a", "--json"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			registry, before, invoke := threadSweepFixture(t)
			out, err := invoke(tc.args...)
			if err != nil {
				t.Fatalf("list: %v: %s", err, out)
			}
			after, err := os.ReadFile(registry)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatalf("list changed registry: %v", err)
			}
		})
	}
}

func TestThreadSweepScoped(t *testing.T) {
	registry, _, invoke := threadSweepFixture(t)
	if out, err := invoke("thread", "sweep", "--json"); err == nil {
		t.Fatalf("unscoped sweep accepted: %s", out)
	}
	out, err := invoke("thread", "sweep", "--agent", "fixture-a", "--json")
	if err != nil {
		t.Fatalf("scoped sweep: %v: %s", err, out)
	}
	var result struct {
		Dead       []struct{ ThreadID string } `json:"dead"`
		DeadError  string                      `json:"dead_error"`
		StrayError string                      `json:"stray_error"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil || len(result.Dead) != 1 || result.Dead[0].ThreadID != "thr-a" || result.DeadError != "" || result.StrayError != "" {
		t.Fatalf("sweep result %s, %v", out, err)
	}
	after, err := os.ReadFile(registry)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Threads map[string]struct {
			Status string `json:"status"`
		} `json:"threads"`
	}
	if err := json.Unmarshal(after, &got); err != nil {
		t.Fatal(err)
	}
	if got.Threads["thr-a"].Status != "reaped" || got.Threads["thr-b"].Status != "active" {
		t.Fatalf("wrong scoped results: %+v", got.Threads)
	}
}

func TestThreadListDoesNotImportLegacyIntoEmptyStore(t *testing.T) {
	root, home := setupTempRouter(t), t.TempDir()
	legacy := filepath.Join(root, ".agents", "idea-router", "threads.json")
	data := []byte(`{"threads":{"thr-legacy":{"thread_id":"thr-legacy","agent_id":"fixture-a","status":"active"}}}`)
	if err := os.WriteFile(legacy, data, 0600); err != nil {
		t.Fatal(err)
	}
	db := filepath.Join(root, "fixture.db")
	store, err := routerstore.OpenPath(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(testBinary, "thread", "list", "--json")
	cmd.Dir = root
	cmd.Env = sirsiTestEnv(root, db, "HOME="+home, "SIRSI_ROUTER_STORE_WAKE=1")
	cmd.Stdin = nil
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("list: %v: %s", err, out)
	}
	var rows []json.RawMessage
	if err := json.Unmarshal(out, &rows); err != nil || len(rows) != 0 {
		t.Fatalf("canonical empty-store result: %s (%v)", out, err)
	}
	sweep := exec.Command(testBinary, "thread", "sweep", "--agent", "fixture-a", "--json")
	sweep.Dir = root
	sweep.Env = sirsiTestEnv(root, db, "HOME="+home, "SIRSI_ROUTER_STORE_WAKE=1")
	sweep.Stdin = nil
	if out, err := sweep.CombinedOutput(); err != nil {
		t.Fatalf("empty scoped sweep: %v: %s", err, out)
	}
	store, err = routerstore.OpenPath(db)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	records, err := store.ListThreads()
	if err != nil || len(records) != 0 {
		t.Fatalf("list or scoped sweep imported legacy records: %d, %v", len(records), err)
	}
	after, err := os.ReadFile(legacy)
	if err != nil || !bytes.Equal(data, after) {
		t.Fatalf("list changed legacy file: %v", err)
	}
}

func TestThreadListDoesNotCreateOrMigrateLocalStore(t *testing.T) {
	root, home := setupTempRouter(t), t.TempDir()
	db := filepath.Join(root, "fixture.db")
	invoke := func() (string, error) {
		cmd := exec.Command(testBinary, "thread", "list", "--json")
		cmd.Dir = root
		cmd.Env = sirsiTestEnv(root, db, "HOME="+home, "SIRSI_ROUTER_STORE_WAKE=1")
		cmd.Stdin = nil
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	if out, err := invoke(); err == nil {
		t.Fatalf("missing store accepted: %s", out)
	}
	if _, err := os.Stat(db); !os.IsNotExist(err) {
		t.Fatalf("list created missing store: %v", err)
	}
	old, err := sql.Open("sqlite", db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := old.Exec(`CREATE TABLE legacy_only (id INTEGER); PRAGMA user_version=1`); err != nil {
		t.Fatal(err)
	}
	if err := old.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(db)
	if err != nil {
		t.Fatal(err)
	}
	if out, err := invoke(); err == nil {
		t.Fatalf("old schema unexpectedly accepted: %s", out)
	}
	after, err := os.ReadFile(db)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("list changed old store: %v", err)
	}
}

func TestThreadSweepJSONReportsFailures(t *testing.T) {
	root, home := setupTempRouter(t), t.TempDir()
	registry := filepath.Join(root, ".agents", "idea-router", "threads.json")
	if err := os.WriteFile(registry, []byte("invalid json"), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(testBinary, "thread", "sweep", "--agent", "fixture-a", "--json")
	cmd.Dir = root
	cmd.Env = sirsiTestEnv(root, filepath.Join(root, "fixture.db"), "HOME="+home, "SIRSI_ROUTER_STORE_WAKE=0")
	cmd.Stdin = nil
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err == nil {
		t.Fatalf("invalid registry accepted: %s", out.String())
	}
	var result struct {
		DeadError  string `json:"dead_error"`
		StrayError string `json:"stray_error"`
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil || result.DeadError == "" || result.StrayError == "" {
		t.Fatalf("missing JSON failure detail: stdout=%q stderr=%q err=%v", out.String(), stderr.String(), err)
	}
}
