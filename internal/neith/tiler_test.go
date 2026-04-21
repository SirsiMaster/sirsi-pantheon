package neith

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// ── ChunkCanon Tests ────────────────────────────────────────────────

func TestChunkCanon_AllSources(t *testing.T) {
	ctx := &CanonContext{
		ClaudeMD:           "# Rules\nRule 1\n",
		ThothMemory:        "project: test\n",
		ThothJournal:       "## Entry 001 — 2026-04-01\nDid stuff\n---\n## Entry 002 — 2026-04-02\nMore stuff\n",
		ContinuationPrompt: "Continue from phase 2\n",
		ADRs: []namedDoc{
			{Name: "ADR-001-FOUNDING.md", Content: "Founding decisions\n"},
			{Name: "ADR-002-KA.md", Content: "Ka ghost detection\n"},
		},
		PlanningDocs: []namedDoc{
			{Name: "ARCHITECTURE_DESIGN.md", Content: "Architecture\n"},
		},
		Changelog: "# Changelog\n## v0.10.0\nStuff\n## v0.9.0\nOlder stuff\n",
		Version:   "0.10.0",
	}

	chunks := ChunkCanon(ctx)

	// Verify we have chunks from all sources
	sources := make(map[string]int)
	for _, c := range chunks {
		sources[c.Source]++
	}

	if sources["identity"] != 1 {
		t.Errorf("expected 1 identity chunk, got %d", sources["identity"])
	}
	if sources["memory"] != 1 {
		t.Errorf("expected 1 memory chunk, got %d", sources["memory"])
	}
	if sources["continuation"] != 1 {
		t.Errorf("expected 1 continuation chunk, got %d", sources["continuation"])
	}
	if sources["journal"] < 2 {
		t.Errorf("expected >=2 journal chunks, got %d", sources["journal"])
	}
	if sources["adr"] != 2 {
		t.Errorf("expected 2 adr chunks, got %d", sources["adr"])
	}
	if sources["planning"] != 1 {
		t.Errorf("expected 1 planning chunk, got %d", sources["planning"])
	}
	// changelog splits at version headers — any count >= 1 is valid.
	if sources["version"] != 1 {
		t.Errorf("expected 1 version chunk, got %d", sources["version"])
	}
}

func TestChunkCanon_EmptyContext(t *testing.T) {
	ctx := &CanonContext{}
	chunks := ChunkCanon(ctx)
	if len(chunks) != 0 {
		t.Errorf("expected 0 chunks for empty context, got %d", len(chunks))
	}
}

func TestChunkCanon_JournalSplitting(t *testing.T) {
	ctx := &CanonContext{
		ThothJournal: "# Journal\n\n## Entry 001 — 2026-04-01\nFirst\n\n---\n\n## Entry 002 — 2026-04-02\nSecond\n\n---\n\n## Entry 003 — 2026-04-03\nThird\n",
	}

	chunks := ChunkCanon(ctx)

	journalChunks := 0
	for _, c := range chunks {
		if c.Source == "journal" {
			journalChunks++
		}
	}

	// Header + 3 entries = 4 chunks (header is everything before first ## Entry)
	if journalChunks < 3 {
		t.Errorf("expected at least 3 journal chunks, got %d", journalChunks)
	}
}

func TestChunkCanon_ChangelogSplitting(t *testing.T) {
	ctx := &CanonContext{
		Changelog: "# Changelog\n\n## v0.10.0\nNew stuff\n\n## v0.9.0\nOld stuff\n\n## v0.8.0\nOlder stuff\n",
	}

	chunks := ChunkCanon(ctx)

	changelogChunks := 0
	for _, c := range chunks {
		if c.Source == "changelog" {
			changelogChunks++
		}
	}

	if changelogChunks < 3 {
		t.Errorf("expected at least 3 changelog chunks, got %d", changelogChunks)
	}
}

func TestChunkCanon_TokenEstimate(t *testing.T) {
	ctx := &CanonContext{
		ClaudeMD: strings.Repeat("a", 400), // 400 chars → ~100 tokens
	}

	chunks := ChunkCanon(ctx)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0].Tokens != 100 {
		t.Errorf("expected ~100 tokens, got %d", chunks[0].Tokens)
	}
}

