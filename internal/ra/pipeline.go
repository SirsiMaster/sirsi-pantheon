// Package ra implements the Ra orchestration pipeline with automatic
// knowledge feedback through Seshat ingestion and Thoth persistence.
package ra

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/seshat"
	"github.com/SirsiMaster/sirsi-pantheon/internal/thoth"
)

// Task describes a Ra orchestration job to execute.
type Task struct {
	Subcmd    string   // orchestrator subcommand (health, test, lint, task, broadcast, nightly)
	ExtraArgs []string // additional arguments (repo name, prompt, etc.)
}

// PipelineResult holds the outcome of a pipeline run.
type PipelineResult struct {
	ItemsIngested int
	ThothSynced   bool
	Duration      time.Duration
}

// PipelineStatus describes the last recorded pipeline state.
type PipelineStatus struct {
	LastRecorded time.Time
	ItemCount    int
	ThothSynced  time.Time
}

// Pipeline represents the Ra -> Seshat -> Thoth knowledge feedback loop.
type Pipeline struct {
	// ThothDir is the path to the project's .thoth/ directory.
	ThothDir string

	// Filter is the Seshat secrets filter applied before persistence.
	Filter *seshat.SecretsFilter

	// ThothAdapter exports KIs into the Thoth knowledge store.
	ThothAdapter *seshat.ThothAdapter

	// RepoRoot is the project root for Thoth sync operations.
	RepoRoot string
}

// NativeFleetRunner is the injectable Go-owned executor used by the native
// recording path. It preserves deterministic tests without invoking a shell,
// Python, or an external agent provider.
type NativeFleetRunner func(context.Context, []NativeRepo, string) ([]NativeResult, error)

// RunNativeAndRecord records the Go-owned health, lint, test, or nightly
// result through the same Seshat/Thoth pipeline as the legacy executor. Task
// and broadcast remain explicit external-provider operations and fail closed
// in RunNativeFleet when no approved provider is available.
func (p *Pipeline) RunNativeAndRecord(ctx context.Context, repos []NativeRepo, task Task, run NativeFleetRunner) (*PipelineResult, error) {
	if run == nil {
		run = RunNativeFleet
	}
	results, err := run(ctx, repos, task.Subcmd)
	encoded, marshalErr := json.Marshal(results)
	if marshalErr != nil {
		return nil, fmt.Errorf("ra pipeline: encode native results: %w", marshalErr)
	}
	if err != nil {
		return nil, fmt.Errorf("ra pipeline: native fleet failed: %w", err)
	}
	return p.recordCapturedOutput(task, string(encoded), "")
}

// NewPipeline creates a pipeline with default configuration for the given project root.
func NewPipeline(repoRoot string) *Pipeline {
	return &Pipeline{
		ThothDir:     filepath.Join(repoRoot, ".thoth"),
		Filter:       seshat.DefaultFilter(),
		ThothAdapter: &seshat.ThothAdapter{ProjectDir: repoRoot},
		RepoRoot:     repoRoot,
	}
}

// RunAndRecord executes a Ra orchestration task, captures the output,
// feeds it to Seshat for ingestion, then syncs to Thoth memory.
func (p *Pipeline) RunAndRecord(ctx context.Context, task Task) (*PipelineResult, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("ra pipeline: resolve fleet home: %w", err)
	}
	return p.RunNativeAndRecord(ctx, DefaultFleetRepos(home), task, RunNativeFleet)
}

func (p *Pipeline) recordCapturedOutput(task Task, stdout, stderr string) (*PipelineResult, error) {
	start := time.Now()

	// Parse executor output into KnowledgeItems.
	items := p.parseOutput(task, stdout, stderr)

	// Step 3: Run Seshat's secrets filter on all items before storage.
	modified, dropped := p.Filter.FilterItems(items)
	if dropped > 0 {
		// Re-slice to remove dropped items.
		items = items[:len(items)-dropped]
	}
	_ = modified // informational only

	// Step 4: Export filtered KIs to Thoth via the Seshat ThothAdapter.
	if len(items) > 0 {
		if err := p.ThothAdapter.Export(items); err != nil {
			return nil, fmt.Errorf("ra pipeline: seshat export failed: %w", err)
		}
	}

	// Step 5: Run Thoth sync to update memory.yaml with latest project stats.
	thothSynced := false
	if err := thoth.Sync(thoth.SyncOptions{RepoRoot: p.RepoRoot, UpdateDate: true}); err == nil {
		thothSynced = true
	}

	// Step 6: Record pipeline metadata for status reporting.
	if err := p.recordStatus(len(items), thothSynced); err != nil {
		// Non-fatal: the knowledge was still saved.
		fmt.Fprintf(os.Stderr, "  warning: failed to record pipeline status: %v\n", err)
	}

	return &PipelineResult{
		ItemsIngested: len(items),
		ThothSynced:   thothSynced,
		Duration:      time.Since(start),
	}, nil
}

