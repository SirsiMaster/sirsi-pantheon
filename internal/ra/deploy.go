package ra

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/neith"
	"github.com/SirsiMaster/sirsi-pantheon/internal/seshat"
	"github.com/SirsiMaster/sirsi-pantheon/internal/thoth"
)

// DeployOptions configures a parallel deployment of Ra windows.
type DeployOptions struct {
	ConfigDir  string   // path to configs/scopes/
	ScopeNames []string // filter to specific scopes (empty = all)
	UseITerm2  bool
	Wait       bool // block until all windows complete
	Record     bool // run Seshat/Thoth pipeline after completion
	DryRun     bool // show assembled prompts without spawning
	RepoRoot   string
}

// DeployResult holds the outcome of a deployment.
type DeployResult struct {
	Spawned []string
	Status  *DeploymentStatus
	Results []WindowResult
}

// RADir returns the runtime state directory for Ra.
func RADir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "ra")
}

// Deploy spawns terminal windows for all requested scopes.
// Neith assembles the prompts, Ra spawns and monitors.
func Deploy(opts DeployOptions) (*DeployResult, error) {
	loom := neith.NewLoom(opts.ConfigDir)

	scopes, err := loom.LoadScopes()
	if err != nil {
		return nil, fmt.Errorf("neith: load scopes: %w", err)
	}

	// Filter scopes if specific names requested
	if len(opts.ScopeNames) > 0 {
		filtered := make([]neith.ScopeConfig, 0)
		nameSet := make(map[string]bool)
		for _, n := range opts.ScopeNames {
			nameSet[n] = true
		}
		for _, s := range scopes {
			if nameSet[s.Name] {
				filtered = append(filtered, s)
			}
		}
		if len(filtered) == 0 {
			return nil, fmt.Errorf("no scopes matched: %v", opts.ScopeNames)
		}
		scopes = filtered
	}

	raDir := RADir()
	var spawned []string

	for _, scope := range scopes {
		// Neith weaves the scope from canon
		prompt, err := loom.WeaveScope(scope)
		if err != nil {
			fmt.Printf("  ⚠️  %s: failed to weave scope: %v\n", scope.DisplayName, err)
			continue
		}

		// Write prompt to disk
		promptFile, err := loom.WritePrompt(scope.Name, prompt)
		if err != nil {
			return nil, fmt.Errorf("neith: write prompt for %s: %w", scope.Name, err)
		}

		if opts.DryRun {
			fmt.Printf("\n𓁯 Neith — Scope: %s\n", scope.DisplayName)
			fmt.Printf("  Repo:     %s\n", scope.RepoPath)
			fmt.Printf("  Deadline: %s\n", scope.Deadline)
			fmt.Printf("  Priority: %s\n", scope.Priority)
			fmt.Printf("  Prompt:   %s (%d chars)\n", promptFile, len(prompt))
			fmt.Printf("  ---\n")
			// Show first 500 chars of prompt
			preview := prompt
			if len(preview) > 500 {
				preview = preview[:500] + "\n  ... (truncated)"
			}
			fmt.Printf("  %s\n", preview)
			spawned = append(spawned, scope.Name)
			continue
		}

		// Expand ~ in repo path
		repoPath := expandHome(scope.RepoPath)

		// Ra spawns the terminal window
		cfg := SpawnConfig{
			Name:       scope.Name,
			Title:      fmt.Sprintf("𓇶 Ra: %s", scope.DisplayName),
			WorkDir:    repoPath,
			PromptFile: promptFile,
			LogFile:    filepath.Join(raDir, "logs", scope.Name+".log"),
			ExitFile:   filepath.Join(raDir, "exits", scope.Name+".exit"),
			PIDFile:    filepath.Join(raDir, "pids", scope.Name+".pid"),
			UseITerm2:  opts.UseITerm2,
		}

		_, err = SpawnWindow(cfg)
		if err != nil {
			fmt.Printf("  ❌ %s: failed to spawn: %v\n", scope.DisplayName, err)
			continue
		}

		fmt.Printf("  𓇶 Spawned: %s → %s\n", scope.DisplayName, repoPath)
		spawned = append(spawned, scope.Name)
	}

	if opts.DryRun {
		return &DeployResult{Spawned: spawned}, nil
	}

	// Write deployment metadata
	writeDeployMeta(raDir, spawned)

	result := &DeployResult{Spawned: spawned}

	// Wait for all windows if requested
	if opts.Wait {
		fmt.Printf("\n  ⏳ Waiting for %d windows to complete...\n", len(spawned))
		status, err := WaitAll(raDir, 2*time.Hour)
		if err != nil {
			return result, fmt.Errorf("ra: wait: %w", err)
		}
		result.Status = status

		// Collect results
		results, err := CollectResults(raDir)
		if err != nil {
			return result, fmt.Errorf("ra: collect: %w", err)
		}
		result.Results = results

		// Print summary
		for _, w := range status.Windows {
			icon := "✅"
			if w.State == "failed" || w.State == "crashed" {
				icon = "❌"
			}
			fmt.Printf("  %s %s — %s (exit %d, %s)\n", icon, w.Name, w.State, w.ExitCode, w.Duration.Round(time.Second))
		}

		// Run pipeline if --record
		if opts.Record {
			pr, err := IngestWindowResults(opts.RepoRoot, results)
			if err != nil {
				fmt.Printf("  ⚠️  Pipeline error: %v\n", err)
			} else {
				fmt.Printf("\n  𓇶 Ra complete → 𓁆 Seshat ingested %d items → 𓁟 Thoth %s\n",
					pr.ItemsIngested, syncStatus(pr.ThothSynced))
			}
		}
	}

	return result, nil
}

