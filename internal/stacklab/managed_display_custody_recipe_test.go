package stacklab

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestManagedDisplayCustodyRecipePinsExactDeploymentArtifacts(t *testing.T) {
	root := filepath.Join("..", "..")
	policy := filepath.Join(root, "scripts", "stacklab", "managed-requirements.toml")
	installer := filepath.Join(root, "scripts", "stacklab", "install-managed-display-guard.py")
	if got := fileSHA256(t, policy); got != "66ee45441852eee31946722a6ea4cdbc1c3267845bb997f153d665a9d83f14c8" {
		t.Fatalf("policy SHA-256 = %s", got)
	}
	if got := fileSHA256(t, installer); got != "45b7a4574198532943eed920c4513473c9ff42d88a0a68bb4404e87156bdcc3d" {
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
	if component.ID != "managed-native-cli-display-guard" || !containsManagedText(component.Writes, "/etc/codex/requirements.toml") || !containsManagedText(component.Writes, "/etc/codex/hooks/sirsi-display-power-guard") || !containsManagedText(component.Outputs, "an explicit limitation that pure GUI coordinate actuators require revocation of the computer-use grant itself") {
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

func TestManagedMatcherCoversCommandTools(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "scripts", "stacklab", "managed-requirements.toml"))
	if err != nil {
		t.Fatal(err)
	}
	// Inspect the PreToolUse blocks, not unrelated strings elsewhere in TOML.
	var matchers []*regexp.Regexp
	for _, block := range strings.Split(string(raw), "[[hooks.PreToolUse]]")[1:] {
		var matcher string
		for _, line := range strings.Split(block, "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "matcher = ") {
				matcher = strings.Trim(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "matcher = ")), "\"")
			}
		}
		if !strings.Contains(block, `command = "/etc/codex/hooks/sirsi-display-power-guard"`) {
			continue
		}
		re, err := regexp.Compile(matcher)
		if err != nil || matcher == "" {
			t.Fatalf("invalid matcher %q: %v", matcher, err)
		}
		matchers = append(matchers, re)
	}
	for _, name := range []string{"Bash", "exec_command", "functions.exec_command", "unified_exec", "shell"} {
		covered := false
		for _, re := range matchers {
			covered = covered || re.MatchString(name)
		}
		if !covered {
			t.Errorf("command tool %q bypasses managed guard", name)
		}
	}
	// Negative control: the legacy Bash matcher alone cannot cover desktop exec.
	if regexp.MustCompile("^Bash$").MatchString("functions.exec_command") {
		t.Fatal("invalid matcher control")
	}
}
