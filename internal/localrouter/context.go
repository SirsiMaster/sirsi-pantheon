package localrouter

import (
	"os"
	"path/filepath"
	"strings"
)

const defaultCanonBudget = 4400

// CanonDocument is one bounded grounding source for Ask Sirsi. The Local LLM
// slot can change models; these facts remain Sirsi's product memory.
type CanonDocument struct {
	Title string `json:"title"`
	Path  string `json:"path"`
	Limit int    `json:"limit"`
}

// GroundingStatus tells surfaces whether the local canon pack is actually
// readable. "ANE detected" is not "ANE used"; likewise, "Pantheon root exists"
// is not "Ask Sirsi was grounded."
type GroundingStatus struct {
	Root     string `json:"root"`
	Value    string `json:"value"`
	Detail   string `json:"detail"`
	Healthy  bool   `json:"healthy"`
	Readable int    `json:"readable"`
	Total    int    `json:"total"`
}

// ContextPack is the model-agnostic Sirsi knowledge payload. Menubar, CLI,
// router triage, and future Nexus consumers should use this instead of
// hand-writing product identity beside each model call.
type ContextPack struct {
	System       string          `json:"system"`
	ShortContext string          `json:"short_context"`
	CanonPack    string          `json:"canon_pack"`
	Grounding    GroundingStatus `json:"grounding"`
	Documents    []CanonDocument `json:"documents"`
}

// CanonDocuments returns the first-class Sirsi knowledge sources Ask Sirsi uses
// before reaching for larger Seshat/Thoth/Hypergraph stores.
func CanonDocuments() []CanonDocument {
	return []CanonDocument{
		{Title: "Pantheon rules", Path: "AGENTS.md", Limit: 650},
		{Title: "Sirsi overview", Path: "README.md", Limit: 550},
		{Title: "Deity registry", Path: "docs/DEITY_REGISTRY.md", Limit: 750},
		{Title: "Portfolio standard", Path: "docs/SIRSI_PORTFOLIO_STANDARD.md", Limit: 550},
		{Title: "Orchestration brain", Path: "docs/prd/ORCHESTRATION_BRAIN.md", Limit: 750},
		{Title: "Pantheon unification", Path: "docs/ADR-005-PANTHEON-UNIFICATION.md", Limit: 550},
		{Title: "Local model doctrine", Path: "docs/ADR-034-ORCHESTRATION-BRAIN.md", Limit: 650},
		{Title: "Knowledge substrate", Path: "docs/ADR-019-KNOWLEDGE-SUBSTRATE.md", Limit: 600},
		{Title: "Seshat specification", Path: "docs/SESHAT_SPECIFICATION.md", Limit: 450},
		{Title: "Thoth specification", Path: "docs/THOTH_SPECIFICATION.md", Limit: 450},
		{Title: "Thoth memory", Path: ".thoth/memory.yaml", Limit: 450},
	}
}

// ShortContext is the compact Sirsi identity pack for narrow prompts or a
// fallback when full canon grounding exceeds a model's answer budget.
func ShortContext() string {
	return strings.Join([]string{
		"SHORT SIRSI CONTEXT",
		"You are Ask Sirsi, the local on-device assistant and internal system manager for Sirsi Pantheon.",
		"Pantheon includes the Mac menubar, CLI, TUI, CTR/router fabric, cleanup, health, memory, knowledge, and agent operations surfaces.",
		"Ra routes work; Horus sees the workstation; Thoth preserves memory; Ma'at governs quality; Seshat moves knowledge; Hapi governs pressure/admission; Seba maps hardware.",
		"The Local LLM slot is provider-agnostic: Gemma/MLX, Qwen, Ollama, Core ML, or a future local backend may occupy it, but Sirsi owns identity and routing above the model.",
		"Hypergraph and Sirsi IO connect routed events, local knowledge, Hedera HCS direction, portfolio context, and agent coordination.",
		"Router/CTR coordinates Claude Home, Claude Pantheon, Codex Pantheon, Codex Home, Gemini, Gemma, Qwen, and repo-scoped worker threads through inboxes and heartbeats.",
		"Portfolio: Sirsi Nexus, Pantheon, FinalWishes, Assiduous, Ask Eliot, Porch and Alley, and the Sirsi deck/investor materials.",
		"User: Cylton Collymore, founder/operator of Sirsi.",
	}, "\n")
}

