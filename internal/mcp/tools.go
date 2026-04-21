package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/brain"
	"github.com/SirsiMaster/sirsi-pantheon/internal/horus"
	"github.com/SirsiMaster/sirsi-pantheon/internal/jackal"
	"github.com/SirsiMaster/sirsi-pantheon/internal/jackal/rules"
	"github.com/SirsiMaster/sirsi-pantheon/internal/ka"
	"github.com/SirsiMaster/sirsi-pantheon/internal/notify"
	"github.com/SirsiMaster/sirsi-pantheon/internal/rtk"
	"github.com/SirsiMaster/sirsi-pantheon/internal/stele"
	"github.com/SirsiMaster/sirsi-pantheon/internal/thoth"
	"github.com/SirsiMaster/sirsi-pantheon/internal/vault"
)

// registerTools adds Pantheon tools to the MCP server.
// Only tools that provide real, distinct value are exposed.
func registerTools(s *Server) {
	s.RegisterTool(Tool{
		Name:        "scan_workspace",
		Description: "Scan a directory for infrastructure waste — stale caches, orphaned build artifacts, unused dependencies. Read-only, never deletes anything. Returns findings with sizes.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]SchemaField{
				"path": {
					Type:        "string",
					Description: "Absolute path to scan. Defaults to current working directory.",
				},
				"category": {
					Type:        "string",
					Description: "Filter: general, dev, ai, vms, ides, cloud, storage. Empty for all.",
				},
			},
		},
	}, handleScanWorkspace)

	s.RegisterTool(Tool{
		Name:        "ghost_report",
		Description: "Detect remnants of uninstalled applications — orphaned preferences, caches, launch agents, and Spotlight registrations that waste disk space.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]SchemaField{
				"target": {
					Type:        "string",
					Description: "Optional: filter by app name or bundle ID.",
				},
			},
		},
	}, handleGhostReport)

	s.RegisterTool(Tool{
		Name:        "health_check",
		Description: "Quick system health summary — CPU, RAM, indexed file count, and watchdog status. Uses cached data only, responds in under 10ms.",
		InputSchema: InputSchema{
			Type:       "object",
			Properties: map[string]SchemaField{},
		},
	}, handleHealthCheck)

	s.RegisterTool(Tool{
		Name:        "thoth_read_memory",
		Description: "Read the project's .thoth/memory.yaml for instant context. Call this at conversation start to understand the project without reading source files. Returns architecture, decisions, stats, and file map.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]SchemaField{
				"path": {
					Type:        "string",
					Description: "Project root path. Defaults to current working directory.",
				},
			},
		},
	}, handleThothReadMemory)

	s.RegisterTool(Tool{
		Name:        "thoth_sync",
		Description: "Sync project memory — discovers codebase facts (module count, test count, line count) and appends recent git commits to the engineering journal.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]SchemaField{
				"path": {
					Type:        "string",
					Description: "Project root path. Defaults to current working directory.",
				},
			},
		},
	}, handleThothSync)

	s.RegisterTool(Tool{
		Name:        "detect_hardware",
		Description: "Detect system hardware — CPU model, GPU vendor (Apple Metal, NVIDIA, AMD), neural engine, and available accelerators. Returns JSON hardware profile.",
		InputSchema: InputSchema{
			Type:       "object",
			Properties: map[string]SchemaField{},
		},
	}, handleDetectHardware)

	// ── RTK: Output Filter ──────────────────────────────────────────

	s.RegisterTool(Tool{
		Name:        "filter_output",
		Description: "Apply RTK output filtering (ANSI strip, dedup, truncation) to raw text. Use this to compress large tool outputs before analysis. Returns filtered text with reduction statistics.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]SchemaField{
				"text": {
					Type:        "string",
					Description: "Raw output text to filter.",
				},
				"max_lines": {
					Type:        "number",
					Description: "Truncate after N lines (0 = unlimited).",
				},
			},
			Required: []string{"text"},
		},
	}, handleFilterOutput)

	// ── Vault: Context Sandbox ──────────────────────────────────────

	s.RegisterTool(Tool{
		Name:        "vault_store",
		Description: "Sandbox large output (logs, data, command results) into the context vault instead of consuming context window tokens. Returns an entry ID and token count. Use vault_search to query later.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]SchemaField{
				"content": {
					Type:        "string",
					Description: "The output to sandbox.",
				},
				"source": {
					Type:        "string",
					Description: "What produced this output (e.g. 'npm test', 'git log').",
				},
				"tag": {
					Type:        "string",
					Description: "Category tag for filtering (e.g. 'logs', 'test-output', 'build').",
				},
			},
			Required: []string{"content"},
		},
	}, handleVaultStore)

	s.RegisterTool(Tool{
		Name:        "vault_search",
		Description: "Search the context vault using full-text search (FTS5 BM25). Returns matching snippets without loading full entries into context. Supports AND, OR, NOT, phrase \"like this\", prefix*.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]SchemaField{
				"query": {
					Type:        "string",
					Description: "FTS5 search query.",
				},
				"limit": {
					Type:        "number",
					Description: "Max results (default 10).",
				},
			},
			Required: []string{"query"},
		},
	}, handleVaultSearch)

	s.RegisterTool(Tool{
		Name:        "vault_get",
		Description: "Retrieve a specific vault entry by ID with full content. Use sparingly — loads full content into context.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]SchemaField{
				"id": {
					Type:        "number",
					Description: "Entry ID from vault_store or vault_search.",
				},
			},
			Required: []string{"id"},
		},
	}, handleVaultGet)

	s.RegisterTool(Tool{
		Name:        "vault_stats",
		Description: "Get vault statistics: total entries, bytes, tokens saved, tag breakdown.",
		InputSchema: InputSchema{
			Type:       "object",
			Properties: map[string]SchemaField{},
		},
	}, handleVaultStats)

	// ── Vault: Code Index ───────────────────────────────────────────

	s.RegisterTool(Tool{
		Name:        "code_index",
		Description: "Build or refresh the code search index for a project. Run this once per project, then use code_search for queries. Indexes Go, Python, TypeScript, Rust, and 15+ other languages.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]SchemaField{
				"path": {
					Type:        "string",
					Description: "Project root to index. Defaults to current directory.",
				},
			},
		},
	}, handleCodeIndex)

	s.RegisterTool(Tool{
		Name:        "code_search",
		Description: "Search indexed source code using BM25 full-text ranking. Returns the most relevant code chunks — not full files. 8-40x smaller than reading entire files.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]SchemaField{
				"query": {
					Type:        "string",
					Description: "Natural language or keyword search.",
				},
				"limit": {
					Type:        "number",
					Description: "Max results (default 5).",
				},
			},
			Required: []string{"query"},
		},
	}, handleCodeSearch)

	// ── Horus: Structural Code Graph ────────────────────────────────

	s.RegisterTool(Tool{
		Name:        "code_symbols",
		Description: "Extract structural code symbols (types, functions, methods, interfaces) from a Go project. Returns declarations and signatures — not full source — for token-efficient code review.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]SchemaField{
				"path": {
					Type:        "string",
					Description: "Project root to analyze. Defaults to current directory.",
				},
				"kind": {
					Type:        "string",
					Description: "Filter by kind: type, func, method, interface, struct, const, var.",
				},
				"filter": {
					Type:        "string",
					Description: "Filter by symbol name pattern (glob with *).",
				},
			},
		},
	}, handleCodeSymbols)

	s.RegisterTool(Tool{
		Name:        "code_outline",
		Description: "Get a compact outline of a source file: package, type declarations, function signatures. No function bodies. 8-49x smaller than the full file.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]SchemaField{
				"path": {
					Type:        "string",
					Description: "File path to outline.",
				},
			},
			Required: []string{"path"},
		},
	}, handleCodeOutline)

	s.RegisterTool(Tool{
		Name:        "code_context",
		Description: "Get minimal context for understanding a specific symbol: its declaration, documentation, parent type, and sibling methods.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]SchemaField{
				"path": {
					Type:        "string",
					Description: "Project root to analyze.",
				},
				"symbol": {
					Type:        "string",
					Description: "Symbol name (e.g. 'Server', 'NewPerson', 'handleToolsCall').",
				},
			},
			Required: []string{"symbol"},
		},
	}, handleCodeContext)

	// ── Notify: Notification History ────────────────────────────────

	s.RegisterTool(Tool{
		Name:        "notification_history",
		Description: "Read recent Sirsi operation results — scan findings, guard alerts, deployment outcomes. Returns the last N notifications from the persistent history.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]SchemaField{
				"limit": {
					Type:        "number",
					Description: "Maximum number of results (default 10).",
				},
				"source": {
					Type:        "string",
					Description: "Filter by source deity (anubis, ka, maat, isis, ra).",
				},
			},
		},
	}, handleNotificationHistory)
}

