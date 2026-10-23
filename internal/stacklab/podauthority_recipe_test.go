package stacklab

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestPodAuthorityServiceRecipePathsAndBoundaries(t *testing.T) {
	root := filepath.Join("..", "..")
	raw, err := os.ReadFile(filepath.Join(root, "contracts", "stacklab", "maat-pod-authority-service-recipe-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var recipe recipeDocument
	if err = json.Unmarshal(raw, &recipe); err != nil {
		t.Fatal(err)
	}
	if recipe.Schema != RecipeSchemaConst || recipe.ID != "stacklab.recipe.maat-pod-authority-service" || recipe.Wing != "stacklab.wing.ra-horus-fabric" || len(recipe.Components) != 1 {
		t.Fatalf("unexpected recipe identity: %+v", recipe)
	}
	component := recipe.Components[0]
	if len(component.Inputs) == 0 || len(component.Outputs) == 0 || len(component.Writes) != 1 || len(component.UpgradeRecipe) == 0 {
		t.Fatal("missing replaceable component contract")
	}
	for _, path := range append(append([]string{}, component.Source...), component.Tests...) {
		if !filepath.IsLocal(path) {
			t.Fatalf("nonlocal component path: %q", path)
		}
		if _, err := os.Stat(filepath.Join(root, path)); err != nil {
			t.Fatalf("component path %q: %v", path, err)
		}
	}
	var authority struct {
		Runbook    string   `json:"runbook"`
		Prohibited []string `json:"prohibited"`
	}
	if err := json.Unmarshal(recipe.Authority, &authority); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, authority.Runbook)); err != nil {
		t.Fatalf("missing runbook: %v", err)
	}
	for _, boundary := range []string{"no automatic bootstrap or login enable", "no private-key generation or copying", "no alternate router store", "no developer binary or PATH fallback", "no network configuration changes", "no claim that a prepared recipe is running or qualified"} {
		found := false
		for _, declared := range authority.Prohibited {
			found = found || boundary == declared
		}
		if !found {
			t.Fatalf("missing boundary %q", boundary)
		}
	}
}
