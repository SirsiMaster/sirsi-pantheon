package neith

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// ── Types ───────────────────────────────────────────────────────────

// CanonChunk is a semantic unit of canon content — one "tile" in the
// context rendering pipeline. Analogous to a triangle in GPU tiled
// deferred rendering: loaded into memory, scored for visibility, then
// either rendered into the prompt or deferred to the manifest.
type CanonChunk struct {
	Source  string // "identity", "memory", "journal", "continuation", "adr", "planning", "changelog", "version"
	Name    string // e.g. "ADR-007-GRPC.md", "Journal Entry 014", "CHANGELOG v0.9.0"
	Content string
	Tokens  int // approximate token count (len(Content) / 4)
}

// ScoredChunk is a chunk after the visibility test (z-test).
type ScoredChunk struct {
	CanonChunk
	Score  float64 // 0.0–1.0 composite relevance score
	Reason string  // human-readable scoring rationale
}

// TileResult is the output of the tiling pass.
type TileResult struct {
	Rendered       []ScoredChunk // chunks woven into prompt (visible tiles)
	Deferred       []ScoredChunk // manifest only (off-screen tiles)
	RenderedTokens int
	DeferredTokens int
}

// ── Constants ───────────────────────────────────────────────────────

const (
	// DefaultTokenBudget is used when no token_budget is set and canon
	// is between 50K–200K tokens. Below 50K everything fits; above 200K
	// the budget tightens to 60K.
	DefaultTokenBudget = 80000

	// LargeCanonBudget is used when total canon exceeds 200K tokens.
	LargeCanonBudget = 60000

	// SmallCanonThreshold: below this, no tiling needed — everything fits.
	SmallCanonThreshold = 50000

	// LargeCanonThreshold: above this, use the tighter budget.
	LargeCanonThreshold = 200000

	// MaxManifestEntries caps the deferred manifest to avoid token waste.
	MaxManifestEntries = 20

	// TemporalDecayDays is the number of days over which temporal score
	// decays linearly from 1.0 to the floor.
	TemporalDecayDays = 90

	// TemporalFloor is the minimum score a chunk gets from temporal proximity.
	TemporalFloor = 0.1

	// CoverageThreshold: if this fraction of a chunk's significant terms
	// already appear in accepted chunks, halve its score (anti-overdraw).
	CoverageThreshold = 0.5
)

// ── ChunkCanon ──────────────────────────────────────────────────────

// ChunkCanon splits a CanonContext into addressable semantic units.
// Each document becomes one or more chunks depending on its structure.
// Journal entries split at "## Entry" boundaries. Changelog splits at
// version sections. Everything else is one chunk per document.
func ChunkCanon(ctx *CanonContext) []CanonChunk {
	var chunks []CanonChunk

	// Identity — always one chunk
	if ctx.ClaudeMD != "" {
		chunks = append(chunks, CanonChunk{
			Source:  "identity",
			Name:    "CLAUDE.md",
			Content: ctx.ClaudeMD,
			Tokens:  tokenEstimate(ctx.ClaudeMD),
		})
	}

	// Thoth memory — always one chunk
	if ctx.ThothMemory != "" {
		chunks = append(chunks, CanonChunk{
			Source:  "memory",
			Name:    "memory.yaml",
			Content: ctx.ThothMemory,
			Tokens:  tokenEstimate(ctx.ThothMemory),
		})
	}

	// Continuation prompt — always one chunk
	if ctx.ContinuationPrompt != "" {
		chunks = append(chunks, CanonChunk{
			Source:  "continuation",
			Name:    "CONTINUATION-PROMPT.md",
			Content: ctx.ContinuationPrompt,
			Tokens:  tokenEstimate(ctx.ContinuationPrompt),
		})
	}

	// Journal — split at entry boundaries
	if ctx.ThothJournal != "" {
		chunks = append(chunks, chunkJournal(ctx.ThothJournal)...)
	}

	// ADRs — one chunk per ADR
	for _, adr := range ctx.ADRs {
		chunks = append(chunks, CanonChunk{
			Source:  "adr",
			Name:    adr.Name,
			Content: adr.Content,
			Tokens:  tokenEstimate(adr.Content),
		})
	}

	// Planning docs — one chunk per doc
	for _, doc := range ctx.PlanningDocs {
		chunks = append(chunks, CanonChunk{
			Source:  "planning",
			Name:    doc.Name,
			Content: doc.Content,
			Tokens:  tokenEstimate(doc.Content),
		})
	}

	// Changelog — split at version sections
	if ctx.Changelog != "" {
		chunks = append(chunks, chunkChangelog(ctx.Changelog)...)
	}

	// Version — tiny, always one chunk
	if ctx.Version != "" {
		chunks = append(chunks, CanonChunk{
			Source:  "version",
			Name:    "VERSION",
			Content: ctx.Version,
			Tokens:  tokenEstimate(ctx.Version),
		})
	}

	return chunks
}

