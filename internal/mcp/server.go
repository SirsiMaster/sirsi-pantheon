// Package mcp implements a Model Context Protocol (MCP) server for Anubis.
// MCP allows AI coding assistants (Claude, Cursor, Windsurf, Codex) to use
// Anubis as a context sanitizer — scanning workspaces before indexing.
//
// Protocol: JSON-RPC 2.0 over stdio (stdin/stdout).
// Spec: https://modelcontextprotocol.io/specification/2025-03-26
//
// Rule A11: No telemetry. All scanning is local.
// Rule A4: Network operations require explicit opt-in.
package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"sync"
)

const (
	// ProtocolVersion is the MCP protocol version we support.
	ProtocolVersion = "2025-03-26"

	// ServerName is our MCP server identity.
	ServerName = "sirsi-anubis"

	// ServerVersion is the current Anubis version.
	ServerVersion = "0.2.0-alpha"
)

// ---- JSON-RPC 2.0 Types ----

// Request is a JSON-RPC 2.0 request.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response is a JSON-RPC 2.0 response.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// RPCError is a JSON-RPC 2.0 error object.
type RPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// Standard JSON-RPC error codes.
const (
	ErrCodeParse          = -32700
	ErrCodeInvalidRequest = -32600
	ErrCodeMethodNotFound = -32601
	ErrCodeInvalidParams  = -32602
	ErrCodeInternal       = -32603
)

// ---- MCP Protocol Types ----

// InitializeParams from the client.
type InitializeParams struct {
	ProtocolVersion string     `json:"protocolVersion"`
	Capabilities    ClientCaps `json:"capabilities"`
	ClientInfo      NameVer    `json:"clientInfo"`
}

// ClientCaps are the client's declared capabilities.
type ClientCaps struct {
	Roots    *RootsCap    `json:"roots,omitempty"`
	Sampling *SamplingCap `json:"sampling,omitempty"`
}

// RootsCap for roots capability.
type RootsCap struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// SamplingCap for sampling capability.
type SamplingCap struct{}

// NameVer is a name + version pair.
type NameVer struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// InitializeResult is the server's response to initialize.
type InitializeResult struct {
	ProtocolVersion string     `json:"protocolVersion"`
	Capabilities    ServerCaps `json:"capabilities"`
	ServerInfo      NameVer    `json:"serverInfo"`
	Instructions    string     `json:"instructions,omitempty"`
}

// ServerCaps are the server's declared capabilities.
type ServerCaps struct {
	Tools     *ToolsCap     `json:"tools,omitempty"`
	Resources *ResourcesCap `json:"resources,omitempty"`
}

// ToolsCap for the tools capability.
type ToolsCap struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// ResourcesCap for the resources capability.
type ResourcesCap struct {
	Subscribe   bool `json:"subscribe,omitempty"`
	ListChanged bool `json:"listChanged,omitempty"`
}

// Tool describes an available tool.
type Tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema InputSchema `json:"inputSchema"`
}

// InputSchema is a JSON Schema for tool parameters.
type InputSchema struct {
	Type       string                 `json:"type"`
	Properties map[string]SchemaField `json:"properties,omitempty"`
	Required   []string               `json:"required,omitempty"`
}

// SchemaField is a single field in the input schema.
type SchemaField struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

// ToolCallParams are the parameters for tools/call.
type ToolCallParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments,omitempty"`
}

// ToolResult is the result of a tool call.
type ToolResult struct {
	Content []ContentBlock `json:"content"`
	IsError bool           `json:"isError,omitempty"`
}

// ContentBlock is a piece of content in a tool result or resource.
type ContentBlock struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
}

// Resource describes an available resource.
type Resource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

// ResourceReadParams are the parameters for resources/read.
type ResourceReadParams struct {
	URI string `json:"uri"`
}

// ResourceReadResult is the result of reading a resource.
type ResourceReadResult struct {
	Contents []ResourceContent `json:"contents"`
}

// ResourceContent is a single resource content item.
type ResourceContent struct {
	URI      string `json:"uri"`
	MimeType string `json:"mimeType,omitempty"`
	Text     string `json:"text,omitempty"`
}

// ---- Server Implementation ----

// Server is the MCP server that handles JSON-RPC messages over stdio.
type Server struct {
	mu           sync.Mutex
	initialized  bool
	tools        []Tool
	toolHandlers map[string]ToolHandler
	resources    []Resource
	resHandlers  map[string]ResourceHandler
	logger       *log.Logger
}

// ToolHandler is a function that handles a tool call.
type ToolHandler func(args map[string]interface{}) (*ToolResult, error)

// ResourceHandler is a function that provides resource content.
type ResourceHandler func() (*ResourceContent, error)

// NewServer creates a new MCP server with the standard Anubis tools and resources.
func NewServer() *Server {
	s := &Server{
		toolHandlers: make(map[string]ToolHandler),
		resHandlers:  make(map[string]ResourceHandler),
		logger:       log.New(os.Stderr, "[anubis-mcp] ", log.LstdFlags),
	}

	// Register tools
	registerTools(s)

	// Register resources
	registerResources(s)

	return s
}

// RegisterTool adds a tool to the server.
func (s *Server) RegisterTool(tool Tool, handler ToolHandler) {
	s.tools = append(s.tools, tool)
	s.toolHandlers[tool.Name] = handler
}

// RegisterResource adds a resource to the server.
func (s *Server) RegisterResource(resource Resource, handler ResourceHandler) {
	s.resources = append(s.resources, resource)
	s.resHandlers[resource.URI] = handler
}

