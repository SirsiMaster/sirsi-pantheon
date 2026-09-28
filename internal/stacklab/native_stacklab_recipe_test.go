package stacklab

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestNativeStackLabSurfaceRecipeIsComplete(t *testing.T) {
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
	raw, err := os.ReadFile(filepath.Join("..", "..", "contracts", "stacklab", "native-stacklab-surface-recipe-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &recipe); err != nil {
		t.Fatal(err)
	}
	if recipe.Schema != "sirsi.stacklab.recipe.v1" || recipe.ID != "stacklab.recipe.native-stacklab-surface" || recipe.Wing != "stacklab.wing.ra-horus-fabric" {
		t.Fatalf("recipe identity = %+v", recipe)
	}
	if len(recipe.Components) != 2 || recipe.Components[0].ID != "stacklab-native-doctor" || recipe.Components[1].ID != "stacklab-native-catalog" {
		t.Fatalf("recipe components = %+v", recipe.Components)
	}
	nativeDoctor := recipe.Components[0]
	if len(nativeDoctor.Source) == 0 || len(nativeDoctor.Tests) == 0 || len(nativeDoctor.Inputs) == 0 || len(nativeDoctor.Outputs) == 0 || len(nativeDoctor.UpgradeRecipe) == 0 {
		t.Fatalf("component is not independently upgradeable: %+v", nativeDoctor)
	}
	for _, listed := range append(append([]string{}, nativeDoctor.Source...), nativeDoctor.Tests...) {
		if _, statErr := os.Stat(filepath.Join("..", "..", listed)); statErr != nil {
			t.Fatalf("recipe references missing path %q: %v", listed, statErr)
		}
	}
}
