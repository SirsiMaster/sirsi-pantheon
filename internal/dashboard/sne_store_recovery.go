package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type sneStoreRecoveryFunc func(context.Context, string, string) error

func runSNEStoreRecovery(ctx context.Context, binary, storeRoot string) error {
	if strings.TrimSpace(binary) == "" || strings.TrimSpace(storeRoot) == "" {
		return fmt.Errorf("SNE model-store recovery helper and store are required")
	}
	if info, err := os.Stat(binary); err != nil || !info.Mode().IsRegular() || info.Mode()&0o111 == 0 {
		return fmt.Errorf("SNE model-store recovery helper is unavailable")
	}
	output, err := exec.CommandContext(ctx, binary, "--store", storeRoot).CombinedOutput()
	if err != nil {
		return fmt.Errorf("SNE model-store recovery failed: %s", strings.TrimSpace(string(output)))
	}
	var envelope struct {
		Type   string `json:"type"`
		Result struct {
			RemovalsRecovered int `json:"removals_recovered"`
			ObjectsRemoved    int `json:"objects_removed"`
			ObjectsRetained   int `json:"objects_retained"`
		} `json:"result"`
	}
	if err := json.Unmarshal(output, &envelope); err != nil || envelope.Type != "result" {
		return fmt.Errorf("SNE model-store recovery returned an invalid result")
	}
	return nil
}
