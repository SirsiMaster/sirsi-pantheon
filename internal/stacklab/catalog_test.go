package stacklab

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadLocalCatalogProjectsRecipeAndWingDeterministically(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "contracts", "stacklab")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	recipe := `{"schema":"sirsi.stacklab.recipe.v1","id":"stacklab.recipe.example","wing":"stacklab.wing.example","product":"pantheon","version":1,"purpose":"example","authority":{},"components":[{"id":"component","source":["a"],"tests":["b"],"inputs":["c"],"outputs":["d"],"upgrade_recipe":["e"]}]}`
	wing := `{"schema":"sirsi.stacklab.wing.v1","id":"stacklab.wing.example","owner":"owner","project_id":"pantheon","router_namespace":"pantheon","class":"application","scope":"example","status":"active","first_gate":"source","workspace":{"repository_root":"/repo","writable_roots":["/repo"],"evidence_root":"/evidence","shared_payload_access":"none","boundary_policy":"default-deny"},"handoffs":{"inbound":"router-receipt-only","outbound":"router-receipt-only","allowed_peer_wings":[]},"provenance":{"lifecycle_task_id":"task","component_catalog":"catalog","receipt_links":[]},"mirrors":{"repository":"current","desktop":"current","workspace":"current"},"next_action":"continue"}`
	if err := os.WriteFile(filepath.Join(dir, "example-recipe-v1.json"), []byte(recipe), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "example-wing-v1.json"), []byte(wing), 0o600); err != nil {
		t.Fatal(err)
	}

	catalog, err := LoadLocalCatalog(root)
	if err != nil {
		t.Fatal(err)
	}
	if !catalog.Complete() || len(catalog.Entries) != 2 || catalog.Entries[0].Kind != "recipe" || catalog.Entries[0].Components[0].ID != "component" || catalog.Entries[1].Kind != "wing" {
		t.Fatalf("catalog = %+v", catalog)
	}
}

func TestLoadLocalCatalogDoesNotSilentlyOmitMalformedContract(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "contracts", "stacklab")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "broken-recipe-v1.json"), []byte(`{`), 0o600); err != nil {
		t.Fatal(err)
	}
	catalog, err := LoadLocalCatalog(root)
	if err != nil {
		t.Fatal(err)
	}
	if catalog.Complete() || len(catalog.Unknown) != 1 || len(catalog.Entries) != 0 {
		t.Fatalf("catalog = %+v", catalog)
	}
}
