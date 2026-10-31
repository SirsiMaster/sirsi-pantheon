//go:build !darwin

package lifecycleevidence

import (
	"strings"
	"testing"
)

func TestInstalledLifecycleRouterRefusesUnsupportedPlatform(t *testing.T) {
	if _, err := NewInstalledRouter(nil); err == nil || !strings.Contains(err.Error(), "only from the verified canonical macOS package") {
		t.Fatalf("unsupported platform received installed lifecycle router: %v", err)
	}
}