// parseOutput converts raw orchestrator output into Seshat KnowledgeItems.
func (p *Pipeline) parseOutput(task Task, stdout, stderr string) []seshat.KnowledgeItem {
	combined := strings.TrimSpace(stdout)
	if combined == "" {
		combined = strings.TrimSpace(stderr)
	}
	if combined == "" {
		return nil
	}

	now := time.Now().Format(time.RFC3339)
	taskDesc := fmt.Sprintf("Ra %s", task.Subcmd)
	if len(task.ExtraArgs) > 0 {
		taskDesc += " " + strings.Join(task.ExtraArgs, " ")
	}

	// Try to parse as JSON (the orchestrator may emit structured output).
	var items []seshat.KnowledgeItem
	if parsed := tryParseJSON(combined); len(parsed) > 0 {
		for _, p := range parsed {
			items = append(items, seshat.KnowledgeItem{
				Title:   fmt.Sprintf("[Ra] %s: %s", task.Subcmd, p.title),
				Summary: p.summary,
				References: []seshat.KIReference{
					{Type: "source", Value: fmt.Sprintf("ra/%s", task.Subcmd)},
					{Type: "timestamp", Value: now},
				},
			})
		}
		return items
	}

	// Fallback: treat the entire output as a single KI.
	// Truncate very long output to keep KIs readable.
	summary := combined
	if len(summary) > 4000 {
		summary = summary[:4000] + "\n\n[... truncated]"
	}

	return []seshat.KnowledgeItem{
		{
			Title:   fmt.Sprintf("[Ra] %s", taskDesc),
			Summary: summary,
			References: []seshat.KIReference{
				{Type: "source", Value: fmt.Sprintf("ra/%s", task.Subcmd)},
				{Type: "timestamp", Value: now},
			},
		},
	}
}

// parsedItem is an intermediate struct for JSON output parsing.
type parsedItem struct {
	title   string
	summary string
}

// tryParseJSON attempts to extract structured items from JSON output.
// The orchestrator may emit a JSON array of results or a single object.
func tryParseJSON(output string) []parsedItem {
	// Try array of objects with "repo" and "result"/"status" fields.
	var arr []map[string]interface{}
	if err := json.Unmarshal([]byte(output), &arr); err == nil && len(arr) > 0 {
		var items []parsedItem
		for _, obj := range arr {
			repo, _ := obj["repo"].(string)
			if repo == "" {
				repo, _ = obj["name"].(string)
			}
			result, _ := obj["result"].(string)
			if result == "" {
				result, _ = obj["status"].(string)
			}
			if result == "" {
				// Marshal the object back as the summary.
				b, _ := json.MarshalIndent(obj, "", "  ")
				result = string(b)
			}
			title := repo
			if title == "" {
				title = "result"
			}
			items = append(items, parsedItem{title: title, summary: result})
		}
		return items
	}

	// Try single object.
	var single map[string]interface{}
	if err := json.Unmarshal([]byte(output), &single); err == nil && len(single) > 0 {
		b, _ := json.MarshalIndent(single, "", "  ")
		return []parsedItem{{title: "result", summary: string(b)}}
	}

	return nil
}

// statusFile returns the path to the pipeline status file.
func (p *Pipeline) statusFile() string {
	return filepath.Join(p.ThothDir, "ra_pipeline_status.json")
}

// pipelineStatusData is the serialized form of pipeline status.
type pipelineStatusData struct {
	LastRecorded string `json:"last_recorded"`
	ItemCount    int    `json:"item_count"`
	ThothSynced  string `json:"thoth_synced,omitempty"`
}

// recordStatus writes the pipeline execution metadata to .thoth/ra_pipeline_status.json.
func (p *Pipeline) recordStatus(itemCount int, thothSynced bool) error {
	if err := os.MkdirAll(p.ThothDir, 0755); err != nil {
		return err
	}

	now := time.Now().Format(time.RFC3339)
	data := pipelineStatusData{
		LastRecorded: now,
		ItemCount:    itemCount,
	}
	if thothSynced {
		data.ThothSynced = now
	}

	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p.statusFile(), b, 0644)
}

// ReadStatus loads the last pipeline status from disk.
func (p *Pipeline) ReadStatus() (*PipelineStatus, error) {
	data, err := os.ReadFile(p.statusFile())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // no status yet
		}
		return nil, err
	}

	var raw pipelineStatusData
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse pipeline status: %w", err)
	}

	status := &PipelineStatus{ItemCount: raw.ItemCount}
	if t, err := time.Parse(time.RFC3339, raw.LastRecorded); err == nil {
		status.LastRecorded = t
	}
	if raw.ThothSynced != "" {
		if t, err := time.Parse(time.RFC3339, raw.ThothSynced); err == nil {
			status.ThothSynced = t
		}
	}
	return status, nil
}
