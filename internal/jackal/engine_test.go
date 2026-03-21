package jackal

import (
	"context"
	"fmt"
	"testing"
)

// mockRule implements ScanRule for testing.
type mockRule struct {
	name      string
	category  Category
	platforms []string
	findings  []Finding
	scanErr   error
}

func (m *mockRule) Name() string        { return m.name }
func (m *mockRule) DisplayName() string { return m.name }
func (m *mockRule) Category() Category  { return m.category }
func (m *mockRule) Description() string { return "mock rule for testing" }
func (m *mockRule) Platforms() []string { return m.platforms }

func (m *mockRule) Scan(_ context.Context, _ ScanOptions) ([]Finding, error) {
	if m.scanErr != nil {
		return nil, m.scanErr
	}
	return m.findings, nil
}

func (m *mockRule) Clean(_ context.Context, findings []Finding, opts CleanOptions) (*CleanResult, error) {
	if !opts.DryRun && !opts.Confirm {
		return nil, fmt.Errorf("requires --dry-run or --confirm")
	}
	return &CleanResult{
		Cleaned:    len(findings),
		BytesFreed: 1024,
	}, nil
}

func TestEngine_RegisterAndRules(t *testing.T) {
	e := NewEngine()
	if len(e.Rules()) != 0 {
		t.Fatalf("new engine has %d rules, want 0", len(e.Rules()))
	}

	r1 := &mockRule{name: "rule1", platforms: []string{"darwin", "linux"}}
	r2 := &mockRule{name: "rule2", platforms: []string{"darwin"}}
	e.RegisterAll(r1, r2)

	rules := e.Rules()
	if len(rules) != 2 {
		t.Fatalf("engine has %d rules, want 2", len(rules))
	}
	if rules[0].Name() != "rule1" || rules[1].Name() != "rule2" {
		t.Errorf("rules = [%s, %s], want [rule1, rule2]", rules[0].Name(), rules[1].Name())
	}
}

func TestEngine_ScanEmpty(t *testing.T) {
	e := NewEngine()
	result, err := e.Scan(context.Background(), ScanOptions{})
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if len(result.Findings) != 0 {
		t.Errorf("empty engine produced %d findings", len(result.Findings))
	}
	if result.RulesRan != 0 {
		t.Errorf("RulesRan = %d, want 0", result.RulesRan)
	}
}

func TestEngine_ScanWithFindings(t *testing.T) {
	e := NewEngine()

	e.Register(&mockRule{
		name:      "test_rule",
		category:  CategoryGeneral,
		platforms: []string{"darwin", "linux", "windows"},
		findings: []Finding{
			{RuleName: "test_rule", Category: CategoryGeneral, SizeBytes: 1000, Path: "/tmp/a"},
			{RuleName: "test_rule", Category: CategoryGeneral, SizeBytes: 5000, Path: "/tmp/b"},
		},
	})

	result, err := e.Scan(context.Background(), ScanOptions{})
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}

	if result.RulesRan != 1 {
		t.Errorf("RulesRan = %d, want 1", result.RulesRan)
	}
	if result.RulesWithFindings != 1 {
		t.Errorf("RulesWithFindings = %d, want 1", result.RulesWithFindings)
	}
	if len(result.Findings) != 2 {
		t.Errorf("len(Findings) = %d, want 2", len(result.Findings))
	}
	if result.TotalSize != 6000 {
		t.Errorf("TotalSize = %d, want 6000", result.TotalSize)
	}
	// Findings should be sorted largest first
	if result.Findings[0].SizeBytes != 5000 {
		t.Errorf("first finding size = %d, want 5000 (largest first)", result.Findings[0].SizeBytes)
	}
}

func TestEngine_ScanWithErrors(t *testing.T) {
	e := NewEngine()
	e.Register(&mockRule{
		name:      "failing_rule",
		category:  CategoryDev,
		platforms: []string{"darwin", "linux", "windows"},
		scanErr:   fmt.Errorf("permission denied"),
	})

	result, err := e.Scan(context.Background(), ScanOptions{})
	if err != nil {
		t.Fatalf("Scan() error = %v (should be non-fatal)", err)
	}
	if len(result.Errors) != 1 {
		t.Errorf("len(Errors) = %d, want 1", len(result.Errors))
	}
	if result.Errors[0].RuleName != "failing_rule" {
		t.Errorf("error rule = %q, want failing_rule", result.Errors[0].RuleName)
	}
}

func TestEngine_CleanRequiresFlag(t *testing.T) {
	e := NewEngine()
	_, err := e.Clean(context.Background(), nil, CleanOptions{})
	if err == nil {
		t.Error("Clean() without --dry-run or --confirm should fail")
	}
}

func TestEngine_CleanDryRun(t *testing.T) {
	e := NewEngine()
	rule := &mockRule{
		name:      "test_rule",
		category:  CategoryGeneral,
		platforms: []string{"darwin", "linux", "windows"},
	}
	e.Register(rule)

	findings := []Finding{
		{RuleName: "test_rule", Path: "/tmp/test", SizeBytes: 1024},
	}
	result, err := e.Clean(context.Background(), findings, CleanOptions{DryRun: true})
	if err != nil {
		t.Fatalf("Clean(dry-run) error = %v", err)
	}
	if result.Cleaned != 1 {
		t.Errorf("Cleaned = %d, want 1", result.Cleaned)
	}
}

func TestEngine_CategoryFilter(t *testing.T) {
	e := NewEngine()
	e.Register(&mockRule{
		name:      "general_rule",
		category:  CategoryGeneral,
		platforms: []string{"darwin", "linux", "windows"},
		findings:  []Finding{{RuleName: "general_rule", SizeBytes: 100}},
	})
	e.Register(&mockRule{
		name:      "dev_rule",
		category:  CategoryDev,
		platforms: []string{"darwin", "linux", "windows"},
		findings:  []Finding{{RuleName: "dev_rule", SizeBytes: 200}},
	})

	// Filter to dev only
	result, err := e.Scan(context.Background(), ScanOptions{
		Categories: []Category{CategoryDev},
	})
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if result.RulesRan != 1 {
		t.Errorf("RulesRan = %d, want 1 (only dev)", result.RulesRan)
	}
	if len(result.Findings) != 1 {
		t.Errorf("len(Findings) = %d, want 1", len(result.Findings))
	}
}
