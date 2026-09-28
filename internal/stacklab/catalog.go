package stacklab

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// RecipeSchemaConst is the on-disk schema used for independently upgradeable
// Stack Lab recipes. A catalog is a local source projection: it never claims
// the remote wing/registry authority that Run evaluates.
const RecipeSchemaConst = "sirsi.stacklab.recipe.v1"

// Catalog is a deterministic projection of this checkout's local Stack Lab
// contracts. Unknown records are explicit: a missing, non-regular, unreadable,
// or malformed contract is never omitted and never rendered as a complete
// workspace.
type Catalog struct {
	Entries []CatalogEntry `json:"entries"`
	Unknown []string       `json:"unknown,omitempty"`
}

// Complete means every discoverable contract could be read and structurally
// projected. It deliberately does not mean its wing is canonical or released.
func (c Catalog) Complete() bool { return len(c.Unknown) == 0 }

// CatalogEntry is the typed, source-level contract presented by the CLI and
// native Stack Lab view. SourcePath remains repo-relative so it is actionable
// without exposing an ambient filesystem path as authority.
type CatalogEntry struct {
	ID         string            `json:"id"`
	Kind       string            `json:"kind"`
	SourcePath string            `json:"source_path"`
	Wing       string            `json:"wing,omitempty"`
	Product    string            `json:"product,omitempty"`
	Version    int               `json:"version,omitempty"`
	Purpose    string            `json:"purpose,omitempty"`
	Components []RecipeComponent `json:"components,omitempty"`
	NextAction string            `json:"next_action,omitempty"`
}

// RecipeComponent names a replaceable implementation slice and the source,
// tests, outputs, writes, and upgrade recipe that travel with it.
type RecipeComponent struct {
	ID            string   `json:"id"`
	Source        []string `json:"source"`
	Tests         []string `json:"tests"`
	Inputs        []string `json:"inputs"`
	Outputs       []string `json:"outputs"`
	Writes        []string `json:"writes"`
	UpgradeRecipe []string `json:"upgrade_recipe"`
}

type recipeDocument struct {
	Schema     string            `json:"schema"`
	ID         string            `json:"id"`
	Wing       string            `json:"wing"`
	Product    string            `json:"product"`
	Version    int               `json:"version"`
	Purpose    string            `json:"purpose"`
	Authority  json.RawMessage   `json:"authority"`
	Components []RecipeComponent `json:"components"`
}

