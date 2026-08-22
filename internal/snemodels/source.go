// Package snemodels owns Pantheon's authenticated acquisition of exact SNE
// checkpoint sources. It prepares bytes; SNE remains the promotion authority.
package snemodels

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const sourceCatalogSchema = "pantheon.sne-model-source.v1"

type SourceCatalog struct {
	Schema    string        `json:"schema"`
	CatalogID string        `json:"catalog_id"`
	Entries   []SourceEntry `json:"entries"`
}

type SourceEntry struct {
	CatalogEntry string       `json:"catalog_entry"`
	Provider     string       `json:"provider"`
	BaseURL      string       `json:"base_url,omitempty"`
	Repository   string       `json:"repository"`
	Revision     string       `json:"revision"`
	LicenseID    string       `json:"license_id"`
	Files        []SourceFile `json:"files"`
}

type SourceFile struct {
	Path      string `json:"path"`
	SHA256    string `json:"sha256"`
	SizeBytes int64  `json:"size_bytes"`
}

func LoadSourceCatalog(path string) (SourceCatalog, error) {
	file, err := os.Open(path)
	if err != nil {
		return SourceCatalog{}, fmt.Errorf("open SNE source catalog: %w", err)
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var catalog SourceCatalog
	if err := decoder.Decode(&catalog); err != nil {
		return SourceCatalog{}, fmt.Errorf("decode SNE source catalog: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return SourceCatalog{}, fmt.Errorf("SNE source catalog contains trailing JSON")
	}
	if catalog.Schema != sourceCatalogSchema || strings.TrimSpace(catalog.CatalogID) == "" || len(catalog.Entries) == 0 {
		return SourceCatalog{}, fmt.Errorf("unsupported SNE source catalog")
	}
	seen := make(map[string]struct{}, len(catalog.Entries))
	for _, entry := range catalog.Entries {
		if err := entry.validate(); err != nil {
			return SourceCatalog{}, fmt.Errorf("source catalog entry %q: %w", entry.CatalogEntry, err)
		}
		if _, duplicate := seen[entry.CatalogEntry]; duplicate {
			return SourceCatalog{}, fmt.Errorf("duplicate source catalog entry %q", entry.CatalogEntry)
		}
		seen[entry.CatalogEntry] = struct{}{}
	}
	return catalog, nil
}

func (catalog SourceCatalog) Resolve(catalogEntry string) (SourceEntry, error) {
	for _, entry := range catalog.Entries {
		if entry.CatalogEntry == catalogEntry {
			return entry, nil
		}
	}
	return SourceEntry{}, fmt.Errorf("SNE source catalog has no entry %q", catalogEntry)
}

func (entry SourceEntry) validate() error {
	if strings.TrimSpace(entry.CatalogEntry) == "" || (entry.Provider != "huggingface" && entry.Provider != "sirsi") || strings.TrimSpace(entry.Repository) == "" || strings.TrimSpace(entry.Revision) == "" || strings.TrimSpace(entry.LicenseID) == "" || len(entry.Files) == 0 {
		return fmt.Errorf("source identity is incomplete")
	}
	if err := safeRevision(entry.Revision); err != nil {
		return err
	}
	if err := safeRepository(entry.Repository, entry.Provider == "huggingface"); err != nil {
		return err
	}
	if entry.Provider == "huggingface" && entry.BaseURL != "" {
		return fmt.Errorf("Hugging Face source must use the controlled provider endpoint")
	}
	if entry.Provider == "sirsi" {
		base, err := url.Parse(entry.BaseURL)
		if err != nil || base.Scheme != "https" || base.Host == "" || base.User != nil || base.RawQuery != "" || base.Fragment != "" {
			return fmt.Errorf("first-party source requires a signed HTTPS base_url")
		}
	}
	seen := make(map[string]struct{}, len(entry.Files))
	for _, file := range entry.Files {
		if file.Path == "" || filepath.IsAbs(file.Path) || filepath.Clean(file.Path) != file.Path || strings.HasPrefix(file.Path, "..") || strings.ContainsAny(file.Path, "\\\t\r\n") {
			return fmt.Errorf("unsafe source file path %q", file.Path)
		}
		decoded, err := hex.DecodeString(file.SHA256)
		if err != nil || len(decoded) != 32 || file.SizeBytes < 1 {
			return fmt.Errorf("invalid source artifact identity %q", file.Path)
		}
		if _, duplicate := seen[file.Path]; duplicate {
			return fmt.Errorf("duplicate source file %q", file.Path)
		}
		seen[file.Path] = struct{}{}
	}
	return nil
}

func (entry SourceEntry) ResolveURL(baseURL, path string) (string, error) {
	base, err := url.Parse(baseURL)
	if err != nil || base.Scheme == "" || base.Host == "" {
		return "", fmt.Errorf("invalid Hugging Face base URL")
	}
	segments := strings.Split(entry.Repository, "/")
	if entry.Provider == "huggingface" {
		segments = append(segments, "resolve")
	}
	segments = append(segments, entry.Revision)
	segments = append(segments, strings.Split(path, "/")...)
	return url.JoinPath(base.String(), segments...)
}

func safeRevision(revision string) error {
	if len(revision) == 0 || len(revision) > 128 || revision == "." || revision == ".." || strings.ContainsAny(revision, "/\\%?#\x00\t\r\n") {
		return fmt.Errorf("unsafe source revision %q", revision)
	}
	for _, character := range revision {
		if !(character >= 'a' && character <= 'z') && !(character >= 'A' && character <= 'Z') && !(character >= '0' && character <= '9') && !strings.ContainsRune("._-", character) {
			return fmt.Errorf("unsafe source revision %q", revision)
		}
	}
	return nil
}

func safeRepository(repository string, exactlyTwoSegments bool) error {
	if strings.HasPrefix(repository, "/") || strings.ContainsAny(repository, "\\%?#\x00\t\r\n") {
		return fmt.Errorf("unsafe source repository %q", repository)
	}
	segments := strings.Split(repository, "/")
	if (exactlyTwoSegments && len(segments) != 2) || (!exactlyTwoSegments && len(segments) < 1) {
		return fmt.Errorf("invalid source repository %q", repository)
	}
	for _, segment := range segments {
		if err := safeRevision(segment); err != nil {
			return fmt.Errorf("invalid source repository %q", repository)
		}
	}
	return nil
}

func sortedSourceFiles(files []SourceFile) []SourceFile {
	ordered := append([]SourceFile(nil), files...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Path < ordered[j].Path })
	return ordered
}
