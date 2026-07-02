package main

// Tests for the `sirsi activity --json` contract (TUI design proof gap V4).
// NOTE: these tests swap package globals (activityPathFn, JsonOutput) under
// explicit save/restore — no t.Parallel(), ever (repo lessons #129/#131).

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// TestRunActivity_JSONContract runs the command end-to-end against a fixture
// ledger: the free-text log goes in, structured newest-first JSON comes out.
func TestRunActivity_JSONContract(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "operations.log")
	fixture := "2026-07-01T09:00:00  clean  /Users/x/Library/Caches/old (4096 bytes)\n" +
		"2026-07-01T09:05:00  purge  /Users/x/dev/app/node_modules (1048576 bytes)\n" +
		"2026-07-01T09:10:00  clean  /Users/x/Library/Logs/stale\n"
	if err := os.WriteFile(logPath, []byte(fixture), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	// Swap the injectable providers + output mode; restore afterwards.
	activityMu.Lock()
	prevPathFn := activityPathFn
	activityPathFn = func() string { return logPath }
	activityMu.Unlock()
	prevJSON, prevLimit := JsonOutput, activityLimit
	JsonOutput, activityLimit = true, 2
	defer func() {
		activityMu.Lock()
		activityPathFn = prevPathFn
		activityMu.Unlock()
		JsonOutput, activityLimit = prevJSON, prevLimit
	}()

	// Capture stdout (the JSON contract goes to stdout, prose to stderr).
	prevStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	runErr := runActivity(activityCmd, nil)
	w.Close()
	os.Stdout = prevStdout
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)

	if runErr != nil {
		t.Fatalf("runActivity: %v", runErr)
	}

	var report struct {
		Command string `json:"command"`
		LogPath string `json:"log_path"`
		Count   int    `json:"count"`
		Entries []struct {
			Time   string `json:"time"`
			Action string `json:"action"`
			Target string `json:"target"`
			Bytes  int64  `json:"bytes"`
			Source string `json:"source"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(buf.Bytes(), &report); err != nil {
		t.Fatalf("output is not the JSON contract: %v\n%s", err, buf.String())
	}
	if report.Command != "sirsi activity" || report.LogPath != logPath {
		t.Errorf("envelope = %q / %q", report.Command, report.LogPath)
	}
	if report.Count != 2 || len(report.Entries) != 2 {
		t.Fatalf("limit 2 should yield 2 entries, got count=%d len=%d", report.Count, len(report.Entries))
	}
	// Newest first.
	first := report.Entries[0]
	if first.Time != "2026-07-01T09:10:00" || first.Action != "clean" || first.Target != "/Users/x/Library/Logs/stale" || first.Bytes != 0 {
		t.Errorf("entries[0] must be the newest line, got %+v", first)
	}
	if report.Entries[1].Bytes != 1048576 || report.Entries[1].Action != "purge" {
		t.Errorf("entries[1] = %+v", report.Entries[1])
	}
	if first.Source != "oplog" {
		t.Errorf("source = %q, want oplog", first.Source)
	}
}

// TestRunActivity_MissingLedger: no operations yet is a normal state — valid
// JSON with entries:[], not an error.
func TestRunActivity_MissingLedger(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "never-written.log")

	activityMu.Lock()
	prevPathFn := activityPathFn
	activityPathFn = func() string { return missing }
	activityMu.Unlock()
	prevJSON := JsonOutput
	JsonOutput = true
	defer func() {
		activityMu.Lock()
		activityPathFn = prevPathFn
		activityMu.Unlock()
		JsonOutput = prevJSON
	}()

	prevStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	runErr := runActivity(activityCmd, nil)
	w.Close()
	os.Stdout = prevStdout
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)

	if runErr != nil {
		t.Fatalf("missing ledger must not error: %v", runErr)
	}
	var report struct {
		Count   int               `json:"count"`
		Entries []json.RawMessage `json:"entries"`
	}
	if err := json.Unmarshal(buf.Bytes(), &report); err != nil {
		t.Fatalf("invalid JSON for empty ledger: %v\n%s", err, buf.String())
	}
	if report.Count != 0 || report.Entries == nil || len(report.Entries) != 0 {
		t.Errorf("empty ledger should be count=0, entries:[] — got count=%d entries=%v", report.Count, report.Entries)
	}
}
