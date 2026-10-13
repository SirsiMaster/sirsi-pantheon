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
	Endpoint        string `json:"endpoint"`
	Model           string `json:"model"`
	ContextTokens   int    `json:"context_tokens"`
	OutputTokens    int    `json:"output_tokens"`
	Streaming       *bool  `json:"streaming"`
	AccessTokenFile string `json:"access_token_file"`
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
	return &p, nil
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
