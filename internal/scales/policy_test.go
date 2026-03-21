package scales

import (
	"testing"
)

func TestParsePolicy(t *testing.T) {
	yaml := `
api_version: v1
policies:
  - name: test-policy
    description: Test policy
    version: "1.0"
    rules:
      - id: waste-limit
        name: Waste limit
        description: Fail if waste exceeds 10 GB
        metric: total_size
        operator: gt
        threshold: 10
        unit: GB
        severity: fail
        remediation: Clean up
`
	pf, err := ParsePolicy([]byte(yaml))
	if err != nil {
		t.Fatalf("ParsePolicy error: %v", err)
	}

	if len(pf.Policies) != 1 {
		t.Fatalf("Expected 1 policy, got %d", len(pf.Policies))
	}

	p := pf.Policies[0]
	if p.Name != "test-policy" {
		t.Errorf("Name = %q, want %q", p.Name, "test-policy")
	}
	if len(p.Rules) != 1 {
		t.Fatalf("Expected 1 rule, got %d", len(p.Rules))
	}

	r := p.Rules[0]
	if r.ID != "waste-limit" {
		t.Errorf("Rule ID = %q, want %q", r.ID, "waste-limit")
	}
	if r.Severity != SeverityFail {
		t.Errorf("Severity = %q, want %q", r.Severity, SeverityFail)
	}
}

func TestParsePolicy_Empty(t *testing.T) {
	yaml := `api_version: v1
policies: []`

	_, err := ParsePolicy([]byte(yaml))
	if err == nil {
		t.Error("Expected error for empty policies")
	}
}

func TestParsePolicy_Invalid(t *testing.T) {
	_, err := ParsePolicy([]byte("not valid yaml: [[["))
	if err == nil {
		t.Error("Expected error for invalid YAML")
	}
}

func TestValidateOperator(t *testing.T) {
	tests := []struct {
		op    string
		valid bool
	}{
		{"gt", true},
		{"lt", true},
		{"gte", true},
		{"lte", true},
		{"eq", true},
		{"ne", false},
		{"", false},
		{"invalid", false},
	}

	for _, tt := range tests {
		t.Run(tt.op, func(t *testing.T) {
			if isValidOperator(tt.op) != tt.valid {
				t.Errorf("isValidOperator(%q) = %v, want %v", tt.op, !tt.valid, tt.valid)
			}
		})
	}
}