// LoadLocalCatalog reads only direct, versioned recipe and wing contracts in
// contracts/stacklab. It rejects symlinks and non-regular files before reading
// them, retains a repo-relative source path, and sorts the final projection so
// every surface receives stable JSON.
func LoadLocalCatalog(repoRoot string) (Catalog, error) {
	contractDir := filepath.Join(repoRoot, "contracts", "stacklab")
	entries, err := os.ReadDir(contractDir)
	if err != nil {
		return Catalog{}, fmt.Errorf("read Stack Lab contract directory: %w", err)
	}

	var catalog Catalog
	for _, entry := range entries {
		name := entry.Name()
		kind := ""
		switch {
		case strings.HasSuffix(name, "-recipe-v1.json"):
			kind = "recipe"
		case strings.HasSuffix(name, "-wing-v1.json"):
			kind = "wing"
		default:
			continue
		}

		relative := filepath.ToSlash(filepath.Join("contracts", "stacklab", name))
		fullPath := filepath.Join(contractDir, name)
		info, statErr := os.Lstat(fullPath)
		if statErr != nil {
			catalog.Unknown = append(catalog.Unknown, fmt.Sprintf("%s: inspect: %v", relative, statErr))
			continue
		}
		if entry.Type()&fs.ModeSymlink != 0 || !info.Mode().IsRegular() {
			catalog.Unknown = append(catalog.Unknown, fmt.Sprintf("%s: expected a regular non-symlink contract", relative))
			continue
		}

		raw, readErr := os.ReadFile(fullPath)
		if readErr != nil {
			catalog.Unknown = append(catalog.Unknown, fmt.Sprintf("%s: read: %v", relative, readErr))
			continue
		}

		var projected CatalogEntry
		if kind == "recipe" {
			projected, readErr = projectRecipe(relative, raw)
		} else {
			projected, readErr = projectWing(relative, raw)
		}
		if readErr != nil {
			catalog.Unknown = append(catalog.Unknown, fmt.Sprintf("%s: %v", relative, readErr))
			continue
		}
		catalog.Entries = append(catalog.Entries, projected)
	}

	// A recipe is only independently upgradeable when its declared owning wing
	// is present in the same source catalog. Without this join, a stale recipe
	// can remain structurally well-formed while pointing at a retired or
	// misspelled wing — exactly the Pantheon release-recipe drift Stack Lab is
	// meant to make impossible to miss. Keep the malformed reference out of
	// Entries and render it as Unknown rather than projecting a false-complete
	// catalog.
	wings := make(map[string]struct{})
	for _, entry := range catalog.Entries {
		if entry.Kind == "wing" {
			wings[entry.ID] = struct{}{}
		}
	}
	resolvedEntries := catalog.Entries[:0]
	for _, entry := range catalog.Entries {
		if entry.Kind == "recipe" {
			if _, ok := wings[entry.Wing]; !ok {
				catalog.Unknown = append(catalog.Unknown, fmt.Sprintf("%s: recipe references missing local wing %q", entry.SourcePath, entry.Wing))
				continue
			}
		}
		resolvedEntries = append(resolvedEntries, entry)
	}
	catalog.Entries = resolvedEntries

	sort.Slice(catalog.Entries, func(i, j int) bool {
		if catalog.Entries[i].Kind != catalog.Entries[j].Kind {
			return catalog.Entries[i].Kind < catalog.Entries[j].Kind
		}
		return catalog.Entries[i].ID < catalog.Entries[j].ID
	})
	sort.Strings(catalog.Unknown)
	return catalog, nil
}

func projectRecipe(relative string, raw []byte) (CatalogEntry, error) {
	var recipe recipeDocument
	if err := json.Unmarshal(raw, &recipe); err != nil {
		return CatalogEntry{}, fmt.Errorf("decode recipe: %w", err)
	}
	if recipe.Schema != RecipeSchemaConst {
		return CatalogEntry{}, fmt.Errorf("recipe schema must equal %q", RecipeSchemaConst)
	}
	if !strings.HasPrefix(recipe.ID, "stacklab.recipe.") || strings.TrimSpace(recipe.Wing) == "" || strings.TrimSpace(recipe.Product) == "" || recipe.Version < 1 || strings.TrimSpace(recipe.Purpose) == "" {
		return CatalogEntry{}, fmt.Errorf("recipe has incomplete identity, ownership, version, or purpose")
	}
	if len(recipe.Authority) == 0 || len(recipe.Components) == 0 {
		return CatalogEntry{}, fmt.Errorf("recipe has no authority or independently upgradeable components")
	}
	seen := make(map[string]struct{}, len(recipe.Components))
	for _, component := range recipe.Components {
		if strings.TrimSpace(component.ID) == "" || len(component.Source) == 0 || len(component.Tests) == 0 || len(component.Inputs) == 0 || len(component.Outputs) == 0 || len(component.UpgradeRecipe) == 0 {
			return CatalogEntry{}, fmt.Errorf("recipe component is not independently upgradeable")
		}
		if _, duplicate := seen[component.ID]; duplicate {
			return CatalogEntry{}, fmt.Errorf("recipe repeats component id %q", component.ID)
		}
		seen[component.ID] = struct{}{}
	}
	return CatalogEntry{ID: recipe.ID, Kind: "recipe", SourcePath: relative, Wing: recipe.Wing, Product: recipe.Product, Version: recipe.Version, Purpose: recipe.Purpose, Components: recipe.Components}, nil
}

func projectWing(relative string, raw []byte) (CatalogEntry, error) {
	wing, err := ValidateWing(raw)
	if err != nil {
		return CatalogEntry{}, fmt.Errorf("validate wing: %w", err)
	}
	return CatalogEntry{ID: wing.ID, Kind: "wing", SourcePath: relative, Product: wing.ProjectID, Purpose: wing.Scope, NextAction: wing.NextAction}, nil
}
