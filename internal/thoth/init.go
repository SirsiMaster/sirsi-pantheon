package thoth

import (
	"bufio"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

//go:embed templates/memory.yaml templates/journal.md templates/session-start.md
var templateFS embed.FS

// InitOptions configures thoth init behavior.
type InitOptions struct {
	RepoRoot string
	Name     string
	Language string
	Version  string
	Yes      bool // non-interactive mode
}

// ProjectInfo holds auto-detected project metadata.
type ProjectInfo struct {
	Name     string
	Language string
	Version  string
}

// IDEFile describes an IDE rules file that Thoth can inject into.
type IDEFile struct {
	Path string
	Name string
	Dir  string // parent dir to create if needed
}

// Init scaffolds the .thoth/ knowledge system in a project directory.
// It creates memory.yaml, journal.md, artifacts/, and injects IDE rules.
func Init(opts InitOptions) error {
	root := opts.RepoRoot
	if root == "" {
		root = "."
	}

	thothDir := filepath.Join(root, ".thoth")
	artifactsDir := filepath.Join(thothDir, "artifacts")
	memoryPath := filepath.Join(thothDir, "memory.yaml")

	// Check for existing memory
	if _, err := os.Stat(memoryPath); err == nil && !opts.Yes {
		return fmt.Errorf(".thoth/memory.yaml already exists (use --yes to overwrite)")
	}

	// Auto-detect project
	info := DetectProject(root)
	lineCount := CountSourceLines(root)
	topDirs := ScanArchitecture(root)

	// Apply overrides from flags
	name := info.Name
	if opts.Name != "" {
		name = opts.Name
	}
	lang := info.Language
	if opts.Language != "" {
		lang = opts.Language
	}
	version := info.Version
	if opts.Version != "" {
		version = opts.Version
	}

	// Create directory structure
	if err := os.MkdirAll(artifactsDir, 0o755); err != nil {
		return fmt.Errorf("thoth init: create dirs: %w", err)
	}

	now := time.Now()
	dateStr := now.Format("2006-01-02")
	isoStr := now.Format(time.RFC3339)

	// Write memory.yaml
	var mem strings.Builder
	mem.WriteString("# 𓁟 Thoth — Project Memory\n")
	mem.WriteString("# Read this FIRST before any source files.\n")
	mem.WriteString(fmt.Sprintf("# Last updated: %s\n\n", isoStr))
	mem.WriteString("## Identity\n")
	mem.WriteString(fmt.Sprintf("project: %s\n", name))
	mem.WriteString(fmt.Sprintf("language: %s\n", lang))
	mem.WriteString(fmt.Sprintf("version: %s\n\n", version))
	mem.WriteString("## Stats\n")
	mem.WriteString(fmt.Sprintf("line_count: ~%s\n\n", formatNumber(lineCount)))
	mem.WriteString("## Architecture Quick Reference\n")
	for _, dir := range topDirs {
		mem.WriteString(fmt.Sprintf("# %s/  — TODO: describe\n", dir))
	}
	mem.WriteString("\n## Critical Design Decisions\n")
	mem.WriteString("# TODO: Add your key technical choices and rationale\n\n")
	mem.WriteString("## Known Limitations\n")
	mem.WriteString("# TODO: What doesn't work, what's incomplete\n\n")
	mem.WriteString("## Recent Changes\n")
	mem.WriteString(fmt.Sprintf("# %s: Thoth initialized\n\n", dateStr))
	mem.WriteString("## File Map\n")
	mem.WriteString("# .thoth/memory.yaml  — THIS FILE\n")
	mem.WriteString("# .thoth/journal.md   — engineering journal\n")

	if err := os.WriteFile(memoryPath, []byte(mem.String()), 0o644); err != nil {
		return fmt.Errorf("thoth init: write memory: %w", err)
	}

	// Write journal.md
	var journal strings.Builder
	journal.WriteString("# 𓁟 Engineering Journal\n")
	journal.WriteString("# Timestamped reasoning — the WHY behind every decision.\n\n")
	journal.WriteString("---\n\n")
	journal.WriteString(fmt.Sprintf("## Entry 001 — %s — Thoth Initialized\n\n", dateStr))
	journal.WriteString(fmt.Sprintf("**Context**: Thoth knowledge system initialized for %s.\n\n", name))
	journal.WriteString(fmt.Sprintf("**Decision**: Three-layer knowledge system (memory → journal → artifacts) adopted to give AI assistants persistent context across sessions. ~%s source lines compressed to ~100 lines of structured YAML.\n\n", formatNumber(lineCount)))
	journal.WriteString("---\n")

	journalPath := filepath.Join(thothDir, "journal.md")
	if err := os.WriteFile(journalPath, []byte(journal.String()), 0o644); err != nil {
		return fmt.Errorf("thoth init: write journal: %w", err)
	}

	// Write artifacts README
	readmePath := filepath.Join(artifactsDir, "README.md")
	if err := os.WriteFile(readmePath, []byte("# Thoth Artifacts\nBenchmarks, audits, design docs, and deep analysis go here.\n"), 0o644); err != nil {
		return fmt.Errorf("thoth init: write artifacts readme: %w", err)
	}

	// Inject IDE rules
	injected := InjectIDERules(root)

	// Create session workflow
	workflowCreated := createSessionWorkflow(root)

	// Print summary
	fmt.Print("\n  𓁟 Thoth — Persistent Knowledge for AI-Assisted Development\n\n")
	fmt.Printf("  Detected: %s project \"%s\" v%s\n", lang, name, version)
	fmt.Printf("  Source: ~%s lines\n\n", formatNumber(lineCount))
	fmt.Println("  ✓ Created .thoth/memory.yaml")
	fmt.Println("  ✓ Created .thoth/journal.md")
	fmt.Println("  ✓ Created .thoth/artifacts/")

	if len(injected) > 0 || workflowCreated {
		fmt.Println("\n  IDE integrations:")
		for _, ide := range injected {
			fmt.Printf("  ✓ %s\n", ide)
		}
		if workflowCreated {
			fmt.Println("  ✓ Claude Code workflow (created)")
		}
	}

	tokensSaved := 0
	if lineCount > 100 {
		tokensSaved = int((1.0 - 100.0/float64(lineCount)) * 100)
	}
	fmt.Printf("\n  𓁟 Context reduction: ~%d%%\n", tokensSaved)
	fmt.Printf("     %s source lines → ~100 Thoth lines\n", formatNumber(lineCount))
	fmt.Printf("\n  Next: Populate memory.yaml with your architecture and decisions.\n\n")

	return nil
}

// DetectProject auto-detects project name, language, and version from common config files.
func DetectProject(root string) ProjectInfo {
	info := ProjectInfo{
		Name:     filepath.Base(root),
		Language: "unknown",
		Version:  "0.1.0",
	}

	absRoot, err := filepath.Abs(root)
	if err == nil {
		info.Name = filepath.Base(absRoot)
	}

	// package.json
	if data, err := os.ReadFile(filepath.Join(root, "package.json")); err == nil {
		// Simple JSON extraction without a dependency
		if n := extractJSON(string(data), "name"); n != "" {
			info.Name = n
		}
		if v := extractJSON(string(data), "version"); v != "" {
			info.Version = v
		}
		info.Language = "TypeScript/JavaScript"
		if fileExists(root, "next.config.js") || fileExists(root, "next.config.ts") || fileExists(root, "next.config.mjs") {
			info.Language = "TypeScript (Next.js)"
		} else if fileExists(root, "vite.config.ts") || fileExists(root, "vite.config.js") {
			info.Language = "TypeScript (Vite)"
		}
	}

	// go.mod (takes precedence over package.json)
	if data, err := os.ReadFile(filepath.Join(root, "go.mod")); err == nil {
		info.Language = "Go"
		re := regexp.MustCompile(`^module\s+(.+)`)
		if m := re.FindStringSubmatch(string(data)); len(m) > 1 {
			parts := strings.Split(m[1], "/")
			info.Name = parts[len(parts)-1]
		}
	}

	// Cargo.toml
	if data, err := os.ReadFile(filepath.Join(root, "Cargo.toml")); err == nil {
		info.Language = "Rust"
		re := regexp.MustCompile(`name\s*=\s*"(.+?)"`)
		if m := re.FindStringSubmatch(string(data)); len(m) > 1 {
			info.Name = m[1]
		}
	}

	// pyproject.toml or setup.py
	if fileExists(root, "pyproject.toml") || fileExists(root, "setup.py") {
		info.Language = "Python"
	}

	return info
}

// CountSourceLines counts actual lines in source files, walking up to 8 levels deep.
func CountSourceLines(root string) int {
	extensions := map[string]bool{
		".go": true, ".ts": true, ".tsx": true, ".js": true, ".jsx": true,
		".py": true, ".rs": true, ".java": true, ".swift": true, ".kt": true,
	}
	excludeDirs := map[string]bool{
		"node_modules": true, ".next": true, "dist": true, "build": true,
		"vendor": true, "__pycache__": true, ".git": true, ".thoth": true,
	}

	total := 0
	walkInit(root, 0, 8, extensions, excludeDirs, &total)
	return total
}

func walkInit(dir string, depth, maxDepth int, exts, excludes map[string]bool, total *int) {
	if depth > maxDepth {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		if excludes[name] {
			continue
		}
		full := filepath.Join(dir, name)
		if entry.IsDir() {
			walkInit(full, depth+1, maxDepth, exts, excludes, total)
		} else if exts[filepath.Ext(name)] {
			if data, err := os.ReadFile(full); err == nil {
				*total += len(strings.Split(string(data), "\n"))
			}
		}
	}
}

// ScanArchitecture returns top-level non-hidden, non-excluded directory names.
func ScanArchitecture(root string) []string {
	excludes := map[string]bool{
		"node_modules": true, "dist": true, "build": true, ".next": true,
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var dirs []string
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") && !excludes[e.Name()] {
			dirs = append(dirs, e.Name())
		}
	}
	return dirs
}

