package sne

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	runtimeCatalogVersionsDir = "runtime-catalogs"
	runtimeCatalogCurrentLink = "runtime-catalog-current"
	runtimeCatalogFile        = "runtime-packages.json"
	runtimeCatalogSignature   = "runtime-packages.json.sig"
)

type RuntimeCatalogInstallResult struct {
	VersionSHA256  string
	PreviousSHA256 string
}

func InstallSignedRuntimeCatalog(storeRoot, sourceCatalog, sourceSignature, publicKeyPath, packagesRoot string) (RuntimeCatalogInstallResult, error) {
	catalog, err := LoadSignedRuntimePackageCatalog(sourceCatalog, sourceSignature, publicKeyPath)
	if err != nil {
		return RuntimeCatalogInstallResult{}, err
	}
	if _, err := catalog.MaterializePackageRoots(packagesRoot); err != nil {
		return RuntimeCatalogInstallResult{}, err
	}
	catalogBytes, err := os.ReadFile(sourceCatalog)
	if err != nil {
		return RuntimeCatalogInstallResult{}, err
	}
	signatureBytes, err := os.ReadFile(sourceSignature)
	if err != nil {
		return RuntimeCatalogInstallResult{}, err
	}
	digest := sha256.Sum256(catalogBytes)
	version := hex.EncodeToString(digest[:])
	root, versionsRoot, err := prepareRuntimeCatalogStore(storeRoot)
	if err != nil {
		return RuntimeCatalogInstallResult{}, err
	}
	versionRoot := filepath.Join(versionsRoot, version)
	if _, err := os.Stat(versionRoot); os.IsNotExist(err) {
		staging, err := os.MkdirTemp(versionsRoot, ".staging-")
		if err != nil {
			return RuntimeCatalogInstallResult{}, err
		}
		defer os.RemoveAll(staging)
		if err := os.WriteFile(filepath.Join(staging, runtimeCatalogFile), catalogBytes, 0o600); err != nil {
			return RuntimeCatalogInstallResult{}, err
		}
		if err := os.WriteFile(filepath.Join(staging, runtimeCatalogSignature), signatureBytes, 0o600); err != nil {
			return RuntimeCatalogInstallResult{}, err
		}
		if err := VerifyRuntimeCatalogSignature(filepath.Join(staging, runtimeCatalogFile), filepath.Join(staging, runtimeCatalogSignature), publicKeyPath); err != nil {
			return RuntimeCatalogInstallResult{}, err
		}
		if err := syncDirectory(staging); err != nil {
			return RuntimeCatalogInstallResult{}, err
		}
		if err := os.Rename(staging, versionRoot); err != nil {
			return RuntimeCatalogInstallResult{}, err
		}
		if err := syncDirectory(versionsRoot); err != nil {
			return RuntimeCatalogInstallResult{}, err
		}
	} else if err != nil {
		return RuntimeCatalogInstallResult{}, err
	}
	if err := verifyStoredRuntimeCatalog(versionRoot, publicKeyPath, packagesRoot, version); err != nil {
		return RuntimeCatalogInstallResult{}, err
	}
	previous, _ := CurrentRuntimeCatalogVersion(root)
	if err := switchRuntimeCatalogCurrent(root, version); err != nil {
		return RuntimeCatalogInstallResult{}, err
	}
	return RuntimeCatalogInstallResult{VersionSHA256: version, PreviousSHA256: previous}, nil
}

func CurrentRuntimeCatalogVersion(storeRoot string) (string, error) {
	root, err := filepath.Abs(strings.TrimSpace(storeRoot))
	if err != nil {
		return "", err
	}
	link := filepath.Join(root, runtimeCatalogCurrentLink)
	target, err := os.Readlink(link)
	if err != nil {
		return "", err
	}
	resolved := target
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(root, resolved)
	}
	resolved = filepath.Clean(resolved)
	versionsRoot := filepath.Join(root, runtimeCatalogVersionsDir)
	relative, err := filepath.Rel(versionsRoot, resolved)
	if err != nil || filepath.Base(relative) != relative || !validSHA256Hex(relative) {
		return "", fmt.Errorf("current SNE runtime catalog pointer escapes version store")
	}
	return relative, nil
}

