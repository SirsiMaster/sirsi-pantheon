// Command sirsi-gemma is a Model Context Protocol (MCP) server that exposes
// a locally-running Gemma model to MCP-capable clients (Claude Code,
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
		"𓂀 Sirsi Gemma — local inference over MCP. Use gemma_chat for "+
			"multi-turn chat and gemma_complete for single-shot completion. "+
			"Select SNE, SNE Native v2, MLX, or OMLX without changing either tool — no tokens billed, "+
			"no data leaves the host.",
		"[sirsi-gemma] ")
	registerGemmaTools(srv, runner)

	if err := srv.Run(); err != nil {
		logger.Fatalf("server: %v", err)
	}
}

// serverVersion is advertised in the MCP initialize handshake.
const serverVersion = "0.2.0"

// selectRunner builds the configured Pantheon Engine ABI adapter and probes it.
// On probe failure a disabledRunner is returned so the MCP handshake still works.
func selectRunner(cfg Config, skipHealth bool, logger *log.Logger) Runner {
	var r Runner
	engine := cfg.EffectiveEngine()
	switch engine {
	case "sne":
		if !isLocalEngineURL(cfg.SNEURL) {
			return &disabledRunner{reason: "engine=sne requires a loopback sne_url"}
		}
		logger.Printf("runner: SNE seam active — %s (model %s)", cfg.SNEURL, cfg.SNEModel)
		r = NewSNERunner(cfg.SNEURL, cfg.SNEModel)
	case "sne-native-v2":
		if !isLocalEngineURL(cfg.SNENativeV2URL) {
			return &disabledRunner{reason: "engine=sne-native-v2 requires a loopback sne_native_v2_url"}
		}
		if cfg.SNENativeV2HostProfile != "m1" && cfg.SNENativeV2HostProfile != "m5" {
			return &disabledRunner{reason: "engine=sne-native-v2 requires sne_native_v2_host_profile=m1 or m5"}
		}
		logger.Printf("runner: SNE Native v2 seam active — %s (model %s)", cfg.SNENativeV2URL, cfg.SNENativeV2Model)
		r = NewSNENativeV2Runner(cfg.SNENativeV2URL, cfg.SNENativeV2Model, cfg.SNENativeV2HostProfile)
	case "omlx":
		if !isLocalEngineURL(cfg.OMLXURL) {
			return &disabledRunner{reason: "engine=omlx requires a loopback omlx_url"}
		}
		logger.Printf("runner: OMLX seam active — %s (model %s)", cfg.OMLXURL, cfg.OMLXModel)
		r = NewOMLXRunner(cfg.OMLXURL, cfg.OMLXModel)
	default:
		r = NewMLXRunner(cfg)
	}
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
	logger.Printf("health: %s engine alive", engine)
	return r
}

func registerGemmaTools(srv *mcp.Server, runner Runner) {
	srv.RegisterTool(mcp.Tool{
		Name:        "gemma_chat",
		Description: "Multi-turn chat with the selected local SNE, SNE Native v2, MLX, or OMLX engine. Pass a system prompt plus a history of {role,content} messages. Returns generated assistant text.",
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
		Description: "Single-shot text completion from a raw prompt using the selected local SNE, SNE Native v2, MLX, or OMLX engine. Use for non-chat workloads (rewrite, summarize, extract).",
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
		return textResult(out, runner), nil
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
		return textResult(out, runner), nil
	}
}

func textResult(text string, runner Runner) *mcp.ToolResult {
	result := &mcp.ToolResult{Content: []mcp.ContentBlock{{Type: "text", Text: text}}}
	if identified, ok := runner.(resultIdentityRunner); ok {
		if identity, present := identified.ResultIdentity(); present {
			result.Content = append(result.Content, mcp.ContentBlock{Type: "text", Text: identity.JSON()})
		}
	}
	return result
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
