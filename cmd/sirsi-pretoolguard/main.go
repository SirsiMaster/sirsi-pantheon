// sirsi-pretoolguard is the small process used by Codex and Claude PreToolUse
// hooks. It blocks direct display-power commands before any shell is launched.
// It has no display actuator and never attempts to repair a screen itself.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/SirsiMaster/sirsi-pantheon/internal/agentguard"
)

type hookInput struct {
	ToolName  string          `json:"tool_name"`
	ToolInput json.RawMessage `json:"tool_input"`
}

func main() {
	if err := run(os.Stdin, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "Pantheon display safety guard: %v\n", err)
		os.Exit(2)
	}
}

func run(in io.Reader, stderr io.Writer) error {
	var input hookInput
	dec := json.NewDecoder(io.LimitReader(in, 64<<10))
	if err := dec.Decode(&input); err != nil {
		return fmt.Errorf("malformed pre-tool hook input: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return fmt.Errorf("pre-tool hook input has trailing JSON")
	}
	// Codex and Claude supply additional envelope and tool-input fields that are
	// unrelated to command safety (turn ids, cwd, timeouts, descriptions, and
	// provider metadata). The hook must accept those real payloads; only the
	// string command/cmd field is relevant to this narrow denial.
	var toolInput map[string]json.RawMessage
	if err := json.Unmarshal(input.ToolInput, &toolInput); err != nil {
		// Valid scalar, array, or absent payloads carry no command fields.
		return nil
	}
	// Inspect both fields: a harmless command must not hide a disruptive cmd.
	for _, key := range []string{"command", "cmd"} {
		var command string
		if raw, ok := toolInput[key]; !ok || json.Unmarshal(raw, &command) != nil {
			continue
		}
		if agentguard.IsDirectDisplayPowerShell(command) {
			return fmt.Errorf("direct pmset display-power operation denied; an explicit, active, unrevoked desktop-custody grant is required")
		}
	}
	return nil
}
