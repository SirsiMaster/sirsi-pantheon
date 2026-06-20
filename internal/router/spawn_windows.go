//go:build windows

package router

import "syscall"

// detachedSysProcAttr is a no-op on Windows (no Setsid); the wake spawn path is
// macOS-first (ADR-032). Present so internal/router cross-compiles.
func detachedSysProcAttr() *syscall.SysProcAttr { return nil }
