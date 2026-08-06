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
		"Ra owns CTR/router",
		"Cylton Collymore",
		"baseline canon as of commit " + SystemPromptCanonCommit,
		"not live state",
	} {
		if !strings.Contains(p, want) {
			t.Fatalf("system prompt missing %q:\n%s", want, p)
		}
	}
}

// The prompt teaches the model what to tell the owner about the architecture, so
// a wrong claim here is repeated confidently and in Sirsi's own voice.
//
// This test replaces an assertion on the literal string "Hypergraph and Sirsi IO",
// which PINNED the defect: the old line read "Hypergraph and Sirsi IO are the
// event, knowledge, conduit, and projection direction for Sirsi" — four nouns,
// two planes, one clause — and the test guaranteed it stayed. A test can lock in
// a bug as easily as a behavior when it asserts wording instead of meaning.
func TestSystemPromptKeepsTheFourPillarsDistinct(t *testing.T) {
	p := SystemPrompt()

	// The four-plane model, sirsi-hypergraph ADR-005. Each plane does ONE thing.
	for _, want := range []string{
		"four pillars of one system",
		"Nexus decides",
		"Hypergraph remembers",
		"Pantheon acts",
		"Sirsi I/O senses and expresses",
	} {
		if !strings.Contains(p, want) {
			t.Fatalf("prompt must state the four-plane model (ADR-005); missing %q:\n%s", want, p)
		}
	}

	// The I/O half, sirsi-io ADR-002 — IO1 (surface monopoly, no authoritative
	// state) and IO4 (provenance: owner and freshness at the value).
	for _, want := range []string{
		"only pillar that addresses a human",
		"holds no authoritative state",
		"owned by another pillar",
		"where it came from and how old it is",
	} {
		if !strings.Contains(p, want) {
			t.Fatalf("prompt must state the I/O law (sirsi-io ADR-002); missing %q:\n%s", want, p)
		}
	}

	// And the merged clause must not come back. Asserting its ABSENCE is the
	// point: the regression is not a missing string, it is two planes collapsing
	// into one again.
	if strings.Contains(p, "Hypergraph and Sirsi IO are") {
		t.Fatalf("the merged pillar clause is back — Hypergraph and I/O are separate planes:\n%s", p)
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
	if route.Defaulted {
		t.Fatal("configured route must not be marked defaulted")
	}
}

func TestResolveDefaultsToLocalSlot(t *testing.T) {
	route := Resolve(brain.DefaultConfig(), brain.RoleTriage)
	if got := route.Provider.String(); got != DefaultLocalProvider {
		t.Fatalf("provider = %q, want %q", got, DefaultLocalProvider)
	}
	if !route.Defaulted {
		t.Fatal("unconfigured route must disclose that the provider was defaulted")
	}
}
