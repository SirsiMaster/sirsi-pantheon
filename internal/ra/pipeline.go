// Package ra implements the Ra orchestration pipeline with automatic
// knowledge feedback through Seshat ingestion and Thoth persistence.
package ra

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
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

	// OrchestratorPath is the resolved path to sirsi-orchestrator.py.
	// If empty, the pipeline will attempt to find it automatically.
	OrchestratorPath string
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
	start := time.Now()

	// Step 1: Execute the orchestrator and capture output.
	stdout, stderr, err := p.executeOrchestrator(ctx, task)
	if err != nil {
		return nil, fmt.Errorf("ra pipeline: orchestrator failed: %w", err)
	}

	// Step 2: Parse orchestrator output into KnowledgeItems.
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

// execCommandContext is an injectable seam for tests (mirrors internal/ka's
// ExecCommand pattern) so unit tests never spawn a real python3 orchestrator.
// Production always uses exec.CommandContext.
var execCommandContext = exec.CommandContext

// executeOrchestrator runs the orchestrator script and captures stdout/stderr separately.
func (p *Pipeline) executeOrchestrator(ctx context.Context, task Task) (string, string, error) {
	scriptPath := p.OrchestratorPath
	if scriptPath == "" {
		var err error
		scriptPath, err = findOrchestratorScript()
		if err != nil {
			return "", "", err
		}
	}

	args := append([]string{scriptPath, task.Subcmd}, task.ExtraArgs...)
	cmd := execCommandContext(ctx, "python3", args...)

	var stdoutBuf, stderrBuf bytes.Buffer
	// Tee to os.Stdout/Stderr so the user still sees live output.
	cmd.Stdout = &teeWriter{buf: &stdoutBuf, passthrough: os.Stdout}
	cmd.Stderr = &teeWriter{buf: &stderrBuf, passthrough: os.Stderr}
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		return stdoutBuf.String(), stderrBuf.String(), err
	}
	return stdoutBuf.String(), stderrBuf.String(), nil
}

// teeWriter writes to both a buffer and a passthrough writer.
type teeWriter struct {
	buf         *bytes.Buffer
	passthrough *os.File
}

func (w *teeWriter) Write(p []byte) (int, error) {
	w.buf.Write(p)
	return w.passthrough.Write(p)
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

// findOrchestratorScript locates sirsi-orchestrator.py using the same
// resolution logic as the Ra CLI command.
func findOrchestratorScript() (string, error) {
	candidates := []string{
		filepath.Join(".", "scripts", "sirsi-orchestrator.py"),
	}

	if root := os.Getenv("PANTHEON_ROOT"); root != "" {
		candidates = append(candidates, filepath.Join(root, "scripts", "sirsi-orchestrator.py"))
	}

	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, "Development", "sirsi-pantheon", "scripts", "sirsi-orchestrator.py"))
	}

	for _, p := range candidates {
		abs, err := filepath.Abs(p)
		if err != nil {
			continue
		}
		if _, err := os.Stat(abs); err == nil {
			return abs, nil
		}
	}

	return "", fmt.Errorf("sirsi-orchestrator.py not found")
}
