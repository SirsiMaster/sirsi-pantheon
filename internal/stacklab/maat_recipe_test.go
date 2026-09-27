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
	if err := json.Unmarshal(raw, &recipe); err != nil {
		t.Fatal(err)
	}
	if recipe.Schema != "sirsi.stacklab.recipe.v1" || recipe.ID != "stacklab.recipe.maat-system-one" || recipe.Wing != "stacklab.wing.maat" {
		t.Fatalf("recipe identity = %+v", recipe)
	}
	want := []string{
		"maat-canon", "maat-casebook", "maat-cli", "maat-core", "maat-coverage",
		"maat-decision-journal", "maat-horus-surface", "maat-pipeline", "maat-pulse-proof-platform",
		"maat-scheduler", "maat-wing-contract",
	}
	got := make([]string, 0, len(recipe.Components))
	for _, component := range recipe.Components {
		got = append(got, component.ID)
		if len(component.Source) == 0 || len(component.Tests) == 0 || len(component.Inputs) == 0 || len(component.Outputs) == 0 || len(component.UpgradeRecipe) == 0 {
			t.Fatalf("component %q is not independently upgradeable: %+v", component.ID, component)
		}
		for _, listed := range append(append([]string{}, component.Source...), component.Tests...) {
			if _, err := os.Stat(filepath.Join("..", "..", listed)); err != nil {
				t.Fatalf("component %q references missing path %q: %v", component.ID, listed, err)
			}
		}
	}
	sort.Strings(got)
	if len(got) != len(want) {
		t.Fatalf("component ids = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("component ids = %v, want %v", got, want)
		}
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
