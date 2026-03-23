package mcp

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// sendRequest encodes a JSON-RPC request and sends it to the server via RunWithIO.
func sendRequest(t *testing.T, method string, params interface{}, id int) (*Response, error) {
	t.Helper()

	req := Request{
		JSONRPC: "2.0",
		Method:  method,
		ID:      mustMarshalRaw(id),
	}

	if params != nil {
		data, err := json.Marshal(params)
		if err != nil {
			return nil, err
		}
		req.Params = data
	}

	input, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	reader := strings.NewReader(string(input) + "\n")
	var output bytes.Buffer

	server := NewServer()
	err = server.RunWithIO(reader, &output)
	if err != nil {
		return nil, err
	}

	// Parse response
	var resp Response
	if output.Len() > 0 {
		if err := json.Unmarshal(output.Bytes(), &resp); err != nil {
			return nil, err
		}
	}

	return &resp, nil
}

func mustMarshalRaw(v interface{}) json.RawMessage {
	data, _ := json.Marshal(v)
	return data
}

func TestNewServer(t *testing.T) {
	s := NewServer()
	if s == nil {
		t.Fatal("NewServer() returned nil")
	}
	if len(s.tools) == 0 {
		t.Error("Server should have registered tools")
	}
	if len(s.resources) == 0 {
		t.Error("Server should have registered resources")
	}
}

func TestInitialize(t *testing.T) {
	params := InitializeParams{
		ProtocolVersion: "2025-03-26",
		Capabilities: ClientCaps{
			Roots: &RootsCap{ListChanged: true},
		},
		ClientInfo: NameVer{
			Name:    "test-client",
			Version: "1.0.0",
		},
	}

	resp, err := sendRequest(t, "initialize", params, 1)
	if err != nil {
		t.Fatalf("Initialize error: %v", err)
	}

	if resp.Error != nil {
		t.Fatalf("Initialize returned error: %v", resp.Error)
	}

	// Parse result
	resultData, err := json.Marshal(resp.Result)
	if err != nil {
		t.Fatalf("Marshal result: %v", err)
	}

	var result InitializeResult
	if err := json.Unmarshal(resultData, &result); err != nil {
		t.Fatalf("Unmarshal result: %v", err)
	}

	if result.ProtocolVersion != ProtocolVersion {
		t.Errorf("ProtocolVersion = %q, want %q", result.ProtocolVersion, ProtocolVersion)
	}
	if result.ServerInfo.Name != ServerName {
		t.Errorf("ServerInfo.Name = %q, want %q", result.ServerInfo.Name, ServerName)
	}
	if result.Capabilities.Tools == nil {
		t.Error("Server should declare tools capability")
	}
	if result.Capabilities.Resources == nil {
		t.Error("Server should declare resources capability")
	}
	if result.Instructions == "" {
		t.Error("Server should have instructions")
	}
}

func TestToolsList(t *testing.T) {
	resp, err := sendRequest(t, "tools/list", nil, 2)
	if err != nil {
		t.Fatalf("tools/list error: %v", err)
	}

	if resp.Error != nil {
		t.Fatalf("tools/list returned error: %v", resp.Error)
	}

	// Parse result
	resultData, err := json.Marshal(resp.Result)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var result struct {
		Tools []Tool `json:"tools"`
	}
	if err := json.Unmarshal(resultData, &result); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if len(result.Tools) < 3 {
		t.Errorf("Expected at least 3 tools, got %d", len(result.Tools))
	}

	// Verify expected tools exist
	toolNames := make(map[string]bool)
	for _, tool := range result.Tools {
		toolNames[tool.Name] = true
		if tool.Description == "" {
			t.Errorf("Tool %q has empty description", tool.Name)
		}
	}

	for _, expected := range []string{"scan_workspace", "ghost_report", "health_check", "classify_files"} {
		if !toolNames[expected] {
			t.Errorf("Missing expected tool: %s", expected)
		}
	}
}

func TestResourcesList(t *testing.T) {
	resp, err := sendRequest(t, "resources/list", nil, 3)
	if err != nil {
		t.Fatalf("resources/list error: %v", err)
	}

	if resp.Error != nil {
		t.Fatalf("resources/list returned error: %v", resp.Error)
	}

	resultData, err := json.Marshal(resp.Result)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var result struct {
		Resources []Resource `json:"resources"`
	}
	if err := json.Unmarshal(resultData, &result); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if len(result.Resources) < 3 {
		t.Errorf("Expected at least 3 resources, got %d", len(result.Resources))
	}

	// Verify expected resources
	uris := make(map[string]bool)
	for _, res := range result.Resources {
		uris[res.URI] = true
	}

	for _, expected := range []string{"anubis://health-status", "anubis://capabilities", "anubis://brain-status"} {
		if !uris[expected] {
			t.Errorf("Missing expected resource: %s", expected)
		}
	}
}

