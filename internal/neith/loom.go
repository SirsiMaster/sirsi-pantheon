package neith

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// ScopeConfig defines a single scope of work for a target repository.
type ScopeConfig struct {
	Name        string `yaml:"name"`
	DisplayName string `yaml:"display_name"`
	RepoPath    string `yaml:"repo_path"`
	Deadline    string `yaml:"deadline"`
	Priority    string `yaml:"priority"`
	ScopeOfWork string `yaml:"scope_of_work"`
	MaxTurns    int    `yaml:"max_turns"`
}

// CanonContext holds all canon documents loaded from a target repo.
type CanonContext struct {
	ClaudeMD           string
	ThothMemory        string
	ThothJournal       string // last 5 entries only
	ContinuationPrompt string
	ADRSummaries       []string // title + decision line per ADR
	BlueprintSummaries []string
	ChangelogRecent    string // last 2 entries
	Version            string
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

// LoadCanon reads canon documents from the target repository at repoPath.
func (l *Loom) LoadCanon(repoPath string) (*CanonContext, error) {
	root := expandHome(repoPath)
	ctx := &CanonContext{}

	// CLAUDE.md or GEMINI.md
	claudeData, err := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	if err != nil {
		geminiData, err2 := os.ReadFile(filepath.Join(root, "GEMINI.md"))
		if err2 == nil {
			ctx.ClaudeMD = string(geminiData)
		}
	} else {
		ctx.ClaudeMD = string(claudeData)
	}

	// .thoth/memory.yaml
	if data, err := os.ReadFile(filepath.Join(root, ".thoth", "memory.yaml")); err == nil {
		ctx.ThothMemory = string(data)
	}

	// .thoth/journal.md — last 5 entries
	if data, err := os.ReadFile(filepath.Join(root, ".thoth", "journal.md")); err == nil {
		ctx.ThothJournal = lastNJournalEntries(string(data), 5)
	}

	// docs/CONTINUATION-PROMPT.md
	if data, err := os.ReadFile(filepath.Join(root, "docs", "CONTINUATION-PROMPT.md")); err == nil {
		ctx.ContinuationPrompt = string(data)
	}

	// ADR summaries — first 3 lines of each ADR-*.md
	adrPattern := filepath.Join(root, "docs", "ADR-*.md")
	if adrFiles, err := filepath.Glob(adrPattern); err == nil {
		for _, f := range adrFiles {
			if summary := readFirstNLines(f, 3); summary != "" {
				ctx.ADRSummaries = append(ctx.ADRSummaries, summary)
			}
		}
	}

	// Blueprint/Plan/Scope summaries — first paragraph
	for _, pattern := range []string{
		filepath.Join(root, "docs", "*BLUEPRINT*.md"),
		filepath.Join(root, "docs", "*PLAN*.md"),
		filepath.Join(root, "docs", "*SCOPE*.md"),
	} {
		if files, err := filepath.Glob(pattern); err == nil {
			for _, f := range files {
				if para := readFirstParagraph(f); para != "" {
					ctx.BlueprintSummaries = append(ctx.BlueprintSummaries, para)
				}
			}
		}
	}

	// CHANGELOG.md — first 2 version sections
	if data, err := os.ReadFile(filepath.Join(root, "CHANGELOG.md")); err == nil {
		ctx.ChangelogRecent = firstNChangelogSections(string(data), 2)
	}

	// VERSION file
	if data, err := os.ReadFile(filepath.Join(root, "VERSION")); err == nil {
		ctx.Version = strings.TrimSpace(string(data))
	}

	return ctx, nil
}

// WeaveScope assembles the final scope prompt for the given scope config.
func (l *Loom) WeaveScope(scope ScopeConfig) (string, error) {
	canon, err := l.LoadCanon(scope.RepoPath)
	if err != nil {
		return "", fmt.Errorf("load canon for %s: %w", scope.Name, err)
	}

	// Build the prompt in two parts: the critical directive+scope goes first
	// (never truncated), then canon context fills remaining token budget.

	// Part 1: Directive and scope (must never be truncated)
	var directive strings.Builder

	directive.WriteString(fmt.Sprintf("# Scope: %s\n", scope.DisplayName))
	directive.WriteString(fmt.Sprintf("Deadline: %s | Priority: %s\n\n", scope.Deadline, scope.Priority))

	// Ra Autonomy Directive — overrides sprint-plan-approval gates in CLAUDE.md.
	// Without this, agents block waiting for human approval that can never arrive
	// in non-interactive --print mode.
	directive.WriteString("## Ra Autonomy Directive\n")
	directive.WriteString("You are a Ra-deployed autonomous agent. This scope was pre-approved by the user.\n")
	directive.WriteString("**Override Rule 14 (Sprint Planning is Mandatory)**: The scope below IS the approved sprint plan.\n")
	directive.WriteString("Do NOT present a plan and ask for approval. Do NOT ask clarifying questions.\n")
	directive.WriteString("Execute the numbered tasks in order. If a task is ambiguous, use your best judgment.\n")
	directive.WriteString("If a task is blocked by a genuine technical issue (missing dependency, broken build),\n")
	directive.WriteString("skip it, log the reason, and continue with the next task.\n\n")

	directive.WriteString("## Your Scope of Work\n")
	directive.WriteString(scope.ScopeOfWork)
	directive.WriteString("\n\nBegin by reading CLAUDE.md in this repo, then assess current state and execute the scope above.\n")
	directive.WriteString("\n---\n\n")

	// Part 2: Canon context (truncated to fit budget)
	var context strings.Builder

	// Project Identity
	context.WriteString("## Project Identity\n")
	claudeContent := canon.ClaudeMD
	if len(claudeContent) > 2000 {
		claudeContent = claudeContent[:2000] + "\n...(truncated)"
	}
	context.WriteString(claudeContent)
	context.WriteString("\n\n")

	// Project State
	if canon.ThothMemory != "" {
		context.WriteString("## Project State (Thoth Memory)\n")
		context.WriteString(canon.ThothMemory)
		context.WriteString("\n\n")
	}

	// Recent Decisions
	if canon.ThothJournal != "" {
		context.WriteString("## Recent Decisions\n")
		context.WriteString(canon.ThothJournal)
		context.WriteString("\n\n")
	}

	// Continuation Context
	if canon.ContinuationPrompt != "" {
		context.WriteString("## Continuation Context\n")
		context.WriteString(canon.ContinuationPrompt)
		context.WriteString("\n\n")
	}

	// Architecture Decisions
	if len(canon.ADRSummaries) > 0 {
		context.WriteString("## Architecture Decisions\n")
		for _, adr := range canon.ADRSummaries {
			context.WriteString(adr)
			context.WriteString("\n\n")
		}
	}

	// Current Version
	if canon.Version != "" {
		context.WriteString("## Current Version\n")
		context.WriteString(canon.Version)
		if canon.ChangelogRecent != "" {
			context.WriteString(" — ")
			context.WriteString(canon.ChangelogRecent)
		}
		context.WriteString("\n\n")
	}

	// Token budget: ~8000 tokens ≈ 32K chars.
	// Directive is sacred; truncate only the canon context if over budget.
	const maxChars = 32000
	directiveStr := directive.String()
	contextStr := context.String()

	remaining := maxChars - len(directiveStr)
	if remaining < 0 {
		remaining = 0
	}
	if len(contextStr) > remaining {
		contextStr = contextStr[:remaining] + "\n\n...(canon context truncated to fit token budget)"
	}

	return directiveStr + contextStr, nil
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