// InjectIDERules appends Thoth read instructions to IDE rules files.
// Returns a list of what was injected (e.g. "Claude Code (appended)").
func InjectIDERules(root string) []string {
	thothRule := `
# 𓁟 Thoth — Persistent Knowledge System
# At the START of every conversation, read .thoth/memory.yaml before reading any source files.
# This file is the project's compressed state (~100 lines) and replaces reading thousands of lines of code.
# When reasoning matters, also read .thoth/journal.md for decision history.
# After making significant changes, update both files.
`

	ideFiles := []IDEFile{
		{Path: ".cursorrules", Name: "Cursor"},
		{Path: ".windsurfrules", Name: "Windsurf"},
		{Path: filepath.Join(".gemini", "style.md"), Name: "Gemini", Dir: ".gemini"},
		{Path: "CLAUDE.md", Name: "Claude Code"},
		{Path: filepath.Join(".github", "copilot-instructions.md"), Name: "Copilot", Dir: ".github"},
	}

	var injected []string

	for _, ide := range ideFiles {
		fullPath := filepath.Join(root, ide.Path)

		if data, err := os.ReadFile(fullPath); err == nil {
			// File exists — check if already injected
			if strings.Contains(string(data), ".thoth/memory.yaml") {
				continue
			}
			// Append to existing file
			f, err := os.OpenFile(fullPath, os.O_APPEND|os.O_WRONLY, 0o644)
			if err != nil {
				continue
			}
			_, _ = f.WriteString("\n" + thothRule)
			f.Close()
			injected = append(injected, ide.Name+" (appended)")
		} else {
			// Create new rules file
			if ide.Dir != "" {
				os.MkdirAll(filepath.Join(root, ide.Dir), 0o755)
			}
			os.WriteFile(fullPath, []byte(strings.TrimSpace(thothRule)+"\n"), 0o644)
			injected = append(injected, ide.Name+" (created)")
		}
	}

	return injected
}

