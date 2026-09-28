//go:build windows

package apollo

import (
	"fmt"
	"os"
)

// Windows has no equivalent implementation in this macOS product slice.
// Refuse the local evidence input rather than silently widening the no-follow
// contract with an ordinary pathname open.
func openTelemetryFile(string) (*os.File, error) {
	return nil, fmt.Errorf("Apollo telemetry requires a no-follow file open on this platform")
}