func TestPing(t *testing.T) {
	resp, err := sendRequest(t, "ping", nil, 4)
	if err != nil {
		t.Fatalf("ping error: %v", err)
	}

	if resp.Error != nil {
		t.Fatalf("ping returned error: %v", resp.Error)
	}
}

func TestMethodNotFound(t *testing.T) {
	resp, err := sendRequest(t, "nonexistent/method", nil, 5)
	if err != nil {
		t.Fatalf("request error: %v", err)
	}

	if resp.Error == nil {
		t.Fatal("Expected error for unknown method")
	}

	if resp.Error.Code != ErrCodeMethodNotFound {
		t.Errorf("Error code = %d, want %d", resp.Error.Code, ErrCodeMethodNotFound)
	}
}

func TestResourcesRead_Capabilities(t *testing.T) {
	params := ResourceReadParams{URI: "anubis://capabilities"}

	resp, err := sendRequest(t, "resources/read", params, 6)
	if err != nil {
		t.Fatalf("resources/read error: %v", err)
	}

	if resp.Error != nil {
		t.Fatalf("resources/read returned error: %v", resp.Error)
	}

	// Parse result
	resultData, err := json.Marshal(resp.Result)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var result ResourceReadResult
	if err := json.Unmarshal(resultData, &result); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if len(result.Contents) == 0 {
		t.Fatal("Expected at least one content item")
	}

	content := result.Contents[0]
	if content.URI != "anubis://capabilities" {
		t.Errorf("URI = %q, want %q", content.URI, "anubis://capabilities")
	}
	if content.MimeType != "application/json" {
		t.Errorf("MimeType = %q, want %q", content.MimeType, "application/json")
	}
	if content.Text == "" {
		t.Error("Content text should not be empty")
	}

	// Verify it's valid JSON
	var caps map[string]interface{}
	if err := json.Unmarshal([]byte(content.Text), &caps); err != nil {
		t.Errorf("Content text is not valid JSON: %v", err)
	}
}

func TestResourcesRead_UnknownURI(t *testing.T) {
	params := ResourceReadParams{URI: "anubis://nonexistent"}

	resp, err := sendRequest(t, "resources/read", params, 7)
	if err != nil {
		t.Fatalf("resources/read error: %v", err)
	}

	if resp.Error == nil {
		t.Fatal("Expected error for unknown resource URI")
	}
}

func TestToolsCall_HealthCheck(t *testing.T) {
	params := ToolCallParams{
		Name:      "health_check",
		Arguments: map[string]interface{}{},
	}

	resp, err := sendRequest(t, "tools/call", params, 8)
	if err != nil {
		t.Fatalf("tools/call error: %v", err)
	}

	if resp.Error != nil {
		t.Fatalf("tools/call returned error: %v", resp.Error)
	}

	// Parse result
	resultData, err := json.Marshal(resp.Result)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var result ToolResult
	if err := json.Unmarshal(resultData, &result); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if len(result.Content) == 0 {
		t.Fatal("Expected at least one content block")
	}

	if result.Content[0].Type != "text" {
		t.Errorf("Content type = %q, want %q", result.Content[0].Type, "text")
	}

	if !strings.Contains(result.Content[0].Text, "Anubis Health Check") {
		t.Error("Health check should contain 'Anubis Health Check' in output")
	}
}

func TestToolsCall_UnknownTool(t *testing.T) {
	params := ToolCallParams{
		Name:      "nonexistent_tool",
		Arguments: map[string]interface{}{},
	}

	resp, err := sendRequest(t, "tools/call", params, 9)
	if err != nil {
		t.Fatalf("tools/call error: %v", err)
	}

	if resp.Error != nil {
		t.Fatalf("Unexpected JSON-RPC error: %v", resp.Error)
	}

	// Unknown tool should return isError=true in the result
	resultData, _ := json.Marshal(resp.Result)
	var result ToolResult
	if err := json.Unmarshal(resultData, &result); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if !result.IsError {
		t.Error("Unknown tool should set isError=true")
	}
}

