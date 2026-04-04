package neith

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/SirsiMaster/sirsi-pantheon/internal/stele"
	"gopkg.in/yaml.v3"
)

// ScopeConfig defines a single scope of work for a target repository.
type ScopeConfig struct {
	Name        string  `yaml:"name"`
	DisplayName string  `yaml:"display_name"`
	RepoPath    string  `yaml:"repo_path"`
	Deadline    string  `yaml:"deadline"`
	Priority    string  `yaml:"priority"`
	ScopeOfWork string  `yaml:"scope_of_work"`
	MaxTurns    int     `yaml:"max_turns"`
	Sprints     int     `yaml:"sprints"`    // number of sprint turns (1 = one-shot, N = loop with --continue)
	BudgetUSD   float64 `yaml:"budget_usd"` // max API spend per deploy — API billing users only
}

// CanonContext holds ALL canon documents loaded from a target repo.
// Neith reads everything — no truncation, no summaries. She is the Weaver
// and must see the full tapestry to keep the loom aligned.
type CanonContext struct {
	ClaudeMD           string     // full CLAUDE.md or GEMINI.md
	ThothMemory        string     // full .thoth/memory.yaml
	ThothJournal       string     // full .thoth/journal.md
	ContinuationPrompt string     // full docs/CONTINUATION-PROMPT.md
	ADRs               []namedDoc // full text of every ADR
	PlanningDocs       []namedDoc // full text of blueprints, plans, specs, roadmaps
	Changelog          string     // full CHANGELOG.md
	Version            string     // VERSION file
}

// namedDoc is a canon document with its filename for prompt labeling.
type namedDoc struct {
	Name    string // e.g. "ADR-002-IMPLEMENTATION-PLAN.md"
	Content string
}

// DriftReport summarizes scope drift detected in a git diff.
type DriftReport struct {
	ScopeName  string
	DriftFound bool
	Findings   []string // e.g. "Modified files outside scope", "New dependency not in plan"
}

// Loom is Neith's scope assembly engine. It reads canon documents from target
// repos, assembles scope prompts, and evaluates drift.
type Loom struct {
	ConfigDir string // path to configs/scopes/
}

// NewLoom creates a Loom pointed at the given scope config directory.
func NewLoom(configDir string) *Loom {
	return &Loom{ConfigDir: configDir}
}

// LoadScopes parses all YAML files in ConfigDir and returns the scope configs.
func (l *Loom) LoadScopes() ([]ScopeConfig, error) {
	pattern := filepath.Join(l.ConfigDir, "*.yaml")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("glob scopes: %w", err)
	}
	if len(matches) == 0 {
		// Also try .yml extension
		pattern = filepath.Join(l.ConfigDir, "*.yml")
		matches, err = filepath.Glob(pattern)
		if err != nil {
			return nil, fmt.Errorf("glob scopes yml: %w", err)
		}
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("no scope configs found in %s", l.ConfigDir)
	}

	var scopes []ScopeConfig
	for _, path := range matches {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read scope %s: %w", path, err)
		}
		var sc ScopeConfig
		if err := yaml.Unmarshal(data, &sc); err != nil {
			return nil, fmt.Errorf("parse scope %s: %w", path, err)
		}
		scopes = append(scopes, sc)
	}
	return scopes, nil
}

// expandHome replaces a leading ~ with the user's home directory.
func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}

