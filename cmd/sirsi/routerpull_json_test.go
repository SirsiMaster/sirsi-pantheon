package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/work"
	"github.com/spf13/cobra"
)

// Exercise the production Cobra command/flags/RunE with only its inbox read
// replaced. No dispatch server, store, registration, acknowledgement or network.
func routerPullFixtureCommand(t *testing.T, read func(string) ([]work.Item, error), out, warnings io.Writer) *cobra.Command {
	t.Helper()
	oldJSON := JsonOutput
	t.Cleanup(func() { JsonOutput = oldJSON })
	root := &cobra.Command{Use: "fixture-sirsi", SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().BoolVar(&JsonOutput, "json", false, "machine output")
	root.SetOut(out)
	root.SetErr(warnings)
	root.AddCommand(newRouterPullCmd(read))
	return root
}

func decodeRouterPullDocument(t *testing.T, raw string) []work.Item {
	t.Helper()
	decoder := json.NewDecoder(strings.NewReader(raw))
	var rows []work.Item
	if err := decoder.Decode(&rows); err != nil {
		t.Fatalf("machine stdout was not an item array: %q: %v", raw, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		t.Fatalf("machine stdout contained trailing text/document: %q: %v", raw, err)
	}
	if rows == nil {
		t.Fatalf("empty inbox encoded null instead of []: %q", raw)
	}
	return rows
}

func TestRouterPullJSONUsesOneCanonicalInboxReadAndOneDocument(t *testing.T) {
	items := []work.Item{
		{ID: "fixture-old", From: "fixture-sender", To: "codex-pantheon", Status: "open", Title: "quoted \"title\"\nnext line", Opened: "2026-10-25T00:00:00Z", Instructions: "canonical own-inbox body; no separate show lookup"},
		{ID: "fixture-new", From: "fixture-reviewer", To: "codex-pantheon", Status: "open", Type: "review", Title: "review fixture", Opened: "2026-10-25T00:00:01Z"},
	}
	var out, warnings bytes.Buffer
	calls := []string{}
	cmd := routerPullFixtureCommand(t, func(agent string) ([]work.Item, error) {
		calls = append(calls, agent)
		return items, nil
	}, &out, &warnings)
	cmd.SetArgs([]string{"pull", "codex-pantheon", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if got := decodeRouterPullDocument(t, out.String()); !reflect.DeepEqual(got, items) {
		t.Fatalf("canonical inbox/order changed or unrelated detail added: %+v", got)
	}
	if !reflect.DeepEqual(calls, []string{"codex-pantheon"}) || warnings.Len() != 0 {
		t.Fatalf("unexpected read or diagnostics: %v %q", calls, warnings.String())
	}
}

func TestRouterPullJSONEmptyAndBuildFilterAreClosedMachineOutput(t *testing.T) {
	rows := []work.Item{
		{ID: "plain", Type: ""}, {ID: "build", Type: "build"}, {ID: "task", Type: "task"},
		{ID: "decision", Type: "decision", Instructions: "deferred-private-body"},
		{ID: "review", Type: " REVIEW "}, {ID: "proposal", Type: "proposal"},
		{ID: "custom", Type: "existing-custom-build-type"},
	}
	for _, tc := range []struct {
		name     string
		rows     []work.Item
		filter   bool
		want     []string
		deferred bool
	}{
		{"nil-empty", nil, false, []string{}, false},
		{"non-nil-empty", []work.Item{}, false, []string{}, false},
		{"filtered-mixed", rows, true, []string{"plain", "build", "task", "custom"}, true},
		{"filtered-all-deferred", rows[3:6], true, []string{}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out, warnings bytes.Buffer
			calls := 0
			cmd := routerPullFixtureCommand(t, func(agent string) ([]work.Item, error) {
				calls++
				if agent != "codex-pantheon" {
					t.Fatalf("reader redirected to %q", agent)
				}
				return tc.rows, nil
			}, &out, &warnings)
			args := []string{"pull", "codex-pantheon", "--json"}
			if tc.filter {
				args = append(args, "--build-filter")
			}
			cmd.SetArgs(args)
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			got := decodeRouterPullDocument(t, out.String())
			ids := make([]string, 0, len(got))
			for _, item := range got {
				ids = append(ids, item.ID)
			}
			if !reflect.DeepEqual(ids, tc.want) || calls != 1 {
				t.Fatalf("filter/read drift: %v calls=%d", ids, calls)
			}
			if len(tc.want) == 0 && out.String() != "[]\n" {
				t.Fatalf("empty machine shape: %q", out.String())
			}
			if (warnings.Len() > 0) != tc.deferred {
				t.Fatalf("deferred warning lost/misplaced: %q", warnings.String())
			}
			if tc.deferred && !strings.Contains(warnings.String(), "route to human/agent lane: sirsi router respond") {
				t.Fatal("deferred guidance disappeared")
			}
			if strings.Contains(out.String(), "deferred-private-body") || strings.Contains(warnings.String(), "deferred-private-body") {
				t.Fatal("filtered body escaped in output/diagnostics")
			}
		})
	}
}

func TestRouterPullHumanOutputAndBuildWarningsRemainUnchanged(t *testing.T) {
	item := work.Item{ID: "fixture-review", From: "fixture-sender", Type: "review", Title: "review fixture", Opened: "2026-10-25T00:00:00Z"}
	var out, warnings bytes.Buffer
	cmd := routerPullFixtureCommand(t, func(string) ([]work.Item, error) { return []work.Item{item}, nil }, &out, &warnings)
	cmd.SetArgs([]string{"pull", "codex-pantheon"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	want := "  1 open items for codex-pantheon:\n\n  • fixture-review\n      type: review\n    from: fixture-sender\n      title: review fixture\n      opened: 2026-10-25T00:00:00Z\n\n  Read full: sirsi router show <id>\n  Close when done: sirsi router close <id> --result @path/to/result.md\n"
	if out.String() != want || warnings.Len() != 0 {
		t.Fatalf("human text drift: %q diagnostics=%q", out.String(), warnings.String())
	}
	out.Reset()
	filtered := routerPullFixtureCommand(t, func(string) ([]work.Item, error) { return []work.Item{item}, nil }, &out, &warnings)
	filtered.SetArgs([]string{"pull", "codex-pantheon", "--build-filter"})
	if err := filtered.Execute(); err != nil {
		t.Fatal(err)
	}
	want = "  ⚠ 1 item(s) skipped — not build-shaped (type: decision/review/proposal):\n    • fixture-review  type:review  from:fixture-sender\n      → route to human/agent lane: sirsi router respond fixture-review --result \"<answer>\"\n\n  No open items for codex-pantheon.\n"
	if out.String() != want || warnings.Len() != 0 {
		t.Fatalf("human filter text drift: %q diagnostics=%q", out.String(), warnings.String())
	}
}

type routerPullFailedWriter struct{ err error }

func (w routerPullFailedWriter) Write([]byte) (int, error) { return 0, w.err }

func TestRouterPullReturnsReaderAndOutputErrors(t *testing.T) {
	for _, mode := range []string{"read", "json-writer", "empty-json-writer", "warning-writer", "human-writer"} {
		t.Run(mode, func(t *testing.T) {
			failure := errors.New("inert " + mode + " failure")
			var out, warnings bytes.Buffer
			var output, diagnostic io.Writer = &out, &warnings
			if mode == "json-writer" || mode == "empty-json-writer" || mode == "human-writer" {
				output = routerPullFailedWriter{failure}
			}
			if mode == "warning-writer" {
				diagnostic = routerPullFailedWriter{failure}
			}
			calls := 0
			cmd := routerPullFixtureCommand(t, func(string) ([]work.Item, error) {
				calls++
				if mode == "read" {
					return nil, failure
				}
				if mode == "empty-json-writer" {
					return nil, nil
				}
				return []work.Item{{ID: "fixture-item", Type: "review"}}, nil
			}, output, diagnostic)
			args := []string{"pull", "codex-pantheon"}
			if mode != "human-writer" {
				args = append(args, "--json")
			}
			if mode == "warning-writer" {
				args = append(args, "--build-filter")
			}
			cmd.SetArgs(args)
			if err := cmd.Execute(); !errors.Is(err, failure) {
				t.Fatalf("error lost or rendered success: %v", err)
			}
			if calls != 1 || out.Len() != 0 {
				t.Fatalf("failed path reread or claimed empty success: calls=%d out=%q", calls, out.String())
			}
		})
	}
}

func TestRouterPullRejectsInvalidArgumentsBeforeRead(t *testing.T) {
	for _, args := range [][]string{{"pull", "--json"}, {"pull", "codex-pantheon", "extra", "--json"}} {
		var out, warnings bytes.Buffer
		calls := 0
		cmd := routerPullFixtureCommand(t, func(string) ([]work.Item, error) { calls++; return nil, nil }, &out, &warnings)
		cmd.SetArgs(args)
		if err := cmd.Execute(); err == nil || calls != 0 || out.Len() != 0 {
			t.Fatalf("invalid argv reached read/output: %v calls=%d", err, calls)
		}
	}
}
