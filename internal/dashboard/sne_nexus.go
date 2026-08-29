package dashboard

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const NexusLocalAIURL = "https://sirsi.ai/local-ai"

// BuildNexusCapabilityURL transfers the local SNE capability in a URL fragment.
// Fragments are consumed by Nexus in the browser and are not sent to sirsi.ai.
func BuildNexusCapabilityURL(base, token string) (string, error) {
	parsed, err := url.Parse(base)
	if err != nil || parsed.Scheme != "https" || !strings.EqualFold(parsed.Host, "sirsi.ai") || parsed.User != nil || strings.TrimSpace(token) == "" {
		return "", fmt.Errorf("invalid Nexus capability launch contract")
	}
	fragment := url.Values{}
	fragment.Set("sne_capability", token)
	parsed.Fragment = fragment.Encode()
	return parsed.String(), nil
}

func openNexusForModel(model SNEReadModel, token string, opener func(string) error) error {
	if _, admitted := activeReleaseSupportedSNETuple(model); !admitted {
		return fmt.Errorf("Nexus requires an exact ready release-supported SNE tuple")
	}
	launchURL, err := BuildNexusCapabilityURL(NexusLocalAIURL, token)
	if err != nil {
		return err
	}
	if opener == nil {
		return fmt.Errorf("Nexus browser opener is unavailable")
	}
	return opener(launchURL)
}

func (s *Server) apiSNENexusOpen(w http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writeSNEOpenAIError(w, http.StatusMethodNotAllowed, "method_not_allowed", "POST required", nil)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 5*time.Second)
	defer cancel()
	model, err := s.sneReadModel(ctx)
	if err != nil {
		writeSNEOpenAIError(w, http.StatusServiceUnavailable, "sne_status_unavailable", "Pantheon could not verify the local SNE status", nil)
		return
	}
	token := ""
	if s.sneAccess != nil {
		token = s.sneAccess.snapshot()
	}
	if err := openNexusForModel(model, token, getOpenBrowserFn()); err != nil {
		writeSNEOpenAIError(w, http.StatusServiceUnavailable, "nexus_open_unavailable", "Nexus could not be opened from the verified local session", &model.Lifecycle)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, map[string]any{"opened": true, "model": model.ActiveModel, "no_fallback": true})
}
