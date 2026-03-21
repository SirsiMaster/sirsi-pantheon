// Package stealth provides ephemeral mode — clean-exit functionality.
// When enabled, Anubis removes all traces of itself after execution.
package stealth

import (
	"os"
	"path/filepath"
)

// CleanExit removes all Anubis-generated files and caches.
// This includes config, profiles, maps, brain weights, and logs.
// The binary itself is NOT deleted (user downloaded it intentionally).
func CleanExit() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	// Paths to remove
	targets := []string{
		filepath.Join(home, ".config", "anubis"), // Config, profiles, seba maps
		filepath.Join(home, ".anubis"),           // Brain weights, cache
		filepath.Join(home, ".cache", "anubis"),  // Any cached data
	}

	for _, target := range targets {
		if _, err := os.Stat(target); err == nil {
			if err := os.RemoveAll(target); err != nil {
				return err
			}
		}
	}

	return nil
}

// CleanBrain removes only the neural brain weights.
func CleanBrain() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	weightsDir := filepath.Join(home, ".anubis", "weights")
	if _, err := os.Stat(weightsDir); err == nil {
		return os.RemoveAll(weightsDir)
	}
	return nil
}

// CleanCache removes only temporary scan caches.
func CleanCache() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	cacheDir := filepath.Join(home, ".cache", "anubis")
	if _, err := os.Stat(cacheDir); err == nil {
		return os.RemoveAll(cacheDir)
	}
	return nil
}
