// Package notify provides macOS toast notifications and persistent notification
// history for the Sirsi Pantheon menubar app. Closes the feedback loop: every
// action now produces a visible result (toast) and a queryable record (SQLite).
package notify

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Severity levels for notifications.
const (
	SeverityInfo    = "info"
	SeveritySuccess = "success"
	SeverityWarning = "warning"
	SeverityError   = "error"
)

// SeverityIcon returns a display icon for a severity level.
func SeverityIcon(severity string) string {
	switch severity {
	case SeveritySuccess:
		return "✅"
	case SeverityWarning:
		return "⚠️"
	case SeverityError:
		return "❌"
	default:
		return "ℹ️"
	}
}

// Rate limiting: minimum gap between toasts to prevent flood.
var (
	lastToastMu sync.Mutex
	lastToast   time.Time
	minToastGap = 5 * time.Second
)

// toastExecFn is injectable for testing (Rule A16/A21).
var (
	toastMu     sync.RWMutex
	toastExecFn = defaultToastExec
)

func getToastExecFn() func(string) error {
	toastMu.RLock()
	defer toastMu.RUnlock()
	return toastExecFn
}

func setToastExecFn(fn func(string) error) {
	toastMu.Lock()
	defer toastMu.Unlock()
	toastExecFn = fn
}

func defaultToastExec(script string) error {
	// A test binary must NEVER post a real user notification: every `go test`
	// of a package that reaches Toast (directly or through the dashboard/menubar
	// paths) was banner-spamming the owner "Sirsi — anubis" on each worker
	// verify cycle (2026-07-02). Injected fakes (setToastExecFn) are unaffected —
	// this guards only the real osascript sink. SIRSI_NO_TOAST=1 is the same
	// mute for headless automation that legitimately runs the real binary.
	if strings.HasSuffix(os.Args[0], ".test") || os.Getenv("SIRSI_NO_TOAST") == "1" {
		return nil
	}
	return exec.Command("osascript", "-e", script).Run()
}

// Toast fires a macOS banner notification. Non-blocking — spawns osascript
// in a goroutine and returns immediately. Rate-limited to prevent floods.
func Toast(title, body string) {
	lastToastMu.Lock()
	if time.Since(lastToast) < minToastGap {
		lastToastMu.Unlock()
		return
	}
	lastToast = time.Now()
	lastToastMu.Unlock()

	go func() {
		safeTitle := escapeAppleScript(title)
		safeBody := escapeAppleScript(body)
		script := fmt.Sprintf(
			`display notification "%s" with title "%s"`,
			safeBody, safeTitle,
		)
		_ = getToastExecFn()(script)
	}()
}

// escapeAppleScript escapes special characters for AppleScript strings.
func escapeAppleScript(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	// Truncate long strings for notification display.
	if len(s) > 200 {
		s = s[:197] + "..."
	}
	return s
}
