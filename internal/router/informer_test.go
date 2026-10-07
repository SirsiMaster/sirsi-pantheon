package router

import "testing"

// TestDeliveryStrategyFor_TypeTable exercises ADR-065 Decision 3a's lookup
// table directly: each registered type/session-mode combination must resolve
// to exactly the strategy the ADR names, including the legacy-default
// fallback (no session_mode set) that isInteractiveSpawn already encodes.
func TestDeliveryStrategyFor_TypeTable(t *testing.T) {
	cases := []struct {
		name string
		cfg  AgentConfig
		want string
	}{
		{"codex", AgentConfig{ID: "a", Type: "codex"}, StrategyFilesystemPush},
		{"claude interactive", AgentConfig{ID: "a", Type: "claude", Wake: WakeConfig{SessionMode: "interactive"}}, StrategySessionMessage},
		{"claude headless", AgentConfig{ID: "a", Type: "claude", Wake: WakeConfig{SessionMode: "headless"}}, StrategyCLISpawn},
		{"claude legacy default (no session_mode)", AgentConfig{ID: "a", Type: "claude"}, StrategySessionMessage},
		{"gemma", AgentConfig{ID: "a", Type: "gemma"}, StrategyResidentNotify},
		{"qwen", AgentConfig{ID: "a", Type: "qwen"}, StrategyResidentNotify},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := DeliveryStrategyFor(tc.cfg)
			if err != nil {
				t.Fatalf("DeliveryStrategyFor: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// TestDeliveryStrategyFor_UnknownTypeErrors is the negative control for an
// unregistered type: the table must refuse to guess rather than default to
// some strategy silently.
func TestDeliveryStrategyFor_UnknownTypeErrors(t *testing.T) {
	_, err := DeliveryStrategyFor(AgentConfig{ID: "a", Type: "human"})
	if err == nil {
		t.Fatal("expected an error for an unmapped type, got nil")
	}
}

// TestDeliveryStrategyNarrowsNotWidens is the task-3 regression test for
// Decision 3a's "a lane may narrow but not widen its type's strategy (a
// codex lane cannot opt into HTTP)." A declared Delivery.Target that matches
// its strategy's one allowed address narrows cleanly; any other declared
// Target — including the HTTP-shaped "endpoint" on a codex lane, and any
// address at all on a strategy with no address concept — is a widen attempt
// and must be refused, not silently accepted.
func TestDeliveryStrategyNarrowsNotWidens(t *testing.T) {
	t.Run("codex narrows to its own spool-dir", func(t *testing.T) {
		cfg := AgentConfig{ID: "codex-a", Type: "codex", Delivery: &DeliveryConfig{Target: "spool-dir", SpoolDir: "/relay/codex-a"}}
		got, err := DeliveryStrategyFor(cfg)
		if err != nil {
			t.Fatalf("DeliveryStrategyFor: %v", err)
		}
		if got != StrategyFilesystemPush {
			t.Errorf("got %q, want %q", got, StrategyFilesystemPush)
		}
	})

	t.Run("codex widening to endpoint is refused", func(t *testing.T) {
		cfg := AgentConfig{ID: "codex-a", Type: "codex", Delivery: &DeliveryConfig{Target: "endpoint", Endpoint: "https://example"}}
		if _, err := DeliveryStrategyFor(cfg); err == nil {
			t.Error("expected a widen-attempt error for codex+endpoint, got nil — rs-22e's no-network boundary would be bypassed by construction")
		}
	})

	t.Run("claude headless declaring spool-dir is refused", func(t *testing.T) {
		cfg := AgentConfig{ID: "claude-a", Type: "claude", Wake: WakeConfig{SessionMode: "headless"}, Delivery: &DeliveryConfig{Target: "spool-dir", SpoolDir: "/relay/claude-a"}}
		if _, err := DeliveryStrategyFor(cfg); err == nil {
			t.Error("expected an error — cli-spawn has no address to narrow, spool-dir is not a subset of it")
		}
	})

	t.Run("no declared Delivery leaves the mandated strategy untouched", func(t *testing.T) {
		got, err := DeliveryStrategyFor(AgentConfig{ID: "codex-a", Type: "codex"})
		if err != nil {
			t.Fatalf("DeliveryStrategyFor: %v", err)
		}
		if got != StrategyFilesystemPush {
			t.Errorf("got %q, want %q", got, StrategyFilesystemPush)
		}
	})
}