// BuildContext assembles the shared Ask Sirsi grounding pack.
func BuildContext(root string) ContextPack {
	docs := CanonDocuments()
	return ContextPack{
		System:       SystemPrompt(),
		ShortContext: ShortContext(),
		CanonPack:    CanonPack(root, defaultCanonBudget),
		Grounding:    Grounding(root),
		Documents:    docs,
	}
}

// Grounding reports whether the configured Pantheon root can ground Ask Sirsi.
func Grounding(root string) GroundingStatus {
	root = strings.TrimSpace(root)
	total := len(CanonDocuments())
	if root == "" {
		return GroundingStatus{Value: "unavailable", Detail: "no Pantheon canon root configured", Total: total}
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return GroundingStatus{Root: root, Value: "unavailable", Detail: "Pantheon canon root is not readable", Total: total}
	}
	readable := 0
	for _, doc := range CanonDocuments() {
		if st, err := os.Stat(filepath.Join(root, doc.Path)); err == nil && !st.IsDir() {
			readable++
		}
	}
	if readable == 0 {
		return GroundingStatus{Root: root, Value: "unavailable", Detail: "Pantheon root found, but canon files are unreadable", Total: total}
	}
	value := strings.TrimSpace(strings.Join([]string{itoa(readable), "/", itoa(total), " sources"}, ""))
	if readable == total {
		return GroundingStatus{Root: root, Value: value, Detail: "live bounded canon pack is readable", Healthy: true, Readable: readable, Total: total}
	}
	return GroundingStatus{Root: root, Value: value, Detail: "canon grounding is partial", Readable: readable, Total: total}
}

// CanonPack returns bounded excerpts from the local canon root.
func CanonPack(root string, maxTotal int) string {
	root = strings.TrimSpace(root)
	if root == "" {
		return "CANON PACK: no Sirsi Pantheon project root is configured or discoverable."
	}
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		return "CANON PACK: Sirsi Pantheon root is not readable at " + root + "."
	}
	if maxTotal <= 0 {
		maxTotal = defaultCanonBudget
	}
	var chunks []string
	total := 0
	for _, doc := range CanonDocuments() {
		if total >= maxTotal {
			break
		}
		data, err := os.ReadFile(filepath.Join(root, doc.Path))
		if err != nil {
			continue
		}
		remaining := maxTotal - total
		limit := doc.Limit
		if limit > remaining {
			limit = remaining
		}
		excerpt := CompactCanonExcerpt(string(data), limit)
		if excerpt == "" {
			continue
		}
		chunks = append(chunks, "### "+doc.Title+" ("+doc.Path+")\n"+excerpt)
		total += len(excerpt)
	}
	if len(chunks) == 0 {
		return "CANON PACK: Sirsi Pantheon root found at " + root + ", but no canon files were readable."
	}
	return "CANON PACK (bounded excerpts from " + root + ")\n" + strings.Join(chunks, "\n\n")
}

// CompactCanonExcerpt keeps grounding useful and cheap enough for local models.
func CompactCanonExcerpt(text string, limit int) string {
	normalized := strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\t", " ")
	var useful []string
	for _, line := range strings.Split(normalized, "\n") {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "![") || strings.HasPrefix(t, "<img") ||
			strings.HasPrefix(t, "| :") || strings.HasPrefix(t, "<!--") {
			continue
		}
		useful = append(useful, t)
	}
	out := strings.Join(useful, "\n")
	if limit <= 0 || len(out) <= limit {
		return out
	}
	return out[:limit] + "\n[excerpt truncated]"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
