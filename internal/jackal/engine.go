package jackal

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"sort"
	"sync"

	"github.com/SirsiMaster/sirsi-pantheon/internal/oplog"
)

// Engine is the Jackal scan engine. It manages a registry of scan rules
// and orchestrates scanning and cleaning operations.
type Engine struct {
	rules []ScanRule
	mu    sync.RWMutex
}

// NewEngine creates a new Jackal scan engine.
func NewEngine() *Engine {
	return &Engine{
		rules: make([]ScanRule, 0),
	}
}

// Register adds a scan rule to the engine.
func (e *Engine) Register(rule ScanRule) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.rules = append(e.rules, rule)
}

// RegisterAll adds multiple scan rules at once.
func (e *Engine) RegisterAll(rules ...ScanRule) {
	for _, r := range rules {
		e.Register(r)
	}
}

// Rules returns all registered rules (for inspection).
func (e *Engine) Rules() []ScanRule {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]ScanRule, len(e.rules))
	copy(out, e.rules)
	return out
}

// applicableRules filters rules by current platform and requested categories.
func (e *Engine) applicableRules(opts ScanOptions) []ScanRule {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var result []ScanRule
	for _, rule := range e.rules {
		// Platform filter
		if !PlatformMatch(rule.Platforms()) {
			continue
		}

		// Category filter (empty = all)
		if len(opts.Categories) > 0 {
			matched := false
			for _, cat := range opts.Categories {
				if rule.Category() == cat {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}

		result = append(result, rule)
	}
	return result
}

// ScanResult holds the aggregated results of a full scan.
type ScanResult struct {
	// All findings across all rules
	Findings []Finding

	// Total size in bytes across ALL findings (including warning-tier items the
	// cleaner will NEVER one-click remove, e.g. AI model weights).
	TotalSize int64

	// ReclaimableSize is the size that is genuinely reclaimable as waste —
	// safe + caution findings only. Warning-tier findings (model weights, data,
	// config — protected from one-click clean) are EXCLUDED so a "waste" headline
	// never counts 67 GB of Gemma weights as trash. This is what surfaces (menubar
	// title, summaries) should show; TotalSize remains the full inventory.
	ReclaimableSize int64

	// Number of rules that ran
	RulesRan int

	// Number of rules that found something
	RulesWithFindings int

	// Errors from individual rules (non-fatal)
	Errors []RuleError

	// Breakdown by category
	ByCategory map[Category]CategorySummary
}

// RuleError pairs a rule name with its error.
type RuleError struct {
	RuleName string
	Err      error
}

// CategorySummary aggregates findings for a category.
type CategorySummary struct {
	Category  Category
	Findings  int
	TotalSize int64
}

// Scan runs all applicable rules and returns aggregated results.
// This method has ZERO side effects (Rule A2).
func (e *Engine) Scan(ctx context.Context, opts ScanOptions) (*ScanResult, error) {
	rules := e.applicableRules(opts)

	result := &ScanResult{
		Findings:   make([]Finding, 0),
		ByCategory: make(map[Category]CategorySummary),
	}

	type ruleResult struct {
		findings []Finding
		err      error
		ruleName string
	}

	// Run rules concurrently with bounded worker pool.
	// Cap goroutines at NumCPU to prevent IPC starvation (B11).
	maxWorkers := runtime.NumCPU()
	if maxWorkers > len(rules) {
		maxWorkers = len(rules)
	}
	if maxWorkers < 1 {
		maxWorkers = 1
	}

	ch := make(chan ruleResult, len(rules))
	sem := make(chan struct{}, maxWorkers)
	var wg sync.WaitGroup

	for _, rule := range rules {
		wg.Add(1)
		sem <- struct{}{} // Acquire semaphore slot
		go func(r ScanRule) {
			defer wg.Done()
			defer func() { <-sem }() // Release slot
			// Pin to dedicated OS thread for true multi-core execution.
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			findings, err := r.Scan(ctx, opts)
			ch <- ruleResult{
				findings: findings,
				err:      err,
				ruleName: r.Name(),
			}
		}(rule)
	}

	// Close channel when all goroutines finish
	go func() {
		wg.Wait()
		close(ch)
	}()

	// Collect results
	result.RulesRan = len(rules)
	completed := 0
	for rr := range ch {
		completed++
		if rr.err != nil {
			result.Errors = append(result.Errors, RuleError{
				RuleName: rr.ruleName,
				Err:      rr.err,
			})
			if opts.OnProgress != nil {
				opts.OnProgress(rr.ruleName, 0, 0, completed, len(rules))
			}
			continue
		}

		ruleSize := int64(0)
		if len(rr.findings) > 0 {
			result.RulesWithFindings++
			result.Findings = append(result.Findings, rr.findings...)
			for _, f := range rr.findings {
				ruleSize += f.SizeBytes
			}
		}
		if opts.OnProgress != nil {
			opts.OnProgress(rr.ruleName, len(rr.findings), ruleSize, completed, len(rules))
		}
	}

	NormalizeFindings(result)

	return result, nil
}

// NormalizeFindings removes duplicate, byte-identical findings and rebuilds
// aggregate totals from the resulting actionable inventory. Callers that add
// findings after Engine.Scan (for example the ghost-residual scanner) must call
// this before persisting or presenting a result.
//
// A duplicate is only coalesced when it names the same cleaned path and reports
// the same object shape and size. Disagreements remain visible rather than being
// silently hidden. When Ka's contextual ghost scanner duplicates a primary scan
// rule, the primary rule remains the cleanup owner so a user never sees the same
// object twice or receives a different cleanup action solely from scan order.
func NormalizeFindings(result *ScanResult) {
	if result == nil {
		return
	}

	result.Findings = coalesceFindings(result.Findings)
	result.TotalSize = 0
	result.ReclaimableSize = 0
	result.ByCategory = make(map[Category]CategorySummary)

	for _, f := range result.Findings {
		result.TotalSize += f.SizeBytes
		// ReclaimableSize is the user-facing waste headline. It excludes warning
		// findings and AI model weights, neither of which is eligible for a
		// one-click clean.
		if f.Severity != SeverityWarning && f.Category != CategoryAI {
			result.ReclaimableSize += f.SizeBytes
		}

		cat := result.ByCategory[f.Category]
		cat.Category = f.Category
		cat.Findings++
		cat.TotalSize += f.SizeBytes
		result.ByCategory[f.Category] = cat
	}

	// Stable ordering keeps persisted scans and the UI reproducible even though
	// rules execute concurrently.
	sort.Slice(result.Findings, func(i, j int) bool {
		if result.Findings[i].SizeBytes != result.Findings[j].SizeBytes {
			return result.Findings[i].SizeBytes > result.Findings[j].SizeBytes
		}
		if result.Findings[i].Path != result.Findings[j].Path {
			return result.Findings[i].Path < result.Findings[j].Path
		}
		return result.Findings[i].RuleName < result.Findings[j].RuleName
	})
}

func coalesceFindings(findings []Finding) []Finding {
	unique := make(map[string]int, len(findings))
	result := make([]Finding, 0, len(findings))
	for _, finding := range findings {
		cleanPath := filepath.Clean(finding.Path)
		// A missing path cannot safely be deduplicated. Keep it as a separate
		// diagnostic finding instead of merging unrelated scan failures.
		if finding.Path == "" || cleanPath == "." {
			result = append(result, finding)
			continue
		}
		finding.Path = cleanPath
		key := fmt.Sprintf("%s\x00%d\x00%d\x00%t", cleanPath, finding.SizeBytes, finding.FileCount, finding.IsDir)
		if existingIndex, ok := unique[key]; ok {
			result[existingIndex] = preferredFinding(result[existingIndex], finding)
			continue
		}
		unique[key] = len(result)
		result = append(result, finding)
	}
	return result
}

func preferredFinding(current, candidate Finding) Finding {
	// Ka supplies an additional explanation for a path. A primary rule owns the
	// remediation because it was registered for that exact artifact class.
	if current.RuleName == "ka_ghost" && candidate.RuleName != "ka_ghost" {
		return candidate
	}
	if candidate.RuleName == "ka_ghost" && current.RuleName != "ka_ghost" {
		return current
	}

	// Concurrent collection must not decide UI ownership. Prefer the most
	// restrictive severity, then a lexical rule name as a deterministic tie-break.
	if severityRank(candidate.Severity) > severityRank(current.Severity) {
		return candidate
	}
	if severityRank(candidate.Severity) == severityRank(current.Severity) && candidate.RuleName < current.RuleName {
		return candidate
	}
	return current
}

func severityRank(severity Severity) int {
	switch severity {
	case SeverityWarning:
		return 3
	case SeverityCaution:
		return 2
	case SeveritySafe:
		return 1
	default:
		return 0
	}
}

// Clean executes the clean phase for a set of findings.
// Findings are grouped by their source rule for efficient cleanup.
func (e *Engine) Clean(ctx context.Context, findings []Finding, opts CleanOptions) (*CleanResult, error) {
	if !opts.DryRun && !opts.Confirm {
		return nil, fmt.Errorf("clean requires either --dry-run or --confirm flag (Rule A1)")
	}

	total := &CleanResult{}

	// Group findings by rule name
	byRule := make(map[string][]Finding)
	for _, f := range findings {
		byRule[f.RuleName] = append(byRule[f.RuleName], f)
	}

	// Find the rule implementation for each group
	ruleMap := make(map[string]ScanRule)
	for _, rule := range e.rules {
		ruleMap[rule.Name()] = rule
	}

	for ruleName, ruleFindings := range byRule {
		rule, ok := ruleMap[ruleName]
		if !ok {
			total.Skipped += len(ruleFindings)
			total.Errors = append(total.Errors, fmt.Errorf("rule %q not found in registry", ruleName))
			continue
		}

		result, err := rule.Clean(ctx, ruleFindings, opts)
		if err != nil {
			total.Errors = append(total.Errors, fmt.Errorf("rule %s: %w", ruleName, err))
			continue
		}

		total.Cleaned += result.Cleaned
		total.BytesFreed += result.BytesFreed
		total.Skipped += result.Skipped
		total.Errors = append(total.Errors, result.Errors...)

		// Log each cleaned item
		if !opts.DryRun && result.Cleaned > 0 {
			for _, f := range ruleFindings {
				oplog.Log("clean", f.Path, f.SizeBytes)
			}
		}
	}

	return total, nil
}

// DefaultEngine creates a new engine. Rules must be registered by the caller
// using engine.RegisterAll(). See cmd/anubis/weigh.go for registration.
func DefaultEngine() *Engine {
	return NewEngine()
}
