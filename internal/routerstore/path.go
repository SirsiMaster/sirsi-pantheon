package routerstore

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DefaultStorePath is the single resolver for the authoritative router store.
func DefaultStorePath() (string, error) {
	if path := strings.TrimSpace(os.Getenv("SIRSI_ROUTER_DB")); path != "" {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("routerstore: resolve home: %w", err)
	}
	return filepath.Join(home, ".sirsi", "router.db"), nil
}
