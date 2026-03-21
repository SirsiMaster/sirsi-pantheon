// Sirsi Anubis Agent — lightweight binary for fleet deployment.
//
// The agent is designed to be deployed to remote targets (VMs, containers,
// bare metal servers) by the Anubis controller. It implements a FIXED
// command set with NO shell access (Rule A3).
//
// Commands:
//
//	scan    — run local scan and output JSON to stdout
//	report  — generate system health report as JSON
//	clean   — clean artifacts (requires explicit --confirm)
//	version — show version
//
// All output is JSON for machine consumption by the controller.
// The agent has ZERO external dependencies (CGO_ENABLED=0).
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/SirsiMaster/sirsi-anubis/internal/jackal"
	"github.com/SirsiMaster/sirsi-anubis/internal/jackal/rules"
)

// Version is set by goreleaser at build time.
var version = "dev"

// AgentResponse is the standard JSON response envelope.
type AgentResponse struct {
	Command   string      `json:"command"`
	Status    string      `json:"status"` // "ok", "error"
	Timestamp string      `json:"timestamp"`
	Version   string      `json:"version"`
	Data      interface{} `json:"data,omitempty"`
	Error     string      `json:"error,omitempty"`
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(0)
	}

	cmd := os.Args[1]

	// FIXED command set — Rule A3: no arbitrary command execution
	switch cmd {
	case "version":
		respondOK("version", map[string]string{
			"version":  version,
			"platform": fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
			"binary":   "anubis-agent",
		})

	case "scan":
		runAgentScan()

	case "report":
		runAgentReport()

	case "clean":
		// Clean requires --confirm flag
		confirm := false
		for _, arg := range os.Args[2:] {
			if arg == "--confirm" {
				confirm = true
			}
		}
		if !confirm {
			respondError("clean", "clean requires --confirm flag (Rule A1)")
			os.Exit(1)
		}
		runAgentClean()

	case "help", "--help", "-h":
		printUsage()

	default:
		respondError("unknown", fmt.Sprintf("unknown command: %s (fixed command set: scan, report, clean, version)", cmd))
		os.Exit(1)
	}
}

func runAgentScan() {
	engine := jackal.NewEngine()
	engine.RegisterAll(rules.AllRules()...)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	result, err := engine.Scan(ctx, jackal.ScanOptions{})
	if err != nil {
		respondError("scan", fmt.Sprintf("scan failed: %v", err))
		os.Exit(1)
	}

	respondOK("scan", map[string]interface{}{
		"total_size_bytes": result.TotalSize,
		"total_size":       jackal.FormatSize(result.TotalSize),
		"findings":         len(result.Findings),
		"rules_ran":        result.RulesRan,
		"rules_matched":    result.RulesWithFindings,
		"categories":       result.ByCategory,
		"errors":           len(result.Errors),
	})
}

func runAgentReport() {
	engine := jackal.NewEngine()
	engine.RegisterAll(rules.AllRules()...)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	result, err := engine.Scan(ctx, jackal.ScanOptions{})
	if err != nil {
		respondError("report", fmt.Sprintf("scan failed: %v", err))
		os.Exit(1)
	}

	// Health grade
	var grade string
	switch {
	case result.TotalSize < 100*1024*1024:
		grade = "EXCELLENT"
	case result.TotalSize < 1024*1024*1024:
		grade = "GOOD"
	case result.TotalSize < 5*1024*1024*1024:
		grade = "FAIR"
	default:
		grade = "NEEDS_ATTENTION"
	}

	respondOK("report", map[string]interface{}{
		"platform":     fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
		"cpus":         runtime.NumCPU(),
		"total_waste":  jackal.FormatSize(result.TotalSize),
		"waste_bytes":  result.TotalSize,
		"findings":     len(result.Findings),
		"rules_ran":    result.RulesRan,
		"health_grade": grade,
	})
}

func runAgentClean() {
	engine := jackal.NewEngine()
	engine.RegisterAll(rules.AllRules()...)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// First scan to find targets
	result, err := engine.Scan(ctx, jackal.ScanOptions{})
	if err != nil {
		respondError("clean", fmt.Sprintf("scan failed: %v", err))
		os.Exit(1)
	}

	// Clean with DryRun=false, Confirm=true
	cleanResult, err := engine.Clean(ctx, result.Findings, jackal.CleanOptions{
		Confirm: true,
	})
	if err != nil {
		respondError("clean", fmt.Sprintf("clean failed: %v", err))
		os.Exit(1)
	}

	respondOK("clean", map[string]interface{}{
		"cleaned":     cleanResult.Cleaned,
		"bytes_freed": cleanResult.BytesFreed,
		"freed":       jackal.FormatSize(cleanResult.BytesFreed),
		"skipped":     cleanResult.Skipped,
		"errors":      len(cleanResult.Errors),
	})
}

func respondOK(command string, data interface{}) {
	resp := AgentResponse{
		Command:   command,
		Status:    "ok",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Version:   version,
		Data:      data,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(resp)
}

func respondError(command, msg string) {
	resp := AgentResponse{
		Command:   command,
		Status:    "error",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Version:   version,
		Error:     msg,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(resp)
}

func printUsage() {
	fmt.Println("𓂀 Sirsi Anubis Agent")
	fmt.Println("  Lightweight agent for fleet deployment")
	fmt.Println()
	fmt.Println("  This binary is deployed to remote targets by the Anubis controller.")
	fmt.Println("  It implements a fixed, auditable command set (Rule A3).")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  anubis-agent scan               Run local scan and output JSON")
	fmt.Println("  anubis-agent report              Generate system health report")
	fmt.Println("  anubis-agent clean --confirm     Clean artifacts (requires --confirm)")
	fmt.Println("  anubis-agent version             Show version")
}
