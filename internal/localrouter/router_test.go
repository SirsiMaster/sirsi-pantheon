package localrouter

import (
	"os"
	"path/filepath"
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
		"Claude Home",
		"Codex Pantheon",
		"Hypergraph and Sirsi IO",
		"Sirsi Nexus",
		"Cylton Collymore",
		"baseline canon as of commit " + SystemPromptCanonCommit,
		"not live state",
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

func TestShortContextTeachesSirsiPortfolioAndRouter(t *testing.T) {
	ctx := ShortContext()
	for _, want := range []string{
		"Ask Sirsi",
		"internal system manager",
		"Mac menubar, CLI, TUI",
		"CTR/router fabric",
		"Claude Home",
		"Codex Pantheon",
		"Gemma",
		"Qwen",
		"Sirsi Nexus",
		"FinalWishes",
		"Assiduous",
		"Ask Eliot",
		"Porch and Alley",
		"Hypergraph",
		"Sirsi IO",
		"Cylton Collymore",
	} {
		if !strings.Contains(ctx, want) {
			t.Fatalf("short context missing %q:\n%s", want, ctx)
		}
	}
}

func TestBuildContextReportsGroundedCanon(t *testing.T) {
	root := t.TempDir()
	for _, doc := range CanonDocuments() {
		path := filepath.Join(root, doc.Path)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte("# "+doc.Title+"\nSirsi canon for "+doc.Title+"\n"), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	pack := BuildContext(root)
	if !pack.Grounding.Healthy {
		t.Fatalf("grounding healthy = false, status = %+v", pack.Grounding)
	}
	if pack.Grounding.Readable != len(CanonDocuments()) {
		t.Fatalf("readable = %d, want %d", pack.Grounding.Readable, len(CanonDocuments()))
	}
	if !strings.Contains(pack.CanonPack, "CANON PACK (bounded excerpts from "+root+")") {
		t.Fatalf("canon pack missing root:\n%s", pack.CanonPack)
	}
	if !strings.Contains(pack.System, "Your operating identity is Sirsi") {
		t.Fatalf("system prompt not included:\n%s", pack.System)
	}
	if !strings.Contains(pack.ShortContext, "Sirsi Nexus") {
		t.Fatalf("short context not included:\n%s", pack.ShortContext)
	}
}

func TestBuildContextReportsUngroundedCanon(t *testing.T) {
	pack := BuildContext(filepath.Join(t.TempDir(), "missing"))
	if pack.Grounding.Healthy {
		t.Fatalf("missing root marked healthy: %+v", pack.Grounding)
	}
	if !strings.Contains(pack.CanonPack, "root is not readable") {
		t.Fatalf("ungrounded canon pack should be explicit:\n%s", pack.CanonPack)
	}
}
