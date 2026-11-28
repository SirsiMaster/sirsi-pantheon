package apollo

// This bridge consumes the installed Apollo app, not the signed native fleet
// profile. Apollo owns admission, model switching and idle teardown on this
// route. Pantheon never starts a worker or falls back to another model/tier.
import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	InstalledAppModel     = "mlx-community/Qwen3.5-9B-4bit"
	InstalledAppPrecision = "Q4-affine-g64-bf16"
	installedAppEndpoint  = "http://127.0.0.1:8520/v1"
	installedAppRevision  = "8b2b98c00a6b4d291155e4890773ca8f769aee53"
	// Public checkpoint identity, not a transport credential. A named digest
	// keeps its role distinct from credential-shaped assignment syntax.
	qwenVocabularySHA256 = "87a7830d63fcf43bf241c3c5242e96e62dd3fdc29224ca26fed8ea333db72de4"
	// The app is bound by its code signature, not per-release file hashes:
	// bundle id ai.sirsi.apollo, signed by a Developer ID Application leaf
	// (Apple Developer ID CA) of team 9D382WV988. codesign --strict --deep
	// checks every sealed file and nested helper, so any changed byte, a
	// re-signed copy or another team's app refuses. Same requirement as the
	// app's designated requirement.
	installedAppRequirement = `anchor apple generic and identifier "ai.sirsi.apollo" and certificate 1[field.1.2.840.113635.100.6.2.6] and certificate leaf[field.1.2.840.113635.100.6.1.13] and certificate leaf[subject.OU] = "9D382WV988"`
)

var (
	installedAppVersionPattern = regexp.MustCompile(`^[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}$`)
	installedAppRoutePattern   = regexp.MustCompile(`^` + regexp.QuoteMeta(InstalledAppModel) + `@apollo:[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}$`)
)

// InstalledAppProfile is the signed installed app's binding, not a resident-model
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

func installedAppProfile(version string) InstalledAppProfile {
	return InstalledAppProfile{
		Version: version, Model: InstalledAppModel, Route: InstalledAppModel + "@apollo:" + version,
		Precision: InstalledAppPrecision, Endpoint: installedAppEndpoint,
		Modes:              []string{"plain", "flash", "mlx-certified"},
		CheckpointRevision: installedAppRevision,
		TokenizerSHA256:    qwenVocabularySHA256,
		TemplateSHA256:     "a4aee8afcf2e0711942cf848899be66016f8d14a889ff9ede07bca099c28f715",
	}
}

// IsInstalledAppModel accepts the short and exact model IDs and any
// model@apollo:X.Y.Z route. The route is always sent as the installed app's own.
func IsInstalledAppModel(model string) bool {
	return model == "qwen3.5-9b" || model == InstalledAppModel || installedAppRoutePattern.MatchString(model)
}

// InstalledApp reads only fixed installation locations. It accepts no profile,
// endpoint, model path or environment override. Merely finding a listener never
// qualifies an app; only the signed bundle does.
func InstalledApp(home string) (InstalledAppProfile, error) {
	for _, bundle := range []string{filepath.Join(home, "Applications", "Apollo.app"), "/Applications/Apollo.app"} {
		if _, err := os.Lstat(bundle); os.IsNotExist(err) {
			continue
		}
		// The first existing installation is authoritative. Never hide drift
		// in the per-user app by finding a stale system-wide copy instead.
		version, err := verifyInstalledApp(bundle)
		if err != nil {
			return InstalledAppProfile{}, fmt.Errorf("installed Apollo binding unavailable: %w", err)
		}
		return installedAppProfile(version), nil
	}
	return InstalledAppProfile{}, fmt.Errorf("installed Apollo app is unavailable")
}

// verifyInstalledApp checks the bundle's code signature against the fixed
// requirement, then reads its (sealed) version. Tools run by absolute path.
func verifyInstalledApp(bundle string) (string, error) {
	info, err := os.Lstat(bundle)
	if err != nil {
		return "", fmt.Errorf("read installed Apollo bundle: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("installed Apollo bundle %s is not a directory", bundle)
	}
	if out, err := exec.Command("/usr/bin/codesign", "--verify", "--strict", "--deep", "-R="+installedAppRequirement, bundle).CombinedOutput(); err != nil {
		return "", fmt.Errorf("%s is not the Developer ID-signed ai.sirsi.apollo of team 9D382WV988: %s", bundle, strings.TrimSpace(string(out)))
	}
	out, err := exec.Command("/usr/bin/plutil", "-extract", "CFBundleShortVersionString", "raw", "-o", "-", filepath.Join(bundle, "Contents", "Info.plist")).Output()
	version := strings.TrimSpace(string(out))
	if err != nil || !installedAppVersionPattern.MatchString(version) {
		return "", fmt.Errorf("installed Apollo bundle has no X.Y.Z version")
	}
	return version, nil
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
	Route          string `json:"route"`
	Model          string `json:"model"`
	Precision      string `json:"precision"`
	Mode           string `json:"mode"`
	Tier           string `json:"tier"`
	Text           string `json:"text"`
	FinishReason   string `json:"finish_reason"`
	AdmissionOwner string `json:"admission_owner"`
}

func appWireRequest(in InstalledAppRequest, route string) (installedAppWireRequest, error) {
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
	out.Model, out.Messages, out.MaxTokens = route, in.Messages, in.MaxOutputTokens
	out.Apollo.Mode = tier
	return out, nil
}

// ChatInstalledApp never retries a POST, follows a redirect, uses an HTTP proxy,
// or resolves a hostname. Context cancellation closes the request; the existing
// Apollo service owns model admission/cancellation/cleanup, not native fences.
func ChatInstalledApp(ctx context.Context, home string, in InstalledAppRequest) (InstalledAppResult, error) {
	if _, err := appWireRequest(in, ""); err != nil {
		return InstalledAppResult{}, err
	}
	profile, err := InstalledApp(home)
	if err != nil {
		return InstalledAppResult{}, err
	}
	client := installedAppHTTPClient()
	defer client.CloseIdleConnections()
	return chatInstalledApp(ctx, client, profile.Route, in)
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
func chatInstalledApp(ctx context.Context, client *http.Client, route string, in InstalledAppRequest) (InstalledAppResult, error) {
	var result InstalledAppResult
	wire, err := appWireRequest(in, route)
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
	return InstalledAppResult{Contract: "sirsi.apollo.installed-loopback.v1", Route: wire.Model, Model: body.Model, Precision: in.Precision, Mode: in.Mode, Tier: body.Apollo.Tier, Text: *c.Message.Content, FinishReason: c.Finish, AdmissionOwner: "installed Apollo service"}, nil
}
