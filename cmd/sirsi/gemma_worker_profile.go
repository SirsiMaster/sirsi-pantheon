package main

import (
	"fmt"
	"github.com/SirsiMaster/sirsi-pantheon/internal/provider"

	"net/http"

	"strings"
	"time"
)

// Explicit selected broker metadata, never credentials or a new model load.
type gemmaWorkerInferenceProfile = provider.LocalProfile

var gemmaWorkerInferenceProfilePath string
var gemmaWorkerSelectedProfile *gemmaWorkerInferenceProfile

func readGemmaWorkerProfile(path, profile string) (*gemmaWorkerInferenceProfile, error) {
	if path == "" {
		if profile == "m1" {
			return nil, fmt.Errorf("M1 requires an explicit installed inference profile; no M5 model fallback")
		}
		return nil, nil
	}
	p, e := provider.ReadLocalProfile(path)
	if e != nil {
		return nil, e
	}
	if profile == "m1" && p.AccessTokenFile == "" {
		return nil, fmt.Errorf("M1 selected profile requires existing private broker capability file")
	}
	return p, nil
}
func gemmaWorkerEndpoint(home string) (string, string) {
	if gemmaWorkerSelectedProfile != nil {
		return strings.TrimSuffix(strings.TrimRight(gemmaWorkerSelectedProfile.Endpoint, "/"), "/v1"), gemmaWorkerSelectedProfile.Model
	}
	return gemmaSelectedLocalEndpoint(home)
}

func gemmaWorkerPing(base string) bool {
	bearer, e := gemmaBrokerBearer(gemmaWorkerTokenFile)
	if e != nil {
		return false
	}
	req, e := http.NewRequest(http.MethodGet, base+"/v1/models", nil)
	if e != nil {
		return false
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, e := (&http.Client{Timeout: 2 * time.Second}).Do(req)
	if e != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