// ── ScoreChunks Tests ───────────────────────────────────────────────

func TestScoreChunks_AlwaysVisible(t *testing.T) {
	chunks := []CanonChunk{
		{Source: "identity", Name: "CLAUDE.md", Content: "rules", Tokens: 100},
		{Source: "memory", Name: "memory.yaml", Content: "state", Tokens: 50},
		{Source: "continuation", Name: "CONTINUATION-PROMPT.md", Content: "continue", Tokens: 30},
		{Source: "version", Name: "VERSION", Content: "0.10.0", Tokens: 5},
	}

	scope := ScopeConfig{ScopeOfWork: "build the auth module"}
	scored := ScoreChunks(chunks, scope)

	for _, s := range scored {
		if s.Score < 0.9 {
			t.Errorf("%s should be always-visible (>=0.9), got %.2f", s.Name, s.Score)
		}
	}
}

func TestScoreChunks_ADR001Floor(t *testing.T) {
	chunks := []CanonChunk{
		{Source: "adr", Name: "ADR-001-FOUNDING.md", Content: "unrelated content", Tokens: 100},
		{Source: "adr", Name: "ADR-007-GRPC.md", Content: "unrelated content", Tokens: 100},
	}

	scope := ScopeConfig{ScopeOfWork: "build the UI dashboard"}
	scored := ScoreChunks(chunks, scope)

	var adr001Score, adr007Score float64
	for _, s := range scored {
		if strings.Contains(s.Name, "ADR-001") {
			adr001Score = s.Score
		}
		if strings.Contains(s.Name, "ADR-007") {
			adr007Score = s.Score
		}
	}

	if adr001Score < 0.8 {
		t.Errorf("ADR-001 should have floor 0.80, got %.2f", adr001Score)
	}
	if adr007Score >= 0.8 {
		t.Errorf("ADR-007 (unrelated) should be below 0.80, got %.2f", adr007Score)
	}
}

func TestScoreChunks_KeywordMatch(t *testing.T) {
	chunks := []CanonChunk{
		{Source: "adr", Name: "ADR-005-AUTH.md", Content: "authentication MFA TOTP security", Tokens: 100},
		{Source: "adr", Name: "ADR-006-UI.md", Content: "dashboard components layout", Tokens: 100},
	}

	scope := ScopeConfig{ScopeOfWork: "implement authentication and security module"}
	scored := ScoreChunks(chunks, scope)

	var authScore, uiScore float64
	for _, s := range scored {
		if strings.Contains(s.Name, "AUTH") {
			authScore = s.Score
		}
		if strings.Contains(s.Name, "UI") {
			uiScore = s.Score
		}
	}

	if authScore <= uiScore {
		t.Errorf("auth ADR (%.2f) should score higher than UI ADR (%.2f)", authScore, uiScore)
	}
}

func TestScoreChunks_TemporalProximity(t *testing.T) {
	recent := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	old := time.Now().AddDate(0, 0, -80).Format("2006-01-02")

	chunks := []CanonChunk{
		{Source: "journal", Name: "Recent Entry", Content: fmt.Sprintf("## Entry 010 — %s\nRecent work", recent), Tokens: 50},
		{Source: "journal", Name: "Old Entry", Content: fmt.Sprintf("## Entry 002 — %s\nOld work", old), Tokens: 50},
	}

	scope := ScopeConfig{}
	scored := ScoreChunks(chunks, scope)

	var recentScore, oldScore float64
	for _, s := range scored {
		if s.Name == "Recent Entry" {
			recentScore = s.Score
		}
		if s.Name == "Old Entry" {
			oldScore = s.Score
		}
	}

	if recentScore <= oldScore {
		t.Errorf("recent entry (%.2f) should score higher than old entry (%.2f)", recentScore, oldScore)
	}
}

func TestScoreChunks_EmptyScope(t *testing.T) {
	chunks := []CanonChunk{
		{Source: "adr", Name: "ADR-005-AUTH.md", Content: "auth stuff", Tokens: 100},
	}

	scope := ScopeConfig{} // empty scope_of_work
	scored := ScoreChunks(chunks, scope)

	// Should still get some score (fallback), not zero
	if scored[0].Score == 0 {
		t.Error("chunk with empty scope should still get a non-zero score")
	}
}