// handleScanWorkspace runs the Jackal scan engine on a workspace.
func handleScanWorkspace(args map[string]interface{}) (*ToolResult, error) {
	// Parse path argument
	scanPath, _ := args["path"].(string)
	if scanPath == "" {
		var err error
		scanPath, err = os.Getwd()
		if err != nil {
			return textResult(fmt.Sprintf("Could not determine working directory: %v", err), true), nil
		}
	}

	// Parse category filter
	categoryStr, _ := args["category"].(string)
	var categories []jackal.Category
	if categoryStr != "" {
		cat, err := parseCategory(categoryStr)
		if err != nil {
			return textResult(fmt.Sprintf("Invalid category %q: %v", categoryStr, err), true), nil
		}
		categories = []jackal.Category{cat}
	}

	// Create engine and run scan with aggressive timeout.
	// MCP callers (AI IDEs) should not wait >5s for context.
	engine := jackal.NewEngine()
	engine.RegisterAll(rules.AllRules()...)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := engine.Scan(ctx, jackal.ScanOptions{
		Categories: categories,
	})
	if err != nil {
		return textResult(fmt.Sprintf("Scan failed: %v", err), true), nil
	}

	// Format results
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("𓁢 Anubis Scan Results for: %s\n\n", scanPath))
	sb.WriteString(fmt.Sprintf("Total waste found: %s\n", jackal.FormatSize(result.TotalSize)))
	sb.WriteString(fmt.Sprintf("Findings: %d across %d rules\n\n", len(result.Findings), result.RulesRan))

	// Category breakdown
	if len(result.ByCategory) > 0 {
		sb.WriteString("Category Breakdown:\n")
		for cat, summary := range result.ByCategory {
			sb.WriteString(fmt.Sprintf("  • %s: %s (%d items)\n",
				string(cat), jackal.FormatSize(summary.TotalSize), summary.Findings))
		}
		sb.WriteString("\n")
	}

	// Top findings (up to 20)
	limit := 20
	if len(result.Findings) < limit {
		limit = len(result.Findings)
	}
	if limit > 0 {
		sb.WriteString(fmt.Sprintf("Top %d Findings:\n", limit))
		for _, f := range result.Findings[:limit] {
			sb.WriteString(fmt.Sprintf("  %s — %s (%s)\n",
				f.Description, shortenHomePath(f.Path), jackal.FormatSize(f.SizeBytes)))
		}
	}

	if len(result.Findings) > limit {
		sb.WriteString(fmt.Sprintf("\n  ... and %d more findings\n", len(result.Findings)-limit))
	}

	sb.WriteString("\nRun 'anubis judge --dry-run' to preview cleanup.")

	return textResult(sb.String(), false), nil
}

