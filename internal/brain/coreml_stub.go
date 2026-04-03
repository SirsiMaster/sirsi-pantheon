//go:build !darwin || !cgo

// Package brain — coreml_stub.go provides no-op stubs when CoreML is unavailable.
// This compiles on Linux, Windows, and darwin without CGO.
package brain

import "fmt"

func coremlAvailable() bool {
	return false
}

func coremlPredict(_, _ string) (string, float64, error) {
	return "", 0, fmt.Errorf("coreml: not available on this platform")
}
