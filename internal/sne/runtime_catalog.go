package sne

import (
	"crypto/sha256"
	"debug/macho"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const RuntimePackageCatalogSchema = "pantheon.sne-runtime-packages.v2"

type RuntimePackageCatalog struct {
	SchemaVersion string           `json:"schema_version"`
	CatalogID     string           `json:"catalog_id"`
	Entries       []RuntimePackage `json:"entries"`
}

type RuntimePackage struct {
	ModelID               string `json:"model_id"`
	RuntimeID             string `json:"runtime_id,omitempty"`
	PackageID             string `json:"package_id,omitempty"`
	ServiceVersion        string `json:"service_version,omitempty"`
	PackageRoot           string `json:"package_root,omitempty"`
	RuntimeSHA256         string `json:"runtime_sha256"`
	NativeRuntimeSHA256   string `json:"native_runtime_sha256"`
	MLXDylibSHA256        string `json:"mlx_dylib_sha256"`
	MetallibSHA256        string `json:"metallib_sha256"`
	JACCLSHA256           string `json:"jaccl_sha256"`
	CacheTopology         string `json:"cache_topology,omitempty"`
	ServingCacheCapacity  int    `json:"serving_cache_capacity,omitempty"`
	PrefixSessionsMaximum int    `json:"prefix_sessions_maximum,omitempty"`
	StreamingMode         string `json:"streaming_mode,omitempty"`
}

func (entry RuntimePackage) EffectiveServiceVersion() string {
	if value := strings.TrimSpace(entry.ServiceVersion); validServiceVersion(value) {
		return value
	}
	identity := strings.TrimSpace(entry.PackageID)
	if identity == "" {
		identity = filepath.Base(strings.TrimSpace(entry.PackageRoot))
	}
	if !strings.HasPrefix(identity, "SNE-") {
		return ""
	}
	value := strings.SplitN(strings.TrimPrefix(identity, "SNE-"), "-", 2)[0]
	if validServiceVersion(value) {
		return value
	}
	return ""
}

func validServiceVersion(value string) bool {
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		return false
	}
	for _, part := range parts {
		if part == "" {
			return false
		}
		for _, character := range part {
			if character < '0' || character > '9' {
				return false
			}
		}
	}
	return true
}

// EffectiveRuntimeID returns the stable identity Pantheon exposes to products.
// v2 catalogs may omit runtime_id only when one runtime exists for a model; in
// that compatibility case the already-signed package ID (or full runtime hash
// for legacy absolute-root entries) becomes the unambiguous runtime identity.
func (entry RuntimePackage) EffectiveRuntimeID() string {
	if value := strings.TrimSpace(entry.RuntimeID); value != "" {
		return value
	}
	if value := strings.TrimSpace(entry.PackageID); value != "" {
		return value
	}
	return "runtime-sha256-" + entry.RuntimeSHA256
}