// ── TilePrompt Tests ────────────────────────────────────────────────

func TestTilePrompt_AlwaysVisibleBypassBudget(t *testing.T) {
	scored := []ScoredChunk{
		{CanonChunk: CanonChunk{Source: "identity", Name: "CLAUDE.md", Tokens: 5000}, Score: 1.0},
		{CanonChunk: CanonChunk{Source: "memory", Name: "memory.yaml", Tokens: 3000}, Score: 1.0},
		{CanonChunk: CanonChunk{Source: "adr", Name: "ADR-005.md", Tokens: 2000}, Score: 0.5},
	}

	// Budget is tiny but always-visible should still be included
	result := TilePrompt(scored, 1000)

	if len(result.Rendered) < 2 {
		t.Errorf("expected at least 2 always-visible chunks rendered, got %d", len(result.Rendered))
	}

	// Check that the always-visible ones are rendered
	rendered := make(map[string]bool)
	for _, r := range result.Rendered {
		rendered[r.Name] = true
	}

	if !rendered["CLAUDE.md"] {
		t.Error("CLAUDE.md should be rendered (always-visible)")
	}
	if !rendered["memory.yaml"] {
		t.Error("memory.yaml should be rendered (always-visible)")
	}
}

func TestTilePrompt_BudgetFilling(t *testing.T) {
	scored := []ScoredChunk{
		{CanonChunk: CanonChunk{Name: "High", Tokens: 3000}, Score: 0.8},
		{CanonChunk: CanonChunk{Name: "Medium", Tokens: 3000}, Score: 0.5},
		{CanonChunk: CanonChunk{Name: "Low", Tokens: 3000}, Score: 0.2},
	}

	result := TilePrompt(scored, 5000)

	// Should fit High (3000) + Medium doesn't fit (6000 > 5000)
	// Actually: High fits (3000 <= 5000), Medium fits (6000 > 5000? No: 3000 <= 5000-3000=2000? No.)
	// High=3000, remaining=2000. Medium=3000 > 2000, deferred. Low=3000 > 2000, deferred.
	if len(result.Rendered) != 1 {
		t.Errorf("expected 1 rendered chunk, got %d", len(result.Rendered))
	}
	if len(result.Deferred) != 2 {
		t.Errorf("expected 2 deferred chunks, got %d", len(result.Deferred))
	}
	if result.Rendered[0].Name != "High" {
		t.Errorf("expected High to be rendered, got %s", result.Rendered[0].Name)
	}
}

func TestTilePrompt_EmptyInput(t *testing.T) {
	result := TilePrompt(nil, 80000)
	if len(result.Rendered) != 0 || len(result.Deferred) != 0 {
		t.Error("empty input should produce empty result")
	}
}

func TestTilePrompt_EverythingFits(t *testing.T) {
	scored := []ScoredChunk{
		{CanonChunk: CanonChunk{Name: "A", Tokens: 1000}, Score: 0.8},
		{CanonChunk: CanonChunk{Name: "B", Tokens: 1000}, Score: 0.5},
		{CanonChunk: CanonChunk{Name: "C", Tokens: 1000}, Score: 0.2},
	}

	result := TilePrompt(scored, 100000)

	if len(result.Rendered) != 3 {
		t.Errorf("expected all 3 rendered, got %d", len(result.Rendered))
	}
	if len(result.Deferred) != 0 {
		t.Errorf("expected 0 deferred, got %d", len(result.Deferred))
	}
}

// ── AutoTokenBudget Tests ───────────────────────────────────────────

func TestAutoTokenBudget(t *testing.T) {
	tests := []struct {
		total    int
		expected int
	}{
		{30000, 0},      // small canon, no tiling
		{50000, 0},      // at threshold, no tiling
		{80000, 80000},  // default budget
		{150000, 80000}, // still default
		{200001, 60000}, // large canon
		{500000, 60000}, // very large
	}

	for _, tt := range tests {
		got := AutoTokenBudget(tt.total)
		if got != tt.expected {
			t.Errorf("AutoTokenBudget(%d) = %d, want %d", tt.total, got, tt.expected)
		}
	}
}

// ── FormatManifest Tests ────────────────────────────────────────────

