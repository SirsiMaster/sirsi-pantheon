package liveness

import (
	"os"
	"path/filepath"
)

// ApolloRouterPort is the loopback port Apollo.app's router serves its
// OpenAI-compatible API on (GET /v1/models, POST /v1/chat/completions).
const ApolloRouterPort = 8520

// ApolloRouterPath is the router helper inside the installed Apollo.app.
func ApolloRouterPath(home string) string {
	return filepath.Join(home, "Applications", "Apollo.app", "Contents", "Helpers", "apollo-router")
}

// ApolloProvidesGemma reports whether local Gemma is provided by Apollo.app
// (owner decision 2026-10-28: repoint Pantheon to Apollo, then retire the r7
// ai.sirsi.gemma-broker). When true, Apollo.app owns the model's lifecycle —
// it loads backends on demand and unloads them when idle — so Pantheon must
// neither write nor restore its own broker: that would hold a second copy of
// the same model in memory.
func ApolloProvidesGemma(home string) bool {
	_, err := os.Stat(ApolloRouterPath(home))
	return err == nil
}