func LoadRuntimePackageCatalog(path string) (RuntimePackageCatalog, error) {
	file, err := os.Open(path)
	if err != nil {
		return RuntimePackageCatalog{}, fmt.Errorf("open SNE runtime package catalog: %w", err)
	}
	defer file.Close()
	var catalog RuntimePackageCatalog
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&catalog); err != nil {
		return RuntimePackageCatalog{}, fmt.Errorf("decode SNE runtime package catalog: %w", err)
	}
	if err := ensureRuntimeCatalogEOF(decoder); err != nil {
		return RuntimePackageCatalog{}, fmt.Errorf("decode SNE runtime package catalog: %w", err)
	}
	if catalog.SchemaVersion != RuntimePackageCatalogSchema || strings.TrimSpace(catalog.CatalogID) == "" || len(catalog.Entries) == 0 {
		return RuntimePackageCatalog{}, fmt.Errorf("unsupported SNE runtime package catalog")
	}
	seen := make(map[string]struct{}, len(catalog.Entries))
	modelCounts := make(map[string]int, len(catalog.Entries))
	modelHasLegacy := make(map[string]bool, len(catalog.Entries))
	for index := range catalog.Entries {
		entry := &catalog.Entries[index]
		entry.ModelID = strings.TrimSpace(entry.ModelID)
		entry.RuntimeID = strings.TrimSpace(entry.RuntimeID)
		entry.PackageID = strings.TrimSpace(entry.PackageID)
		entry.PackageRoot = strings.TrimSpace(entry.PackageRoot)
		if entry.PackageRoot != "" {
			entry.PackageRoot = filepath.Clean(entry.PackageRoot)
		}
		if entry.ModelID == "" || (entry.PackageID == "") == (entry.PackageRoot == "") {
			return RuntimePackageCatalog{}, fmt.Errorf("runtime package entry %d must have exactly one of package_id or package_root", index)
		}
		if entry.PackageID != "" && !validRuntimePackageID(entry.PackageID) {
			return RuntimePackageCatalog{}, fmt.Errorf("runtime package entry %q has unsafe package_id", entry.ModelID)
		}
		if entry.PackageRoot != "" && !filepath.IsAbs(entry.PackageRoot) {
			return RuntimePackageCatalog{}, fmt.Errorf("runtime package entry %q has non-absolute package_root", entry.ModelID)
		}
		if entry.ServingCacheCapacity < 0 || entry.PrefixSessionsMaximum < 0 {
			return RuntimePackageCatalog{}, fmt.Errorf("runtime package entry %q has negative capacity", entry.ModelID)
		}
		if (entry.CacheTopology == "") != (entry.ServingCacheCapacity == 0) {
			return RuntimePackageCatalog{}, fmt.Errorf("runtime package entry %q has an incomplete cache-capacity contract", entry.ModelID)
		}
		if entry.StreamingMode != "" && entry.StreamingMode != "incremental-sse" && entry.StreamingMode != "buffered-compatibility-sse" {
			return RuntimePackageCatalog{}, fmt.Errorf("runtime package entry %q has unsupported streaming mode %q", entry.ModelID, entry.StreamingMode)
		}
		identities := map[string]string{
			"runtime": entry.RuntimeSHA256, "native runtime": entry.NativeRuntimeSHA256,
			"MLX dylib": entry.MLXDylibSHA256, "metallib": entry.MetallibSHA256,
			"JACCL dylib": entry.JACCLSHA256,
		}
		for label, identity := range identities {
			decoded, err := hex.DecodeString(identity)
			if err != nil || len(decoded) != sha256.Size {
				return RuntimePackageCatalog{}, fmt.Errorf("runtime package entry %q has invalid %s SHA-256", entry.ModelID, label)
			}
		}
		key := entry.ModelID + "\x00" + entry.RuntimeID
		if _, duplicate := seen[key]; duplicate {
			return RuntimePackageCatalog{}, fmt.Errorf("duplicate SNE runtime selection model=%q runtime=%q", entry.ModelID, entry.RuntimeID)
		}
		seen[key] = struct{}{}
		modelCounts[entry.ModelID]++
		modelHasLegacy[entry.ModelID] = modelHasLegacy[entry.ModelID] || entry.RuntimeID == ""
	}
	for modelID, count := range modelCounts {
		if count > 1 && modelHasLegacy[modelID] {
			return RuntimePackageCatalog{}, fmt.Errorf("model %q has multiple runtime packages but an entry lacks runtime_id", modelID)
		}
	}
	return catalog, nil
}

func (catalog RuntimePackageCatalog) MaterializePackageRoots(packagesRoot string) (RuntimePackageCatalog, error) {
	root, err := filepath.Abs(strings.TrimSpace(packagesRoot))
	if err != nil || !filepath.IsAbs(root) {
		return RuntimePackageCatalog{}, fmt.Errorf("resolve SNE packages root")
	}
	root = filepath.Clean(root)
	materialized := catalog
	materialized.Entries = append([]RuntimePackage(nil), catalog.Entries...)
	seenRoots := make(map[string]string, len(materialized.Entries))
	for index := range materialized.Entries {
		entry := &materialized.Entries[index]
		if entry.PackageID != "" {
			entry.PackageRoot = filepath.Join(root, entry.PackageID)
		}
		resolved, err := filepath.Abs(entry.PackageRoot)
		if err != nil {
			return RuntimePackageCatalog{}, fmt.Errorf("materialize package root for model %q: %w", entry.ModelID, err)
		}
		resolved = filepath.Clean(resolved)
		relative, err := filepath.Rel(root, resolved)
		if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
			return RuntimePackageCatalog{}, fmt.Errorf("runtime package for model %q escapes packages root", entry.ModelID)
		}
		entry.PackageRoot = resolved
		selection := entry.ModelID + "@" + entry.RuntimeID
		if prior, duplicate := seenRoots[resolved]; duplicate && prior != selection {
			return RuntimePackageCatalog{}, fmt.Errorf("runtime selections %q and %q share package root", prior, selection)
		}
		seenRoots[resolved] = selection
	}
	return materialized, nil
}

