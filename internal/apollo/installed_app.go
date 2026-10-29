package apollo

// This bridge consumes the installed Apollo app, not the signed native fleet
// profile. Apollo owns admission, model switching and idle teardown on this
// route. Pantheon never starts a worker or falls back to another model/tier.
import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"
	"unicode/utf8"
)

const (
	InstalledAppModel     = "mlx-community/Qwen3.5-9B-4bit"
	InstalledAppRoute     = InstalledAppModel + "@apollo:2.2.0"
	InstalledAppPrecision = "Q4-affine-g64-bf16"
	installedAppEndpoint  = "http://127.0.0.1:8520/v1"
	// Public checkpoint identity, not a transport credential. A named digest
	// keeps its role distinct from credential-shaped assignment syntax.
	qwenVocabularySHA256 = "87a7830d63fcf43bf241c3c5242e96e62dd3fdc29224ca26fed8ea333db72de4"
)

// InstalledAppProfile is an exact compiled release binding, not a resident-model
// observation or a numerical certification. Precision includes activation dtype.
type InstalledAppProfile struct {
	Version            string   `json:"version"`
	Model              string   `json:"model"`
	Route              string   `json:"route"`
	Precision          string   `json:"precision"`
	Endpoint           string   `json:"endpoint"`
	Modes              []string `json:"modes"`
	CheckpointRevision string   `json:"checkpoint_revision"`
	TokenizerSHA256    string   `json:"tokenizer_sha256"`
	TemplateSHA256     string   `json:"template_sha256"`
}

func installedAppProfile() InstalledAppProfile {
	return InstalledAppProfile{
		Version: "2.2.6", Model: InstalledAppModel, Route: InstalledAppRoute,
		Precision: InstalledAppPrecision, Endpoint: installedAppEndpoint,
		Modes:              []string{"plain", "flash", "mlx-certified"},
		CheckpointRevision: "8b2b98c00a6b4d291155e4890773ca8f769aee53",
		TokenizerSHA256:    qwenVocabularySHA256,
		TemplateSHA256:     "a4aee8afcf2e0711942cf848899be66016f8d14a889ff9ede07bca099c28f715",
	}
}

func IsInstalledAppModel(model string) bool {
	return model == "qwen3.5-9b" || model == InstalledAppModel || model == InstalledAppRoute
}

var installedAppFiles = map[string]string{
	"Helpers/apollo-router":               "2aa330c40c09f947bbda69cbb7e1bbbd3089205559ea5d31742d0f66e4c6f769",
	"Helpers/qwen35_serve":                "1c3ede1cacc7bb9b6c3b57517596a52166fb832771e6e4a30dcf15b4aa672a3a",
	"Helpers/qwen35_flash":                "27bbe5cac64b43911cb2a325f39dc69b254aaebf49e023d9620fabb2a3f4677a",
	"Resources/qwen/apollo_qwen_serve.py": "15fa38b17ac4bf29f870f13ba8273239d725c14ac59d021b564bb6bc707d2257",
	"MacOS/Apollo":                        "7d8b43cf9a3fcfb677f918ffc1aba4fb42b3eb069eaeb4e2c76d962c50463ad3",
}

// InstalledApp reads only fixed installation locations. It accepts no profile,
// endpoint, model path or environment override. A newer app needs a reviewed
// binding update; merely finding a listener never qualifies this release.
func InstalledApp(home string) (InstalledAppProfile, error) {
	for _, bundle := range []string{filepath.Join(home, "Applications", "Apollo.app"), "/Applications/Apollo.app"} {
		if _, err := os.Stat(bundle); os.IsNotExist(err) {
			continue
		}
		// The first existing installation is authoritative. Never hide drift
		// in the per-user app by finding a stale system-wide copy instead.
		if err := verifyInstalledApp(bundle, installedAppFiles); err != nil {
			return InstalledAppProfile{}, fmt.Errorf("installed Apollo 2.2.6 binding unavailable: %w", err)
		}
		return installedAppProfile(), nil
	}
	return InstalledAppProfile{}, fmt.Errorf("installed Apollo 2.2.6 app is unavailable")
}