// IngestWindowResults converts window results into KnowledgeItems and
// feeds them through the Seshat → Thoth pipeline.
func IngestWindowResults(repoRoot string, results []WindowResult) (*PipelineResult, error) {
	start := time.Now()
	filter := seshat.DefaultFilter()

	var items []seshat.KnowledgeItem
	for _, r := range results {
		summary := r.LogText
		if len(summary) > 4000 {
			summary = summary[len(summary)-4000:]
		}

		status := "completed"
		if r.ExitCode != 0 {
			status = "failed"
		}

		items = append(items, seshat.KnowledgeItem{
			Title:   fmt.Sprintf("[Ra Deploy] %s — %s (exit %d)", r.Name, status, r.ExitCode),
			Summary: summary,
			References: []seshat.KIReference{
				{Type: "source", Value: "ra/deploy/" + r.Name},
				{Type: "duration", Value: r.Duration.String()},
				{Type: "timestamp", Value: time.Now().Format(time.RFC3339)},
			},
		})
	}

	// Filter secrets
	filter.FilterItems(items)

	// Export to Thoth
	adapter := &seshat.ThothAdapter{ProjectDir: repoRoot}
	if err := adapter.Export(items); err != nil {
		return nil, fmt.Errorf("seshat export: %w", err)
	}

	// Sync Thoth
	synced := false
	if err := thoth.Sync(thoth.SyncOptions{RepoRoot: repoRoot}); err == nil {
		synced = true
	}

	return &PipelineResult{
		ItemsIngested: len(items),
		ThothSynced:   synced,
		Duration:      time.Since(start),
	}, nil
}

func writeDeployMeta(raDir string, scopes []string) {
	metaDir := raDir
	os.MkdirAll(metaDir, 0755)

	meta := deploymentMeta{
		StartedAt: time.Now().Format(time.RFC3339),
		Scopes:    scopes,
	}
	data, _ := json.MarshalIndent(meta, "", "  ")
	os.WriteFile(filepath.Join(metaDir, "deployment.json"), data, 0644)
}

func expandHome(path string) string {
	if len(path) > 1 && path[:2] == "~/" {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}

func syncStatus(synced bool) string {
	if synced {
		return "synced ✅"
	}
	return "skipped ⚠️"
}