// chunkJournal splits journal content at "## Entry" boundaries.
func chunkJournal(content string) []CanonChunk {
	lines := strings.Split(content, "\n")
	var chunks []CanonChunk
	var current strings.Builder
	currentName := "Journal Header"
	entryNum := 0

	for _, line := range lines {
		if strings.HasPrefix(line, "## Entry ") {
			// Flush previous
			if current.Len() > 0 {
				text := current.String()
				chunks = append(chunks, CanonChunk{
					Source:  "journal",
					Name:    currentName,
					Content: text,
					Tokens:  tokenEstimate(text),
				})
			}
			current.Reset()
			entryNum++
			currentName = fmt.Sprintf("Journal Entry %03d", entryNum)
		}
		current.WriteString(line)
		current.WriteString("\n")
	}

	// Flush last
	if current.Len() > 0 {
		text := current.String()
		chunks = append(chunks, CanonChunk{
			Source:  "journal",
			Name:    currentName,
			Content: text,
			Tokens:  tokenEstimate(text),
		})
	}

	return chunks
}

// chunkChangelog splits changelog content at version section boundaries.
func chunkChangelog(content string) []CanonChunk {
	lines := strings.Split(content, "\n")
	var chunks []CanonChunk
	var current strings.Builder
	currentName := "CHANGELOG Header"
	sectionCount := 0

	for _, line := range lines {
		isVersionHeader := (strings.HasPrefix(line, "## ") || strings.HasPrefix(line, "# ")) &&
			(strings.Contains(line, "[") || strings.Contains(line, "v") || strings.Contains(line, "V"))

		if isVersionHeader && current.Len() > 0 {
			text := current.String()
			chunks = append(chunks, CanonChunk{
				Source:  "changelog",
				Name:    currentName,
				Content: text,
				Tokens:  tokenEstimate(text),
			})
			current.Reset()
			sectionCount++
			// Extract version from header
			currentName = fmt.Sprintf("CHANGELOG %s", strings.TrimSpace(strings.TrimLeft(line, "#")))
		}

		current.WriteString(line)
		current.WriteString("\n")
	}

	if current.Len() > 0 {
		text := current.String()
		chunks = append(chunks, CanonChunk{
			Source:  "changelog",
			Name:    currentName,
			Content: text,
			Tokens:  tokenEstimate(text),
		})
	}

	return chunks
}

// ── ScoreChunks ─────────────────────────────────────────────────────

// ScoreChunks runs the multi-signal visibility test on each chunk.
// Signals: structural weight, keyword match, temporal proximity, coverage.
func ScoreChunks(chunks []CanonChunk, scope ScopeConfig) []ScoredChunk {
	// Combine path-based keywords with significant terms from scope text
	// for broader matching beyond just tech/path tokens.
	keywords := extractScopeKeywords(scope.ScopeOfWork)
	for term := range extractSignificantTerms(scope.ScopeOfWork) {
		keywords = append(keywords, term)
	}
	// Deduplicate
	keywords = deduplicateStrings(keywords)
	scored := make([]ScoredChunk, len(chunks))

	for i, chunk := range chunks {
		score, reason := scoreChunk(chunk, keywords)
		scored[i] = ScoredChunk{
			CanonChunk: chunk,
			Score:      score,
			Reason:     reason,
		}
	}

	// Coverage detection pass — reduce score for chunks whose content
	// is already represented in higher-scored chunks (anti-overdraw).
	applyCoverageDetection(scored)

	return scored
}

