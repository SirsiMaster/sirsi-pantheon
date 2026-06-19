package main

import (
	"strings"
	"testing"
)

// TestHapiDaemonPlist locks the LaunchAgent contract: it must run `hapi watch`,
// add --govern only when governing, carry the interval, restart on crash
// (KeepAlive), and start at login (RunAtLoad). A regression here ships a daemon
// that silently doesn't protect.
func TestHapiDaemonPlist(t *testing.T) {
	govern := hapiDaemonPlist("/usr/local/bin/sirsi", true, 5)
	for _, want := range []string{
		"<string>ai.sirsi.hapi</string>",
		"hapi watch --interval 5",
		"--govern",
		"<key>RunAtLoad</key>",
		"<key>KeepAlive</key>",
		"/usr/local/bin/sirsi",
	} {
		if !strings.Contains(govern, want) {
			t.Errorf("govern plist missing %q", want)
		}
	}

	warn := hapiDaemonPlist("/usr/local/bin/sirsi", false, 3)
	if strings.Contains(warn, "--govern") {
		t.Error("warn-only plist must NOT contain --govern")
	}
	if !strings.Contains(warn, "hapi watch --interval 3") {
		t.Error("warn-only plist must still run the watcher at the given interval")
	}
}