// Run starts the MCP server, reading from stdin and writing to stdout.
// This blocks until stdin is closed or an error occurs.
func (s *Server) Run() error {
	s.logger.Println("𓂀 Anubis MCP server starting (stdio mode)")

	reader := bufio.NewReader(os.Stdin)
	writer := os.Stdout

	return s.serve(reader, writer)
}

// RunWithIO starts the MCP server with custom reader/writer (for testing).
func (s *Server) RunWithIO(reader io.Reader, writer io.Writer) error {
	return s.serve(bufio.NewReader(reader), writer)
}

// serve is the main message loop.
func (s *Server) serve(reader *bufio.Reader, writer io.Writer) error {
	decoder := json.NewDecoder(reader)

	for {
		var req Request
		if err := decoder.Decode(&req); err != nil {
			if err == io.EOF {
				s.logger.Println("Client disconnected (EOF)")
				return nil
			}
			// Send parse error
			s.writeResponse(writer, Response{
				JSONRPC: "2.0",
				Error: &RPCError{
					Code:    ErrCodeParse,
					Message: fmt.Sprintf("Parse error: %v", err),
				},
			})
			continue
		}

		resp := s.handleRequest(&req)
		if resp != nil {
			s.writeResponse(writer, *resp)
		}
	}
}

// handleRequest routes a request to the appropriate handler.
func (s *Server) handleRequest(req *Request) *Response {
	s.logger.Printf("→ %s", req.Method)

	switch req.Method {
	case "initialize":
		return s.handleInitialize(req)
	case "notifications/initialized":
		// Notification — no response needed
		s.logger.Println("  Client initialized")
		return nil
	case "ping":
		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  map[string]interface{}{},
		}
	case "tools/list":
		return s.handleToolsList(req)
	case "tools/call":
		return s.handleToolsCall(req)
	case "resources/list":
		return s.handleResourcesList(req)
	case "resources/read":
		return s.handleResourcesRead(req)
	default:
		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &RPCError{
				Code:    ErrCodeMethodNotFound,
				Message: fmt.Sprintf("Method not found: %s", req.Method),
			},
		}
	}
}

// handleInitialize handles the MCP initialize handshake.
func (s *Server) handleInitialize(req *Request) *Response {
	s.mu.Lock()
	s.initialized = true
	s.mu.Unlock()

	result := InitializeResult{
		ProtocolVersion: ProtocolVersion,
		Capabilities: ServerCaps{
			Tools:     &ToolsCap{ListChanged: false},
			Resources: &ResourcesCap{Subscribe: false, ListChanged: false},
		},
		ServerInfo: NameVer{
			Name:    ServerName,
			Version: ServerVersion,
		},
		Instructions: "𓂀 Sirsi Anubis — Context Sanitizer for AI Development. " +
			"Use scan_workspace to check a project directory for waste, " +
			"ghost_report to find remnants of uninstalled apps, and " +
			"health_check for a quick system health summary. " +
			"All operations are local — no data leaves this machine.",
	}

	s.logger.Printf("  Initialized (protocol: %s)", ProtocolVersion)

	return &Response{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  result,
	}
}

// handleToolsList returns the list of available tools.
func (s *Server) handleToolsList(req *Request) *Response {
	return &Response{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: map[string]interface{}{
			"tools": s.tools,
		},
	}
}

// handleToolsCall executes a tool and returns the result.
func (s *Server) handleToolsCall(req *Request) *Response {
	var params ToolCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &RPCError{
				Code:    ErrCodeInvalidParams,
				Message: fmt.Sprintf("Invalid params: %v", err),
			},
		}
	}

	handler, ok := s.toolHandlers[params.Name]
	if !ok {
		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: &ToolResult{
				Content: []ContentBlock{
					{Type: "text", Text: fmt.Sprintf("Unknown tool: %s", params.Name)},
				},
				IsError: true,
			},
		}
	}

	s.logger.Printf("  Calling tool: %s", params.Name)

	result, err := handler(params.Arguments)
	if err != nil {
		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: &ToolResult{
				Content: []ContentBlock{
					{Type: "text", Text: fmt.Sprintf("Tool error: %v", err)},
				},
				IsError: true,
			},
		}
	}

	return &Response{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  result,
	}
}

// handleResourcesList returns the list of available resources.
func (s *Server) handleResourcesList(req *Request) *Response {
	return &Response{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: map[string]interface{}{
			"resources": s.resources,
		},
	}
}

// handleResourcesRead returns the content of a resource.
func (s *Server) handleResourcesRead(req *Request) *Response {
	var params ResourceReadParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &RPCError{
				Code:    ErrCodeInvalidParams,
				Message: fmt.Sprintf("Invalid params: %v", err),
			},
		}
	}

	handler, ok := s.resHandlers[params.URI]
	if !ok {
		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &RPCError{
				Code:    ErrCodeInvalidParams,
				Message: fmt.Sprintf("Unknown resource: %s", params.URI),
			},
		}
	}

	content, err := handler()
	if err != nil {
		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &RPCError{
				Code:    ErrCodeInternal,
				Message: fmt.Sprintf("Resource error: %v", err),
			},
		}
	}

	return &Response{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: ResourceReadResult{
			Contents: []ResourceContent{*content},
		},
	}
}

// writeResponse writes a JSON-RPC response to the writer.
func (s *Server) writeResponse(writer io.Writer, resp Response) {
	data, err := json.Marshal(resp)
	if err != nil {
		s.logger.Printf("Failed to marshal response: %v", err)
		return
	}
	// MCP stdio: one JSON object per line
	_, _ = writer.Write(data)
	_, _ = writer.Write([]byte("\n"))
}
