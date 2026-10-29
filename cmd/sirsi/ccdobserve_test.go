package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCcdObservationRefusesPartialOrFailedIdentity(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		err        error
		startOK    bool
	}{
		{"query denied", "", errors.New("fixture denied"), true},
		{"partial query", "42\n", errors.New("fixture partial"), true},
		{"unreadable start", "42\n", nil, false},
		{"malformed PID", "42\nnot-a-pid\n", nil, true},
		{"duplicate PID", "42\n42\n", nil, true},
		{"nonpositive PID", "0\n", nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rows, err := ccdObserveRunners(func() ([]byte, error) { return []byte(tc.body), tc.err }, func(int) (float64, bool) { return 100, tc.startOK })
			if err == nil || len(rows) != 0 {
				t.Fatal("incomplete observation became usable", rows, err)
			}
		})
	}
	rows, err := ccdObserveRunners(func() ([]byte, error) { return []byte("42\n43\n"), nil }, func(pid int) (float64, bool) { return float64(pid) + 10, true })
	if err != nil || len(rows) != 2 || rows[0].pid != 42 || rows[1].start != 53 {
		t.Fatal("complete observation refused", rows, err)
	}
}

func TestCcdObservationDistinguishesNoMatchFromDiagnostics(t *testing.T) {
	for _, tc := range []struct {
		name, script string
		wantError    bool
	}{
		{"successful empty", "exit 0", false},
		{"documented no match", "exit 1", false},
		{"error status", "exit 2", true},
		{"no match with diagnostic", "echo fixture-denied >&2; exit 1", true},
		{"partial failure", "echo 42; exit 1", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rows, err := ccdObserveRunners(func() ([]byte, error) {
				// Inert owned child supplies real ExitError/Stderr semantics;
				// never invokes pgrep, ps or any reap/archive operation.
				return exec.Command("/bin/sh", "-c", tc.script).Output()
			}, func(int) (float64, bool) { t.Fatal("empty/failed query inspected a PID"); return 0, false })
			if (err != nil) != tc.wantError || len(rows) != 0 {
				t.Fatal("query status misclassified", rows, err)
			}
		})
	}
}

func TestCcdIncompleteObservationPreservesPrivateArchiveCanary(t *testing.T) {
	for _, failure := range []string{"query failed", "unattributable runner"} {
		t.Run(failure, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			path := filepath.Join(home, "Library/Application Support/Claude/claude-code-sessions/local_fixture.json")
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			body := []byte(`{"sessionId":"owned-inert-canary","createdAt":1,"lastActivityAt":1,"isArchived":false}`)
			if err := os.WriteFile(path, body, 0600); err != nil {
				t.Fatal(err)
			}
			oldCollect, oldApply := ccdCollectRunners, ccdReapApply
			t.Cleanup(func() { ccdCollectRunners, ccdReapApply = oldCollect, oldApply })
			ccdReapApply = true // A missing refusal would touch ONLY this inert private canary.
			ccdCollectRunners = func() ([]ccdRunner, error) {
				if failure == "query failed" {
					return nil, errors.New("fixture process query denied")
				}
				return []ccdRunner{{pid: 424242, start: 1_800_000_000}}, nil
			}
			if err := ccdReapCmd.RunE(ccdReapCmd, nil); err == nil || !strings.Contains(err.Error(), "refusing reap/archive") {
				t.Fatal("incomplete observation did not refuse before effects", err)
			}
			after, err := os.ReadFile(path)
			if err != nil || string(after) != string(body) {
				t.Fatal("private archive canary changed", err)
			}
		})
	}
}
