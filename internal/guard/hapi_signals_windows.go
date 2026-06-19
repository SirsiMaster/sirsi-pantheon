//go:build windows

// Package guard — hapi_signals_windows.go
//
// Windows stubs for the Hapi memory-governor's POSIX-signal primitives. Windows
// has no kill(2)/SIGSTOP/SIGCONT/SIGTERM, so the governor's "teeth" (suspend a
// runaway, kill at emergency) cannot operate the same way — these report
// unsupported, and the governor's warn/surface path still works. This exists so
// the cross-platform release build (goreleaser darwin/linux/windows) compiles;
// the primary surface is macOS (ADR-032). Unix uses hapi_signals_unix.go.
package guard

import "errors"

// errHapiSignalsUnsupported is returned by the intervention primitives on Windows.
var errHapiSignalsUnsupported = errors.New("hapi process intervention (suspend/resume/kill) is not supported on Windows: no POSIX signals")

// hapiProcessAlive cannot use a signal-0 probe on Windows; treat any positive PID
// as possibly-alive (the governor's teeth no-op here anyway, so the governed
// registry simply isn't pruned by liveness on Windows).
func hapiProcessAlive(pid int) bool { return pid > 0 }

func hapiSuspend(int, string) error { return errHapiSignalsUnsupported }
func hapiResume(int) error          { return errHapiSignalsUnsupported }
func hapiKill(int, string) error    { return errHapiSignalsUnsupported }
