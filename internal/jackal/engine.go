package jackal

import (
	"context"
	"fmt"
	"runtime"
	"sort"
	"sync"
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

	// Total size in bytes
	TotalSize int64

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
	for rr := range ch {
		if rr.err != nil {
			result.Errors = append(result.Errors, RuleError{
				RuleName: rr.ruleName,
				Err:      rr.err,
			})
			continue
		}

		if len(rr.findings) > 0 {
			result.RulesWithFindings++
			result.Findings = append(result.Findings, rr.findings...)

			for _, f := range rr.findings {
				result.TotalSize += f.SizeBytes

				cat := result.ByCategory[f.Category]
				cat.Category = f.Category
				cat.Findings++
				cat.TotalSize += f.SizeBytes
				result.ByCategory[f.Category] = cat
			}
		}
	}

	// Sort findings by size (largest first)
	sort.Slice(result.Findings, func(i, j int) bool {
		return result.Findings[i].SizeBytes > result.Findings[j].SizeBytes
	})

	return result, nil
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
	}

	return total, nil
}

// DefaultEngine creates a new engine. Rules must be registered by the caller
// using engine.RegisterAll(). See cmd/anubis/weigh.go for registration.
func DefaultEngine() *Engine {
	return NewEngine()
}
