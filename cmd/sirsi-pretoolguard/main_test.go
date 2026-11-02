package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"testing"
)

// These input-boundary fixtures never dispatch a tool or execute a payload.
func TestHookInputBound(t *testing.T) {
	const prefix = `{"tool_name":"apply_patch","tool_input":{}}`
	for _, tc := range []struct {
		name      string
		input     string
		wantError string
	}{
		{"exact_limit", prefix + strings.Repeat(" ", maxHookInputBytes-len(prefix)), ""},
		{"one_byte_over", prefix + strings.Repeat(" ", maxHookInputBytes-len(prefix)+1), "exceeds"},
		{"oversized_json_string", `{"tool_input":{"description":"` + strings.Repeat("x", maxHookInputBytes) + `"}}`, "exceeds"},
		{"hidden_trailing_json", prefix + strings.Repeat(" ", maxHookInputBytes-len(prefix)) + `{}`, "exceeds"},
		{"hidden_trailing_garbage", prefix + strings.Repeat(" ", maxHookInputBytes-len(prefix)) + `x`, "exceeds"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := run(strings.NewReader(tc.input), io.Discard)
			if tc.wantError == "" {
				if err != nil {
					t.Fatalf("bounded input rejected: %v", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("error=%v, want %q", err, tc.wantError)
			}
		})
	}
	// Oversized input is consumed only through cap+1, not to the actual EOF.
	r := strings.NewReader(strings.Repeat(" ", maxHookInputBytes*2))
	if err := run(r, io.Discard); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversized reader accepted: %v", err)
	}
	if got := maxHookInputBytes*2 - r.Len(); got != maxHookInputBytes+1 {
		t.Fatalf("read %d bytes, want exactly cap+1", got)
	}
}

func TestHarmlessGitHubMCPCreatePayloadSizes(t *testing.T) {
	for _, operation := range []string{"create_blob", "create_tree"} {
		for _, size := range []int{65535, 65536, 65537, 1251377, 3510687} {
			t.Run(operation+"/"+strconv.Itoa(size), func(t *testing.T) {
				payload := githubMCPFixture(t, operation, size)
				if len(payload) != size {
					t.Fatalf("fixture length=%d, want %d", len(payload), size)
				}
				err := run(bytes.NewReader(payload), io.Discard)
				if size <= maxHookInputBytes {
					if err != nil {
						t.Fatalf("complete harmless MCP envelope rejected: %v", err)
					}
					return
				}
				want := "exceeds " + strconv.Itoa(maxHookInputBytes) + " bytes"
				if err == nil || !strings.Contains(err.Error(), want) {
					t.Fatalf("oversized complete MCP envelope error=%v, want explicit %q", err, want)
				}
			})
		}
	}
}

