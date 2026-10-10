package main

import (
	"strings"
	"testing"
)

func TestRunDeniesEveryRecordedDisplayPowerWrapper(t *testing.T) {
	for _, command := range []string{
		"pmset displaysleepnow",
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
	replacer := strings.NewReplacer(`\\`, `\\\\`, `"`, `\\"`)
	return `"` + replacer.Replace(value) + `"`
}
