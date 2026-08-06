package logging

import (
	"bytes"
	"log"
	"log/slog"
	"strings"
	"testing"
)

// The bug this guards: slog.SetDefault routes the standard `log` package through
// this handler at LevelInfo, while Init defaults to LevelWarn. INFO < WARN, so
// every log.Printf in the repo was silently discarded in production — which is
// why the launchd wake-*.log files sat at 0 bytes for months while the loops ran.
func TestEnableDaemonLogging_MakesStdlibLogVisible(t *testing.T) {
	var buf bytes.Buffer

	// Default (non-verbose, non-quiet) — reproduces the production blackout.
	Init(false, false, false)
	SetOutput(&buf)
	log.Printf("wake-loop probe: started")
	if strings.Contains(buf.String(), "wake-loop probe") {
		t.Fatal("precondition failed: stdlib log was visible at the default level, so this test cannot detect the regression it exists to catch")
	}

	// Daemon verbs raise the floor to Info.
	buf.Reset()
	EnableDaemonLogging()
	SetOutput(&buf)
	log.Printf("wake-loop probe: started")
	if !strings.Contains(buf.String(), "wake-loop probe") {
		t.Errorf("stdlib log still discarded after EnableDaemonLogging; got %q", buf.String())
	}
}

// --quiet must still win: an operator asking for silence gets it.
func TestEnableDaemonLogging_DoesNotOverrideQuiet(t *testing.T) {
	var buf bytes.Buffer
	Init(false, true /* quiet */, false)
	EnableDaemonLogging()
	SetOutput(&buf)

	log.Printf("wake-loop probe: should stay silent")
	if strings.Contains(buf.String(), "should stay silent") {
		t.Error("EnableDaemonLogging overrode --quiet")
	}
	if got := level.Level(); got != slog.LevelError {
		t.Errorf("quiet level changed: want %v got %v", slog.LevelError, got)
	}
}

// -v already exceeds Info and must not be lowered.
func TestEnableDaemonLogging_DoesNotLowerVerbose(t *testing.T) {
	Init(true /* verbose */, false, false)
	EnableDaemonLogging()
	if got := level.Level(); got != slog.LevelDebug {
		t.Errorf("verbose level was lowered: want %v got %v", slog.LevelDebug, got)
	}
}