// handleGhostReport runs Ka ghost detection.
func handleGhostReport(args map[string]interface{}) (*ToolResult, error) {
	target, _ := args["target"].(string)

	scanner := ka.NewScanner()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ghosts, err := scanner.Scan(ctx, false) // no sudo in MCP mode
	if err != nil {
		return textResult(fmt.Sprintf("Ghost scan failed: %v", err), true), nil
	}

	// Filter by target if specified
	if target != "" {
		var filtered []ka.Ghost
		target = strings.ToLower(target)
		for _, g := range ghosts {
			if strings.Contains(strings.ToLower(g.AppName), target) ||
				strings.Contains(strings.ToLower(g.BundleID), target) {
				filtered = append(filtered, g)
			}
		}
		ghosts = filtered
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("⚠️ Ka Ghost Report — %d ghosts detected\n\n", len(ghosts)))

	if len(ghosts) == 0 {
		sb.WriteString("No ghost apps found. Machine is spiritually clean.")
		return textResult(sb.String(), false), nil
	}

	var totalSize int64
	var totalResiduals int

	for i, ghost := range ghosts {
		totalSize += ghost.TotalSize
		totalResiduals += len(ghost.Residuals)

		spotlightTag := ""
		if ghost.InLaunchServices {
			spotlightTag = " 👻 (in Spotlight)"
		}

		sizeStr := ""
		if ghost.TotalSize > 0 {
			sizeStr = fmt.Sprintf(" — %s", jackal.FormatSize(ghost.TotalSize))
		}

		sb.WriteString(fmt.Sprintf("[%d] %s%s%s\n", i+1, ghost.AppName, sizeStr, spotlightTag))
		sb.WriteString(fmt.Sprintf("    Bundle: %s\n", ghost.BundleID))

		for _, r := range ghost.Residuals {
			sb.WriteString(fmt.Sprintf("    ├─ %s %s %s\n",
				string(r.Type), jackal.FormatSize(r.SizeBytes), shortenHomePath(r.Path)))
		}
		sb.WriteString("\n")

		// Limit output for MCP (avoid huge context)
		if i >= 29 {
			sb.WriteString(fmt.Sprintf("  ... and %d more ghosts\n\n", len(ghosts)-30))
			break
		}
	}

	sb.WriteString(fmt.Sprintf("Summary: %d ghosts, %d residuals, %s total waste\n",
		len(ghosts), totalResiduals, jackal.FormatSize(totalSize)))
	sb.WriteString("Run 'anubis ka --clean --dry-run' to preview cleanup.")

	return textResult(sb.String(), false), nil
}