func (catalog RuntimePackageCatalog) PortablePackageCatalog(packagesRoot string) (RuntimePackageCatalog, error) {
	root, err := filepath.Abs(strings.TrimSpace(packagesRoot))
	if err != nil || !filepath.IsAbs(root) {
		return RuntimePackageCatalog{}, fmt.Errorf("resolve SNE packages root")
	}
	root = filepath.Clean(root)
	portable := catalog
	portable.Entries = append([]RuntimePackage(nil), catalog.Entries...)
	for index := range portable.Entries {
		entry := &portable.Entries[index]
		if entry.PackageID != "" && entry.PackageRoot == "" {
			continue
		}
		if entry.PackageID != "" || entry.PackageRoot == "" {
			return RuntimePackageCatalog{}, fmt.Errorf("runtime package for model %q has mixed or missing package identity", entry.ModelID)
		}
		resolved, err := filepath.Abs(entry.PackageRoot)
		if err != nil {
			return RuntimePackageCatalog{}, fmt.Errorf("resolve package root for model %q: %w", entry.ModelID, err)
		}
		relative, err := filepath.Rel(root, filepath.Clean(resolved))
		if err != nil || !validRuntimePackageID(relative) || filepath.Base(relative) != relative {
			return RuntimePackageCatalog{}, fmt.Errorf("runtime package for model %q is not a direct governed package", entry.ModelID)
		}
		entry.PackageID = relative
		entry.PackageRoot = ""
	}
	return portable, nil
}

func validRuntimePackageID(value string) bool {
	if value == "." || value == ".." || len(value) > 160 {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || character == '.' || character == '_' || character == '-' {
			continue
		}
		return false
	}
	return value != ""
}

func (catalog RuntimePackageCatalog) Resolve(modelID string) (RuntimePackage, error) {
	return catalog.ResolveRuntime(modelID, "")
}

func (catalog RuntimePackageCatalog) ResolveRuntime(modelID, runtimeID string) (RuntimePackage, error) {
	modelID = strings.TrimSpace(modelID)
	runtimeID = strings.TrimSpace(runtimeID)
	var matched RuntimePackage
	matches := 0
	for _, entry := range catalog.Entries {
		if entry.ModelID != modelID || (runtimeID != "" && entry.EffectiveRuntimeID() != runtimeID) {
			continue
		}
		entry.RuntimeID = entry.EffectiveRuntimeID()
		matched = entry
		matches++
	}
	if matches == 1 {
		return matched, nil
	}
	if matches > 1 {
		return RuntimePackage{}, fmt.Errorf("model %q has multiple SNE runtimes; runtime_id is required", modelID)
	}
	if runtimeID != "" {
		return RuntimePackage{}, fmt.Errorf("no SNE runtime package for model %q and runtime %q", modelID, runtimeID)
	}
	return RuntimePackage{}, fmt.Errorf("no SNE runtime package for model %q", modelID)
}

// VerifyRuntimePackageBoundary proves that an admitted package is self-contained
// before Pantheon exposes or launches it. Hash identity alone is insufficient:
// a correctly hashed Mach-O can still carry a build-tree rpath or resolve a
// symlink outside the installed package after a reboot removes warm state.
func VerifyRuntimePackageBoundary(entry RuntimePackage) error {
	root, err := filepath.Abs(entry.PackageRoot)
	if err != nil {
		return fmt.Errorf("resolve SNE runtime package root: %w", err)
	}
	root = filepath.Clean(root)
	if err := verifyPackageTreeSymlinkBoundary(root); err != nil {
		return err
	}
	artifacts := []struct {
		label, relative, expected string
		macho                     bool
	}{
		{"runtime binary", "bin/sned", entry.RuntimeSHA256, true},
		{"native runtime", "lib/runtime/libsirsi_native_runtime.dylib", entry.NativeRuntimeSHA256, true},
		{"MLX dylib", "lib/libmlx.dylib", entry.MLXDylibSHA256, true},
		{"metallib", "share/mlx.metallib", entry.MetallibSHA256, false},
		{"JACCL dylib", "lib/libjaccl.dylib", entry.JACCLSHA256, true},
	}
	for _, artifact := range artifacts {
		path := filepath.Join(root, filepath.FromSlash(artifact.relative))
		if err := verifyRegularContainedFile(root, path); err != nil {
			return fmt.Errorf("SNE %s: %w", artifact.label, err)
		}
		digest, err := runtimeSHA256File(path)
		if err != nil || digest != artifact.expected {
			return fmt.Errorf("SNE %s identity mismatch", artifact.label)
		}
		if artifact.macho {
			if err := verifyMachOBoundary(root, path); err != nil {
				return fmt.Errorf("SNE %s dependency boundary: %w", artifact.label, err)
			}
		}
	}
	return nil
}

