package stacklab

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// TestMaatSystemOneRecipeIsComplete keeps the Stack Lab recipe from becoming
// prose that drifts away from code. Every listed source and test path must
// exist, and every independently upgradeable component must name its inputs,
// outputs, and safe upgrade recipe.
func TestMaatSystemOneRecipeIsComplete(t *testing.T) {
	type component struct {
		ID            string   `json:"id"`
		Source        []string `json:"source"`
		Tests         []string `json:"tests"`
		Inputs        []string `json:"inputs"`
		Outputs       []string `json:"outputs"`
		UpgradeRecipe []string `json:"upgrade_recipe"`
	}
	var recipe struct {
		Schema     string      `json:"schema"`
		ID         string      `json:"id"`
		Wing       string      `json:"wing"`
		Components []component `json:"components"`
	}
	path := filepath.Join("..", "..", "contracts", "stacklab", "maat-system-one-recipe-v1.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if unmarshalErr := json.Unmarshal(raw, &recipe); unmarshalErr != nil {
		t.Fatal(unmarshalErr)
	}
	if recipe.Schema != "sirsi.stacklab.recipe.v1" || recipe.ID != "stacklab.recipe.maat-system-one" || recipe.Wing != "stacklab.wing.maat" {
		t.Fatalf("recipe identity = %+v", recipe)
	}
	want := []string{
		"maat-canon", "maat-casebook", "maat-cli", "maat-confirmed-diagnostic-repair", "maat-core", "maat-coverage",
		"maat-decision-journal", "maat-guided-managed-repair", "maat-horus-surface", "maat-knowledge-surface", "maat-mcp-surface", "maat-native-resolution-surface", "maat-pipeline", "maat-pulse-proof-platform",
		"maat-host-health-screen", "maat-release-contract-preflight", "maat-release-credential-preflight", "maat-scheduler", "maat-system-one-screen", "maat-terminal-console-surface", "maat-wing-contract", "stacklab-apollo-run-planner",
	}
	got := make([]string, 0, len(recipe.Components))
	for _, component := range recipe.Components {
		got = append(got, component.ID)
		if len(component.Source) == 0 || len(component.Tests) == 0 || len(component.Inputs) == 0 || len(component.Outputs) == 0 || len(component.UpgradeRecipe) == 0 {
			t.Fatalf("component %q is not independently upgradeable: %+v", component.ID, component)
		}
		for _, listed := range append(append([]string{}, component.Source...), component.Tests...) {
			if _, statErr := os.Stat(filepath.Join("..", "..", listed)); statErr != nil {
				t.Fatalf("component %q references missing path %q: %v", component.ID, listed, statErr)
			}
		}
	}
	sort.Strings(got)
	sort.Strings(want)
	if len(got) != len(want) {
		t.Fatalf("component ids = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("component ids = %v, want %v", got, want)
		}
	}
	componentsByID := make(map[string]component, len(recipe.Components))
	for _, component := range recipe.Components {
		componentsByID[component.ID] = component
	}
	knowledge, ok := componentsByID["maat-knowledge-surface"]
	if !ok {
		t.Fatal("Ma'at knowledge component is missing")
	}
	for _, path := range []string{
		"internal/maat/knowledge/knowledge.go",
		"cmd/sirsi/maatknowledge.go",
		"internal/mcp/tools.go",
		"internal/dashboard/maat.go",
		"cmd/sirsi/dashboard.go",
		"macapp/Sources/SirsiMenubar/MaatCasebookView.swift",
		"internal/maat/knowledge/knowledge_test.go",
		"internal/mcp/maat_test.go",
		"internal/dashboard/dashboard_test.go",
	} {
		if !contains(append(append([]string{}, knowledge.Source...), knowledge.Tests...), path) {
			t.Fatalf("Ma'at knowledge recipe omits canonical surface %q: %+v", path, knowledge)
		}
	}
	mcp, ok := componentsByID["maat-mcp-surface"]
	if !ok {
		t.Fatal("Ma'at MCP component is missing")
	}
	if !contains(mcp.Outputs, "read-only maat_knowledge MCP JSON projection equivalent to the CLI and Horus knowledge view") {
		t.Fatalf("Ma'at MCP recipe omits the shared knowledge projection: %+v", mcp.Outputs)
	}
	horus, ok := componentsByID["maat-horus-surface"]
	if !ok {
		t.Fatal("Ma'at Horus component is missing")
	}
	if !contains(horus.Inputs, "MaatKnowledgeProducer") || !contains(horus.Outputs, "GET /api/maat/knowledge with the same sensitivity-filtered local view as CLI and MCP") {
		t.Fatalf("Ma'at Horus recipe omits the shared knowledge projection: %+v", horus)
	}
	credentials, ok := componentsByID["maat-release-credential-preflight"]
	if !ok {
		t.Fatal("Ma'at release credential preflight component is missing")
	}
	for _, path := range []string{
		"internal/maat/credentialpreflight.go",
		"cmd/sirsi/maatpreflight.go",
		"macapp/Sources/SirsiMenubar/MaatCasebookView.swift",
		"internal/maat/credentialpreflight_test.go",
		"scripts/verify-maat-credential-preflight-contract.sh",
	} {
		if !contains(append(append([]string{}, credentials.Source...), credentials.Tests...), path) {
			t.Fatalf("Ma'at credential recipe omits canonical surface %q: %+v", path, credentials)
		}
	}
	maatCLI, ok := componentsByID["maat-cli"]
	if !ok {
		t.Fatal("Ma'at CLI component is missing")
	}
	if !contains(maatCLI.Tests, "cmd/sirsi/maat_scales_test.go") ||
		!contains(maatCLI.Outputs, "post-heal observation failures retain a same-policy read-only recheck and Casebook resolution path") ||
		!contains(maatCLI.UpgradeRecipe, "never repeat a mutation after an unverified post-heal observation; offer re-observation plus retained evidence") {
		t.Fatalf("Ma'at CLI recipe omits post-heal recovery contract: %+v", maatCLI)
	}
	pulse, ok := componentsByID["maat-pulse-proof-platform"]
	if !ok {
		t.Fatal("Ma'at pulse component is missing")
	}
	if !contains(pulse.Outputs, "metrics with explicit measured, partial, skipped, or unavailable test scope") ||
		!contains(pulse.UpgradeRecipe, "never serialize skipped or unavailable coverage as a real zero measurement") {
		t.Fatalf("Ma'at pulse recipe omits measurement-availability truthfulness: %+v", pulse)
	}
	terminal, ok := componentsByID["maat-terminal-console-surface"]
	if !ok {
		t.Fatal("Ma'at terminal console component is missing")
	}
	if !contains(terminal.Source, "internal/tui/screen_health.go") {
		t.Fatalf("Ma'at terminal recipe omits the Health resolution surface: %+v", terminal.Source)
	}
	if !contains(terminal.Outputs, "confirmation-gated Ma'at evidence review for every guidance-only health finding") {
		t.Fatalf("Ma'at terminal recipe omits guidance resolution behavior: %+v", terminal.Outputs)
	}
	wingRaw, err := os.ReadFile(filepath.Join("..", "..", "contracts", "stacklab", "maat-wing-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	wing, err := ValidateWing(wingRaw)
	if err != nil {
		t.Fatalf("Ma'at wing: %v", err)
	}
	if wing.Provenance.ComponentCatalog != "docs/qa/MAAT_SYSTEM_ONE_CATALOG.md" {
		t.Fatalf("component catalog = %q", wing.Provenance.ComponentCatalog)
	}
	if _, err := os.Stat(filepath.Join("..", "..", wing.Provenance.ComponentCatalog)); err != nil {
		t.Fatalf("component catalog unavailable: %v", err)
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
