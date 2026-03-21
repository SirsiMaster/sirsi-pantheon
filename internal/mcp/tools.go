package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-anubis/internal/brain"
	"github.com/SirsiMaster/sirsi-anubis/internal/jackal"
	"github.com/SirsiMaster/sirsi-anubis/internal/jackal/rules"
	"github.com/SirsiMaster/sirsi-anubis/internal/ka"
)

// registerTools adds all Anubis tools to the MCP server.
func registerTools(s *Server) {
	s.RegisterTool(Tool{
		Name:        "scan_workspace",
		Description: "Scan a workspace directory for infrastructure waste (stale caches, orphaned build artifacts, unused dependencies). Read-only — never deletes anything.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]SchemaField{
				"path": {
					Type:        "string",
					Description: "Absolute path to the workspace directory to scan. Defaults to current working directory.",
				},
				"category": {
					Type:        "string",
					Description: "Optional category filter: general, dev, ai, vms, ides, cloud, storage. Leave empty for all.",
				},
			},
		},
	}, handleScanWorkspace)

	s.RegisterTool(Tool{
		Name:        "ghost_report",
		Description: "Hunt for ghost apps — remnants of uninstalled applications lingering on the system (preferences, caches, launch agents, Spotlight registrations).",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]SchemaField{
				"target": {
					Type:        "string",
					Description: "Optional: hunt for a specific ghost by app name or bundle ID.",
				},
			},
		},
	}, handleGhostReport)

	s.RegisterTool(Tool{
		Name:        "health_check",
		Description: "Quick system health summary — CPU, RAM, GPU, disk usage, and infrastructure hygiene score.",
		InputSchema: InputSchema{
			Type:       "object",
			Properties: map[string]SchemaField{},
		},
	}, handleHealthCheck)

	s.RegisterTool(Tool{
		Name:        "classify_files",
		Description: "Classify files semantically using Anubis brain (junk, project, config, model, media, etc). Uses heuristic classifier or neural model if installed.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]SchemaField{
				"paths": {
					Type:        "string",
					Description: "Comma-separated list of file paths to classify.",
				},
			},
			Required: []string{"paths"},
		},
	}, handleClassifyFiles)
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

	// Create engine and run scan
	engine := jackal.NewEngine()
	engine.RegisterAll(rules.AllRules()...)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := engine.Scan(ctx, jackal.ScanOptions{
		Categories: categories,
	})
	if err != nil {
		return textResult(fmt.Sprintf("Scan failed: %v", err), true), nil
	}

	// Format results
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("𓂀 Anubis Scan Results for: %s\n\n", scanPath))
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
	ghosts, err := scanner.Scan(false) // no sudo in MCP mode
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
	sb.WriteString(fmt.Sprintf("𓂓 Ka Ghost Report — %d ghosts detected\n\n", len(ghosts)))

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

// handleHealthCheck provides a quick system health summary.
func handleHealthCheck(_ map[string]interface{}) (*ToolResult, error) {
	var sb strings.Builder
	sb.WriteString("𓂀 Anubis Health Check\n\n")

	// System info
	sb.WriteString(fmt.Sprintf("Platform: %s/%s\n", runtime.GOOS, runtime.GOARCH))
	sb.WriteString(fmt.Sprintf("CPUs: %d\n", runtime.NumCPU()))

	// Quick scan to get waste total
	engine := jackal.NewEngine()
	engine.RegisterAll(rules.AllRules()...)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	result, err := engine.Scan(ctx, jackal.ScanOptions{})
	if err != nil {
		sb.WriteString(fmt.Sprintf("Scan error: %v\n", err))
	} else {
		sb.WriteString(fmt.Sprintf("Infrastructure waste: %s (%d findings)\n",
			jackal.FormatSize(result.TotalSize), len(result.Findings)))
		sb.WriteString(fmt.Sprintf("Rules matched: %d/%d\n", result.RulesWithFindings, result.RulesRan))
	}

	// Ghost count
	scanner := ka.NewScanner()
	ghosts, err := scanner.Scan(false)
	if err == nil {
		sb.WriteString(fmt.Sprintf("Ghost apps: %d\n", len(ghosts)))
	}

	// Brain status
	if brain.IsInstalled() {
		sb.WriteString("Neural brain: ✅ Installed\n")
	} else {
		sb.WriteString("Neural brain: Not installed (run 'anubis install-brain')\n")
	}

	// Health grade
	var grade string
	if result != nil {
		switch {
		case result.TotalSize < 1024*1024*100: // < 100 MB
			grade = "EXCELLENT"
		case result.TotalSize < 1024*1024*1024: // < 1 GB
			grade = "GOOD"
		case result.TotalSize < 1024*1024*1024*5: // < 5 GB
			grade = "FAIR"
		default:
			grade = "NEEDS ATTENTION"
		}
		sb.WriteString(fmt.Sprintf("\nHealth Grade: %s\n", grade))
	}

	return textResult(sb.String(), false), nil
}

// handleClassifyFiles classifies files using the brain module.
func handleClassifyFiles(args map[string]interface{}) (*ToolResult, error) {
	pathsRaw, _ := args["paths"].(string)
	if pathsRaw == "" {
		return textResult("Missing required parameter: paths", true), nil
	}

	paths := strings.Split(pathsRaw, ",")
	for i := range paths {
		paths[i] = strings.TrimSpace(paths[i])
	}

	classifier, err := brain.GetClassifier()
	if err != nil {
		return textResult(fmt.Sprintf("Classifier error: %v", err), true), nil
	}
	defer classifier.Close()

	result, err := classifier.ClassifyBatch(paths, 4)
	if err != nil {
		return textResult(fmt.Sprintf("Classification failed: %v", err), true), nil
	}

	// Format as JSON for structured consumption
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return textResult(fmt.Sprintf("Marshal error: %v", err), true), nil
	}

	return textResult(string(data), false), nil
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