func TestFormatManifest_EmptyDeferred(t *testing.T) {
	result := FormatManifest(nil)
	if result != "" {
		t.Error("empty deferred should produce empty manifest")
	}
}

func TestFormatManifest_BasicOutput(t *testing.T) {
	deferred := []ScoredChunk{
		{CanonChunk: CanonChunk{Source: "adr", Name: "ADR-007.md", Tokens: 1200}, Score: 0.28},
	}

	manifest := FormatManifest(deferred)

	if !strings.Contains(manifest, "Deferred Context") {
		t.Error("manifest should contain header")
	}
	if !strings.Contains(manifest, "ADR-007.md") {
		t.Error("manifest should contain deferred doc name")
	}
	if !strings.Contains(manifest, "0.28") {
		t.Error("manifest should contain score")
	}
}

func TestFormatManifest_JournalGrouping(t *testing.T) {
	deferred := []ScoredChunk{
		{CanonChunk: CanonChunk{Source: "journal", Name: "Journal Entry 001", Tokens: 100}, Score: 0.1},
		{CanonChunk: CanonChunk{Source: "journal", Name: "Journal Entry 002", Tokens: 100}, Score: 0.1},
		{CanonChunk: CanonChunk{Source: "journal", Name: "Journal Entry 003", Tokens: 100}, Score: 0.1},
		{CanonChunk: CanonChunk{Source: "adr", Name: "ADR-010.md", Tokens: 500}, Score: 0.2},
	}

	manifest := FormatManifest(deferred)

	// Journal entries should be grouped
	if strings.Contains(manifest, "Journal Entry 001") {
		t.Error("individual journal entries should be grouped, not listed separately")
	}
	if !strings.Contains(manifest, "Journal Entries") {
		t.Error("manifest should contain grouped journal entry")
	}
	if !strings.Contains(manifest, "ADR-010.md") {
		t.Error("manifest should still contain non-journal entries")
	}
}

func TestFormatManifest_MaxEntries(t *testing.T) {
	var deferred []ScoredChunk
	for i := 0; i < 30; i++ {
		deferred = append(deferred, ScoredChunk{
			CanonChunk: CanonChunk{Source: "adr", Name: fmt.Sprintf("ADR-%03d.md", i), Tokens: 100},
			Score:      0.1,
		})
	}

	manifest := FormatManifest(deferred)

	// Should cap at MaxManifestEntries and show "... and N more"
	if !strings.Contains(manifest, "more") {
		t.Error("manifest should indicate remaining entries beyond cap")
	}
}

// ── Helper Tests ────────────────────────────────────────────────────

func TestExtractAgeDays(t *testing.T) {
	today := time.Now().Format("2006-01-02")
	content := fmt.Sprintf("## Entry 001 — %s\nSome content", today)

	age := extractAgeDays(content)
	if age != 0 {
		t.Errorf("today's entry should be 0 days old, got %d", age)
	}

	oldContent := "## Entry 001 — 2020-01-01\nOld content"
	oldAge := extractAgeDays(oldContent)
	if oldAge < 365 {
		t.Errorf("2020 entry should be >365 days old, got %d", oldAge)
	}

	noDateContent := "No date here"
	noAge := extractAgeDays(noDateContent)
	if noAge != -1 {
		t.Errorf("no-date content should return -1, got %d", noAge)
	}
}

func TestExtractSignificantTerms(t *testing.T) {
	content := "authentication MFA TOTP security module the a an is"
	terms := extractSignificantTerms(content)

	if !terms["authentication"] {
		t.Error("should include 'authentication'")
	}
	if !terms["security"] {
		t.Error("should include 'security'")
	}
	if !terms["module"] {
		t.Error("should include 'module'")
	}
	// Short words should be filtered
	if terms["the"] || terms["a"] || terms["an"] || terms["is"] {
		t.Error("should filter short words")
	}
}

func TestTokenEstimate(t *testing.T) {
	if tokenEstimate("1234") != 1 {
		t.Errorf("4 chars should be ~1 token, got %d", tokenEstimate("1234"))
	}
	if tokenEstimate(strings.Repeat("x", 4000)) != 1000 {
		t.Errorf("4000 chars should be ~1000 tokens")
	}
}

