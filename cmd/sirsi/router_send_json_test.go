package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/dispatch"
	"github.com/spf13/cobra"
)

type routerSendFailWriter struct{ err error }

func (w routerSendFailWriter) Write([]byte) (int, error) { return 0, w.err }

func newRouterSendFixtureCommand(t *testing.T, output io.Writer, operation routerSendOperation) *cobra.Command {
	return newRouterSendFixtureCommandWithLoader(t, output, operation, nil)
}

func newRouterSendFixtureCommandWithLoader(t *testing.T, output io.Writer, operation routerSendOperation, load func(string) (string, error)) *cobra.Command {
	t.Helper()
	oldJSON := JsonOutput
	JsonOutput = false
	t.Cleanup(func() { JsonOutput = oldJSON })

	root := &cobra.Command{Use: "fixture-sirsi", SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().BoolVar(&JsonOutput, "json", false, "machine output")
	root.SetIn(strings.NewReader("private fixture body"))
	root.SetOut(output)
	root.SetErr(io.Discard)
	router := &cobra.Command{Use: "router"}
	if load == nil {
		load = func(value string) (string, error) {
			if value != "fixture-reader" {
				return "", errors.New("unexpected body selector")
			}
			body, err := io.ReadAll(root.InOrStdin())
			return string(body), err
		}
	}
	router.AddCommand(newRouterSendCmd(load, operation))
	root.AddCommand(router)
	root.SetArgs([]string{"router", "send", "--from", "sender-fixture", "--to", "recipient-fixture", "--title", "fixture title", "--type", "decision", "--instructions", "fixture-reader"})
	return root
}

func TestRouterSendJSONWritesTypedCommitWithoutHumanOrBodyText(t *testing.T) {
	for _, tc := range []struct {
		name        string
		inputTo     string
		effectiveTo string
		deduped     bool
	}{
		{name: "new-send", inputTo: "recipient-fixture", effectiveTo: "recipient-fixture"},
		{name: "deduped", inputTo: "recipient-fixture", effectiveTo: "recipient-fixture", deduped: true},
		{name: "resolved-alias", inputTo: "old-b", effectiveTo: "b"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			calls := 0
			finishCalls := 0
			showVerdict := true
			cmd := newRouterSendFixtureCommand(t, &out, func(from, to, title, typ, body string) (dispatch.SendResult, func(bool), error) {
				calls++
				if from != "sender-fixture" || to != tc.inputTo || title != "fixture title" || typ != "decision" || body != "private fixture body" {
					t.Fatalf("send input = %q %q %q %q %q", from, to, title, typ, body)
				}
				return dispatch.SendResult{ID: "committed-fixture-id", To: tc.effectiveTo, Deduped: tc.deduped}, func(show bool) {
					finishCalls++
					showVerdict = show
				}, nil
			})
			// The fixture owns its reader and writer; no router client is opened.
			cmd.SetArgs([]string{"router", "send", "--json", "--from", "sender-fixture", "--to", tc.inputTo, "--title", "fixture title", "--type", "decision", "--instructions", "fixture-reader"})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("Execute(): %v", err)
			}
			if calls != 1 {
				t.Fatalf("send calls = %d, want exactly one", calls)
			}
			if finishCalls != 1 || showVerdict {
				t.Fatalf("finish calls=%d showVerdict=%t, want one closure without human verdict", finishCalls, showVerdict)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(out.Bytes(), &fields); err != nil {
				t.Fatalf("stdout is not one JSON object (legacy human output is the causal negative): %q: %v", out.String(), err)
			}
			if len(fields) != 4 {
				t.Fatalf("JSON fields = %v, want only id/from/to/deduped", fields)
			}
			var got routerSendJSON
			if err := json.Unmarshal(out.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			want := routerSendJSON{ID: "committed-fixture-id", From: "sender-fixture", To: tc.effectiveTo, Deduped: tc.deduped}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("JSON result = %+v, want %+v", got, want)
			}
			if strings.Contains(out.String(), "private fixture body") || strings.Contains(out.String(), "Sent ") || strings.Contains(out.String(), "Deduped ") {
				t.Fatalf("JSON stdout leaked body or human output: %q", out.String())
			}
		})
	}
}

