// sirsi-pretoolguard is a deny-only adapter for managed command-tool hooks.
// It never executes command text, actuates power, or grants machine authority.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/SirsiMaster/sirsi-pantheon/internal/agentguard"
)

// This is a local resource bound, not a claimed limit of the hook emitter.
// Full-envelope decoding can hold bounded copies of the input while inspecting
// RawMessage fields, so keep a finite cap and reject cap+1 before parsing.
const maxHookInputBytes = 8 << 20

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

func run(in io.Reader, _ io.Writer) error {
	// Read one extra byte before parsing: decoder EOF on a truncated reader
	// could otherwise accept valid JSON followed by whitespace/hidden trailing
	// data beyond the cap. The complete accepted input must fit this bound.
	raw, err := io.ReadAll(io.LimitReader(in, maxHookInputBytes+1))
	if err != nil {
		return fmt.Errorf("read pre-tool hook input: %w", err)
	}
	if len(raw) > maxHookInputBytes {
		return fmt.Errorf("pre-tool hook input exceeds %d bytes", maxHookInputBytes)
	}
	var input hookInput
	dec := json.NewDecoder(bytes.NewReader(raw))
	if err := dec.Decode(&input); err != nil {
		return fmt.Errorf("malformed pre-tool hook input: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return fmt.Errorf("pre-tool hook input has trailing JSON")
	}
	// Valid non-command payloads are outside this narrow command guard. Extra
	// envelope metadata confers no authority and is not interpreted.
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(input.ToolInput, &fields); err != nil {
		return nil
	}
	// Inspect both strings regardless of tool name; a harmless command must
	// never conceal a disruptive cmd. This adapter has no grant bypass.
	for _, key := range []string{"command", "cmd"} {
		var command string
		if value, ok := fields[key]; !ok || json.Unmarshal(value, &command) != nil {
			continue
		}
		if agentguard.IsDirectDisplayPowerShell(command) {
			return fmt.Errorf("direct display-power operation denied; use the typed sirsi desktop-sleep workflow for governed sleep, never raw shell power commands")
		}
	}
	return nil
}
