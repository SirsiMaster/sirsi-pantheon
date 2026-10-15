//go:build !windows && !darwin

package router

// Other Unix platforms retain their existing ps start-signature probe.
func nativePIDStart(int) string { return "" }
