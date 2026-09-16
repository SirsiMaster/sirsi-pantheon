package mcp

import (
	"os"
	"testing"
)

func TestHandleRouterNotify_DisabledByDefault(t *testing.T) {
	// SIRSI_ROUTER_NOTIFY is not set — should return error
	os.Unsetenv("SIRSI_ROUTER_NOTIFY")

	result, err := handleRouterNotify(map[string]interface{}{
		"target":   "codex",
		"doc_type": "proposal",
		"doc_id":   "test-id",
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when SIRSI_ROUTER_NOTIFY is not set")
	}
}

func TestHandleRouterNotify_MissingArgs(t *testing.T) {
	t.Setenv("SIRSI_ROUTER_NOTIFY", "1")

	tests := []struct {
		name string
		args map[string]interface{}
	}{
		{"missing target", map[string]interface{}{"doc_type": "proposal", "doc_id": "id"}},
		{"missing doc_type", map[string]interface{}{"target": "codex", "doc_id": "id"}},
		{"missing doc_id", map[string]interface{}{"target": "codex", "doc_type": "proposal"}},
		{"all empty", map[string]interface{}{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := handleRouterNotify(tt.args)
			if err != nil {
				t.Fatalf("unexpected Go error: %v", err)
			}
			if !result.IsError {
				t.Error("expected IsError=true for missing args")
			}
		})
	}
}

func TestHandleRouterNotify_InvalidTarget(t *testing.T) {
	t.Setenv("SIRSI_ROUTER_NOTIFY", "1")

	result, err := handleRouterNotify(map[string]interface{}{
		"target":   "mallory",
		"doc_type": "proposal",
		"doc_id":   "test-id",
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	// NotifyAgent validates target — should fail for non-whitelisted agent
	if !result.IsError {
		t.Error("expected IsError=true for invalid target 'mallory'")
	}
}

func TestHandleRouterClose_MissingID(t *testing.T) {
	result, err := handleRouterClose(map[string]interface{}{"result": "done"})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when id is missing")
	}
}

func TestHandleRouterSubmit_MissingArgs(t *testing.T) {
	tests := []struct {
		name string
		args map[string]interface{}
	}{
		{"missing type", map[string]interface{}{"author": "claude", "title": "t", "content": "c"}},
		{"missing author", map[string]interface{}{"type": "proposal", "title": "t", "content": "c"}},
		{"missing title", map[string]interface{}{"type": "proposal", "author": "claude", "content": "c"}},
		{"missing content", map[string]interface{}{"type": "proposal", "author": "claude", "title": "t"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := handleRouterSubmit(tt.args)
			if err != nil {
				t.Fatalf("unexpected Go error: %v", err)
			}
			if !result.IsError {
				t.Error("expected IsError=true for missing args")
			}
		})
	}
}