func CurrentRuntimeCatalogPaths(storeRoot string) (catalogPath, signaturePath string, err error) {
	version, err := CurrentRuntimeCatalogVersion(storeRoot)
	if err != nil {
		return "", "", err
	}
	root, err := filepath.Abs(strings.TrimSpace(storeRoot))
	if err != nil {
		return "", "", err
	}
	versionRoot := filepath.Join(root, runtimeCatalogVersionsDir, version)
	return filepath.Join(versionRoot, runtimeCatalogFile), filepath.Join(versionRoot, runtimeCatalogSignature), nil
}

func ListRuntimeCatalogVersions(storeRoot string) ([]string, error) {
	root, err := filepath.Abs(strings.TrimSpace(storeRoot))
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(filepath.Join(root, runtimeCatalogVersionsDir))
	if err != nil {
		return nil, err
	}
	versions := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() && validSHA256Hex(entry.Name()) {
			versions = append(versions, entry.Name())
		}
	}
	sort.Strings(versions)
	return versions, nil
}

func RollbackSignedRuntimeCatalog(storeRoot, version, publicKeyPath, packagesRoot string) error {
	if !validSHA256Hex(version) {
		return fmt.Errorf("invalid SNE runtime catalog version")
	}
	root, versionsRoot, err := prepareRuntimeCatalogStore(storeRoot)
	if err != nil {
		return err
	}
	if err := verifyStoredRuntimeCatalog(filepath.Join(versionsRoot, version), publicKeyPath, packagesRoot, version); err != nil {
		return err
	}
	return switchRuntimeCatalogCurrent(root, version)
}

func RemoveInactiveRuntimeCatalog(storeRoot, version string) error {
	if !validSHA256Hex(version) {
		return fmt.Errorf("invalid SNE runtime catalog version")
	}
	current, err := CurrentRuntimeCatalogVersion(storeRoot)
	if err != nil {
		return err
	}
	if current == version {
		return fmt.Errorf("cannot remove active SNE runtime catalog")
	}
	root, err := filepath.Abs(strings.TrimSpace(storeRoot))
	if err != nil {
		return err
	}
	versionRoot := filepath.Join(root, runtimeCatalogVersionsDir, version)
	entries, err := os.ReadDir(versionRoot)
	if err != nil {
		return err
	}
	if len(entries) != 2 {
		return fmt.Errorf("runtime catalog version contains unexpected files")
	}
	for _, name := range []string{runtimeCatalogFile, runtimeCatalogSignature} {
		info, err := os.Lstat(filepath.Join(versionRoot, name))
		if err != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("runtime catalog version has unsafe %s", name)
		}
	}
	if err := os.Remove(filepath.Join(versionRoot, runtimeCatalogFile)); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(versionRoot, runtimeCatalogSignature)); err != nil {
		return err
	}
	if err := os.Remove(versionRoot); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(versionRoot))
}

func prepareRuntimeCatalogStore(storeRoot string) (string, string, error) {
	root, err := filepath.Abs(strings.TrimSpace(storeRoot))
	if err != nil || root == string(os.PathSeparator) {
		return "", "", fmt.Errorf("invalid SNE runtime catalog store root")
	}
	versionsRoot := filepath.Join(root, runtimeCatalogVersionsDir)
	if err := os.MkdirAll(versionsRoot, 0o700); err != nil {
		return "", "", err
	}
	return root, versionsRoot, nil
}

func verifyStoredRuntimeCatalog(versionRoot, publicKeyPath, packagesRoot, expectedVersion string) error {
	catalogPath := filepath.Join(versionRoot, runtimeCatalogFile)
	signaturePath := filepath.Join(versionRoot, runtimeCatalogSignature)
	catalogBytes, err := os.ReadFile(catalogPath)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(catalogBytes)
	if hex.EncodeToString(digest[:]) != expectedVersion {
		return fmt.Errorf("stored SNE runtime catalog version mismatch")
	}
	catalog, err := LoadSignedRuntimePackageCatalog(catalogPath, signaturePath, publicKeyPath)
	if err != nil {
		return err
	}
	_, err = catalog.MaterializePackageRoots(packagesRoot)
	return err
}

func switchRuntimeCatalogCurrent(root, version string) error {
	temporary := filepath.Join(root, ".runtime-catalog-current-new")
	_ = os.Remove(temporary)
	if err := os.Symlink(filepath.Join(runtimeCatalogVersionsDir, version), temporary); err != nil {
		return err
	}
	if err := os.Rename(temporary, filepath.Join(root, runtimeCatalogCurrentLink)); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return syncDirectory(root)
}

func validSHA256Hex(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && strings.ToLower(value) == value
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