// LoadCanon reads ALL canon documents from the target repository at repoPath.
// Neith sees everything — no truncation, no summaries, no artificial limits.
// She is the Weaver and must have full visibility to keep the loom aligned
// and to charge Thoth and Seshat with scribing the next continuation prompts.
func (l *Loom) LoadCanon(repoPath string) (*CanonContext, error) {
	root := expandHome(repoPath)
	ctx := &CanonContext{}

	// CLAUDE.md or GEMINI.md — full text
	claudeData, err := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	if err != nil {
		geminiData, err2 := os.ReadFile(filepath.Join(root, "GEMINI.md"))
		if err2 == nil {
			ctx.ClaudeMD = string(geminiData)
		}
	} else {
		ctx.ClaudeMD = string(claudeData)
	}

	// .thoth/memory.yaml — full text
	if data, err := os.ReadFile(filepath.Join(root, ".thoth", "memory.yaml")); err == nil {
		ctx.ThothMemory = string(data)
	}

	// .thoth/journal.md — full text (every entry, every decision)
	if data, err := os.ReadFile(filepath.Join(root, ".thoth", "journal.md")); err == nil {
		ctx.ThothJournal = string(data)
	}

	// docs/CONTINUATION-PROMPT.md — full text
	if data, err := os.ReadFile(filepath.Join(root, "docs", "CONTINUATION-PROMPT.md")); err == nil {
		ctx.ContinuationPrompt = string(data)
	}

	// ADRs — full text of every Architecture Decision Record.
	// ADRs define what SHOULD be built and WHY. Neith needs all of them.
	adrPattern := filepath.Join(root, "docs", "ADR-*.md")
	if adrFiles, err := filepath.Glob(adrPattern); err == nil {
		for _, f := range adrFiles {
			if data, err := os.ReadFile(f); err == nil {
				ctx.ADRs = append(ctx.ADRs, namedDoc{
					Name:    filepath.Base(f),
					Content: string(data),
				})
			}
		}
	}

	// Planning docs — full text of everything in docs/ that describes
	// what needs to be built: blueprints, plans, scopes, roadmaps, specs,
	// status reports, build logs, architecture docs, design docs.
	for _, pattern := range []string{
		filepath.Join(root, "docs", "*BLUEPRINT*.md"),
		filepath.Join(root, "docs", "*PLAN*.md"),
		filepath.Join(root, "docs", "*SCOPE*.md"),
		filepath.Join(root, "docs", "*ROADMAP*.md"),
		filepath.Join(root, "docs", "*SPECIFICATION*.md"),
		filepath.Join(root, "docs", "*PRODUCT*.md"),
		filepath.Join(root, "docs", "*STATUS*.md"),
		filepath.Join(root, "docs", "*BUILD_LOG*.md"),
		filepath.Join(root, "docs", "*ARCHITECTURE*.md"),
		filepath.Join(root, "docs", "*DESIGN*.md"),
		filepath.Join(root, "docs", "*MIGRATION*.md"),
	} {
		if files, err := filepath.Glob(pattern); err == nil {
			for _, f := range files {
				if data, err := os.ReadFile(f); err == nil {
					ctx.PlanningDocs = append(ctx.PlanningDocs, namedDoc{
						Name:    filepath.Base(f),
						Content: string(data),
					})
				}
			}
		}
	}

	// Deduplicate planning docs (patterns may overlap, e.g. ARCHITECTURE_DESIGN.md)
	ctx.PlanningDocs = deduplicateDocs(ctx.PlanningDocs)

	// CHANGELOG.md — full text
	if data, err := os.ReadFile(filepath.Join(root, "CHANGELOG.md")); err == nil {
		ctx.Changelog = string(data)
	}

	// VERSION file
	if data, err := os.ReadFile(filepath.Join(root, "VERSION")); err == nil {
		ctx.Version = strings.TrimSpace(string(data))
	}

	return ctx, nil
}

// deduplicateDocs removes duplicate documents by filename.
func deduplicateDocs(docs []namedDoc) []namedDoc {
	seen := make(map[string]bool)
	var unique []namedDoc
	for _, d := range docs {
		if !seen[d.Name] {
			seen[d.Name] = true
			unique = append(unique, d)
		}
	}
	return unique
}

