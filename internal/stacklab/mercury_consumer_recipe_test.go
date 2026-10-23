package stacklab

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestMercuryConsumerRecipeIsCompleteAndDiscoverable(t *testing.T) {
	type component struct {
		ID            string   `json:"id"`
		Source        []string `json:"source"`
		Tests         []string `json:"tests"`
		Inputs        []string `json:"inputs"`
		Outputs       []string `json:"outputs"`
		Writes        []string `json:"writes"`
		UpgradeRecipe []string `json:"upgrade_recipe"`
	}
	type authority struct {
		AdoptionRecord string   `json:"adoption_record"`
		Runbook        string   `json:"runbook"`
		Prohibited     []string `json:"prohibited"`
	}
	var recipe struct {
		Schema     string      `json:"schema"`
		ID         string      `json:"id"`
		Wing       string      `json:"wing"`
		Product    string      `json:"product"`
		Version    int         `json:"version"`
		Authority  authority   `json:"authority"`
		Components []component `json:"components"`
	}

	repoRoot := filepath.Join("..", "..")
	recipePath := "contracts/stacklab/mercury-consumer-recipe-v1.json"
	raw, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(recipePath)))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &recipe); err != nil {
		t.Fatal(err)
	}
	if recipe.Schema != RecipeSchemaConst || recipe.ID != "stacklab.recipe.mercury-consumer" ||
		recipe.Wing != "stacklab.wing.ra-horus-fabric" || recipe.Product != "sirsi-pantheon" || recipe.Version != 1 {
		t.Fatalf("recipe identity = %+v", recipe)
	}
	if recipe.Authority.AdoptionRecord != "docs/stacklab/MERCURY_CONSUMER_ADOPTION_20261023.md" ||
		recipe.Authority.Runbook != "docs/user-guides/MERCURY_CONSUMER.md" {
		t.Fatalf("recipe adoption/runbook references = %+v", recipe.Authority)
	}
	for _, path := range []string{recipe.Authority.AdoptionRecord, recipe.Authority.Runbook} {
		if _, err := os.Stat(filepath.Join(repoRoot, filepath.FromSlash(path))); err != nil {
			t.Fatalf("recipe references missing adoption/runbook path %q: %v", path, err)
		}
	}
	if len(recipe.Components) != 2 {
		t.Fatalf("component count = %d, want 2", len(recipe.Components))
	}
	wantIDs := []string{"mercury-dashboard-read-model", "mercury-canonical-cli-mcp"}
	for i, component := range recipe.Components {
		if component.ID != wantIDs[i] {
			t.Fatalf("component[%d] id = %q, want %q", i, component.ID, wantIDs[i])
		}
		if len(component.Source) == 0 || len(component.Tests) == 0 || len(component.Inputs) == 0 ||
			len(component.Outputs) == 0 || len(component.UpgradeRecipe) == 0 {
			t.Fatalf("component %q is not independently upgradeable: %+v", component.ID, component)
		}
		if len(component.Writes) != 0 {
			t.Fatalf("component %q declares writes: %v", component.ID, component.Writes)
		}
		for _, listed := range append(append([]string{}, component.Source...), component.Tests...) {
			if _, err := os.Stat(filepath.Join(repoRoot, filepath.FromSlash(listed))); err != nil {
				t.Fatalf("component %q references missing path %q: %v", component.ID, listed, err)
			}
		}
	}
	wantProhibited := []string{
		"no run or service control",
		"no inference or bandwidth qualification",
		"no new telemetry writer or authority",
		"no public-network destination or redirect",
		"no fabricated unknown zero",
		"no current-native or web parity claim from CLI/MCP tests",
	}
	if len(recipe.Authority.Prohibited) != len(wantProhibited) {
		t.Fatalf("prohibited boundaries = %v, want exactly %v", recipe.Authority.Prohibited, wantProhibited)
	}
	for _, boundary := range wantProhibited {
		if !contains(recipe.Authority.Prohibited, boundary) {
			t.Fatalf("recipe omits prohibited boundary %q: %v", boundary, recipe.Authority.Prohibited)
		}
	}
	componentsByID := make(map[string]component, len(recipe.Components))
	for _, component := range recipe.Components {
		componentsByID[component.ID] = component
	}
	for componentID, boundary := range map[string]string{
		"mercury-dashboard-read-model": "preserve bounded HTTP, private-address checks, no proxy and no redirects",
		"mercury-canonical-cli-mcp":    "keep endpoint selection out of MCP arguments",
	} {
		if !contains(componentsByID[componentID].UpgradeRecipe, boundary) {
			t.Fatalf("component %q upgrade recipe omits boundary %q", componentID, boundary)
		}
	}
	if !contains(componentsByID["mercury-dashboard-read-model"].UpgradeRecipe, "do not retain idle connections from per-call clients") ||
		!contains(componentsByID["mercury-dashboard-read-model"].UpgradeRecipe, "never turn a monitor read into admission or performance qualification") {
		t.Fatal("dashboard reader upgrade recipe omits transport or qualification boundary")
	}

	catalog, err := LoadLocalCatalog(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range catalog.Entries {
		if entry.ID != recipe.ID {
			continue
		}
		if entry.Kind != "recipe" || entry.SourcePath != recipePath || len(entry.Components) != 2 {
			t.Fatalf("catalog entry = %+v", entry)
		}
		return
	}
	t.Fatalf("local Stack Lab catalog omitted %q", recipe.ID)
}
