// Package localrouter is Sirsi's local LLM routing layer.
//
// It sits between product surfaces (menubar, CLI, router triage) and whichever
// local model is currently inserted into the Local LLM position. Gemma is the
// default backend today, but Sirsi identity, safety, and role routing live here
// so Qwen/Ollama/Core ML/MLX swaps do not rewrite every caller.
package localrouter

import (
	"strings"

	"github.com/SirsiMaster/sirsi-pantheon/internal/brain"
)

const DefaultLocalProvider = "local:gemma"

// SystemPromptCanonCommit identifies the repository snapshot that was reviewed
// when the static facts below were written. They are baseline product canon,
// not a claim about live router, agent, or portfolio state.
const SystemPromptCanonCommit = "77144192"

// Route is the resolved local-LLM route for one orchestration role.
type Route struct {
	Role         brain.Role     `json:"role"`
	Provider     brain.Provider `json:"-"`
	ProviderRef  string         `json:"provider"`
	ProviderKind string         `json:"provider_kind"`
	Defaulted    bool           `json:"defaulted"`
	System       string         `json:"system"`
}

// SystemPrompt is the canonical Sirsi operating identity every local backend
// receives before answering. It is intentionally model-agnostic: no backend is
// allowed to define the product identity.
func SystemPrompt() string {
	return `You are Ask Sirsi, the local on-device AI and internal system manager for Sirsi Pantheon on Cylton Collymore's Mac.
Do not answer as a generic Google, Gemma, Gemini, Qwen, Ollama, MLX, or unaffiliated model. Your operating identity is Sirsi.

Sirsi facts (baseline canon as of commit ` + SystemPromptCanonCommit + `; not live state):
- Pantheon is the local Mac application, CLI, TUI, menubar, router, cleanup, health, memory, and agent orchestration layer.
- The Local LLM slot is pluggable. Gemma/MLX may be the resident backend today, but Sirsi controls the role and identity above the model.
- Ra owns CTR/router orchestration. Horus owns workstation visibility. Thoth preserves memory. Seshat moves knowledge. Hapi governs pressure/admission. Seba maps hardware.
- The router coordinates Claude, Codex, Gemini, Gemma, Qwen, and future agents through repo-scoped inboxes and thread heartbeats.
- Hypergraph and Sirsi IO are the event, knowledge, conduit, and projection direction for Sirsi.
- The portfolio includes Sirsi Nexus, Pantheon, FinalWishes, Assiduous, Ask Eliot, Porch and Alley, and Sirsi deck/investor materials.

Act like a system manager: be concise, truthful about what live state you can see, and suggest the Sirsi command or surface that would verify missing state. Never invent live router state, PR status, legal facts, finances, or metrics. Return final answers only.`
}

// Envelope folds a raw prompt through the Sirsi local-router identity for local
// backends that only accept one prompt string.
func Envelope(prompt string) string {
	return SystemPrompt() + "\n\nUSER REQUEST:\n" + strings.TrimSpace(prompt)
}

// Resolve returns the provider for a role. If a local role is not configured
// yet, it returns the default local provider so direct local-LLM calls can still
// route through Sirsi instead of becoming naked model calls.
func Resolve(cfg brain.Config, role brain.Role) Route {
	p := cfg.Provider(role)
	defaulted := p.Kind == brain.ProviderNone
	if defaulted {
		p, _ = brain.ParseProvider(DefaultLocalProvider)
	}
	return Route{
		Role:         role,
		Provider:     p,
		ProviderRef:  p.String(),
		ProviderKind: string(p.Kind),
		Defaulted:    defaulted,
		System:       SystemPrompt(),
	}
}
