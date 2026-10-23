package mcp

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/mercury"
)

func TestReadMercuryStatusReturnsSharedReadJSON(t *testing.T) {
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	want := mercury.Read{
		State:   "partial",
		Message: "Mercury snapshot · host-1",
		ReadAt:  now,
		Total:   nil,
	}
	called := false
	result, err := readMercuryStatus(nil, func(ctx context.Context) mercury.Read {
		called = true
		if ctx.Done() != nil {
			t.Error("reader context has a cancellation channel; want background context")
		}
		if _, ok := ctx.Deadline(); ok {
			t.Error("reader context has a deadline; the shared reader owns the HTTP deadline")
		}
		return want
	})
	if err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("reader was not called")
	}
	if result.IsError || len(result.Content) != 1 {
		t.Fatalf("result = %+v", result)
	}
	if result.Content[0].Type != "text" || result.Content[0].MimeType != "application/json" {
		t.Fatalf("content = %+v", result.Content[0])
	}
	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	if result.Content[0].Text != string(wantJSON) {
		t.Fatalf("JSON = %s, want exact shared Read %s", result.Content[0].Text, wantJSON)
	}
}

func TestReadMercuryStatusRejectsArgumentsBeforeReading(t *testing.T) {
	called := false
	result, err := readMercuryStatus(map[string]interface{}{"endpoint": "http://127.0.0.1:1/api/v1/dashboard"}, func(context.Context) mercury.Read {
		called = true
		return mercury.Read{}
	})
	if err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("reader was called for invalid arguments")
	}
	if !result.IsError {
		t.Fatalf("result = %+v, want an argument error", result)
	}
}

func TestReadMercuryStatusPreservesUnavailableUnknowns(t *testing.T) {
	want := mercury.Read{
		State:   "offline",
		Message: "Mercury monitor is unavailable. Check its connection and refresh.",
	}
	result, err := readMercuryStatus(map[string]interface{}{}, func(context.Context) mercury.Read { return want })
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError || len(result.Content) != 1 {
		t.Fatalf("result = %+v", result)
	}
	var got mercury.Read
	if err := json.Unmarshal([]byte(result.Content[0].Text), &got); err != nil {
		t.Fatal(err)
	}
	if got.State != want.State || got.Snapshot != nil || got.Headline != nil || got.Scene != nil ||
		got.Outgoing != nil || got.Incoming != nil || got.Total != nil {
		t.Fatalf("unavailable read fabricated data: %+v", got)
	}
}

func TestMercuryToolRegistrationAndToolsListSchemaParity(t *testing.T) {
	server := NewServer()
	registered := 0
	for _, tool := range server.tools {
		if tool.Name != "mercury_status" {
			continue
		}
		registered++
		if tool.InputSchema.Type != "object" || len(tool.InputSchema.Properties) != 0 || len(tool.InputSchema.Required) != 0 {
			t.Fatalf("input schema = %+v", tool.InputSchema)
		}
		if tool.InputSchema.AdditionalProperties == nil || *tool.InputSchema.AdditionalProperties {
			t.Fatalf("additionalProperties = %v, want false", tool.InputSchema.AdditionalProperties)
		}
	}
	if registered != 1 {
		t.Fatalf("mercury_status registrations = %d, want 1", registered)
	}
	if server.toolHandlers["mercury_status"] == nil {
		t.Fatal("mercury_status handler is not registered")
	}

	response := server.handleToolsList(&Request{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "tools/list"})
	if response == nil || response.Error != nil {
		t.Fatalf("tools/list response = %+v", response)
	}
	raw, err := json.Marshal(response.Result)
	if err != nil {
		t.Fatal(err)
	}
	var listed struct {
		Tools []Tool `json:"tools"`
	}
	if err := json.Unmarshal(raw, &listed); err != nil {
		t.Fatal(err)
	}
	listedCount := 0
	for _, tool := range listed.Tools {
		if tool.Name == "mercury_status" {
			listedCount++
			if tool.InputSchema.AdditionalProperties == nil || *tool.InputSchema.AdditionalProperties {
				t.Fatalf("listed schema additionalProperties = %v, want false", tool.InputSchema.AdditionalProperties)
			}
		}
	}
	if listedCount != 1 {
		t.Fatalf("tools/list mercury_status count = %d, want 1", listedCount)
	}
}
