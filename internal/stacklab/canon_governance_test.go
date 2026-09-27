package stacklab

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPantheonCanonGovernance keeps Stack Lab as an enforced engineering
// method. It validates the release inputs and every source ADR in place rather
// than copying documents into a second, drift-prone registry.
func TestPantheonCanonGovernance(t *testing.T) {
	type documents struct {
		ADRRegistry     string   `json:"adr_registry"`
		ADRSourceGlob   string   `json:"adr_source_glob"`
		ContractRoot    string   `json:"contract_root"`
		ProductCatalogs []string `json:"product_catalogs"`
	}
	type releaseRecipe struct {
		VersionSource string   `json:"version_source"`
		NativeApp     string   `json:"native_app"`
		CLI           string   `json:"cli"`
		DMGBuilder    string   `json:"dmg_builder"`
		PKGBuilder    string   `json:"pkg_builder"`
		Workflow      string   `json:"release_workflow"`
		Artifacts     []string `json:"required_artifacts"`
	}
	var canon struct {
		Schema        string        `json:"schema"`
		ID            string        `json:"id"`
		CanonicalDocs documents     `json:"canonical_documents"`
		ReleaseRecipe releaseRecipe `json:"release_recipe"`
	}

	root := filepath.Join("..", "..")
	raw, err := os.ReadFile(filepath.Join(root, "contracts", "stacklab", "pantheon-canon-governance-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &canon); err != nil {
		t.Fatal(err)
	}
	if canon.Schema != "sirsi.stacklab.canon.v1" || canon.ID != "stacklab.canon.pantheon" {
		t.Fatalf("canon identity = schema %q id %q", canon.Schema, canon.ID)
	}

	required := append([]string{
		canon.CanonicalDocs.ADRRegistry,
		canon.CanonicalDocs.ContractRoot,
		canon.ReleaseRecipe.VersionSource,
		canon.ReleaseRecipe.NativeApp,
		canon.ReleaseRecipe.CLI,
		canon.ReleaseRecipe.DMGBuilder,
		canon.ReleaseRecipe.PKGBuilder,
		canon.ReleaseRecipe.Workflow,
	}, canon.CanonicalDocs.ProductCatalogs...)
	for _, path := range required {
		if strings.TrimSpace(path) == "" {
			t.Fatal("canonical governance contains an empty required path")
		}
		if _, err := os.Stat(filepath.Join(root, path)); err != nil {
			t.Fatalf("canonical path %q: %v", path, err)
		}
	}
	if len(canon.ReleaseRecipe.Artifacts) != 3 {
		t.Fatalf("required release artifacts = %v", canon.ReleaseRecipe.Artifacts)
	}

	adrs, err := filepath.Glob(filepath.Join(root, canon.CanonicalDocs.ADRSourceGlob))
	if err != nil {
		t.Fatal(err)
	}
	if len(adrs) == 0 {
		t.Fatal("Stack Lab canonical ADR glob resolved no documents")
	}
	for _, adr := range adrs {
		if strings.HasSuffix(adr, "ADR-INDEX.md") || strings.HasSuffix(adr, "ADR-TEMPLATE.md") {
			continue
		}
		body, err := os.ReadFile(adr)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(string(body), "# ADR-") {
			t.Fatalf("canonical ADR %q has no ADR heading", adr)
		}
	}
}
