package knowledge

import (
	"context"
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

func TestLoadIgnoresMalformedCompatibilityCacheUntilExplicitMigration(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".config", "seshat", "store", "latest.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	malformed := "{not json"
	if err := os.WriteFile(path, []byte(malformed), 0o600); err != nil {
		t.Fatal(err)
	}
	// Read-only canonical projection must not implicitly parse or migrate a
	// legacy cache. Malformed legacy bytes are not a corrupt canonical archive.
	view, err := Load(home, "")
	if err != nil || view.Items == nil || len(view.Items) != 0 || view.Total != 0 || view.Withheld != 0 || len(view.Coverage.Sources) != 0 {
		t.Fatalf("canonical read implicitly consumed legacy cache: view=%+v err=%v", view, err)
	}
	archive, err := Open(home)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	runs, err := archive.MigrateLegacy(context.Background(), filepath.Dir(path))
	if err == nil || len(runs) != 1 || runs[0].State != ImportUnverifiable || runs[0].Rejected != 1 || runs[0].ErrorCode != "legacy_json_unverifiable" {
		t.Fatalf("explicit migration did not refuse malformed legacy cache: runs=%+v err=%v", runs, err)
	}
	for _, table := range []string{"records", "versions", "knowledge_fts", "legacy_imports"} {
		var count int
		if err := archive.db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("malformed migration left %s count=%d err=%v", table, count, err)
		}
	}
	coverage, err := archive.Coverage(context.Background(), 10)
	if err != nil || len(coverage.Sources) != 1 || coverage.Sources[0].State != ImportUnverifiable || coverage.Sources[0].ErrorCode != "legacy_json_unverifiable" {
		t.Fatalf("malformed migration lost truthful failure coverage: %+v err=%v", coverage, err)
	}
	retained, err := os.ReadFile(path)
	if err != nil || string(retained) != malformed {
		t.Fatalf("explicit refusal modified legacy cache: err=%v", err)
	}
}
