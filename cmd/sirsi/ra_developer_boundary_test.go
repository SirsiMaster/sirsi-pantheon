package main

import "testing"

func TestLegacyRaTerminalEnabledRequiresExactOptIn(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  bool
	}{
		{"", false},
		{"true", false},
		{"0", false},
		{"1", true},
	} {
		t.Run(tc.value, func(t *testing.T) {
			t.Setenv("SIRSI_RA_ENABLE_LEGACY_TERMINAL", tc.value)
			if got := legacyRaTerminalEnabled(); got != tc.want {
				t.Fatalf("legacyRaTerminalEnabled() = %v, want %v", got, tc.want)
			}
		})
	}
}