// createSessionWorkflow creates the .agent/workflows/session-start.md file.
func createSessionWorkflow(root string) bool {
	workflowDir := filepath.Join(root, ".agent", "workflows")
	workflowPath := filepath.Join(workflowDir, "session-start.md")

	if _, err := os.Stat(workflowPath); err == nil {
		return false // already exists
	}

	data, err := templateFS.ReadFile("templates/session-start.md")
	if err != nil {
		return false
	}

	os.MkdirAll(workflowDir, 0o755)
	if err := os.WriteFile(workflowPath, data, 0o644); err != nil {
		return false
	}
	return true
}

// extractJSON is a simple key extractor for JSON strings (avoids encoding/json import for trivial use).
func extractJSON(data, key string) string {
	re := regexp.MustCompile(fmt.Sprintf(`"%s"\s*:\s*"([^"]*)"`, regexp.QuoteMeta(key)))
	if m := re.FindStringSubmatch(data); len(m) > 1 {
		return m[1]
	}
	return ""
}

// fileExists checks if a file exists at root/name.
func fileExists(root, name string) bool {
	_, err := os.Stat(filepath.Join(root, name))
	return err == nil
}

// InteractiveInit runs the init flow with user prompts on stdin.
func InteractiveInit(root string) error {
	scanner := bufio.NewScanner(os.Stdin)

	info := DetectProject(root)
	lineCount := CountSourceLines(root)

	fmt.Print("\n  𓁟 Thoth — Persistent Knowledge for AI-Assisted Development\n\n")
	fmt.Printf("  Detected: %s project \"%s\" v%s\n", info.Language, info.Name, info.Version)
	fmt.Printf("  Source: ~%s lines\n\n", formatNumber(lineCount))

	// Check for existing memory
	memoryPath := filepath.Join(root, ".thoth", "memory.yaml")
	if _, err := os.Stat(memoryPath); err == nil {
		fmt.Print("  ⚠ .thoth/memory.yaml already exists. Overwrite? (y/N): ")
		scanner.Scan()
		if strings.ToLower(strings.TrimSpace(scanner.Text())) != "y" {
			fmt.Println("  Aborted.")
			return nil
		}
	}

	name := prompt(scanner, "  Project name", info.Name)
	lang := prompt(scanner, "  Language", info.Language)
	version := prompt(scanner, "  Version", info.Version)

	return Init(InitOptions{
		RepoRoot: root,
		Name:     name,
		Language: lang,
		Version:  version,
		Yes:      true, // skip the overwrite check since we already asked
	})
}

func prompt(scanner *bufio.Scanner, question, defaultVal string) string {
	fmt.Printf("%s [%s]: ", question, defaultVal)
	scanner.Scan()
	answer := strings.TrimSpace(scanner.Text())
	if answer == "" {
		return defaultVal
	}
	return answer
}