func verifyPackageTreeSymlinkBoundary(root string) error {
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("SNE runtime package root is unavailable or symlinked")
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("resolve SNE runtime package root: %w", err)
	}
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			resolved, err := filepath.EvalSymlinks(path)
			if err != nil {
				return fmt.Errorf("SNE runtime package contains broken symlink %q", path)
			}
			relative, err := filepath.Rel(resolvedRoot, resolved)
			if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
				return fmt.Errorf("SNE runtime package symlink %q escapes the package root", path)
			}
			target, err := os.Stat(resolved)
			if err != nil || !target.Mode().IsRegular() {
				return fmt.Errorf("SNE runtime package symlink %q does not resolve to a regular file", path)
			}
		}
		return nil
	})
}

func verifyRegularContainedFile(root, path string) error {
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return fmt.Errorf("artifact escapes package root")
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("artifact is unavailable, non-regular, or symlinked")
	}
	return nil
}

func verifyContainedDirectory(root, path string) error {
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return fmt.Errorf("directory escapes package root")
	}
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("directory is unavailable, non-directory, or symlinked")
	}
	return nil
}

func verifyMachOBoundary(root, path string) error {
	file, err := macho.Open(path)
	if err != nil {
		return fmt.Errorf("open Mach-O: %w", err)
	}
	defer file.Close()
	dependencies, err := file.ImportedLibraries()
	if err != nil {
		return fmt.Errorf("read Mach-O dependencies: %w", err)
	}
	for _, dependency := range dependencies {
		if err := validateMachOLoadPath(root, path, dependency, false); err != nil {
			return err
		}
	}
	for _, load := range file.Loads {
		if rpath, ok := load.(*macho.Rpath); ok {
			if err := validateMachOLoadPath(root, path, rpath.Path, true); err != nil {
				return fmt.Errorf("invalid LC_RPATH %q: %w", rpath.Path, err)
			}
		}
	}
	return nil
}

func validateMachOLoadPath(root, binaryPath, loadPath string, rpath bool) error {
	loadPath = strings.TrimSpace(loadPath)
	if loadPath == "" || strings.ContainsRune(loadPath, '\x00') {
		return fmt.Errorf("empty or malformed Mach-O load path")
	}
	if filepath.IsAbs(loadPath) {
		if strings.HasPrefix(loadPath, "/System/Library/") || strings.HasPrefix(loadPath, "/usr/lib/") {
			return nil
		}
		return fmt.Errorf("absolute dependency %q is outside the operating-system boundary", loadPath)
	}
	if strings.HasPrefix(loadPath, "@rpath/") {
		if rpath {
			return fmt.Errorf("nested @rpath is not a concrete package boundary")
		}
		name := strings.TrimPrefix(loadPath, "@rpath/")
		allowed := map[string]string{
			"libsirsi_native_runtime.dylib": "lib/runtime/libsirsi_native_runtime.dylib",
			"libmlx.dylib":                  "lib/libmlx.dylib",
			"libjaccl.dylib":                "lib/libjaccl.dylib",
		}
		relative, ok := allowed[name]
		if !ok || strings.Contains(name, "/") {
			return fmt.Errorf("unbound @rpath dependency %q", loadPath)
		}
		return verifyRegularContainedFile(root, filepath.Join(root, filepath.FromSlash(relative)))
	}
	resolve := func(marker string, base string) error {
		relative := strings.TrimPrefix(loadPath, marker)
		relative = strings.TrimPrefix(relative, "/")
		resolved := filepath.Clean(filepath.Join(base, filepath.FromSlash(relative)))
		if rpath {
			return verifyContainedDirectory(root, resolved)
		}
		return verifyRegularContainedFile(root, resolved)
	}
	if loadPath == "@loader_path" || strings.HasPrefix(loadPath, "@loader_path/") {
		return resolve("@loader_path", filepath.Dir(binaryPath))
	}
	if loadPath == "@executable_path" || strings.HasPrefix(loadPath, "@executable_path/") {
		return resolve("@executable_path", filepath.Join(root, "bin"))
	}
	return fmt.Errorf("unsupported relative Mach-O load path %q", loadPath)
}

func runtimeSHA256File(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func ensureRuntimeCatalogEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return err
	}
	return nil
}
