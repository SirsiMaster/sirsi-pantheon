package updater

import (
	"errors"
	"testing"
)

func TestValidateUpdateVersion(t *testing.T) {
	for _, tc := range []struct {
		name, current, candidate string
		allowed                  bool
	}{
		{"repaired dev suffix must not reinstall broken114", "0.24.113-maat.7d37fc8d", "v0.24.114", false},
		{"first paired release", "0.24.113-maat.7d37fc8d", "v0.24.115", true},
		{"future update", "0.24.115", "v0.24.116", true},
		{"downgrade despite compatible protocol", "0.24.116", "v0.24.115", false},
		{"same version", "0.24.115", "v0.24.115", false},
		{"pre-floor prerelease", "0.24.114", "v0.24.115-beta", false},
		{"unstamped current", "dev", "v0.24.115", false},
		{"empty candidate", "0.24.115", "", false},
		{"malformed candidate", "0.24.115", "v999.0.0 garbage", false},
		{"extra numeric component", "0.24.115", "v999.0.0.1", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateUpdateVersion(tc.current, tc.candidate)
			if tc.allowed && err != nil {
				t.Fatal(err)
			}
			if !tc.allowed && !errors.Is(err, ErrIncompatibleUpdate) {
				t.Fatalf("expected incompatible-update refusal, got %v", err)
			}
		})
	}
}
