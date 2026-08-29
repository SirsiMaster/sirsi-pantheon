package sne

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	supportMatrixVersionsDir = "support-matrices"
	supportMatrixCurrentLink = "support-matrix-current"
	supportMatrixFile        = "support-matrix.json"
	supportMatrixSignature   = "support-matrix.json.sig"
)

func InstallSignedSupportMatrix(storeRoot, sourceMatrix, sourceSignature, publicKeyPath string) (RuntimeCatalogInstallResult, error) {
	if _, err := LoadSignedSupportMatrix(sourceMatrix, sourceSignature, publicKeyPath); err != nil {
		return RuntimeCatalogInstallResult{}, err
	}
	matrixBytes, err := os.ReadFile(sourceMatrix)
	if err != nil {
		return RuntimeCatalogInstallResult{}, err
	}
	signatureBytes, err := os.ReadFile(sourceSignature)
	if err != nil {
		return RuntimeCatalogInstallResult{}, err
	}
	digest := sha256.Sum256(matrixBytes)
	version := hex.EncodeToString(digest[:])
	root, err := filepath.Abs(strings.TrimSpace(storeRoot))
	if err != nil || root == string(os.PathSeparator) {
		return RuntimeCatalogInstallResult{}, fmt.Errorf("invalid SNE support matrix store root")
	}
	versionsRoot := filepath.Join(root, supportMatrixVersionsDir)
	if err = os.MkdirAll(versionsRoot, 0o700); err != nil {
		return RuntimeCatalogInstallResult{}, err
	}
	versionRoot := filepath.Join(versionsRoot, version)
	if _, statErr := os.Stat(versionRoot); os.IsNotExist(statErr) {
		stage, stageErr := os.MkdirTemp(versionsRoot, ".staging-")
		if stageErr != nil {
			return RuntimeCatalogInstallResult{}, stageErr
		}
		defer os.RemoveAll(stage)
		if err = os.WriteFile(filepath.Join(stage, supportMatrixFile), matrixBytes, 0o600); err != nil {
			return RuntimeCatalogInstallResult{}, err
		}
		if err = os.WriteFile(filepath.Join(stage, supportMatrixSignature), signatureBytes, 0o600); err != nil {
			return RuntimeCatalogInstallResult{}, err
		}
		if _, err = LoadSignedSupportMatrix(filepath.Join(stage, supportMatrixFile), filepath.Join(stage, supportMatrixSignature), publicKeyPath); err != nil {
			return RuntimeCatalogInstallResult{}, err
		}
		if err = syncDirectory(stage); err != nil {
			return RuntimeCatalogInstallResult{}, err
		}
		if err = os.Rename(stage, versionRoot); err != nil {
			return RuntimeCatalogInstallResult{}, err
		}
		if err = syncDirectory(versionsRoot); err != nil {
			return RuntimeCatalogInstallResult{}, err
		}
	} else if statErr != nil {
		return RuntimeCatalogInstallResult{}, statErr
	}
	if _, err := LoadSignedSupportMatrix(filepath.Join(versionRoot, supportMatrixFile), filepath.Join(versionRoot, supportMatrixSignature), publicKeyPath); err != nil {
		return RuntimeCatalogInstallResult{}, err
	}
	previous, _ := CurrentSupportMatrixVersion(root)
	temporary := filepath.Join(root, ".support-matrix-current-new")
	_ = os.Remove(temporary)
	if err := os.Symlink(filepath.Join(supportMatrixVersionsDir, version), temporary); err != nil {
		return RuntimeCatalogInstallResult{}, err
	}
	if err := os.Rename(temporary, filepath.Join(root, supportMatrixCurrentLink)); err != nil {
		_ = os.Remove(temporary)
		return RuntimeCatalogInstallResult{}, err
	}
	if err := syncDirectory(root); err != nil {
		return RuntimeCatalogInstallResult{}, err
	}
	return RuntimeCatalogInstallResult{VersionSHA256: version, PreviousSHA256: previous}, nil
}

func CurrentSupportMatrixVersion(storeRoot string) (string, error) {
	root, err := filepath.Abs(strings.TrimSpace(storeRoot))
	if err != nil {
		return "", err
	}
	target, err := os.Readlink(filepath.Join(root, supportMatrixCurrentLink))
	if err != nil {
		return "", err
	}
	resolved := target
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(root, resolved)
	}
	relative, err := filepath.Rel(filepath.Join(root, supportMatrixVersionsDir), filepath.Clean(resolved))
	if err != nil || filepath.Base(relative) != relative || !validSHA256Hex(relative) {
		return "", fmt.Errorf("current SNE support matrix pointer escapes version store")
	}
	return relative, nil
}
