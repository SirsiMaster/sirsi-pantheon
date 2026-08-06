package runner

import (
	"testing"
)

func TestActionsOutage(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		fetchOK bool
		want    string // "" means no outage warning expected
	}{
		{
			name:    "operational",
			fetchOK: true,
			body:    `{"components":[{"name":"GitHub Actions","status":"operational"}]}`,
			want:    "",
		},
		{
			name:    "partial_outage",
			fetchOK: true,
			body:    `{"components":[{"name":"GitHub Actions","status":"partial_outage"}]}`,
			want:    "partial_outage",
		},
		{
			name:    "major_outage",
			fetchOK: true,
			body:    `{"components":[{"name":"GitHub Actions","status":"major_outage"}]}`,
			want:    "major_outage",
		},
		{
			name:    "network_error_is_silent",
			fetchOK: false,
			want:    "",
		},
		{
			name:    "actions_component_absent",
			fetchOK: true,
			body:    `{"components":[{"name":"Git Operations","status":"partial_outage"}]}`,
			want:    "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			old := ghStatusFetch
			if tc.fetchOK {
				ghStatusFetch = func(string) ([]byte, error) { return []byte(tc.body), nil }
			} else {
				ghStatusFetch = func(string) ([]byte, error) {
					return nil, &networkErr{"simulated"}
				}
			}
			defer func() { ghStatusFetch = old }()

			got := ActionsOutage()
			if tc.want == "" && got != "" {
				t.Errorf("expected no outage warning, got %q", got)
			}
			if tc.want != "" && got == "" {
				t.Errorf("expected outage warning containing %q, got empty", tc.want)
			}
			if tc.want != "" && got != "" {
				// just check it contains the status word
				if !contains(got, tc.want) {
					t.Errorf("warning %q does not contain %q", got, tc.want)
				}
			}
		})
	}
}

type networkErr struct{ s string }

func (e *networkErr) Error() string { return e.s }

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}
