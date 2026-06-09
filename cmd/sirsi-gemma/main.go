// Command sirsi-gemma is a Model Context Protocol (MCP) server that exposes
// a locally-running MLX-Gemma model to MCP-capable clients (Claude Code,
// Cursor, IDE plugins) via two tools: gemma_chat and gemma_complete.
//
// Transport: JSON-RPC 2.0 over stdio (the MCP standard). The server reuses
// internal/mcp.Server for framing and dispatch — only the tool handlers and
// the MLX subprocess runner live in this package.
//
// See cmd/sirsi-gemma/README.md for architecture, docs/setup/MLX_GEMMA_LOCAL.md
// for the install layout, and docs/user-guides/sirsi-gemma.md for end-user
// docs.
//
// Rule A3: static binary, no cgo.
// Rule A11: no telemetry; all generation is local.
// Rule A16: subprocess runner is injectable (see Runner in runner.go).
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/mcp"
)

func main() {
	configPath := flag.String("config", DefaultConfigPath(), "path to gemma.toml")
	skipHealth := flag.Bool("skip-health", false, "skip the startup health probe (debugging only)")
	flag.Parse()

	logger := log.New(os.Stderr, "[sirsi-gemma] ", log.LstdFlags)
	cfg, err := LoadConfig(*configPath)
	if err != nil {
		logger.Printf("config: %v — falling back to defaults", err)
		cfg = DefaultConfig()
	}

	runner := selectRunner(cfg, *skipHealth, logger)

	// Bare server — sirsi-gemma exposes ONLY its two tools, not the full
	// Anubis toolset. A client running both the pantheon MCP server and
	// sirsi-gemma must not see scan_workspace/vault/code_* duplicated.
	srv := mcp.NewBareServer("sirsi-gemma", serverVersion,
		"𓂀 Sirsi Gemma — local MLX-Gemma over MCP. Use gemma_chat for "+
			"multi-turn chat and gemma_complete for single-shot completion. "+
			"Runs entirely on this machine via Apple MLX — no tokens billed, "+
			"no data leaves the host.",
		"[sirsi-gemma] ")
	registerGemmaTools(srv, runner)

	if err := srv.Run(); err != nil {
		logger.Fatalf("server: %v", err)
	}
}

// serverVersion is advertised in the MCP initialize handshake.
const serverVersion = "0.1.0"

// selectRunner builds the production MLX runner and probes it. On probe
// failure we substitute a disabledRunner so the MCP handshake still works
// and every tool call returns an actionable error. This means an operator
// who hasn't run chip A's install yet can still wire sirsi-gemma into
// ~/.claude/mcp.json and see exactly what's wrong.
func selectRunner(cfg Config, skipHealth bool, logger *log.Logger) Runner {
	r := NewMLXRunner(cfg)
	if skipHealth {
		logger.Println("health: skipped via --skip-health")
		return r
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := r.Health(ctx); err != nil {
		logger.Printf("health: probe failed — tools will report disabled: %v", err)
		return &disabledRunner{reason: err.Error()}
	}
	logger.Println("health: MLX-Gemma alive")
	return r
}

func registerGemmaTools(srv *mcp.Server, runner Runner) {
	srv.RegisterTool(mcp.Tool{
		Name:        "gemma_chat",
		Description: "Multi-turn chat with local MLX-Gemma. Pass a system prompt plus a history of {role,content} messages. Returns generated assistant text. Runs entirely on this machine.",
		InputSchema: mcp.InputSchema{
			Type: "object",
			Properties: map[string]mcp.SchemaField{
				"system":      {Type: "string", Description: "Optional system instruction folded into the first user turn."},
				"messages":    {Type: "array", Description: "Chat history. Each item is {role: user|assistant, content: string}."},
				"max_tokens":  {Type: "integer", Description: "Optional override; falls back to config."},
				"temperature": {Type: "number", Description: "Optional override; falls back to config."},
			},
			Required: []string{"messages"},
		},
	}, makeChatHandler(runner))

	srv.RegisterTool(mcp.Tool{
		Name:        "gemma_complete",
		Description: "Single-shot text completion from a raw prompt using local MLX-Gemma. Use for non-chat workloads (rewrite, summarize, extract). Runs entirely on this machine.",
		InputSchema: mcp.InputSchema{
			Type: "object",
			Properties: map[string]mcp.SchemaField{
				"prompt":      {Type: "string", Description: "Raw prompt text — no chat templating applied."},
				"max_tokens":  {Type: "integer", Description: "Optional override; falls back to config."},
				"temperature": {Type: "number", Description: "Optional override; falls back to config."},
			},
			Required: []string{"prompt"},
		},
	}, makeCompleteHandler(runner))
}

func makeChatHandler(runner Runner) mcp.ToolHandler {
	return func(args map[string]any) (*mcp.ToolResult, error) {
		system, _ := args["system"].(string)
		maxTok := intArg(args, "max_tokens")
		temp := floatArg(args, "temperature")

		rawMsgs, ok := args["messages"].([]any)
		if !ok {
			return errResult("messages: required, must be an array"), nil
		}
		msgs := make([]ChatMessage, 0, len(rawMsgs))
		for i, raw := range rawMsgs {
			m, ok := raw.(map[string]any)
			if !ok {
				return errResult(fmt.Sprintf("messages[%d]: not an object", i)), nil
			}
			role, _ := m["role"].(string)
			content, _ := m["content"].(string)
			msgs = append(msgs, ChatMessage{Role: role, Content: content})
		}
		prompt, err := RenderChatPrompt(system, msgs)
		if err != nil {
			return errResult(err.Error()), nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		out, err := runner.Generate(ctx, prompt, maxTok, temp)
		if err != nil {
			return errResult(err.Error()), nil
		}
		return textResult(out), nil
	}
}

func makeCompleteHandler(runner Runner) mcp.ToolHandler {
	return func(args map[string]any) (*mcp.ToolResult, error) {
		prompt, ok := args["prompt"].(string)
		if !ok || prompt == "" {
			return errResult("prompt: required, must be a non-empty string"), nil
		}
		maxTok := intArg(args, "max_tokens")
		temp := floatArg(args, "temperature")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		out, err := runner.Generate(ctx, prompt, maxTok, temp)
		if err != nil {
			return errResult(err.Error()), nil
		}
		return textResult(out), nil
	}
}

func textResult(text string) *mcp.ToolResult {
	return &mcp.ToolResult{Content: []mcp.ContentBlock{{Type: "text", Text: text}}}
}

func errResult(msg string) *mcp.ToolResult {
	return &mcp.ToolResult{Content: []mcp.ContentBlock{{Type: "text", Text: msg}}, IsError: true}
}

func intArg(args map[string]any, key string) int {
	switch v := args[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	}
	return 0
}

func floatArg(args map[string]any, key string) float64 {
	switch v := args[key].(type) {
	case float64:
		return v
	case int:
		return float64(v)
	}
	return 0
}