func TestToolsCall_ClassifyFiles(t *testing.T) {
	params := ToolCallParams{
		Name: "classify_files",
		Arguments: map[string]interface{}{
			"paths": "/test/main.go, /tmp/debug.log, /data/export.csv",
		},
	}

	resp, err := sendRequest(t, "tools/call", params, 10)
	if err != nil {
		t.Fatalf("tools/call error: %v", err)
	}

	if resp.Error != nil {
		t.Fatalf("tools/call returned error: %v", resp.Error)
	}

	resultData, _ := json.Marshal(resp.Result)
	var result ToolResult
	if err := json.Unmarshal(resultData, &result); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if result.IsError {
		t.Fatal("classify_files should not return error for valid paths")
	}

	if len(result.Content) == 0 {
		t.Fatal("Expected content in classify_files result")
	}

	// Should be valid JSON containing classifications
	if !strings.Contains(result.Content[0].Text, "classifications") {
		t.Error("classify_files result should contain 'classifications'")
	}
}

func TestParseCategory(t *testing.T) {
	tests := []struct {
		input   string
		wantErr bool
	}{
		{"general", false},
		{"dev", false},
		{"developer", false},
		{"ai", false},
		{"ml", false},
		{"vms", false},
		{"ides", false},
		{"cloud", false},
		{"storage", false},
		{"invalid", true},
		{"", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			_, err := parseCategory(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseCategory(%q) error = %v, wantErr = %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestShortenHomePath(t *testing.T) {
	result := shortenHomePath("/nonexistent/path")
	if result != "/nonexistent/path" {
		t.Errorf("Non-home path should be unchanged, got %q", result)
	}
}

func TestTextResult(t *testing.T) {
	r := textResult("hello", false)
	if r.IsError {
		t.Error("IsError should be false")
	}
	if len(r.Content) != 1 {
		t.Fatalf("Expected 1 content block, got %d", len(r.Content))
	}
	if r.Content[0].Type != "text" {
		t.Errorf("Type = %q, want %q", r.Content[0].Type, "text")
	}
	if r.Content[0].Text != "hello" {
		t.Errorf("Text = %q, want %q", r.Content[0].Text, "hello")
	}

	// Error variant
	re := textResult("error msg", true)
	if !re.IsError {
		t.Error("IsError should be true for error result")
	}
}

func TestResourcesRead_HealthStatus(t *testing.T) {
	params := ResourceReadParams{URI: "anubis://health-status"}
	resp, err := sendRequest(t, "resources/read", params, 11)
	if err != nil {
		t.Fatalf("Read health-status error: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("Read health-status returned error: %v", resp.Error)
	}
}

func TestResourcesRead_BrainStatus(t *testing.T) {
	params := ResourceReadParams{URI: "anubis://brain-status"}
	resp, err := sendRequest(t, "resources/read", params, 12)
	if err != nil {
		t.Fatalf("Read brain-status error: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("Read brain-status returned error: %v", resp.Error)
	}
}

func TestToolsCall_ThothReadMemory_NotFound(t *testing.T) {
	params := ToolCallParams{
		Name: "thoth_read_memory",
		Arguments: map[string]interface{}{
			"path": "/tmp/nonexistent-thoth-project-dir",
		},
	}
	resp, err := sendRequest(t, "tools/call", params, 13)
	if err != nil {
		t.Fatalf("thoth_read_memory error: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("thoth_read_memory returned error: %v", resp.Error)
	}

	resultData, _ := json.Marshal(resp.Result)
	var result ToolResult
	json.Unmarshal(resultData, &result)
	if !strings.Contains(result.Content[0].Text, "No Thoth memory file found") {
		t.Errorf("Expected 'No Thoth memory file found', got %q", result.Content[0].Text)
	}
}

func TestToolsCall_ScanWorkspace_InvalidCategory(t *testing.T) {
	params := ToolCallParams{
		Name: "scan_workspace",
		Arguments: map[string]interface{}{
			"category": "invalid-category-name",
		},
	}
	resp, err := sendRequest(t, "tools/call", params, 14)
	if err != nil {
		t.Fatalf("scan_workspace error: %v", err)
	}

	resultData, _ := json.Marshal(resp.Result)
	var result ToolResult
	json.Unmarshal(resultData, &result)
	if !result.IsError {
		t.Error("Invalid category should set isError=true")
	}
}
