// Package scales implements the Scales policy engine for Anubis.
// Named after the Scales of Ma'at — the Egyptian scales of justice that
// weighed the hearts of the dead against the feather of truth.
//
// Policies define thresholds, rules, and notification targets for
// infrastructure hygiene enforcement across workstations and fleets.
package scales

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Severity levels for policy verdicts.
type Severity string

const (
	SeverityPass Severity = "pass"
	SeverityWarn Severity = "warn"
	SeverityFail Severity = "fail"
)

// Policy is a named set of rules and thresholds for infrastructure hygiene.
type Policy struct {
	Name        string         `yaml:"name" json:"name"`
	Description string         `yaml:"description" json:"description"`
	Version     string         `yaml:"version" json:"version"`
	Rules       []PolicyRule   `yaml:"rules" json:"rules"`
	Notify      []NotifyTarget `yaml:"notify,omitempty" json:"notify,omitempty"`
}

// PolicyRule is a single threshold or condition in a policy.
type PolicyRule struct {
	ID          string   `yaml:"id" json:"id"`
	Name        string   `yaml:"name" json:"name"`
	Description string   `yaml:"description" json:"description"`
	Category    string   `yaml:"category" json:"category"`                           // scan category to check
	Metric      string   `yaml:"metric" json:"metric"`                               // "total_size", "finding_count", "ghost_count"
	Operator    string   `yaml:"operator" json:"operator"`                           // "gt", "lt", "gte", "lte", "eq"
	Threshold   int64    `yaml:"threshold" json:"threshold"`                         // threshold value
	Unit        string   `yaml:"unit,omitempty" json:"unit,omitempty"`               // "bytes", "count", "MB", "GB"
	Severity    Severity `yaml:"severity" json:"severity"`                           // verdict if threshold breached
	Remediation string   `yaml:"remediation,omitempty" json:"remediation,omitempty"` // suggested fix
	AutoClean   bool     `yaml:"auto_clean,omitempty" json:"auto_clean,omitempty"`   // auto-remediate (with approval)
}

// NotifyTarget is a notification destination for policy violations.
type NotifyTarget struct {
	Type    string `yaml:"type" json:"type"` // "slack", "teams", "webhook", "stdout"
	URL     string `yaml:"url,omitempty" json:"url,omitempty"`
	Channel string `yaml:"channel,omitempty" json:"channel,omitempty"`
}

// PolicyFile is the top-level structure of a policy YAML file.
type PolicyFile struct {
	APIVersion string   `yaml:"api_version" json:"api_version"`
	Policies   []Policy `yaml:"policies" json:"policies"`
}

// LoadPolicyFile reads and parses a policy YAML file.
func LoadPolicyFile(path string) (*PolicyFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read policy file: %w", err)
	}

	return ParsePolicy(data)
}

// ParsePolicy parses policy YAML from bytes.
func ParsePolicy(data []byte) (*PolicyFile, error) {
	var pf PolicyFile
	if err := yaml.Unmarshal(data, &pf); err != nil {
		return nil, fmt.Errorf("parse policy YAML: %w", err)
	}

	// Validate
	if err := validatePolicyFile(&pf); err != nil {
		return nil, err
	}

	return &pf, nil
}

// ValidatePolicy checks a policy file for correctness.
func ValidatePolicy(path string) []ValidationError {
	pf, err := LoadPolicyFile(path)
	if err != nil {
		return []ValidationError{{
			Path:    path,
			Message: err.Error(),
		}}
	}

	return validatePolicies(pf)
}

// ValidationError describes a problem found in a policy file.
type ValidationError struct {
	Path       string `json:"path"`
	PolicyName string `json:"policy_name,omitempty"`
	RuleID     string `json:"rule_id,omitempty"`
	Field      string `json:"field,omitempty"`
	Message    string `json:"message"`
}

// validatePolicyFile does basic structural validation.
func validatePolicyFile(pf *PolicyFile) error {
	if len(pf.Policies) == 0 {
		return fmt.Errorf("policy file contains no policies")
	}
	return nil
}