func TestHookJSONFramingWithShortAndMalformedReaders(t *testing.T) {
	complete := githubMCPFixture(t, "create_blob", 512)
	truncated := append([]byte(nil), complete[:len(complete)-1]...)
	invalidUTF8 := []byte(`{"tool_name":"mcp__github__create_blob","tool_input":{"owner":"fixture","repo":"fixture","content":"`)
	invalidUTF8 = append(invalidUTF8, 0xff)
	invalidUTF8 = append(invalidUTF8, []byte(`","encoding":"utf-8"}}`)...)
	trailingJSON := append(append([]byte(nil), complete...), []byte(` {}`)...)
	trailingGarbage := append(append([]byte(nil), complete...), []byte(` x`)...)

	for _, tc := range []struct {
		name      string
		payload   []byte
		chunkSize int
		wantError string
	}{
		{name: "complete", payload: complete},
		{name: "complete_short_chunks", payload: complete, chunkSize: 7},
		{name: "truncated_short_chunks", payload: truncated, chunkSize: 5, wantError: "malformed pre-tool hook input"},
		{name: "invalid_utf8_uses_encoding_json_semantics", payload: invalidUTF8},
		{name: "trailing_json", payload: trailingJSON, wantError: "trailing JSON"},
		{name: "trailing_garbage", payload: trailingGarbage, wantError: "trailing JSON"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var reader io.Reader = bytes.NewReader(tc.payload)
			if tc.chunkSize > 0 {
				reader = &shortChunkHookReader{reader: bytes.NewReader(tc.payload), maxChunk: tc.chunkSize}
			}
			err := run(reader, io.Discard)
			if tc.wantError == "" {
				if err != nil {
					t.Fatalf("hook envelope rejected: %v", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("error=%v, want %q", err, tc.wantError)
			}
		})
	}
}

type shortChunkHookReader struct {
	reader   *bytes.Reader
	maxChunk int
}

func (r *shortChunkHookReader) Read(p []byte) (int, error) {
	if len(p) > r.maxChunk {
		p = p[:r.maxChunk]
	}
	return r.reader.Read(p)
}

func githubMCPFixture(t *testing.T, operation string, targetBytes int) []byte {
	t.Helper()
	var encode func(string) []byte
	switch operation {
	case "create_blob":
		encode = func(content string) []byte {
			input, err := json.Marshal(struct {
				Owner    string `json:"owner"`
				Repo     string `json:"repo"`
				Content  string `json:"content"`
				Encoding string `json:"encoding"`
			}{"SirsiMaster", "sirsi-pantheon-fixture", content, "utf-8"})
			if err != nil {
				t.Fatal(err)
			}
			return marshalMCPEnvelope(t, "mcp__github__create_blob", input)
		}
	case "create_tree":
		encode = func(content string) []byte {
			input, err := json.Marshal(struct {
				Owner    string `json:"owner"`
				Repo     string `json:"repo"`
				BaseTree string `json:"base_tree"`
				Tree     []struct {
					Path    string `json:"path"`
					Mode    string `json:"mode"`
					Type    string `json:"type"`
					Content string `json:"content"`
				} `json:"tree"`
			}{
				Owner:    "SirsiMaster",
				Repo:     "sirsi-pantheon-fixture",
				BaseTree: strings.Repeat("0", 40),
				Tree: []struct {
					Path    string `json:"path"`
					Mode    string `json:"mode"`
					Type    string `json:"type"`
					Content string `json:"content"`
				}{{Path: "fixture.txt", Mode: "100644", Type: "blob", Content: content}},
			})
			if err != nil {
				t.Fatal(err)
			}
			return marshalMCPEnvelope(t, "mcp__github__create_tree", input)
		}
	default:
		t.Fatalf("unknown fixture operation %q", operation)
	}

	base := encode("")
	padding := targetBytes - len(base)
	if padding < 0 {
		t.Fatalf("target size %d is below %s envelope base size %d", targetBytes, operation, len(base))
	}
	payload := encode(strings.Repeat("x", padding))
	if len(payload) != targetBytes {
		t.Fatalf("%s fixture serialized to %d bytes, want exactly %d", operation, len(payload), targetBytes)
	}
	return payload
}

func marshalMCPEnvelope(t *testing.T, tool string, input json.RawMessage) []byte {
	t.Helper()
	data, err := json.Marshal(struct {
		ToolName  string          `json:"tool_name"`
		ToolInput json.RawMessage `json:"tool_input"`
	}{tool, input})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestRunFailsClosedOnMalformedHookInput(t *testing.T) {
	for _, input := range []string{"", " ", `{`, `{"tool_input":`, `[]`, `42`, `{"tool_input":{}} {}`, `{"tool_input":{}} x`} {
		if err := run(strings.NewReader(input), io.Discard); err == nil {
			t.Fatalf("malformed/trailing envelope accepted: %q", input)
		}
	}
}

type failedHookReader struct{}

func (failedHookReader) Read([]byte) (int, error) {
	return 0, errors.New("fixture read failure")
}

func TestHookReadFailure(t *testing.T) {
	if err := run(failedHookReader{}, io.Discard); err == nil || !strings.Contains(err.Error(), "fixture read failure") {
		t.Fatalf("read failure not propagated: %v", err)
	}
}

func TestNonCommandPayloadsAllow(t *testing.T) {
	for _, raw := range []string{`{"x":12,"y":34}`, `"coordinate action"`, `[12,34]`, `null`, `{}`, `{"command":[]}`, `{"cmd":null}`, `{"command":""}`} {
		payload := `{"tool_name":"computer_use","tool_input":` + raw + `}`
		if err := run(strings.NewReader(payload), io.Discard); err != nil {
			t.Fatalf("non-command payload rejected: %s: %v", raw, err)
		}
	}
	if err := run(strings.NewReader(`{"tool_name":"apply_patch"}`), io.Discard); err != nil {
		t.Fatalf("absent command payload rejected: %v", err)
	}
}

func TestHarmlessCommandPayloadsAllow(t *testing.T) {
	for _, tool := range []string{"Bash", "exec_command", "functions.exec_command", "unified_exec", "shell", "unknown_tool"} {
		for _, field := range []string{"command", "cmd"} {
			raw, err := json.Marshal(map[string]any{
				"tool_name":  tool,
				"tool_input": map[string]any{field: "printf harmless", "grant": "metadata-is-not-authority"},
				"session_id": "fixture",
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := run(strings.NewReader(string(raw)), io.Discard); err != nil {
				t.Fatalf("harmless command rejected for %s/%s: %v", tool, field, err)
			}
		}
	}
}

// The fake boundary only counts calls; it has no shell/process implementation.
func hookFakeDispatch(payload []byte, calls *int) error {
	if err := run(strings.NewReader(string(payload)), io.Discard); err != nil {
		return err
	}
	*calls++
	return nil
}

func TestEveryCommandToolDeniesBeforeFakeDispatch(t *testing.T) {
	for _, tool := range []string{"Bash", "exec_command", "functions.exec_command", "unified_exec", "shell", "unknown_tool", ""} {
		for _, field := range []string{"command", "cmd"} {
			t.Run(tool+"/"+field, func(t *testing.T) {
				for _, command := range []string{
					"pmset displaysleepnow",
					"env pmset displaysleepnow",
					"X=1 pmset displaysleepnow",
					"command pmset displaysleepnow",
				} {
					payload, err := json.Marshal(map[string]any{
						"tool_name":  tool,
						"tool_input": map[string]any{field: command, "grant": map[string]any{"active": true}},
					})
					if err != nil {
						t.Fatal(err)
					}
					calls := 0
					if err := hookFakeDispatch(payload, &calls); err == nil || calls != 0 {
						t.Fatalf("denial failed for %q: calls=%d err=%v", command, calls, err)
					}
				}
				payload, err := json.Marshal(map[string]any{"tool_name": tool, "tool_input": map[string]string{field: "pmset -g"}})
				if err != nil {
					t.Fatal(err)
				}
				calls := 0
				if err := hookFakeDispatch(payload, &calls); err != nil || calls != 1 {
					t.Fatalf("read-only control failed: calls=%d err=%v", calls, err)
				}
			})
		}
	}
}

func TestBothCommandFieldsAreInspected(t *testing.T) {
	for _, fields := range []map[string]any{
		{"command": "pmset -g", "cmd": "pmset displaysleepnow"},
		{"command": "pmset displaysleepnow", "cmd": "pmset -g"},
		{"command": nil, "cmd": "pmset displaysleepnow"},
		{"command": "pmset displaysleepnow", "cmd": []string{}},
	} {
		payload, err := json.Marshal(map[string]any{"tool_name": "computer_use", "tool_input": fields})
		if err != nil {
			t.Fatal(err)
		}
		calls := 0
		if err := hookFakeDispatch(payload, &calls); err == nil || calls != 0 {
			t.Fatalf("field hidden by sibling: calls=%d err=%v", calls, err)
		}
	}
}

func TestHookNestedInputBudget(t *testing.T) {
	// Deeply nested harmless substitutions fit within the byte cap. Exhausting
	// the parser's depth budget must deny, not overflow or silently allow.
	const depth = 8192
	command := strings.Repeat("$(", depth) + "printf harmless" + strings.Repeat(")", depth)
	payload, err := json.Marshal(map[string]any{"tool_name": "shell", "tool_input": map[string]string{"cmd": command}})
	if err != nil {
		t.Fatal(err)
	}
	if len(payload) > maxHookInputBytes {
		t.Fatal("nested fixture must exercise parser budget, not input cap")
	}
	calls := 0
	if err := hookFakeDispatch(payload, &calls); err == nil || calls != 0 || !strings.Contains(err.Error(), "denied") {
		t.Fatalf("nested input not conservatively denied: calls=%d err=%v", calls, err)
	}
}
