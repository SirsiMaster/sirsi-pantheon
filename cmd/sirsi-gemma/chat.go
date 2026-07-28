package main

import (
	"fmt"
	"strings"
)

// ChatMessage is one turn in a chat exchange. Role is "system", "user",
// or "assistant" — same vocabulary the OpenAI/Anthropic chat APIs use, so
// MCP clients can hand us their existing message arrays unchanged.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// RenderChatPrompt collapses a system prompt + message history into the
// Gemma instruction-tuned template. Gemma 2 uses <start_of_turn>role and
// <end_of_turn> markers; mlx_lm applies the same template internally for
// "-it" models but only when you pass a chat array via the Python API.
// Since we drive it via the CLI's --prompt flag, we render the template
// ourselves so multi-turn context is preserved.
func RenderChatPrompt(system string, msgs []ChatMessage) (string, error) {
	if len(msgs) == 0 {
		return "", fmt.Errorf("chat: at least one message required")
	}
	system = applySirsiIdentity(system)
	var b strings.Builder
	if system != "" {
		// Gemma has no dedicated system role — convention is to fold the
		// system instruction into the first user turn.
		b.WriteString("<start_of_turn>user\n")
		b.WriteString(system)
		b.WriteString("\n\n")
	}
	for i, m := range msgs {
		role := m.Role
		switch role {
		case "user", "assistant":
		case "system":
			return "", fmt.Errorf("chat: system messages must be passed via the `system` param, not the history")
		default:
			return "", fmt.Errorf("chat: unknown role %q (want user|assistant)", role)
		}
		if system != "" && i == 0 && role == "user" {
			b.WriteString(m.Content)
			b.WriteString("<end_of_turn>\n")
			continue
		}
		b.WriteString("<start_of_turn>")
		b.WriteString(role)
		b.WriteString("\n")
		b.WriteString(m.Content)
		b.WriteString("<end_of_turn>\n")
	}
	b.WriteString("<start_of_turn>model\n")
	return b.String(), nil
}
