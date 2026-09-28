package knowledge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/seshat"
)

func TestProjectWithholdsLegacySecretsBeforeSearching(t *testing.T) {
	secret := "token" + "=" + "example" + "value"
	view := Project([]seshat.KnowledgeItem{
		{Title: "Architecture", Summary: "Ma'at owns the operator knowledge view."},
		{Title: "Legacy secret", Summary: secret},
		{Title: "Reference secret", Summary: "Metadata is also scanned.", References: []seshat.KIReference{{Type: "source", Value: secret}}},
	}, "architecture")

	if view.Withheld != 2 || view.Total != 1 || len(view.Items) != 1 || view.Items[0].Title != "Architecture" {
		t.Fatalf("projection = %#v", view)
	}
}

func TestProjectWithholdsStandaloneCredentialsAndLocalIdentifiers(t *testing.T) {
	credential := "A" + "b3" + strings.Repeat("!", 10)
	identifier := strings.Repeat("2", 16)
	view := Project([]seshat.KnowledgeItem{
		{Title: credential, Summary: "unlabeled legacy note"},
		{Title: "Utility account", Summary: "Account number: " + identifier},
		{Title: "Safe architecture note", Summary: "Ma'at projects only evidence-safe local knowledge."},
	}, "")

	if view.Withheld != 2 || view.Total != 1 || len(view.Items) != 1 || view.Items[0].Title != "Safe architecture note" {
		t.Fatalf("projection did not withhold sensitive compatibility records: withheld=%d total=%d", view.Withheld, view.Total)
	}
}

func TestProjectKeepsOrdinaryVersionText(t *testing.T) {
	view := Project([]seshat.KnowledgeItem{{
		Title:   "Pantheon release notes",
		Summary: "Version 0.24.28 is ready for a signed package after the release checks.",
	}}, "")
	if view.Withheld != 0 || view.Total != 1 {
		t.Fatalf("ordinary product text was withheld: %#v", view)
	}
}

func TestLoadReturnsEmptyViewForAbsentCompatibilityCache(t *testing.T) {
	view, err := Load(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	if view.Total != 0 || view.Withheld != 0 || view.Items == nil {
		t.Fatalf("empty view = %#v", view)
	}
}

func TestLoadRejectsMalformedCompatibilityCache(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".config", "seshat", "store", "latest.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(home, ""); err == nil {
		t.Fatal("malformed cache accepted")
	}
}
