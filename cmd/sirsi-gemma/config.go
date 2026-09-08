package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config controls which local inference engine backs the common Gemma tools.
//
// Loaded from ~/.config/sirsi/gemma.toml (flat key=value). Missing file
// or missing keys fall back to the defaults below.
//
// Set engine explicitly for new configurations. Existing configurations that
// only set sne_url continue to select SNE.
//
//	engine = sne
//	sne_url = http://localhost:11434/v1
type Config struct {
	Engine      string  // "mlx", "sne", or "omlx"; empty preserves legacy autodetection
	ModelID     string  // e.g. "mlx-community/gemma-2-27b-it-4bit"
	VenvPath    string  // absolute path to the Python venv root
	MaxTokens   int     // default max tokens per generation
	Temperature float64 // default sampling temperature
	// SNE seam — when non-empty, sirsi-gemma uses SNERunner instead of MLXRunner.
	SNEURL          string // base URL of SNE's OpenAI-compatible API, e.g. "http://localhost:11434/v1"
	SNEModel        string // model name forwarded to SNE (default: "gemma-2-27b-it")
	SNEAPITokenFile string // optional private file containing the SNE bearer token
	OMLXURL         string // base URL of oMLX's OpenAI-compatible API
	OMLXModel       string // model name forwarded to oMLX
}

// DefaultConfig matches chip A's MLX_GEMMA_LOCAL.md install layout.
// The bf16- prefix on the model id is REQUIRED on mlx-lm 0.31+; the venv
// lives at ~/.venvs/mlx with the mlx_lm.generate console script in bin/.
func DefaultConfig() Config {
	home, _ := os.UserHomeDir()
	return Config{
		ModelID:     "mlx-community/gemma-2-27b-it-bf16-4bit",
		VenvPath:    filepath.Join(home, ".venvs", "mlx"),
		MaxTokens:   1024,
		Temperature: 0.7,
	}
}

// DefaultConfigPath is ~/.config/sirsi/gemma.toml.
func DefaultConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "sirsi", "gemma.toml")
}

// LoadConfig reads path and overlays values onto DefaultConfig(). A missing
// file is not an error — defaults are returned. Malformed lines fail loud.
func LoadConfig(path string) (Config, error) {
	cfg := DefaultConfig()
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		rawKey, rawVal, ok := strings.Cut(line, "=")
		if !ok {
			return cfg, fmt.Errorf("%s:%d: expected key = value", path, lineNo)
		}
		key := strings.TrimSpace(rawKey)
		val := strings.Trim(strings.TrimSpace(rawVal), `"`)
		if err := cfg.set(key, val); err != nil {
			return cfg, fmt.Errorf("%s:%d: %w", path, lineNo, err)
		}
	}
	return cfg, scanner.Err()
}

func (c *Config) set(key, val string) error {
	switch key {
	case "engine":
		val = strings.ToLower(val)
		if val != "mlx" && val != "sne" && val != "omlx" {
			return fmt.Errorf("engine must be mlx, sne, or omlx")
		}
		c.Engine = val
	case "model_id":
		c.ModelID = val
	case "venv_path":
		c.VenvPath = expandHome(val)
	case "max_tokens":
		n, err := strconv.Atoi(val)
		if err != nil {
			return fmt.Errorf("max_tokens: %w", err)
		}
		c.MaxTokens = n
	case "temperature":
		f, err := strconv.ParseFloat(val, 64)
		if err != nil {
			return fmt.Errorf("temperature: %w", err)
		}
		c.Temperature = f
	case "sne_url":
		c.SNEURL = val
	case "sne_model":
		c.SNEModel = val
	case "sne_api_token_file":
		c.SNEAPITokenFile = expandHome(val)
	case "omlx_url":
		c.OMLXURL = val
	case "omlx_model":
		c.OMLXModel = val
	default:
		return fmt.Errorf("unknown key %q", key)
	}
	return nil
}

// EffectiveEngine resolves the explicit selector while preserving the original
// sne_url-only configuration contract.
func (c Config) EffectiveEngine() string {
	if c.Engine != "" {
		return c.Engine
	}
	if c.SNEURL != "" {
		return "sne"
	}
	if c.OMLXURL != "" {
		return "omlx"
	}
	return "mlx"
}

func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}