// scoreChunk computes the raw score for a single chunk before coverage.
func scoreChunk(chunk CanonChunk, scopeKeywords []string) (float64, string) {
	// Signal 1: Structural weight — always-visible HUD elements
	switch chunk.Source {
	case "identity":
		return 1.0, "identity: always visible"
	case "memory":
		return 1.0, "memory: always visible"
	case "continuation":
		return 0.95, "continuation: near plane"
	case "version":
		return 0.90, "version: always visible"
	}

	// ADR-001 gets a high floor regardless of keyword match
	if chunk.Source == "adr" && strings.HasPrefix(strings.ToUpper(chunk.Name), "ADR-001") {
		return 0.80, "ADR-001: foundational"
	}

	var best float64
	var bestReason string

	// Signal 2: Keyword match against scope_of_work
	// Uses both path-based keywords (from extractScopeKeywords) and
	// significant terms (from extractSignificantTerms) for broader matching.
	if len(scopeKeywords) > 0 {
		matches := 0
		lower := strings.ToLower(chunk.Name + "\n" + chunk.Content)
		for _, kw := range scopeKeywords {
			if strings.Contains(lower, strings.ToLower(kw)) {
				matches++
			}
		}
		if matches > 0 {
			ratio := float64(matches) / float64(len(scopeKeywords))
			kwScore := ratio * 0.8
			if kwScore > best {
				best = kwScore
				bestReason = fmt.Sprintf("keyword match: %d/%d (%.0f%%)", matches, len(scopeKeywords), ratio*100)
			}
		}
	}

	// Signal 3: Temporal proximity (journal + changelog)
	if chunk.Source == "journal" || chunk.Source == "changelog" {
		age := extractAgeDays(chunk.Content)
		if age >= 0 {
			recency := math.Max(TemporalFloor, 1.0-float64(age)/float64(TemporalDecayDays))
			temporalScore := recency * 0.7
			if temporalScore > best {
				best = temporalScore
				bestReason = fmt.Sprintf("temporal: %d days old (%.2f)", age, recency)
			}
		}
	}

	// Fallback: if no signal matched, give a low base score
	if best == 0 {
		best = 0.10
		bestReason = "no signal match"
	}

	return best, bestReason
}

// applyCoverageDetection reduces scores for chunks whose significant
// terms are already well-represented in higher-scored chunks.
func applyCoverageDetection(scored []ScoredChunk) {
	// Sort by score descending to process highest first
	sort.SliceStable(scored, func(i, j int) bool {
		return scored[i].Score > scored[j].Score
	})

	// Build accepted term set from highest-scored chunks
	var acceptedTerms map[string]bool

	for i := range scored {
		if i == 0 {
			acceptedTerms = extractSignificantTerms(scored[0].Content)
			continue
		}

		chunkTerms := extractSignificantTerms(scored[i].Content)
		if len(chunkTerms) == 0 {
			continue
		}

		// Count how many of this chunk's terms are already in accepted
		overlap := 0
		for term := range chunkTerms {
			if acceptedTerms[term] {
				overlap++
			}
		}

		coverageRatio := float64(overlap) / float64(len(chunkTerms))
		if coverageRatio > CoverageThreshold {
			scored[i].Score *= 0.5
			scored[i].Reason += fmt.Sprintf(" | coverage: %.0f%% overlap, halved", coverageRatio*100)
		}

		// Add this chunk's terms to accepted set
		for term := range chunkTerms {
			acceptedTerms[term] = true
		}
	}
}

// ── TilePrompt ──────────────────────────────────────────────────────

// TilePrompt fills the token budget with highest-scored chunks.
// Always-visible chunks (score >= 0.9) are included regardless of budget.
// Returns rendered chunks and deferred manifest.
func TilePrompt(scored []ScoredChunk, tokenBudget int) TileResult {
	// Sort by score descending
	sorted := make([]ScoredChunk, len(scored))
	copy(sorted, scored)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Score > sorted[j].Score
	})

	var result TileResult
	remaining := tokenBudget

	for _, chunk := range sorted {
		// Always-visible chunks skip budget check
		if chunk.Score >= 0.9 {
			result.Rendered = append(result.Rendered, chunk)
			result.RenderedTokens += chunk.Tokens
			remaining -= chunk.Tokens
			continue
		}

		// Budget check for other chunks
		if chunk.Tokens <= remaining {
			result.Rendered = append(result.Rendered, chunk)
			result.RenderedTokens += chunk.Tokens
			remaining -= chunk.Tokens
		} else {
			result.Deferred = append(result.Deferred, chunk)
			result.DeferredTokens += chunk.Tokens
		}
	}

	return result
}

// AutoTokenBudget determines the token budget based on total canon size.
// Returns 0 if total is below SmallCanonThreshold (meaning: render everything).
func AutoTokenBudget(totalTokens int) int {
	if totalTokens <= SmallCanonThreshold {
		return 0 // everything fits, no tiling needed
	}
	if totalTokens > LargeCanonThreshold {
		return LargeCanonBudget
	}
	return DefaultTokenBudget
}