// WeaveScope assembles the final scope prompt for the given scope config.
//
// Neith sees everything. No truncation. The full canon is woven into the
// prompt so the agent has complete context. Claude's 1M token context window
// can hold all of it. The prompt is assembled in this order:
//
//  1. Ra Autonomy Directive
//  2. Scope of Work — static (from YAML) OR dynamic (from canon)
//  3. Continuation Prompt — current state and next phases
//  4. Planning Docs — blueprints, dev plans, specs, roadmaps, architecture
//  5. ADRs — every Architecture Decision Record, full text
//  6. Thoth Memory + Journal — project state and every decision
//  7. Project Identity — full CLAUDE.md
//  8. Changelog + Version
func (l *Loom) WeaveScope(scope ScopeConfig) (string, error) {
	canon, err := l.LoadCanon(scope.RepoPath)
	if err != nil {
		return "", fmt.Errorf("load canon for %s: %w", scope.Name, err)
	}

	var b strings.Builder

	// ── 1. Ra Autonomy Directive ────────────────────────────────────────
	b.WriteString(fmt.Sprintf("# Scope: %s\n", scope.DisplayName))
	b.WriteString(fmt.Sprintf("Deadline: %s | Priority: %s\n\n", scope.Deadline, scope.Priority))

	b.WriteString("## Ra Autonomy Directive\n")
	b.WriteString("You are a Ra-deployed autonomous agent. This scope was pre-approved by the user.\n")
	b.WriteString("**Override Rule 14 (Sprint Planning is Mandatory)**: The canon below IS the approved plan.\n")
	b.WriteString("Do NOT present a plan and ask for approval. Do NOT ask clarifying questions.\n")
	b.WriteString("Execute as much work as you can this sprint. If a task is blocked, skip it and continue.\n\n")
	b.WriteString("**Sprint loop**: You are running in a multi-sprint session. Between sprints, the governance\n")
	b.WriteString("loop runs automatically: Thoth compacts your memory, Ma'at checks build/tests, and you\n")
	b.WriteString("receive a status report as your next prompt. Do NOT repeat work from previous sprints.\n")
	b.WriteString("Each sprint should advance the canon's plan further. Commit and push at the end of EACH sprint.\n\n")

	// ── 2. Scope of Work ────────────────────────────────────────────────
	if strings.TrimSpace(scope.ScopeOfWork) != "" {
		b.WriteString("## Your Scope of Work\n")
		b.WriteString(scope.ScopeOfWork)
		b.WriteString("\n\n")
	} else {
		b.WriteString("## Your Scope of Work (Dynamic — Derived from Canon)\n")
		b.WriteString("No static scope was provided. Determine your scope from the full canon below.\n\n")
		b.WriteString("**Instructions:**\n")
		b.WriteString("1. Read the Continuation Prompt — it describes the current state and next phases.\n")
		b.WriteString("2. Read the Planning Docs — they contain blueprints, dev plans, and specs.\n")
		b.WriteString("3. Read the ADRs — they define architectural decisions and what should be built.\n")
		b.WriteString("4. Check `git log --oneline -30` to see what was recently completed.\n")
		b.WriteString("5. Identify the NEXT incomplete phase/sprint/milestone from the plans.\n")
		b.WriteString("6. Execute it. Build working code. Do not just verify or audit.\n")
		b.WriteString("7. When done, update docs/CONTINUATION-PROMPT.md with the new state.\n")
		b.WriteString("8. Commit, push, and run `pantheon thoth compact`.\n\n")
	}

	b.WriteString("---\n\n")

	// ── 3. Continuation Prompt ──────────────────────────────────────────
	if canon.ContinuationPrompt != "" {
		b.WriteString("## Continuation Prompt (Current State & Next Phases)\n")
		b.WriteString(canon.ContinuationPrompt)
		b.WriteString("\n\n")
	}

	// ── 4. Planning Docs ────────────────────────────────────────────────
	if len(canon.PlanningDocs) > 0 {
		b.WriteString("## Planning Documents\n\n")
		for _, doc := range canon.PlanningDocs {
			b.WriteString(fmt.Sprintf("### %s\n", doc.Name))
			b.WriteString(doc.Content)
			b.WriteString("\n\n")
		}
	}

	// ── 5. Architecture Decision Records ────────────────────────────────
	if len(canon.ADRs) > 0 {
		b.WriteString("## Architecture Decision Records\n\n")
		for _, adr := range canon.ADRs {
			b.WriteString(fmt.Sprintf("### %s\n", adr.Name))
			b.WriteString(adr.Content)
			b.WriteString("\n\n")
		}
	}

	// ── 6. Thoth Memory + Journal ──────────────────────────────────────
	if canon.ThothMemory != "" {
		b.WriteString("## Project State (Thoth Memory)\n")
		b.WriteString(canon.ThothMemory)
		b.WriteString("\n\n")
	}

	if canon.ThothJournal != "" {
		b.WriteString("## Engineering Journal (Thoth)\n")
		b.WriteString(canon.ThothJournal)
		b.WriteString("\n\n")
	}

	// ── 7. Project Identity ────────────────────────────────────────────
	if canon.ClaudeMD != "" {
		b.WriteString("## Project Identity (CLAUDE.md)\n")
		b.WriteString(canon.ClaudeMD)
		b.WriteString("\n\n")
	}

	// ── 8. Changelog + Version ─────────────────────────────────────────
	if canon.Version != "" {
		b.WriteString("## Current Version: ")
		b.WriteString(canon.Version)
		b.WriteString("\n\n")
	}

	if canon.Changelog != "" {
		b.WriteString("## Changelog\n")
		b.WriteString(canon.Changelog)
		b.WriteString("\n\n")
	}

	stele.Inscribe("neith", stele.TypeNeithWeave, scope.Name, map[string]string{
		"scope": scope.DisplayName,
		"chars": fmt.Sprintf("%d", b.Len()),
	})
	return b.String(), nil
}

