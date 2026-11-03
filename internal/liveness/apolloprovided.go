package liveness

import (
	"fmt"
	"os"
	"path/filepath"
)

// ApolloAppPath is the installed Apollo.app bundle.
func ApolloAppPath(home string) string {
	return filepath.Join(home, "Applications", "Apollo.app")
}

// ApolloRouterPath is the router helper inside the installed Apollo.app.
func ApolloRouterPath(home string) string {
	return filepath.Join(ApolloAppPath(home), "Contents", "Helpers", "apollo-router")
}

// ApolloProvidesGemma reports whether local Gemma is provided by Apollo.app
// (owner decision 2026-10-28: repoint Pantheon to Apollo, then retire the r7
// ai.sirsi.gemma-broker). When true, Apollo.app owns the model's lifecycle —
// it loads backends on demand and unloads them when idle — so Pantheon must
// neither write nor restore its own broker: that would hold a second copy of
// the same model in memory.
func ApolloProvidesGemma(home string) bool {
	provided, err := CheckApolloGemmaProvider(home)
	return provided && err == nil
}

// CheckApolloGemmaProvider distinguishes absence from an invalid installed
// marker. Callers must not fall back to legacy lifecycle actions on an error.
// This checks only installation shape, not signature, liveness, model readiness,
// or the selected host endpoint. It never probes, launches, or changes a route.
func CheckApolloGemmaProvider(home string) (bool, error) {
	if home == "" {
		return false, fmt.Errorf("Apollo provider check requires a home directory")
	}
	fi, err := os.Lstat(ApolloRouterPath(home))
	if os.IsNotExist(err) {
		// Only a wholly absent Apollo.app means "not provided". An app bundle
		// that exists without its helper (e.g. mid-upgrade) fails closed.
		_, appErr := os.Lstat(ApolloAppPath(home))
		if os.IsNotExist(appErr) {
			return false, nil
		}
		if appErr != nil {
			return false, fmt.Errorf("inspect installed Apollo.app: %w", appErr)
		}
		return false, fmt.Errorf("installed Apollo.app is incomplete: router helper missing")
	}
	if err != nil {
		return false, fmt.Errorf("inspect installed Apollo router: %w", err)
	}
	if !fi.Mode().IsRegular() || fi.Mode().Perm()&0o111 == 0 {
		return false, fmt.Errorf("installed Apollo router must be a regular executable file")
	}
	return true, nil
}
