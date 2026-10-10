package stacklab

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestManagedDisplayCustodyRecipePinsExactDeploymentArtifacts(t *testing.T) {
	root := filepath.Join("..", "..")
	policy := filepath.Join(root, "scripts", "stacklab", "managed-requirements.toml")
	installer := filepath.Join(root, "scripts", "stacklab", "install-managed-display-guard.py")
	if got := fileSHA256(t, policy); got != "370956d366430011542ec6303e0c4eaa3e2290f81aab8fd91905e0d4ebcb9156" {
		t.Fatalf("policy SHA-256 = %s", got)
	}
	if got := fileSHA256(t, installer); got != "a67137916f87325356fb6abec8928a670f3734e0d0203e37f3db7461a858efc6" {
		t.Fatalf("installer SHA-256 = %s", got)
	}

	var recipe struct {
		Schema     string `json:"schema"`
		ID         string `json:"id"`
		Wing       string `json:"wing"`
		Components []struct {
			ID      string   `json:"id"`
			Writes  []string `json:"writes"`
			Outputs []string `json:"outputs"`
		} `json:"components"`
	}
	raw, err := os.ReadFile(filepath.Join(root, "contracts", "stacklab", "pantheon-managed-display-custody-recipe-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &recipe); err != nil {
		t.Fatal(err)
	}
	if recipe.Schema != RecipeSchemaConst || recipe.ID != "stacklab.recipe.pantheon-managed-display-custody" || recipe.Wing != "stacklab.wing.pantheon.pt-wing-001" || len(recipe.Components) != 1 {
		t.Fatalf("recipe identity/components = %+v", recipe)
	}
	component := recipe.Components[0]
	if component.ID != "managed-native-cli-display-guard" || !containsManagedText(component.Writes, "/etc/codex/requirements.toml") || !containsManagedText(component.Writes, "/etc/codex/hooks/sirsi-display-power-guard") || !containsManagedText(component.Outputs, "an explicit limitation that desktop-session tool bridging requires a separate Ra integration") {
		t.Fatalf("recipe does not preserve scoped managed-policy authority: %+v", component)
	}
}

func fileSHA256(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func containsManagedText(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