// WritePrompt writes the assembled prompt to ~/.config/ra/scopes/<name>-prompt.md.
// Returns the file path written.
func (l *Loom) WritePrompt(name, content string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home dir: %w", err)
	}

	dir := filepath.Join(home, ".config", "ra", "scopes")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create scopes dir: %w", err)
	}

	path := filepath.Join(dir, name+"-prompt.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("write prompt %s: %w", path, err)
	}

	return path, nil
}

// EvaluateDrift analyzes a git diff to detect scope drift for the given scope.
func (l *Loom) EvaluateDrift(scope ScopeConfig, gitDiff string) (*DriftReport, error) {
	report := &DriftReport{
		ScopeName: scope.Name,
	}

	if gitDiff == "" {
		return report, nil
	}

	// Extract modified file paths from the diff
	modifiedFiles := extractDiffFiles(gitDiff)

	// Extract scope keywords from scope_of_work for directory matching
	scopeKeywords := extractScopeKeywords(scope.ScopeOfWork)

	// Check for files modified outside expected directories
	for _, file := range modifiedFiles {
		if !fileMatchesScopeKeywords(file, scopeKeywords) {
			report.Findings = append(report.Findings,
				fmt.Sprintf("Modified file outside scope: %s", file))
		}
	}

	// Check for new dependencies (go.mod / package.json changes)
	lines := strings.Split(gitDiff, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			trimmed := strings.TrimPrefix(line, "+")
			trimmed = strings.TrimSpace(trimmed)
			// go.mod dependency additions
			if strings.Contains(gitDiff, "go.mod") && strings.HasPrefix(trimmed, "require") {
				report.Findings = append(report.Findings,
					fmt.Sprintf("New Go dependency added: %s", trimmed))
			}
			// package.json dependency additions
			if strings.Contains(gitDiff, "package.json") &&
				(strings.Contains(trimmed, "\"dependencies\"") || strings.Contains(trimmed, "\"devDependencies\"")) {
				report.Findings = append(report.Findings,
					fmt.Sprintf("New npm dependency section modified: %s", trimmed))
			}
		}
	}

	report.DriftFound = len(report.Findings) > 0

	stele.Inscribe("neith", stele.TypeNeithDrift, scope.Name, map[string]string{
		"drift":    fmt.Sprintf("%v", report.DriftFound),
		"findings": fmt.Sprintf("%d", len(report.Findings)),
	})
	return report, nil
}

// ── helpers ──────────────────────────────────────────────────────────

// lastNJournalEntries returns the last n entries from a journal, split by "---" separators.
func lastNJournalEntries(content string, n int) string {
	// Split by --- separators or ## date headers
	var entries []string

	// Try --- separator first
	parts := strings.Split(content, "\n---\n")
	if len(parts) > 1 {
		entries = parts
	} else {
		// Try splitting by ## headers
		lines := strings.Split(content, "\n")
		var current strings.Builder
		for _, line := range lines {
			if strings.HasPrefix(line, "## ") && current.Len() > 0 {
				entries = append(entries, current.String())
				current.Reset()
			}
			current.WriteString(line)
			current.WriteString("\n")
		}
		if current.Len() > 0 {
			entries = append(entries, current.String())
		}
	}

	if len(entries) <= n {
		return content
	}
	return strings.Join(entries[len(entries)-n:], "\n---\n")
}