// handleHealthCheck provides an instant system health summary.
// PERFORMANCE: Uses cached Horus index + static system info. No live scans.
// Target: <10ms response time (was 17s with live Jackal+Ka scans).
func handleHealthCheck(_ map[string]interface{}) (*ToolResult, error) {
	start := time.Now()
	var sb strings.Builder
	sb.WriteString("𓁢 Anubis Health Check\n\n")

	// System info — instant (runtime constants)
	sb.WriteString(fmt.Sprintf("Platform: %s/%s\n", runtime.GOOS, runtime.GOARCH))
	sb.WriteString(fmt.Sprintf("CPUs: %d\n", runtime.NumCPU()))
	sb.WriteString(fmt.Sprintf("GOMAXPROCS: %d\n", runtime.GOMAXPROCS(0)))

	// Scan index status — check if cached index exists.
	indexPath := filepath.Join(os.Getenv("HOME"), ".config", "sirsi", "index.gob")
	if info, err := os.Stat(indexPath); err == nil {
		sb.WriteString(fmt.Sprintf("Index age: %s\n",
			time.Since(info.ModTime()).Truncate(time.Second)))
	} else {
		sb.WriteString("Scan index: not cached (run 'sirsi scan' to build)\n")
	}

	// Brain status — instant (file existence check)
	if brain.IsInstalled() {
		sb.WriteString("Neural brain: ✅ Installed\n")
	} else {
		sb.WriteString("Neural brain: Not installed (run 'sirsi install-brain')\n")
	}

	// Watchdog status — instant (ring buffer read)
	bridge := GetWatchdogBridge()
	if bridge != nil {
		buffered, lifetime := bridge.Ring().Stats()
		sb.WriteString(fmt.Sprintf("Watchdog: active (%d alerts, %d lifetime)\n", buffered, lifetime))
	} else {
		sb.WriteString("Watchdog: dormant\n")
	}

	sb.WriteString(fmt.Sprintf("\nResponse time: %s\n", time.Since(start).Round(time.Microsecond)))
	sb.WriteString("\nFor full scan: 'sirsi weigh' or call scan_workspace tool.")

	return textResult(sb.String(), false), nil
}

// handleThothReadMemory reads the project's Thoth memory and journal files.
// This gives AI assistants instant project context without reading source files.
func handleThothReadMemory(args map[string]interface{}) (*ToolResult, error) {
	projectPath, _ := args["path"].(string)
	if projectPath == "" {
		var err error
		projectPath, err = os.Getwd()
		if err != nil {
			return textResult("Could not determine working directory", true), nil
		}
	}

	var sb strings.Builder
	sb.WriteString("𓁟 Thoth Memory\n\n")

	// Try .thoth/memory.yaml first, then legacy .anubis-memory.yaml
	memoryPaths := []string{
		projectPath + "/.thoth/memory.yaml",
		projectPath + "/.anubis-memory.yaml",
	}

	found := false
	for _, mp := range memoryPaths {
		data, err := os.ReadFile(mp)
		if err == nil {
			sb.WriteString("=== Memory ===\n")
			sb.WriteString(string(data))
			sb.WriteString("\n")
			found = true
			break
		}
	}

	if !found {
		sb.WriteString("No Thoth memory file found.\n")
		sb.WriteString("Create one: mkdir -p .thoth && touch .thoth/memory.yaml\n")
		sb.WriteString("See: docs/THOTH.md for the specification.\n")
		return textResult(sb.String(), false), nil
	}

	// Try to read journal (optional)
	journalPaths := []string{
		projectPath + "/.thoth/journal.md",
		projectPath + "/.anubis-journal.md",
	}
	for _, jp := range journalPaths {
		data, err := os.ReadFile(jp)
		if err == nil {
			sb.WriteString("\n=== Journal (last 2000 chars) ===\n")
			content := string(data)
			if len(content) > 2000 {
				content = content[len(content)-2000:]
			}
			sb.WriteString(content)
			break
		}
	}

	return textResult(sb.String(), false), nil
}