func TestFormatTokenCount(t *testing.T) {
	if formatTokenCount(500) != "~500" {
		t.Errorf("expected ~500, got %s", formatTokenCount(500))
	}
	if formatTokenCount(1500) != "~1.5K" {
		t.Errorf("expected ~1.5K, got %s", formatTokenCount(1500))
	}
}

// ── Integration Test ────────────────────────────────────────────────

func TestTilingPipeline_EndToEnd(t *testing.T) {
	ctx := &CanonContext{
		ClaudeMD:           strings.Repeat("rules ", 1000),                                   // ~1500 tokens
		ThothMemory:        strings.Repeat("state ", 500),                                    // ~750 tokens
		ContinuationPrompt: strings.Repeat("continue ", 200),                                 // ~300 tokens
		ThothJournal:       "## Entry 001 — 2026-04-01\n" + strings.Repeat("journal ", 2000), // ~3000 tokens
		ADRs: []namedDoc{
			{Name: "ADR-001-FOUNDING.md", Content: strings.Repeat("founding ", 1000)},
			{Name: "ADR-005-AUTH.md", Content: strings.Repeat("authentication security ", 1000)},
			{Name: "ADR-010-MENUBAR.md", Content: strings.Repeat("menubar ui panel ", 1000)},
		},
		PlanningDocs: []namedDoc{
			{Name: "ARCHITECTURE_DESIGN.md", Content: strings.Repeat("architecture design ", 2000)},
		},
		Changelog: "# Changelog\n\n## v0.10.0\n" + strings.Repeat("change ", 500) + "\n## v0.9.0\n" + strings.Repeat("older ", 500),
		Version:   "0.10.0",
	}

	// Step 1: Chunk
	chunks := ChunkCanon(ctx)
	if len(chunks) == 0 {
		t.Fatal("expected chunks from canon")
	}

	// Step 2: Score
	scope := ScopeConfig{
		Name:        "auth-module",
		ScopeOfWork: "implement authentication and security",
	}
	scored := ScoreChunks(chunks, scope)
	if len(scored) != len(chunks) {
		t.Fatalf("scored count (%d) should match chunk count (%d)", len(scored), len(chunks))
	}

	// Step 3: Compute budget
	totalTokens := 0
	for _, c := range chunks {
		totalTokens += c.Tokens
	}
	budget := AutoTokenBudget(totalTokens)
	if budget == 0 {
		budget = totalTokens // everything fits
	}

	// Step 4: Tile
	result := TilePrompt(scored, budget)

	// Always-visible chunks must be rendered
	renderedNames := make(map[string]bool)
	for _, r := range result.Rendered {
		renderedNames[r.Name] = true
	}

	if !renderedNames["CLAUDE.md"] {
		t.Error("CLAUDE.md must be rendered")
	}
	if !renderedNames["memory.yaml"] {
		t.Error("memory.yaml must be rendered")
	}
	if !renderedNames["CONTINUATION-PROMPT.md"] {
		t.Error("CONTINUATION-PROMPT.md must be rendered")
	}

	// ADR-001 should be rendered (high floor score)
	if !renderedNames["ADR-001-FOUNDING.md"] {
		t.Error("ADR-001 must be rendered (foundational)")
	}

	// Auth ADR should score higher than menubar ADR for auth scope
	var authRendered, menubarRendered bool
	for _, r := range result.Rendered {
		if r.Name == "ADR-005-AUTH.md" {
			authRendered = true
		}
		if r.Name == "ADR-010-MENUBAR.md" {
			menubarRendered = true
		}
	}

	// If budget forced a choice, auth should win over menubar
	if !authRendered && menubarRendered {
		t.Error("auth ADR should be preferred over menubar ADR for auth scope")
	}

	// Step 5: Manifest
	if len(result.Deferred) > 0 {
		manifest := FormatManifest(result.Deferred)
		if manifest == "" {
			t.Error("deferred chunks should produce a non-empty manifest")
		}
	}

	t.Logf("Pipeline: %d chunks → %d rendered (%d tokens) + %d deferred (%d tokens)",
		len(chunks), len(result.Rendered), result.RenderedTokens,
		len(result.Deferred), result.DeferredTokens)
}