// readFirstNLines reads the first n lines of a file and returns them joined.
func readFirstNLines(path string, n int) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	lines := strings.SplitN(string(data), "\n", n+1)
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}

// readFirstParagraph reads the first non-empty paragraph from a file.
func readFirstParagraph(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	content := strings.TrimSpace(string(data))
	// Split on double newlines to get paragraphs
	paragraphs := strings.SplitN(content, "\n\n", 3)
	// Skip a heading-only first paragraph
	for _, p := range paragraphs {
		trimmed := strings.TrimSpace(p)
		if trimmed == "" {
			continue
		}
		// If it's just a heading line, skip to next
		lines := strings.Split(trimmed, "\n")
		if len(lines) == 1 && strings.HasPrefix(lines[0], "#") {
			continue
		}
		return trimmed
	}
	if len(paragraphs) > 0 {
		return strings.TrimSpace(paragraphs[0])
	}
	return ""
}

// firstNChangelogSections returns the first n version sections from a CHANGELOG.
func firstNChangelogSections(content string, n int) string {
	lines := strings.Split(content, "\n")
	var sections []string
	var current strings.Builder
	count := 0

	for _, line := range lines {
		// Version headers typically start with ## [, ## v, or # [
		if (strings.HasPrefix(line, "## ") || strings.HasPrefix(line, "# ")) &&
			(strings.Contains(line, "[") || strings.Contains(line, "v") || strings.Contains(line, "V")) {
			if current.Len() > 0 {
				sections = append(sections, current.String())
				count++
				if count >= n {
					break
				}
				current.Reset()
			}
		}
		current.WriteString(line)
		current.WriteString("\n")
	}
	if current.Len() > 0 && count < n {
		sections = append(sections, current.String())
	}

	return strings.Join(sections, "")
}

// extractDiffFiles pulls file paths from git diff output (--- a/ and +++ b/ lines).
func extractDiffFiles(diff string) []string {
	var files []string
	seen := make(map[string]bool)
	for _, line := range strings.Split(diff, "\n") {
		if strings.HasPrefix(line, "+++ b/") {
			file := strings.TrimPrefix(line, "+++ b/")
			if !seen[file] {
				seen[file] = true
				files = append(files, file)
			}
		} else if strings.HasPrefix(line, "--- a/") {
			file := strings.TrimPrefix(line, "--- a/")
			if !seen[file] {
				seen[file] = true
				files = append(files, file)
			}
		}
	}
	return files
}

// extractScopeKeywords pulls meaningful directory/file keywords from the scope of work text.
func extractScopeKeywords(scopeOfWork string) []string {
	var keywords []string
	// Look for path-like tokens and meaningful words
	for _, word := range strings.Fields(scopeOfWork) {
		word = strings.Trim(word, ".,;:()\"'`")
		word = strings.ToLower(word)
		// Include path-like tokens (contain / or .)
		if strings.Contains(word, "/") || strings.Contains(word, ".") {
			keywords = append(keywords, word)
			continue
		}
		// Include common directory/technology keywords
		techKeywords := map[string]bool{
			"src": true, "web": true, "api": true, "cmd": true, "internal": true,
			"components": true, "pages": true, "firebase": true, "firestore": true,
			"go": true, "npm": true, "docs": true, "configs": true, "tests": true,
		}
		if techKeywords[word] {
			keywords = append(keywords, word)
		}
	}
	return keywords
}

// fileMatchesScopeKeywords checks if a file path relates to any scope keyword.
func fileMatchesScopeKeywords(filePath string, keywords []string) bool {
	if len(keywords) == 0 {
		return true // No keywords means we can't evaluate
	}
	lower := strings.ToLower(filePath)
	for _, kw := range keywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}