// ---- Helpers ----

// textResult creates a simple text ToolResult.
func textResult(text string, isError bool) *ToolResult {
	return &ToolResult{
		Content: []ContentBlock{
			{Type: "text", Text: text},
		},
		IsError: isError,
	}
}

// parseCategory converts a string to a jackal.Category.
func parseCategory(s string) (jackal.Category, error) {
	switch strings.ToLower(s) {
	case "general":
		return jackal.CategoryGeneral, nil
	case "dev", "developer":
		return jackal.CategoryDev, nil
	case "ai", "ml", "ai-ml":
		return jackal.CategoryAI, nil
	case "vms", "virtualization":
		return jackal.CategoryVirtualization, nil
	case "ides", "ide":
		return jackal.CategoryIDEs, nil
	case "cloud", "infra":
		return jackal.CategoryCloud, nil
	case "storage":
		return jackal.CategoryStorage, nil
	default:
		return "", fmt.Errorf("unknown category: %s. Valid: general, dev, ai, vms, ides, cloud, storage", s)
	}
}

// shortenHomePath replaces the home directory with ~.
func shortenHomePath(path string) string {
	home, _ := os.UserHomeDir()
	if home != "" && strings.HasPrefix(path, home) {
		return "~" + path[len(home):]
	}
	return path
}

// ── RTK Handlers ────────────────────────────────────────────────

func handleFilterOutput(args map[string]interface{}) (*ToolResult, error) {
	text, _ := args["text"].(string)
	if text == "" {
		return textResult("Error: text parameter is required", true), nil
	}

	cfg := rtk.DefaultConfig()
	if maxLines, ok := args["max_lines"].(float64); ok && maxLines > 0 {
		cfg.MaxLines = int(maxLines)
	}

	f := rtk.New(cfg)
	result := f.Apply(text)

	stele.Inscribe("rtk", stele.TypeRTKFilter, "", map[string]string{
		"original_bytes": fmt.Sprintf("%d", result.OriginalBytes),
		"filtered_bytes": fmt.Sprintf("%d", result.FilteredBytes),
		"ratio":          fmt.Sprintf("%.2f", result.Ratio),
		"dupes":          fmt.Sprintf("%d", result.DupsCollapsed),
	})

	var sb strings.Builder
	sb.WriteString(result.Output)
	sb.WriteString("\n\n─── RTK Stats ───\n")
	sb.WriteString(fmt.Sprintf("Original: %d bytes → Filtered: %d bytes (%.0f%% reduction)\n",
		result.OriginalBytes, result.FilteredBytes, (1-result.Ratio)*100))
	sb.WriteString(fmt.Sprintf("Lines removed: %d (duplicates: %d)\n", result.LinesRemoved, result.DupsCollapsed))
	if result.Truncated {
		sb.WriteString("⚠ Output was truncated\n")
	}

	return textResult(sb.String(), false), nil
}

// ── Vault Handlers ──────────────────────────────────────────────

func openVaultStore() (*vault.Store, error) {
	return vault.Open(vault.DefaultPath())
}

func handleVaultStore(args map[string]interface{}) (*ToolResult, error) {
	content, _ := args["content"].(string)
	if content == "" {
		return textResult("Error: content parameter is required", true), nil
	}
	source, _ := args["source"].(string)
	tag, _ := args["tag"].(string)

	s, err := openVaultStore()
	if err != nil {
		return textResult(fmt.Sprintf("Error opening vault: %v", err), true), nil
	}
	defer s.Close()

	// Estimate tokens (rough: ~4 chars per token).
	tokens := len(content) / 4

	entry, err := s.Store(source, tag, content, tokens)
	if err != nil {
		return textResult(fmt.Sprintf("Error storing: %v", err), true), nil
	}

	stele.Inscribe("vault", stele.TypeVaultStore, "", map[string]string{
		"source": source,
		"tag":    tag,
		"tokens": fmt.Sprintf("%d", tokens),
		"bytes":  fmt.Sprintf("%d", len(content)),
	})

	return textResult(fmt.Sprintf("Stored in vault (ID: %d, ~%d tokens, %d bytes).\nUse vault_search to query or vault_get %d to retrieve.",
		entry.ID, tokens, len(content), entry.ID), false), nil
}

