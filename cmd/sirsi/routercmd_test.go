package main

import "testing"

func TestCanonicalInferenceRecipientUsesApolloLane(t *testing.T) {
	for _, alias := range []string{"sne", "inference", "codex-sne-runtime", "claude-inference", "  SNE  "} {
		if got := canonicalInferenceRecipient(alias); got != "codex-apollo" {
			t.Errorf("canonicalInferenceRecipient(%q) = %q, want codex-apollo", alias, got)
		}
	}
	if got := canonicalInferenceRecipient("codex-apollo"); got != "codex-apollo" {
		t.Errorf("canonicalInferenceRecipient(codex-apollo) = %q, want codex-apollo", got)
	}
}
