package dashboard

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const sneLocalAccessTokenFilename = "sne-local-api.token"

func DefaultSNELocalAccessTokenPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home for SNE local capability: %w", err)
	}
	return filepath.Join(home, "Library", "Application Support", "Sirsi", "Pantheon", sneLocalAccessTokenFilename), nil
}

func LoadOrCreateDefaultSNELocalAccessToken() (string, error) {
	path, err := DefaultSNELocalAccessTokenPath()
	if err != nil {
		return "", err
	}
	return LoadOrCreateSNELocalAccessToken(path)
}

// LoadOrCreateSNELocalAccessToken returns a restart-stable capability from a
// private regular file. It never follows a symlink and never replaces an
// existing file, so concurrent startup cannot silently rotate the credential.
func LoadOrCreateSNELocalAccessToken(path string) (string, error) {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "." || !filepath.IsAbs(path) {
		return "", fmt.Errorf("SNE local capability path must be absolute")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", fmt.Errorf("create SNE capability directory: %w", err)
	}
	if err := os.Chmod(filepath.Dir(path), 0o700); err != nil {
		return "", fmt.Errorf("secure SNE capability directory: %w", err)
	}
	if token, found, err := readSNELocalAccessToken(path); err != nil || found {
		return token, err
	}

	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("generate SNE local capability: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(random)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if os.IsExist(err) {
		loaded, found, readErr := readSNELocalAccessToken(path)
		if readErr != nil {
			return "", readErr
		}
		if !found {
			return "", fmt.Errorf("SNE capability appeared but could not be read")
		}
		return loaded, nil
	}
	if err != nil {
		return "", fmt.Errorf("create SNE local capability: %w", err)
	}
	if _, err := file.WriteString(token + "\n"); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return "", fmt.Errorf("write SNE local capability: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return "", fmt.Errorf("sync SNE local capability: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("close SNE local capability: %w", err)
	}
	return token, nil
}

func RotateSNELocalAccessToken(path string) (string, error) {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "." || !filepath.IsAbs(path) {
		return "", fmt.Errorf("SNE local capability path must be absolute")
	}
	if _, found, err := readSNELocalAccessToken(path); err != nil || !found {
		if err != nil {
			return "", err
		}
		return "", fmt.Errorf("SNE local capability does not exist")
	}
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("generate rotated SNE local capability: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(random)
	temporary, err := os.CreateTemp(filepath.Dir(path), ".sne-local-api-rotate-")
	if err != nil {
		return "", fmt.Errorf("stage rotated SNE local capability: %w", err)
	}
	temporaryPath := temporary.Name()
	cleanup := func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}
	if err := temporary.Chmod(0o600); err != nil {
		cleanup()
		return "", fmt.Errorf("secure rotated SNE local capability: %w", err)
	}
	if _, err := temporary.WriteString(token + "\n"); err != nil {
		cleanup()
		return "", fmt.Errorf("write rotated SNE local capability: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		cleanup()
		return "", fmt.Errorf("sync rotated SNE local capability: %w", err)
	}
	if err := temporary.Close(); err != nil {
		_ = os.Remove(temporaryPath)
		return "", fmt.Errorf("close rotated SNE local capability: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		_ = os.Remove(temporaryPath)
		return "", fmt.Errorf("commit rotated SNE local capability: %w", err)
	}
	return token, nil
}

func readSNELocalAccessToken(path string) (string, bool, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("inspect SNE local capability: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return "", false, fmt.Errorf("SNE local capability must be a regular non-symlink file")
	}
	if info.Mode().Perm()&0o077 != 0 {
		return "", false, fmt.Errorf("SNE local capability permissions must not grant group or other access")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false, fmt.Errorf("read SNE local capability: %w", err)
	}
	token := strings.TrimSpace(string(data))
	if len(token) < 32 || strings.ContainsAny(token, " \t\r\n") {
		return "", false, fmt.Errorf("SNE local capability is malformed")
	}
	return token, true, nil
}