func handleVaultSearch(args map[string]interface{}) (*ToolResult, error) {
	query, _ := args["query"].(string)
	if query == "" {
		return textResult("Error: query parameter is required", true), nil
	}
	limit := 10
	if l, ok := args["limit"].(float64); ok && l > 0 {
		limit = int(l)
	}

	s, err := openVaultStore()
	if err != nil {
		return textResult(fmt.Sprintf("Error opening vault: %v", err), true), nil
	}
	defer s.Close()

	result, err := s.Search(query, limit)
	if err != nil {
		return textResult(fmt.Sprintf("Search error: %v", err), true), nil
	}

	stele.Inscribe("vault", stele.TypeVaultSearch, "", map[string]string{
		"query": query,
		"hits":  fmt.Sprintf("%d", result.TotalHits),
	})

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Vault search: %q — %d results\n\n", query, result.TotalHits))
	for _, e := range result.Entries {
		sb.WriteString(fmt.Sprintf("[%d] source=%s tag=%s (%s)\n  %s\n\n",
			e.ID, e.Source, e.Tag, e.CreatedAt, e.Snippet))
	}

	return textResult(sb.String(), false), nil
}

func handleVaultGet(args map[string]interface{}) (*ToolResult, error) {
	idFloat, ok := args["id"].(float64)
	if !ok {
		return textResult("Error: id parameter is required", true), nil
	}

	s, err := openVaultStore()
	if err != nil {
		return textResult(fmt.Sprintf("Error opening vault: %v", err), true), nil
	}
	defer s.Close()

	entry, err := s.Get(int64(idFloat))
	if err != nil {
		return textResult(fmt.Sprintf("Error: %v", err), true), nil
	}

	return textResult(fmt.Sprintf("Vault Entry %d (source=%s, tag=%s, %d tokens)\n\n%s",
		entry.ID, entry.Source, entry.Tag, entry.Tokens, entry.Content), false), nil
}

func handleVaultStats(_ map[string]interface{}) (*ToolResult, error) {
	s, err := openVaultStore()
	if err != nil {
		return textResult(fmt.Sprintf("Error opening vault: %v", err), true), nil
	}
	defer s.Close()

	stats, err := s.Stats()
	if err != nil {
		return textResult(fmt.Sprintf("Error: %v", err), true), nil
	}

	var sb strings.Builder
	sb.WriteString("Vault Statistics\n\n")
	sb.WriteString(fmt.Sprintf("Entries: %d\n", stats.TotalEntries))
	sb.WriteString(fmt.Sprintf("Total bytes: %d\n", stats.TotalBytes))
	sb.WriteString(fmt.Sprintf("Total tokens saved: %d\n", stats.TotalTokens))
	if stats.OldestEntry != "" {
		sb.WriteString(fmt.Sprintf("Oldest: %s\n", stats.OldestEntry))
		sb.WriteString(fmt.Sprintf("Newest: %s\n", stats.NewestEntry))
	}
	if len(stats.TagCounts) > 0 {
		sb.WriteString("\nTags:\n")
		for tag, count := range stats.TagCounts {
			label := tag
			if label == "" {
				label = "(untagged)"
			}
			sb.WriteString(fmt.Sprintf("  %s: %d\n", label, count))
		}
	}

	return textResult(sb.String(), false), nil
}

// ── Code Index Handlers ─────────────────────────────────────────

func handleCodeIndex(args map[string]interface{}) (*ToolResult, error) {
	path, _ := args["path"].(string)
	if path == "" {
		var err error
		path, err = os.Getwd()
		if err != nil {
			return textResult("Could not determine working directory", true), nil
		}
	}

	ci, err := vault.OpenCodeIndex(vault.DefaultCodeIndexPath())
	if err != nil {
		return textResult(fmt.Sprintf("Error opening code index: %v", err), true), nil
	}
	defer ci.Close()

	stats, err := ci.IndexDir(path)
	if err != nil {
		return textResult(fmt.Sprintf("Indexing failed: %v", err), true), nil
	}

	stele.Inscribe("vault", stele.TypeVaultIndex, path, map[string]string{
		"files":  fmt.Sprintf("%d", stats.FilesIndexed),
		"chunks": fmt.Sprintf("%d", stats.ChunksCreated),
	})

	return textResult(fmt.Sprintf("Code index built: %d files, %d chunks in %s.\nUse code_search to query.",
		stats.FilesIndexed, stats.ChunksCreated, stats.Duration), false), nil
}

