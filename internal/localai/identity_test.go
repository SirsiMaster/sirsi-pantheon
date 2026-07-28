package localai

import (
	"strings"
	"testing"
)

func TestRenderPromptCarriesSirsiIdentityAndTask(t *testing.T) {
	p := RenderPrompt("Are you the local implementation of Sirsi?")
	for _, want := range []string{
		"Ask Sirsi",
		"Sirsi Pantheon",
		"Cylton Collymore",
		"Claude Home",
		"Sirsi IO",
		"Hypergraph",
		"Hedera HCS",
		"Assiduous",
		"FinalWishes",
		"Ask Eliot",
		"Porch and Alley",
		"VS Code",
		"Cursor",
		"Windsurf",
		"Antigravity",
		"Task:\nAre you the local implementation of Sirsi?",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt missing %q:\n%s", want, p)
		}
	}
}

func TestApplyIdentityCarriesSessionInstructions(t *testing.T) {
	p := ApplyIdentity("be concise")
	for _, want := range []string{"Ask Sirsi", "User/session instructions:\nbe concise"} {
		if !strings.Contains(p, want) {
			t.Errorf("identity prompt missing %q:\n%s", want, p)
		}
	}
}
