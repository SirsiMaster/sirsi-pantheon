package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRunDeniesEveryRecordedDisplayPowerWrapper(t *testing.T) {
	for _, command := range []string{
		"pmset displaysleepnow",
		"env pmset displaysleepnow",
		"env -u EXAMPLE X=1 pmset displaysleepnow",
		"X=1 pmset displaysleepnow",
		"command pmset displaysleepnow",
		"command -p pmset displaysleepnow",
		"printf '%s' \"`pmset displaysleepnow`\"",
		"/usr/bin/pmset displaysleepnow",
		"sudo /usr/bin/pmset sleepnow",
		"zsh -lc 'pmset displaysleepnow'",
		"echo harmless; /usr/bin/pmset displaysleepnow",
		"printf '%s' $(pmset displaysleepnow)",
	} {
		t.Run(command, func(t *testing.T) {
			err := run(strings.NewReader(`{"tool_name":"Bash","tool_input":{"command":`+quote(command)+`}}`), &strings.Builder{})
			if err == nil {
				t.Fatal("display-power command unexpectedly allowed")
			}
		})
	}
}

func TestRunAllowsReadOnlyPmsetAndNonBash(t *testing.T) {
	if err := run(strings.NewReader(`{"hook_event_name":"PreToolUse","session_id":"s1","cwd":"/tmp","tool_name":"Bash","tool_input":{"command":"pmset -g","timeout":30,"description":"read power status"}}`), &strings.Builder{}); err != nil {
		t.Fatalf("read-only pmset rejected: %v", err)
	}
	if err := run(strings.NewReader(`{"tool_name":"apply_patch","tool_input":{}}`), &strings.Builder{}); err != nil {
		t.Fatalf("non-Bash hook rejected: %v", err)
	}
	if err := run(strings.NewReader(`{"tool_name":"Bash","tool_input":{"command":"printf '%s' 'pmset displaysleepnow'"}}`), &strings.Builder{}); err != nil {
		t.Fatalf("harmless command literal rejected: %v", err)
	}
}

func TestRunDeniesCodexCommandShape(t *testing.T) {
	err := run(strings.NewReader(`{"turn_id":"t1","tool_name":"functions.exec_command","tool_input":{"cmd":"sudo /usr/bin/pmset displaysleepnow","yield_time_ms":1000}}`), &strings.Builder{})
	if err == nil {
		t.Fatal("Codex exec command shape unexpectedly allowed")
	}
}

func TestRunFailsClosedOnMalformedHookInput(t *testing.T) {
	if err := run(strings.NewReader(`{"tool_name":"Bash","tool_input":`), &strings.Builder{}); err == nil {
		t.Fatal("malformed hook input unexpectedly allowed")
	}
}

func quote(value string) string {
	raw, _ := json.Marshal(value)
	return string(raw)
}

// This fake dispatch boundary never launches a shell or a display operation.
func fakeDispatch(payload string, calls *int) int {
	if err := run(strings.NewReader(payload), &strings.Builder{}); err != nil {
		return 2
	}
	*calls++
	return 0
}

func TestEveryCommandToolDeniesBeforeFakeDispatch(t *testing.T) {
	for _, name := range []string{"Bash", "exec_command", "functions.exec_command", "unified_exec", "shell"} {
		for _, field := range []string{"command", "cmd"} {
			t.Run(name+"/"+field, func(t *testing.T) {
				calls := 0
				payload := `{"tool_name":` + quote(name) + `,"tool_input":{` + quote(field) + `:"pmset displaysleepnow"}}`
				if code := fakeDispatch(payload, &calls); code != 2 || calls != 0 {
					t.Fatalf("denied dispatch: exit=%d calls=%d", code, calls)
				}
				payload = `{"tool_name":` + quote(name) + `,"tool_input":{` + quote(field) + `:"pmset -g"}}`
				if code := fakeDispatch(payload, &calls); code != 0 || calls != 1 {
					t.Fatalf("negative control: exit=%d calls=%d", code, calls)
				}
			})
		}
	}
}

func TestNonCommandPayloadsAllow(t *testing.T) {
	for _, payload := range []string{`{"x":12,"y":34}`, `"coordinate action"`, `[12,34]`, `null`, `{}`, `{"command":[]}`, `{"cmd":null}`, `{"command":""}`} {
		t.Run(payload, func(t *testing.T) {
			calls := 0
			if code := fakeDispatch(`{"tool_name":"computer_use","tool_input":`+payload+`}`, &calls); code != 0 || calls != 1 {
				t.Fatalf("non-command: exit=%d calls=%d", code, calls)
			}
		})
	}
}

func TestBothCommandFieldsAreInspected(t *testing.T) {
	for _, payload := range []string{
		`{"command":"pmset -g","cmd":"pmset displaysleepnow"}`,
		`{"command":null,"cmd":"pmset displaysleepnow"}`,
	} {
		calls := 0
		if code := fakeDispatch(`{"tool_input":`+payload+`}`, &calls); code != 2 || calls != 0 {
			t.Fatalf("hidden cmd: exit=%d calls=%d", code, calls)
		}
	}
}
