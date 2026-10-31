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

func routerOutputFixture(t *testing.T, child *cobra.Command, out io.Writer) *cobra.Command {
	t.Helper()
	oldJSON := JsonOutput
	JsonOutput = false
	t.Cleanup(func() { JsonOutput = oldJSON })
	root := &cobra.Command{Use: "fixture-sirsi", SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().BoolVar(&JsonOutput, "json", false, "machine output")
	root.SetOut(out)
	root.SetErr(io.Discard)
	root.AddCommand(child)
	return root
}

func TestRouterShowJSONUsesTypedItemAndHumanMarkdownRemainsUnchanged(t *testing.T) {
	item := work.Item{ID: "show-fixture", From: "sender-fixture", To: "recipient-fixture", Title: "canonical title", Status: "open", Instructions: "canonical body"}
	for _, machine := range []bool{false, true} {
		t.Run(map[bool]string{false: "human", true: "json"}[machine], func(t *testing.T) {
			var out bytes.Buffer
			showCalls, getCalls := 0, 0
			cmd := routerOutputFixture(t, newRouterShowCmd(
				func(id string) (string, error) {
					showCalls++
					if id != item.ID {
						t.Fatalf("Show id = %q", id)
					}
					return "---\nid: show-fixture\n---\n## Instructions\ncanonical body\n", nil
				},
				func(id string) (work.Item, error) {
					getCalls++
					if id != item.ID {
						t.Fatalf("Get id = %q", id)
					}
					return item, nil
				}), &out)
			args := []string{"show", item.ID}
			if machine {
				args = append(args, "--json")
			}
			cmd.SetArgs(args)
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			if machine {
				var got work.Item
				if err := json.Unmarshal(out.Bytes(), &got); err != nil || !reflect.DeepEqual(got, item) {
					t.Fatalf("JSON item = %+v, err=%v stdout=%q", got, err, out.String())
				}
				if showCalls != 0 || getCalls != 1 {
					t.Fatalf("show/get calls = %d/%d, want 0/1", showCalls, getCalls)
				}
			} else {
				want := "---\nid: show-fixture\n---\n## Instructions\ncanonical body\n"
				if out.String() != want || showCalls != 1 || getCalls != 0 {
					t.Fatalf("human output/calls = %q %d/%d", out.String(), showCalls, getCalls)
				}
			}
		})
	}
}

func TestRouterShowReadErrorsRemainErrorsWithoutOutput(t *testing.T) {
	readErr := errors.New("inert show read failure")
	for _, machine := range []bool{false, true} {
		t.Run(map[bool]string{false: "human", true: "json"}[machine], func(t *testing.T) {
			var out bytes.Buffer
			calls := 0
			cmd := routerOutputFixture(t, newRouterShowCmd(
				func(string) (string, error) { calls++; return "", readErr },
				func(string) (work.Item, error) { calls++; return work.Item{}, readErr }), &out)
			args := []string{"show", "missing-fixture"}
			if machine {
				args = append(args, "--json")
			}
			cmd.SetArgs(args)
			if err := cmd.Execute(); !errors.Is(err, readErr) {
				t.Fatalf("Execute error = %v, want original read error", err)
			}
			if calls != 1 || out.Len() != 0 {
				t.Fatalf("calls=%d stdout=%q, want one selected read and empty output", calls, out.String())
			}
		})
	}
}

func TestRouterReassignJSONAndHumanSuccessUseSameInjectedMutation(t *testing.T) {
	for _, machine := range []bool{false, true} {
		t.Run(map[bool]string{false: "human", true: "json"}[machine], func(t *testing.T) {
			var out bytes.Buffer
			calls := 0
			cmd := routerOutputFixture(t, newRouterReassignCmd(func(id, to, agent string) error {
				calls++
				if id != "reassign-fixture" || to != "recipient-next" || agent != "actor-fixture" {
					t.Fatalf("reassign args = %q %q %q", id, to, agent)
				}
				return nil
			}), &out)
			args := []string{"reassign", "reassign-fixture", "--to", "recipient-next", "--agent", "actor-fixture"}
			if machine {
				args = append(args, "--json")
			}
			cmd.SetArgs(args)
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			if calls != 1 {
				t.Fatalf("mutation calls = %d, want exactly one", calls)
			}
			if machine {
				var fields map[string]json.RawMessage
				if err := json.Unmarshal(out.Bytes(), &fields); err != nil {
					t.Fatalf("stdout is not JSON: %q: %v", out.String(), err)
				}
				var got routerReassignJSON
				if err := json.Unmarshal(out.Bytes(), &got); err != nil {
					t.Fatal(err)
				}
				want := routerReassignJSON{ID: "reassign-fixture", To: "recipient-next", Status: "open"}
				if !reflect.DeepEqual(got, want) || len(fields) != 3 {
					t.Fatalf("JSON receipt = %+v fields=%v, want only id/to/status", got, fields)
				}
			} else if got, want := out.String(), "  Reassigned reassign-fixture → recipient-next\n"; got != want {
				t.Fatalf("human output = %q, want %q", got, want)
			}
		})
	}
}

func TestRouterReassignOperationFailureReturnsErrorWithoutSuccessOutput(t *testing.T) {
	operationErr := errors.New("inert reassign refused")
	for _, machine := range []bool{false, true} {
		t.Run(map[bool]string{false: "human", true: "json"}[machine], func(t *testing.T) {
			var out bytes.Buffer
			calls := 0
			cmd := routerOutputFixture(t, newRouterReassignCmd(func(string, string, string) error {
				calls++
				return operationErr
			}), &out)
			args := []string{"reassign", "reassign-fixture", "--to", "recipient-next", "--agent", "actor-fixture"}
			if machine {
				args = append(args, "--json")
			}
			cmd.SetArgs(args)
			if err := cmd.Execute(); !errors.Is(err, operationErr) {
				t.Fatalf("Execute error = %v, want original operation error", err)
			}
			if calls != 1 || out.Len() != 0 || strings.Contains(out.String(), "Reassigned") {
				t.Fatalf("calls=%d stdout=%q, want one operation and no success output", calls, out.String())
			}
		})
	}
}