func handleCodeSearch(args map[string]interface{}) (*ToolResult, error) {
	query, _ := args["query"].(string)
	if query == "" {
		return textResult("Error: query parameter is required", true), nil
	}
	limit := 5
	if l, ok := args["limit"].(float64); ok && l > 0 {
		limit = int(l)
	}

	ci, err := vault.OpenCodeIndex(vault.DefaultCodeIndexPath())
	if err != nil {
		return textResult(fmt.Sprintf("Error opening code index: %v", err), true), nil
	}
	defer ci.Close()

	chunks, err := ci.Search(query, limit)
	if err != nil {
		return textResult(fmt.Sprintf("Search error: %v", err), true), nil
	}

	stele.Inscribe("vault", stele.TypeVaultCodeSearch, "", map[string]string{
		"query": query,
		"hits":  fmt.Sprintf("%d", len(chunks)),
	})

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Code search: %q — %d results\n\n", query, len(chunks)))
	for _, c := range chunks {
		label := c.File
		if c.Name != "" {
			label = fmt.Sprintf("%s:%s (%s)", c.File, c.Name, c.Kind)
		}
		sb.WriteString(fmt.Sprintf("── %s [lines %d-%d] ──\n", label, c.StartLine, c.EndLine))
		sb.WriteString(c.Content)
		sb.WriteString("\n\n")
	}

	return textResult(sb.String(), false), nil
}

// ── Horus Handlers ──────────────────────────────────────────────

func handleCodeSymbols(args map[string]interface{}) (*ToolResult, error) {
	path, _ := args["path"].(string)
	if path == "" {
		var err error
		path, err = os.Getwd()
		if err != nil {
			return textResult("Could not determine working directory", true), nil
		}
	}

	p := horus.NewGoParser()
	graph, err := p.ParseDir(path)
	if err != nil {
		return textResult(fmt.Sprintf("Parse error: %v", err), true), nil
	}

	stele.Inscribe("horus", stele.TypeHorusScan, path, map[string]string{
		"files":   fmt.Sprintf("%d", graph.Stats.Files),
		"symbols": fmt.Sprintf("%d", len(graph.Symbols)),
	})

	q := horus.NewQuery(graph)
	symbols := graph.Symbols

	// Apply kind filter.
	if kind, ok := args["kind"].(string); ok && kind != "" {
		symbols = q.ByKind(horus.SymbolKind(kind))
	}

	// Apply name filter.
	if filter, ok := args["filter"].(string); ok && filter != "" {
		symbols = q.MatchSymbols(filter)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Horus: %d symbols (%d files, %d types, %d funcs, %d methods)\n\n",
		len(graph.Symbols), graph.Stats.Files, graph.Stats.Types,
		graph.Stats.Functions, graph.Stats.Methods))

	for _, s := range symbols {
		if s.Kind == horus.KindPackage {
			continue
		}
		prefix := " "
		if s.Exported {
			prefix = "▸"
		}
		sig := s.Signature
		if sig == "" {
			sig = fmt.Sprintf("%s %s", s.Kind, s.Name)
		}
		sb.WriteString(fmt.Sprintf("%s %s:%d  %s\n", prefix, s.File, s.Line, sig))
	}

	return textResult(sb.String(), false), nil
}

func handleCodeOutline(args map[string]interface{}) (*ToolResult, error) {
	path, _ := args["path"].(string)
	if path == "" {
		return textResult("Error: path parameter is required", true), nil
	}

	src, err := os.ReadFile(path)
	if err != nil {
		return textResult(fmt.Sprintf("Error reading file: %v", err), true), nil
	}

	p := horus.NewGoParser()
	symbols, err := p.ParseFile(path, src)
	if err != nil {
		return textResult(fmt.Sprintf("Parse error: %v", err), true), nil
	}

	graph := &horus.SymbolGraph{Root: filepath.Dir(path), Symbols: symbols}
	q := horus.NewQuery(graph)
	outline := q.FileOutline(path)

	stele.Inscribe("horus", stele.TypeHorusQuery, path, map[string]string{
		"type":    "outline",
		"symbols": fmt.Sprintf("%d", len(symbols)),
	})

	ratio := float64(len(outline)) / float64(len(src)) * 100
	return textResult(fmt.Sprintf("%s\n\n── %.0f%% of original file size (%d → %d bytes) ──",
		outline, ratio, len(src), len(outline)), false), nil
}

