package platform

import "runtime"

// current holds the active platform instance.
var current Platform

func init() {
	current = detect()
}

// Current returns the Platform for the running OS.
func Current() Platform {
	return current
}

// Set overrides the current platform (for testing).
func Set(p Platform) {
	current = p
}

// Reset restores the auto-detected platform.
func Reset() {
	current = detect()
}

func detect() Platform {
	return detectFor(runtime.GOOS)
}

func detectFor(goos string) Platform {
	switch goos {
	case "darwin":
		return &Darwin{}
	case "linux":
		return &Linux{}
	default:
		// Fallback to a minimal implementation for unsupported platforms.
		// Uses Linux as base since it's the most portable.
		return &Linux{}
	}
}