func verifyInstalledApp(bundle string, files map[string]string) error {
	for path, expected := range files {
		name := filepath.Join(bundle, "Contents", filepath.FromSlash(path))
		info, err := os.Lstat(name)
		if err != nil {
			return fmt.Errorf("read installed Apollo artifact %s: %w", path, err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("installed Apollo artifact %s is not a regular file", path)
		}
		f, err := os.Open(name)
		if err != nil {
			return fmt.Errorf("open installed Apollo artifact %s: %w", path, err)
		}
		h := sha256.New()
		_, readErr := io.Copy(h, f)
		closeErr := f.Close()
		if readErr != nil {
			return fmt.Errorf("hash installed Apollo artifact %s: %w", path, readErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close installed Apollo artifact %s: %w", path, closeErr)
		}
		if hex.EncodeToString(h.Sum(nil)) != expected {
			return fmt.Errorf("installed Apollo artifact %s differs from release 2.2.6", path)
		}
	}
	return nil
}

type InstalledAppRequest struct {
	Model           string
	Precision       string
	Mode            string
	Messages        []ChatMessage
	MaxOutputTokens uint32
}

type installedAppWireRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	MaxTokens   uint32        `json:"max_tokens"`
	Temperature int           `json:"temperature"`
	Stream      bool          `json:"stream"`
	Apollo      struct {
		Mode string `json:"mode"`
	} `json:"apollo"`
	Template struct {
		EnableThinking bool `json:"enable_thinking"`
	} `json:"chat_template_kwargs"`
}

type InstalledAppResult struct {
	Contract       string `json:"contract"`
	Model          string `json:"model"`
	Precision      string `json:"precision"`
	Mode           string `json:"mode"`
	Tier           string `json:"tier"`
	Text           string `json:"text"`
	FinishReason   string `json:"finish_reason"`
	AdmissionOwner string `json:"admission_owner"`
}

func appWireRequest(in InstalledAppRequest) (installedAppWireRequest, error) {
	var out installedAppWireRequest
	if !IsInstalledAppModel(in.Model) || in.Precision != InstalledAppPrecision || in.MaxOutputTokens == 0 || in.MaxOutputTokens > 4096 {
		return out, fmt.Errorf("Apollo app request requires Qwen3.5-9B, precision %s and 1..4096 output tokens", InstalledAppPrecision)
	}
	tier := in.Mode
	if tier == "flash" {
		tier = "flash-max"
	}
	if in.Mode != "plain" && in.Mode != "flash" && in.Mode != "mlx-certified" {
		return out, fmt.Errorf("Apollo app mode must be plain, flash or mlx-certified (no tier fallback)")
	}
	if len(in.Messages) != 1 || in.Messages[0].Role != "user" || len(in.Messages[0].Content) == 0 || len(in.Messages[0].Content) > 32768 || !utf8.ValidString(in.Messages[0].Content) {
		return out, fmt.Errorf("Apollo app requires one UTF-8 user message of 1..32768 bytes")
	}
	out.Model, out.Messages, out.MaxTokens = InstalledAppRoute, in.Messages, in.MaxOutputTokens
	out.Apollo.Mode = tier
	return out, nil
}

// ChatInstalledApp never retries a POST, follows a redirect, uses an HTTP proxy,
// or resolves a hostname. Context cancellation closes the request; the existing
// Apollo service owns model admission/cancellation/cleanup, not native fences.
func ChatInstalledApp(ctx context.Context, home string, in InstalledAppRequest) (InstalledAppResult, error) {
	if _, err := appWireRequest(in); err != nil {
		return InstalledAppResult{}, err
	}
	if _, err := InstalledApp(home); err != nil {
		return InstalledAppResult{}, err
	}
	client := installedAppHTTPClient()
	defer client.CloseIdleConnections()
	return chatInstalledApp(ctx, client, in)
}

func installedAppHTTPClient() *http.Client {
	transport := &http.Transport{Proxy: nil, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		if network != "tcp" || address != "127.0.0.1:8520" {
			return nil, fmt.Errorf("Apollo app transport refused a non-installed destination")
		}
		return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "tcp", "127.0.0.1:8520")
	}}
	return &http.Client{Transport: transport, Timeout: 120 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
}

// The transport seam is private and only tests replace it. The URL is fixed even
// in fixtures; no production caller can provide an alternate endpoint/client.
func chatInstalledApp(ctx context.Context, client *http.Client, in InstalledAppRequest) (InstalledAppResult, error) {
	var result InstalledAppResult
	wire, err := appWireRequest(in)
	if err != nil {
		return result, err
	}
	raw, err := json.Marshal(wire)
	if err != nil {
		return result, fmt.Errorf("encode Apollo app request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, installedAppEndpoint+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return result, fmt.Errorf("prepare Apollo app request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return result, fmt.Errorf("Apollo app request failed (not retried): %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return result, fmt.Errorf("Apollo app refused request: HTTP %d (not retried)", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024+1))
	if err != nil {
		return result, fmt.Errorf("read Apollo app response: %w", err)
	}
	if len(data) > 4*1024*1024 {
		return result, fmt.Errorf("Apollo app response exceeds 4 MiB")
	}
	var body struct {
		Model  string `json:"model"`
		Apollo struct {
			Engine  string          `json:"engine"`
			Model   string          `json:"model"`
			Tier    string          `json:"tier"`
			Drafter json.RawMessage `json:"drafter"`
		} `json:"apollo"`
		Choices []struct {
			Index   int `json:"index"`
			Message struct {
				Role      string          `json:"role"`
				Content   *string         `json:"content"`
				ToolCalls json.RawMessage `json:"tool_calls"`
			} `json:"message"`
			Finish string `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &body); err != nil {
		return result, fmt.Errorf("decode Apollo app response: %w", err)
	}
	var drafter string
	drafterOK := bytes.Equal(bytes.TrimSpace(body.Apollo.Drafter), []byte("null"))
	if wire.Apollo.Mode != "plain" {
		drafterOK = json.Unmarshal(body.Apollo.Drafter, &drafter) == nil && drafter != ""
	}
	if body.Model != InstalledAppModel || body.Apollo.Model != InstalledAppModel || body.Apollo.Engine != "apollo-qwen" || body.Apollo.Tier != wire.Apollo.Mode || !drafterOK {
		return result, fmt.Errorf("Apollo app response model/tier/drafter differs from the explicit binding")
	}
	if len(body.Choices) != 1 || body.Choices[0].Index != 0 {
		return result, fmt.Errorf("Apollo app returned an invalid choice set")
	}
	c := body.Choices[0]
	if c.Message.Role != "assistant" || c.Message.Content == nil || (len(c.Message.ToolCalls) != 0 && string(c.Message.ToolCalls) != "null" && string(c.Message.ToolCalls) != "[]") || (c.Finish != "stop" && c.Finish != "length") {
		return result, fmt.Errorf("Apollo app returned an incomplete or unsupported text completion")
	}
	return InstalledAppResult{Contract: "sirsi.apollo.installed-loopback.v1", Model: body.Model, Precision: in.Precision, Mode: in.Mode, Tier: body.Apollo.Tier, Text: *c.Message.Content, FinishReason: c.Finish, AdmissionOwner: "installed Apollo service"}, nil
}
