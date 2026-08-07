package main

import (
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
)

func TestThreadGlyph(t *testing.T) {
	tests := []struct {
		status router.ThreadStatus
		stale  bool
		want   string
	}{
		// Active thread, fresh heartbeat → green.
		{router.ThreadStatusActive, false, "🟢"},
		// Active thread, stale heartbeat → warning (the stale-but-alive signal).
		// This was the bug: before the fix, glyph logic set ⚠️ first and then
		// idle/other statuses could override it; the ordering is now status-first.
		{router.ThreadStatusActive, true, "⚠️"},
		// Idle thread, fresh → sleep glyph.
		{router.ThreadStatusIdle, false, "💤"},
		// Idle thread, stale → warning (not 💤; the heartbeat has gone quiet).
		// This was the other ordering bug: ⚠️ set before idle, then 💤 overrode it.
		{router.ThreadStatusIdle, true, "⚠️"},
		// Explicitly stale-heartbeat status → warning.
		{router.ThreadStatusStale, false, "⚠️"},
		{router.ThreadStatusStale, true, "⚠️"},
		// Blocked: stale does NOT override — blocked is an intentional state.
		{router.ThreadStatusBlocked, false, "⛔"},
		{router.ThreadStatusBlocked, true, "⛔"},
		// Terminal: stale never applies (IsTerminal guards it).
		{router.ThreadStatusClosed, false, "⚫"},
		{router.ThreadStatusClosed, true, "⚫"},
		{router.ThreadStatusReaped, false, "💀"},
		{router.ThreadStatusReaped, true, "💀"},
	}
	for _, tc := range tests {
		got := threadGlyph(tc.status, tc.stale)
		if got != tc.want {
			t.Errorf("threadGlyph(%q, stale=%v) = %q, want %q", tc.status, tc.stale, got, tc.want)
		}
	}
}
