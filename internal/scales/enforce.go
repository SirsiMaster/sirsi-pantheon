package scales

import (
	"context"
	"fmt"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/jackal"
	"github.com/SirsiMaster/sirsi-pantheon/internal/jackal/rules"
	"github.com/SirsiMaster/sirsi-pantheon/internal/ka"
	"sync"
)

var (
	metricsCollector   func() (*ScanMetrics, error) = CollectMetrics
	metricsCollectorMu sync.RWMutex
)

// SetMetricsCollector allows overriding the metrics collection logic (for testing).
// Compliant with Rule A21 (Concurrency-Safe Injectable Mocks).
func SetMetricsCollector(collector func() (*ScanMetrics, error)) {
	metricsCollectorMu.Lock()
	defer metricsCollectorMu.Unlock()
	metricsCollector = collector
}

func getMetricsCollector() func() (*ScanMetrics, error) {
	metricsCollectorMu.RLock()
	defer metricsCollectorMu.RUnlock()
	return metricsCollector
}

var (
	newJackalEngine = func() *jackal.Engine {
		e := jackal.NewEngine()
		e.RegisterAll(rules.AllRules()...)
		return e
	}
	newKaScanner = func() *ka.Scanner {
		s := ka.NewScanner()
		s.SkipLaunchServices = true // Default for speed in metrics
		return s
	}
	factoryMu sync.RWMutex
)

// SetScannerFactories allows overriding the engines used by CollectMetrics (for testing).
func SetScannerFactories(
	jackalFactory func() *jackal.Engine,
	kaFactory func() *ka.Scanner,
) {
	factoryMu.Lock()
	defer factoryMu.Unlock()
	newJackalEngine = jackalFactory
	newKaScanner = kaFactory
}

func getJackalEngine() *jackal.Engine {
	factoryMu.RLock()
	defer factoryMu.RUnlock()
	return newJackalEngine()
}

func getKaScanner() *ka.Scanner {
	factoryMu.RLock()
	defer factoryMu.RUnlock()
	return newKaScanner()
}

// Verdict is the result of evaluating a single policy rule.
type Verdict struct {
	RuleID      string   `json:"rule_id"`
	RuleName    string   `json:"rule_name"`
	Severity    Severity `json:"severity"`
	Passed      bool     `json:"passed"`
	Metric      string   `json:"metric"`
	ActualValue int64    `json:"actual_value"`
	Threshold   int64    `json:"threshold"`
	Unit        string   `json:"unit,omitempty"`
	Remediation string   `json:"remediation,omitempty"`
	Message     string   `json:"message"`
}

// EnforceResult is the aggregated result of enforcing a policy.
type EnforceResult struct {
	PolicyName  string    `json:"policy_name"`
	Verdicts    []Verdict `json:"verdicts"`
	OverallPass bool      `json:"overall_pass"`
	Warnings    int       `json:"warnings"`
	Failures    int       `json:"failures"`
	Passes      int       `json:"passes"`
	EvaluatedAt time.Time `json:"evaluated_at"`
}

// ScanMetrics holds the measured values from a scan for policy evaluation.
type ScanMetrics struct {
	TotalSize    int64 `json:"total_size"`
	FindingCount int   `json:"finding_count"`
	GhostCount   int   `json:"ghost_count"`
}

// CollectMetrics runs the necessary scans to gather current metrics.
func CollectMetrics() (*ScanMetrics, error) {
	metrics := &ScanMetrics{}

	// Run Jackal scan for waste metrics
	engine := getJackalEngine()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := engine.Scan(ctx, jackal.ScanOptions{})
	if err != nil {
		return nil, fmt.Errorf("scan for metrics: %w", err)
	}

	metrics.TotalSize = result.TotalSize
	metrics.FindingCount = len(result.Findings)

	// Run Ka scan for ghost count
	scanner := getKaScanner()
	ghosts, err := scanner.Scan(false) // no sudo
	if err == nil {
		metrics.GhostCount = len(ghosts)
	}

	return metrics, nil
}

// Enforce evaluates a policy against the current system state.
func Enforce(policy Policy) (*EnforceResult, error) {
	collector := getMetricsCollector()
	metrics, err := collector()
	if err != nil {
		return nil, fmt.Errorf("collect metrics: %w", err)
	}

	return EnforceWithMetrics(policy, metrics), nil
}

// EnforceWithMetrics evaluates a policy against provided metrics (for testing).
func EnforceWithMetrics(policy Policy, metrics *ScanMetrics) *EnforceResult {
	result := &EnforceResult{
		PolicyName:  policy.Name,
		OverallPass: true,
		EvaluatedAt: time.Now(),
	}

	for _, rule := range policy.Rules {
		verdict := evaluateRule(rule, metrics)
		result.Verdicts = append(result.Verdicts, verdict)

		switch {
		case verdict.Passed:
			result.Passes++
		case verdict.Severity == SeverityWarn:
			result.Warnings++
		case verdict.Severity == SeverityFail:
			result.Failures++
			result.OverallPass = false
		}
	}

	return result
}

// evaluateRule checks a single policy rule against metrics.
func evaluateRule(rule PolicyRule, metrics *ScanMetrics) Verdict {
	// Get the actual metric value
	var actual int64
	switch rule.Metric {
	case "total_size":
		actual = metrics.TotalSize
	case "finding_count":
		actual = int64(metrics.FindingCount)
	case "ghost_count":
		actual = int64(metrics.GhostCount)
	default:
		return Verdict{
			RuleID:   rule.ID,
			RuleName: rule.Name,
			Severity: SeverityFail,
			Passed:   false,
			Message:  fmt.Sprintf("Unknown metric: %s", rule.Metric),
		}
	}

	// Normalize threshold (convert units)
	threshold := NormalizeThreshold(rule)

	// Evaluate the comparison
	breached := compareValue(actual, threshold, rule.Operator)

	verdict := Verdict{
		RuleID:      rule.ID,
		RuleName:    rule.Name,
		Severity:    rule.Severity,
		Passed:      !breached,
		Metric:      rule.Metric,
		ActualValue: actual,
		Threshold:   threshold,
		Unit:        rule.Unit,
		Remediation: rule.Remediation,
	}

	if breached {
		verdict.Message = fmt.Sprintf("%s: %s %s %d %s (actual: %d)",
			rule.Name, rule.Metric, rule.Operator, rule.Threshold, rule.Unit, actual)
	} else {
		verdict.Message = fmt.Sprintf("%s: within threshold", rule.Name)
	}

	return verdict
}

// compareValue performs the comparison: returns true if "actual op threshold" is true.
func compareValue(actual, threshold int64, op string) bool {
	switch op {
	case "gt":
		return actual > threshold
	case "lt":
		return actual < threshold
	case "gte":
		return actual >= threshold
	case "lte":
		return actual <= threshold
	case "eq":
		return actual == threshold
	default:
		return false
	}
}

// FormatVerdict returns a human-readable string for a verdict.
func FormatVerdict(v Verdict) string {
	icon := "✅"
	if !v.Passed {
		switch v.Severity {
		case SeverityWarn:
			icon = "⚠️"
		case SeverityFail:
			icon = "❌"
		}
	}
	return fmt.Sprintf("%s %s", icon, v.Message)
}