func TestValidateSeverity(t *testing.T) {
	tests := []struct {
		sev   Severity
		valid bool
	}{
		{SeverityPass, true},
		{SeverityWarn, true},
		{SeverityFail, true},
		{"critical", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(string(tt.sev), func(t *testing.T) {
			if isValidSeverity(tt.sev) != tt.valid {
				t.Errorf("isValidSeverity(%q) = %v, want %v", tt.sev, !tt.valid, tt.valid)
			}
		})
	}
}

func TestValidateMetric(t *testing.T) {
	tests := []struct {
		metric string
		valid  bool
	}{
		{"total_size", true},
		{"finding_count", true},
		{"ghost_count", true},
		{"cpu_usage", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.metric, func(t *testing.T) {
			if isValidMetric(tt.metric) != tt.valid {
				t.Errorf("isValidMetric(%q) = %v, want %v", tt.metric, !tt.valid, tt.valid)
			}
		})
	}
}

func TestNormalizeThreshold(t *testing.T) {
	tests := []struct {
		threshold int64
		unit      string
		expected  int64
	}{
		{1, "KB", 1024},
		{1, "MB", 1024 * 1024},
		{1, "GB", 1024 * 1024 * 1024},
		{5, "GB", 5 * 1024 * 1024 * 1024},
		{100, "", 100},
		{42, "bytes", 42},
		{10, "count", 10},
	}

	for _, tt := range tests {
		t.Run(tt.unit, func(t *testing.T) {
			rule := PolicyRule{Threshold: tt.threshold, Unit: tt.unit}
			got := NormalizeThreshold(rule)
			if got != tt.expected {
				t.Errorf("NormalizeThreshold(%d %s) = %d, want %d", tt.threshold, tt.unit, got, tt.expected)
			}
		})
	}
}

func TestCompareValue(t *testing.T) {
	tests := []struct {
		actual    int64
		threshold int64
		op        string
		expected  bool
	}{
		{10, 5, "gt", true},
		{5, 10, "gt", false},
		{5, 5, "gt", false},
		{3, 5, "lt", true},
		{5, 3, "lt", false},
		{5, 5, "gte", true},
		{6, 5, "gte", true},
		{4, 5, "gte", false},
		{5, 5, "lte", true},
		{4, 5, "lte", true},
		{6, 5, "lte", false},
		{5, 5, "eq", true},
		{4, 5, "eq", false},
	}

	for _, tt := range tests {
		got := compareValue(tt.actual, tt.threshold, tt.op)
		if got != tt.expected {
			t.Errorf("compareValue(%d, %d, %q) = %v, want %v",
				tt.actual, tt.threshold, tt.op, got, tt.expected)
		}
	}
}

func TestEnforceWithMetrics(t *testing.T) {
	policy := Policy{
		Name: "test",
		Rules: []PolicyRule{
			{
				ID:        "size-warn",
				Name:      "Size warning",
				Metric:    "total_size",
				Operator:  "gt",
				Threshold: 1,
				Unit:      "GB",
				Severity:  SeverityWarn,
			},
			{
				ID:        "ghosts-fail",
				Name:      "Ghost limit",
				Metric:    "ghost_count",
				Operator:  "gt",
				Threshold: 100,
				Severity:  SeverityFail,
			},
		},
	}

	metrics := &ScanMetrics{
		TotalSize:    2 * 1024 * 1024 * 1024, // 2 GB — should trigger warn
		FindingCount: 50,
		GhostCount:   30, // under 100 — should pass
	}

	result := EnforceWithMetrics(policy, metrics)

	if result.PolicyName != "test" {
		t.Errorf("PolicyName = %q, want %q", result.PolicyName, "test")
	}
	if len(result.Verdicts) != 2 {
		t.Fatalf("Expected 2 verdicts, got %d", len(result.Verdicts))
	}

	// Size warn should be breached
	sizeVerdict := result.Verdicts[0]
	if sizeVerdict.Passed {
		t.Error("Size warn verdict should NOT pass (2 GB > 1 GB)")
	}
	if sizeVerdict.Severity != SeverityWarn {
		t.Errorf("Size verdict severity = %q, want %q", sizeVerdict.Severity, SeverityWarn)
	}

	// Ghost limit should pass
	ghostVerdict := result.Verdicts[1]
	if !ghostVerdict.Passed {
		t.Error("Ghost verdict should pass (30 < 100)")
	}

	// Overall should pass (warnings don't cause failure)
	if !result.OverallPass {
		t.Error("OverallPass should be true (warnings only)")
	}
	if result.Warnings != 1 {
		t.Errorf("Warnings = %d, want 1", result.Warnings)
	}
	if result.Passes != 1 {
		t.Errorf("Passes = %d, want 1", result.Passes)
	}
}

func TestEnforceWithMetrics_Failure(t *testing.T) {
	policy := Policy{
		Name: "strict",
		Rules: []PolicyRule{
			{
				ID:        "size-fail",
				Name:      "Size critical",
				Metric:    "total_size",
				Operator:  "gt",
				Threshold: 1,
				Unit:      "MB",
				Severity:  SeverityFail,
			},
		},
	}

	metrics := &ScanMetrics{
		TotalSize: 10 * 1024 * 1024, // 10 MB — exceeds 1 MB threshold
	}

	result := EnforceWithMetrics(policy, metrics)

	if result.OverallPass {
		t.Error("OverallPass should be false when a fail verdict is breached")
	}
	if result.Failures != 1 {
		t.Errorf("Failures = %d, want 1", result.Failures)
	}
}

func TestEnforceWithMetrics_AllPass(t *testing.T) {
	policy := Policy{
		Name: "lenient",
		Rules: []PolicyRule{
			{
				ID:        "size-check",
				Name:      "Size check",
				Metric:    "total_size",
				Operator:  "gt",
				Threshold: 100,
				Unit:      "GB",
				Severity:  SeverityFail,
			},
		},
	}

	metrics := &ScanMetrics{
		TotalSize: 1024 * 1024, // 1 MB — well under 100 GB
	}

	result := EnforceWithMetrics(policy, metrics)

	if !result.OverallPass {
		t.Error("OverallPass should be true when all rules pass")
	}
	if result.Passes != 1 {
		t.Errorf("Passes = %d, want 1", result.Passes)
	}
}

func TestDefaultPolicy(t *testing.T) {
	pf := DefaultPolicy()
	if pf == nil {
		t.Fatal("DefaultPolicy returned nil")
	}
	if len(pf.Policies) == 0 {
		t.Fatal("DefaultPolicy has no policies")
	}

	p := pf.Policies[0]
	if p.Name == "" {
		t.Error("Default policy should have a name")
	}
	if len(p.Rules) < 2 {
		t.Errorf("Default policy should have at least 2 rules, got %d", len(p.Rules))
	}
}

func TestFormatVerdict(t *testing.T) {
	pass := Verdict{Passed: true, Message: "test passed"}
	result := FormatVerdict(pass)
	if result == "" {
		t.Error("FormatVerdict should return non-empty string")
	}

	warn := Verdict{Passed: false, Severity: SeverityWarn, Message: "test warned"}
	result = FormatVerdict(warn)
	if result == "" {
		t.Error("FormatVerdict should return non-empty string for warn")
	}

	fail := Verdict{Passed: false, Severity: SeverityFail, Message: "test failed"}
	result = FormatVerdict(fail)
	if result == "" {
		t.Error("FormatVerdict should return non-empty string for fail")
	}
}