// validatePolicies does deep validation of all policies.
func validatePolicies(pf *PolicyFile) []ValidationError {
	var errs []ValidationError

	for _, p := range pf.Policies {
		if p.Name == "" {
			errs = append(errs, ValidationError{
				PolicyName: "(unnamed)",
				Field:      "name",
				Message:    "policy must have a name",
			})
		}

		if len(p.Rules) == 0 {
			errs = append(errs, ValidationError{
				PolicyName: p.Name,
				Message:    "policy must have at least one rule",
			})
		}

		ruleIDs := make(map[string]bool)
		for _, r := range p.Rules {
			// Duplicate ID check
			if r.ID != "" {
				if ruleIDs[r.ID] {
					errs = append(errs, ValidationError{
						PolicyName: p.Name,
						RuleID:     r.ID,
						Field:      "id",
						Message:    fmt.Sprintf("duplicate rule ID: %s", r.ID),
					})
				}
				ruleIDs[r.ID] = true
			}

			// Validate operator
			if !isValidOperator(r.Operator) {
				errs = append(errs, ValidationError{
					PolicyName: p.Name,
					RuleID:     r.ID,
					Field:      "operator",
					Message:    fmt.Sprintf("invalid operator %q (valid: gt, lt, gte, lte, eq)", r.Operator),
				})
			}

			// Validate severity
			if !isValidSeverity(r.Severity) {
				errs = append(errs, ValidationError{
					PolicyName: p.Name,
					RuleID:     r.ID,
					Field:      "severity",
					Message:    fmt.Sprintf("invalid severity %q (valid: pass, warn, fail)", r.Severity),
				})
			}

			// Validate metric
			if !isValidMetric(r.Metric) {
				errs = append(errs, ValidationError{
					PolicyName: p.Name,
					RuleID:     r.ID,
					Field:      "metric",
					Message:    fmt.Sprintf("invalid metric %q (valid: total_size, finding_count, ghost_count)", r.Metric),
				})
			}
		}
	}

	return errs
}

func isValidOperator(op string) bool {
	switch op {
	case "gt", "lt", "gte", "lte", "eq":
		return true
	}
	return false
}

func isValidSeverity(s Severity) bool {
	switch s {
	case SeverityPass, SeverityWarn, SeverityFail:
		return true
	}
	return false
}

func isValidMetric(m string) bool {
	switch m {
	case "total_size", "finding_count", "ghost_count":
		return true
	}
	return false
}

// NormalizeThreshold converts the threshold to bytes if the unit is a size unit.
func NormalizeThreshold(rule PolicyRule) int64 {
	switch strings.ToUpper(rule.Unit) {
	case "KB":
		return rule.Threshold * 1024
	case "MB":
		return rule.Threshold * 1024 * 1024
	case "GB":
		return rule.Threshold * 1024 * 1024 * 1024
	case "TB":
		return rule.Threshold * 1024 * 1024 * 1024 * 1024
	default:
		return rule.Threshold // bytes or count
	}
}

// DefaultPolicy returns the built-in default policy for local workstation scans.
func DefaultPolicy() *PolicyFile {
	return &PolicyFile{
		APIVersion: "v1",
		Policies: []Policy{
			{
				Name:        "workstation-hygiene",
				Description: "Default infrastructure hygiene policy for developer workstations",
				Version:     "1.0.0",
				Rules: []PolicyRule{
					{
						ID:          "waste-warning",
						Name:        "Infrastructure waste warning",
						Description: "Warn when total infrastructure waste exceeds 5 GB",
						Metric:      "total_size",
						Operator:    "gt",
						Threshold:   5,
						Unit:        "GB",
						Severity:    SeverityWarn,
						Remediation: "Run 'anubis judge --dry-run' to review cleanup opportunities",
					},
					{
						ID:          "waste-critical",
						Name:        "Infrastructure waste critical",
						Description: "Fail when total infrastructure waste exceeds 20 GB",
						Metric:      "total_size",
						Operator:    "gt",
						Threshold:   20,
						Unit:        "GB",
						Severity:    SeverityFail,
						Remediation: "Run 'anubis judge --confirm' to clean up immediately",
					},
					{
						ID:          "ghost-warning",
						Name:        "Ghost app accumulation",
						Description: "Warn when more than 50 ghost apps are detected",
						Metric:      "ghost_count",
						Operator:    "gt",
						Threshold:   50,
						Severity:    SeverityWarn,
						Remediation: "Run 'anubis ka --clean --dry-run' to review ghost cleanup",
					},
					{
						ID:          "findings-warning",
						Name:        "High finding count",
						Description: "Warn when scan produces more than 100 findings",
						Metric:      "finding_count",
						Operator:    "gt",
						Threshold:   100,
						Severity:    SeverityWarn,
						Remediation: "Review findings with 'anubis weigh' and clean selectively",
					},
				},
			},
		},
	}
}