func TestRouterSendErrorDoesNotEmitCommitSuccess(t *testing.T) {
	var out bytes.Buffer
	calls := 0
	cmd := newRouterSendFixtureCommand(t, &out, func(string, string, string, string, string) (dispatch.SendResult, func(bool), error) {
		calls++
		return dispatch.SendResult{}, nil, errors.New("guard refused before commit")
	})
	cmd.SetArgs([]string{"router", "send", "--json", "--from", "sender-fixture", "--to", "recipient-fixture", "--title", "fixture title", "--type", "decision", "--instructions", "fixture-reader"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "guard refused before commit") {
		t.Fatalf("Execute() error = %v, want pre-commit refusal", err)
	}
	if out.Len() != 0 || calls != 1 {
		t.Fatalf("stdout=%q send calls=%d, want empty output and one attempt", out.String(), calls)
	}
}

func TestRouterSendAuditPublicationFailureRetainsCommittedIDWithoutSuccessOutput(t *testing.T) {
	var out bytes.Buffer
	commitErr := errors.New("inert audit publication failure after store commit")
	calls := 0
	cmd := newRouterSendFixtureCommand(t, &out, func(string, string, string, string, string) (dispatch.SendResult, func(bool), error) {
		calls++
		return dispatch.SendResult{ID: "committed-before-audit-failure", To: "recipient-fixture"}, nil, commitErr
	})
	cmd.SetArgs([]string{"router", "send", "--json", "--from", "sender-fixture", "--to", "recipient-fixture", "--title", "fixture title", "--type", "decision", "--instructions", "fixture-reader"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "committed-before-audit-failure") || !strings.Contains(err.Error(), "do not retry") || !errors.Is(err, commitErr) {
		t.Fatalf("Execute() error = %v, want committed ID, no-retry guidance and publication cause", err)
	}
	if out.Len() != 0 || calls != 1 {
		t.Fatalf("stdout=%q send calls=%d, want no success output and exactly one send", out.String(), calls)
	}
}

func TestRouterSendMissingFlagsAndBodyReadErrorDoNotSend(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{name: "from", args: []string{"router", "send", "--json", "--to", "recipient-fixture", "--title", "fixture title", "--instructions", "fixture-reader"}, want: "--from and --to are required"},
		{name: "to", args: []string{"router", "send", "--json", "--from", "sender-fixture", "--title", "fixture title", "--instructions", "fixture-reader"}, want: "--from and --to are required"},
		{name: "title", args: []string{"router", "send", "--json", "--from", "sender-fixture", "--to", "recipient-fixture", "--instructions", "fixture-reader"}, want: "--title is required"},
		{name: "instructions", args: []string{"router", "send", "--json", "--from", "sender-fixture", "--to", "recipient-fixture", "--title", "fixture title"}, want: "--instructions is required"},
	} {
		t.Run("missing-"+tc.name, func(t *testing.T) {
			var out bytes.Buffer
			calls, loads := 0, 0
			cmd := newRouterSendFixtureCommandWithLoader(t, &out, func(string, string, string, string, string) (dispatch.SendResult, func(bool), error) {
				calls++
				return dispatch.SendResult{}, nil, nil
			}, func(string) (string, error) {
				loads++
				return "", nil
			})
			cmd.SetArgs(tc.args)
			if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Execute() error = %v, want refusal containing %q", err, tc.want)
			}
			if calls != 0 || loads != 0 || out.Len() != 0 {
				t.Fatalf("send=%d load=%d stdout=%q, want no load/send/output", calls, loads, out.String())
			}
		})
	}

	t.Run("body-reader-error", func(t *testing.T) {
		var out bytes.Buffer
		readErr := errors.New("inert body reader failure")
		calls := 0
		cmd := newRouterSendFixtureCommandWithLoader(t, &out, func(string, string, string, string, string) (dispatch.SendResult, func(bool), error) {
			calls++
			return dispatch.SendResult{}, nil, nil
		}, func(string) (string, error) { return "", readErr })
		cmd.SetArgs([]string{"router", "send", "--json", "--from", "sender-fixture", "--to", "recipient-fixture", "--title", "fixture title", "--instructions", "fixture-reader"})
		if err := cmd.Execute(); err == nil || !errors.Is(err, readErr) {
			t.Fatalf("Execute() error = %v, want body-reader failure", err)
		}
		if calls != 0 || out.Len() != 0 {
			t.Fatalf("send=%d stdout=%q, want no send or success output", calls, out.String())
		}
	})
}

func TestRouterSendJSONWriterFailureRetainsCommittedIDWithoutRetry(t *testing.T) {
	writeErr := errors.New("inert writer failure")
	calls := 0
	finishCalls := 0
	showVerdict := true
	cmd := newRouterSendFixtureCommand(t, routerSendFailWriter{err: writeErr}, func(string, string, string, string, string) (dispatch.SendResult, func(bool), error) {
		calls++
		return dispatch.SendResult{ID: "committed-before-write-failure"}, func(show bool) {
			finishCalls++
			showVerdict = show
		}, nil
	})
	cmd.SetArgs([]string{"router", "send", "--json", "--from", "sender-fixture", "--to", "recipient-fixture", "--title", "fixture title", "--type", "decision", "--instructions", "fixture-reader"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "committed-before-write-failure") || !strings.Contains(err.Error(), "do not retry") || !errors.Is(err, writeErr) {
		t.Fatalf("Execute() error = %v, want committed ID, no-retry guidance and writer cause", err)
	}
	if calls != 1 {
		t.Fatalf("send calls = %d, want exactly one with no retry", calls)
	}
	if finishCalls != 1 || showVerdict {
		t.Fatalf("finish calls=%d showVerdict=%t, want one close without human verdict", finishCalls, showVerdict)
	}
}

func TestRouterSendHumanOutputRemainsUnchanged(t *testing.T) {
	var out bytes.Buffer
	calls := 0
	showVerdict := false
	cmd := newRouterSendFixtureCommand(t, &out, func(string, string, string, string, string) (dispatch.SendResult, func(bool), error) {
		calls++
		return dispatch.SendResult{ID: "human-fixture-id"}, func(show bool) { showVerdict = show }, nil
	})
	cmd.SetArgs([]string{"router", "send", "--from", "sender-fixture", "--to", "recipient-fixture", "--title", "fixture title", "--type", "decision", "--instructions", "fixture-reader"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute(): %v", err)
	}
	if got, want := out.String(), "  Sent sender-fixture → recipient-fixture: human-fixture-id\n"; got != want {
		t.Fatalf("human stdout = %q, want %q", got, want)
	}
	if calls != 1 || !showVerdict {
		t.Fatalf("send calls=%d verdict=%t, want one send and existing verdict", calls, showVerdict)
	}
}
