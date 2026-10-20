package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/provider"
)

func TestGemmaToolTurnRequiresExactProfileAndExplicitAllowlistBeforeNetwork(t *testing.T) {
	oldProfile, oldAllowed, oldTokens := gemmaToolTurnProfile, gemmaToolTurnAllowed, gemmaToolTurnTokens
	defer func() {
		gemmaToolTurnProfile, gemmaToolTurnAllowed, gemmaToolTurnTokens = oldProfile, oldAllowed, oldTokens
	}()
	no, yes := false, true
	write := func(p provider.LocalProfile) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), "profile.json")
		b, err := json.Marshal(p)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, b, 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}

	gemmaToolTurnTokens = 32
	gemmaToolTurnAllowed = []string{"process.census"}
	gemmaToolTurnProfile = write(provider.LocalProfile{ID: "wrong", Endpoint: "http://127.0.0.1:9/v1", Model: "model", ContextTokens: 512, OutputTokens: 64, Streaming: &no, ToolCalls: &yes})
	if err := runGemmaToolTurn(gemmaToolTurnCmd, []string{"status"}); err == nil || !strings.Contains(err.Error(), "profile id") {
		t.Fatalf("wrong profile error = %v", err)
	}

	gemmaToolTurnProfile = write(provider.LocalProfile{ID: apollo21ProfileID, Endpoint: "http://127.0.0.1:9/v1", Model: "model", ContextTokens: 512, OutputTokens: 64, Streaming: &no, ToolCalls: &no})
	if err := runGemmaToolTurn(gemmaToolTurnCmd, []string{"status"}); err == nil || !strings.Contains(err.Error(), "tool_calls") {
		t.Fatalf("no tool capability error = %v", err)
	}

	gemmaToolTurnProfile = write(provider.LocalProfile{ID: apollo21ProfileID, Endpoint: "http://127.0.0.1:9/v1", Model: "model", ContextTokens: 512, OutputTokens: 64, Streaming: &no, ToolCalls: &yes})
	gemmaToolTurnAllowed = nil
	if err := runGemmaToolTurn(gemmaToolTurnCmd, []string{"status"}); err == nil || !strings.Contains(err.Error(), "allowlist") {
		t.Fatalf("missing allowlist error = %v", err)
	}
}