// FormatManifest creates the deferred context manifest section.
// Groups similar items and caps at MaxManifestEntries.
func FormatManifest(deferred []ScoredChunk) string {
	if len(deferred) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("## Deferred Context (available — not pre-loaded)\n\n")
	b.WriteString("The following documents were not included in this prompt to conserve context.\n")
	b.WriteString("Read any of them directly if needed for your current task.\n\n")
	b.WriteString("| Document | ~Tokens | Score |\n")
	b.WriteString("|----------|---------|-------|\n")

	// Group journal entries
	deferred = groupJournalEntries(deferred)

	shown := 0
	for _, chunk := range deferred {
		if shown >= MaxManifestEntries {
			remaining := len(deferred) - shown
			if remaining > 0 {
				b.WriteString(fmt.Sprintf("| ... and %d more | | |\n", remaining))
			}
			break
		}

		tokenStr := formatTokenCount(chunk.Tokens)
		b.WriteString(fmt.Sprintf("| %s | %s | %.2f |\n", chunk.Name, tokenStr, chunk.Score))
		shown++
	}

	b.WriteString("\n")
	return b.String()
}

// ── Helpers ─────────────────────────────────────────────────────────

func tokenEstimate(s string) int {
	return len(s) / 4
}

// extractAgeDays attempts to parse a date from chunk content and returns
// the age in days. Returns -1 if no date found.
func extractAgeDays(content string) int {
	// Look for dates in common formats: YYYY-MM-DD
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		// Check first few lines only (dates are usually in headers)
		if len(line) < 10 {
			continue
		}
		// Try to find YYYY-MM-DD pattern
		for i := 0; i <= len(line)-10; i++ {
			candidate := line[i : i+10]
			if parsed, err := time.Parse("2006-01-02", candidate); err == nil {
				now := time.Now()
				todayStr := now.Format("2006-01-02")
				today, _ := time.Parse("2006-01-02", todayStr)
				return int(today.Sub(parsed).Hours() / 24)
			}
		}
		// Only check first 5 lines for performance
		if strings.HasPrefix(line, "## ") || strings.HasPrefix(line, "# ") {
			break
		}
	}
	return -1
}

// extractSignificantTerms returns a set of meaningful terms from content,
// filtering out common stop words and short tokens.
func extractSignificantTerms(content string) map[string]bool {
	terms := make(map[string]bool)
	for _, word := range strings.Fields(strings.ToLower(content)) {
		word = strings.Trim(word, ".,;:()\"'`*#|-[]{}!?/\\")
		if len(word) < 4 {
			continue
		}
		// Skip very common words
		if isStopWord(word) {
			continue
		}
		terms[word] = true
	}
	return terms
}

func isStopWord(w string) bool {
	stops := map[string]bool{
		"this": true, "that": true, "with": true, "from": true,
		"have": true, "been": true, "will": true, "would": true,
		"should": true, "could": true, "must": true, "shall": true,
		"each": true, "every": true, "which": true, "their": true,
		"there": true, "these": true, "those": true, "when": true,
		"where": true, "what": true, "about": true, "into": true,
		"over": true, "after": true, "before": true, "between": true,
		"under": true, "above": true, "below": true, "does": true,
		"more": true, "most": true, "also": true, "only": true,
		"than": true, "then": true, "some": true, "such": true,
		"other": true, "like": true, "just": true, "very": true,
		"same": true, "they": true, "them": true, "were": true,
		"your": true, "being": true, "using": true, "used": true,
	}
	return stops[w]
}

// groupJournalEntries consolidates sequential journal entry chunks
// into a single manifest entry (e.g., "Journal Entries 001-008").
func groupJournalEntries(chunks []ScoredChunk) []ScoredChunk {
	var result []ScoredChunk
	var journalChunks []ScoredChunk

	for _, c := range chunks {
		if c.Source == "journal" && strings.HasPrefix(c.Name, "Journal Entry") {
			journalChunks = append(journalChunks, c)
		} else {
			result = append(result, c)
		}
	}

	if len(journalChunks) == 0 {
		return result
	}

	if len(journalChunks) == 1 {
		return append(result, journalChunks...)
	}

	// Group into one entry
	totalTokens := 0
	var minScore float64 = 1.0
	for _, jc := range journalChunks {
		totalTokens += jc.Tokens
		if jc.Score < minScore {
			minScore = jc.Score
		}
	}

	grouped := ScoredChunk{
		CanonChunk: CanonChunk{
			Source: "journal",
			Name:   fmt.Sprintf("Journal Entries (%d older entries)", len(journalChunks)),
			Tokens: totalTokens,
		},
		Score:  minScore,
		Reason: "grouped journal entries",
	}

	return append(result, grouped)
}

func deduplicateStrings(ss []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
}

func formatTokenCount(tokens int) string {
	if tokens >= 1000 {
		return fmt.Sprintf("~%.1fK", float64(tokens)/1000)
	}
	return fmt.Sprintf("~%d", tokens)
}