func handleCodeContext(args map[string]interface{}) (*ToolResult, error) {
	symbol, _ := args["symbol"].(string)
	if symbol == "" {
		return textResult("Error: symbol parameter is required", true), nil
	}

	path, _ := args["path"].(string)
	if path == "" {
		var err error
		path, err = os.Getwd()
		if err != nil {
			return textResult("Could not determine working directory", true), nil
		}
	}

	p := horus.NewGoParser()
	graph, err := p.ParseDir(path)
	if err != nil {
		return textResult(fmt.Sprintf("Parse error: %v", err), true), nil
	}

	q := horus.NewQuery(graph)
	ctx := q.ContextFor(symbol)

	stele.Inscribe("horus", stele.TypeHorusQuery, path, map[string]string{
		"type":   "context",
		"symbol": symbol,
	})

	return textResult(ctx, false), nil
}

// handleThothSync triggers Thoth memory and journal sync.
func handleThothSync(args map[string]interface{}) (*ToolResult, error) {
	projectPath, _ := args["path"].(string)
	if projectPath == "" {
		var err error
		projectPath, err = os.Getwd()
		if err != nil {
			return textResult("Could not determine working directory", true), nil
		}
	}

	// Sync memory
	err := thoth.Sync(thoth.SyncOptions{RepoRoot: projectPath, UpdateDate: true})
	if err != nil {
		return textResult(fmt.Sprintf("Thoth memory sync failed: %v", err), true), nil
	}

	// Sync journal
	commitCount, err := thoth.SyncJournal(thoth.JournalSyncOptions{
		RepoRoot: projectPath,
		Since:    "24 hours ago",
	})
	if err != nil {
		// Journal sync failure is non-fatal
		return textResult(fmt.Sprintf("Memory synced. Journal sync failed: %v", err), false), nil
	}

	return textResult(fmt.Sprintf("Thoth sync complete.\n- Memory updated: %s/.thoth/memory.yaml\n- Journal: %d commits processed", projectPath, commitCount), false), nil
}

// handleDetectHardware returns the system hardware profile via Hapi.
func handleDetectHardware(_ map[string]interface{}) (*ToolResult, error) {
	bridge, err := brain.NewHapiBridge()
	if err != nil {
		return &ToolResult{
			Content: []ContentBlock{
				{Type: "text", Text: fmt.Sprintf("Hardware detection failed: %v", err)},
			},
			IsError: true,
		}, nil
	}

	profile := bridge.Profile()
	data, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return nil, err
	}

	return &ToolResult{
		Content: []ContentBlock{
			{Type: "text", Text: fmt.Sprintf("𓁢 Hardware Profile Detected:\n\n```json\n%s\n```\nRecommended Backend: %s", string(data), bridge.BackendPreference())},
		},
	}, nil
}

// ── Notification History Handler ────────────────────────────────

func handleNotificationHistory(args map[string]interface{}) (*ToolResult, error) {
	store, err := notify.Open(notify.DefaultPath())
	if err != nil {
		return textResult(fmt.Sprintf("Error opening notification store: %v", err), true), nil
	}
	defer store.Close()

	limit := 10
	if l, ok := args["limit"].(float64); ok && l > 0 {
		limit = int(l)
	}

	var results []notify.Notification
	if source, ok := args["source"].(string); ok && source != "" {
		results, err = store.BySource(source, limit)
	} else {
		results, err = store.Recent(limit)
	}
	if err != nil {
		return textResult(fmt.Sprintf("Query error: %v", err), true), nil
	}

	if len(results) == 0 {
		return textResult("No notifications yet. Run a scan from the menubar or CLI.", false), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Notification History (%d results)\n\n", len(results)))
	for _, n := range results {
		icon := notify.SeverityIcon(n.Severity)
		sb.WriteString(fmt.Sprintf("%s [%s] %s/%s — %s",
			icon, n.Timestamp.Format("Jan 02 15:04"), n.Source, n.Action, n.Summary))
		if n.DurationMs > 0 {
			sb.WriteString(fmt.Sprintf(" (%dms)", n.DurationMs))
		}
		sb.WriteString("\n")
	}

	return textResult(sb.String(), false), nil
}
