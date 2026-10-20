package provider

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// One selected profile is shared by all local inference consumers. Credentials
// stay in the existing private file, never in profile JSON or diagnostic output.
type LocalProfile struct {
	// ID names an installed, reviewed profile. It is optional for ordinary
	// text-only local inference; the structured Apollo worker requires the
	// exact gemma-m5-apollo21 identity instead of guessing from a port.
	ID                    string `json:"id,omitempty"`
	Endpoint              string `json:"endpoint"`
	Model                 string `json:"model"`
	ContextTokens         int    `json:"context_tokens"`
	OutputTokens          int    `json:"output_tokens"`
	Streaming             *bool  `json:"streaming"`
	AccessTokenFile       string `json:"access_token_file"`
	RequestTimeoutSeconds *int   `json:"request_timeout_seconds,omitempty"`
	// ToolCalls is an explicit capability declaration. A compatible HTTP shape
	// alone never grants a model authority to request local tools.
	ToolCalls *bool `json:"tool_calls,omitempty"`
}

func ProfileSupportsTools(p *LocalProfile) bool {
	return p != nil && p.ToolCalls != nil && *p.ToolCalls
}

func ReadLocalProfile(path string) (*LocalProfile, error) {
	b, e := os.ReadFile(path)
	if e != nil {
		return nil, fmt.Errorf("selected local profile unavailable")
	}
	if len(b) > 16384 {
		return nil, fmt.Errorf("selected local profile oversized")
	}
	var p LocalProfile
	if e = json.Unmarshal(b, &p); e != nil {
		return nil, fmt.Errorf("invalid selected local profile")
	}
	if !isLoopbackEndpoint(p.Endpoint) || strings.Contains(p.Endpoint, "@") || strings.ContainsAny(p.Endpoint, "?#") {
		return nil, fmt.Errorf("selected profile requires credential-free loopback endpoint")
	}
	if p.Model == "" {
		return nil, fmt.Errorf("selected profile requires exact full model identity")
	}
	if p.ContextTokens <= 0 || p.OutputTokens <= 0 || p.OutputTokens >= p.ContextTokens || p.Streaming == nil {
		return nil, fmt.Errorf("selected profile capabilities unknown: explicit context/output/streaming required")
	}
	if p.RequestTimeoutSeconds != nil && (*p.RequestTimeoutSeconds < 1 || *p.RequestTimeoutSeconds > 600) {
		return nil, fmt.Errorf("selected profile request_timeout_seconds must be 1..600")
	}
	return &p, nil
}

// Zero is an internal absent-field marker, not an accepted profile value.
func ProfileRequestTimeoutSeconds(p *LocalProfile) int {
	if p == nil || p.RequestTimeoutSeconds == nil {
		return 0
	}
	return *p.RequestTimeoutSeconds
}
func selectedProfilePath(home string, conf Conf) string {
	p := strings.TrimSpace(os.Getenv("SIRSI_LOCAL_PROFILE"))
	if p == "" {
		p = conf.LocalProfile
	}
	if strings.HasPrefix(p, "~/") {
		p = filepath.Join(home, p[2:])
	}
	return p
}

func SelectedLocalProfilePath(home string, conf Conf) string { return selectedProfilePath(home, conf) }
