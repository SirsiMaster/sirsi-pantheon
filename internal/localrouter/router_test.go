package localrouter

import (
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/brain"
)

func TestSystemPromptIsModelAgnosticSirsiIdentity(t *testing.T) {
	p := SystemPrompt()
	for _, want := range []string{
		"Ask Sirsi",
		"internal system manager",
		"Local LLM slot is pluggable",
		"Do not answer as a generic Google, Gemma, Gemini, Qwen, Ollama, MLX",
		"canon summary as of PR #382 / commit 77144192; not live state",
		"Ra owns CTR/router",
		"Hypergraph and Sirsi IO",
		"Cylton Collymore",
	} {
		if !strings.Contains(p, want) {
			t.Fatalf("system prompt missing %q:\n%s", want, p)
		}
	}
}

func TestEnvelopePreservesUserRequest(t *testing.T) {
	got := Envelope("Are you the local implementation of Sirsi?")
	if !strings.Contains(got, "Your operating identity is Sirsi") {
		t.Fatalf("wrapped prompt missing Sirsi identity:\n%s", got)
	}
	if !strings.Contains(got, "USER REQUEST:\nAre you the local implementation of Sirsi?") {
		t.Fatalf("wrapped prompt missing original user request:\n%s", got)
	}
}

func TestResolveUsesConfiguredLocalProvider(t *testing.T) {
	cfg := brain.Config{Roles: map[string]string{"triage": "local:qwen2.5-7b-instruct"}}
	route := Resolve(cfg, brain.RoleTriage)
	if got := route.Provider.String(); got != "local:qwen2.5-7b-instruct" {
		t.Fatalf("provider = %q, want qwen route", got)
	}
	if route.ProviderRef != "local:qwen2.5-7b-instruct" || route.ProviderKind != "local" {
		t.Fatalf("json route fields = %q/%q, want local qwen", route.ProviderKind, route.ProviderRef)
	}
	if route.SubstitutedDefault {
		t.Fatal("configured local provider must not report default substitution")
	}
}

func TestResolveDefaultsToLocalSlot(t *testing.T) {
	route := Resolve(brain.DefaultConfig(), brain.RoleTriage)
	if got := route.Provider.String(); got != DefaultLocalProvider {
		t.Fatalf("provider = %q, want %q", got, DefaultLocalProvider)
	}
	if !route.SubstitutedDefault {
		t.Fatal("unconfigured local route must report default substitution")
	}
}
