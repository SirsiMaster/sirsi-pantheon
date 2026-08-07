package runner

import (
	"encoding/json"
	"os"
	"strings"
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
			body:    `{"components":[{"name":"Actions","status":"operational"}]}`,
			want:    "",
		},
		{
			name:    "partial_outage",
			fetchOK: true,
			body:    `{"components":[{"name":"Actions","status":"partial_outage"}]}`,
			want:    "partial_outage",
		},
		{
			name:    "major_outage",
			fetchOK: true,
			body:    `{"components":[{"name":"Actions","status":"major_outage"}]}`,
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
				if !strings.Contains(got, tc.want) {
					t.Errorf("warning %q does not contain %q", got, tc.want)
				}
			}
		})
	}
}

// TestActionsOutageRealShape asserts against a captured real-API fixture so the
// component name constant stays honest. The fixture preserves the actual JSON
// field names returned by githubstatus.com — if ghActionsName is wrong, the
// parser finds nothing and warnings are silently suppressed.
func TestActionsOutageRealShape(t *testing.T) {
	// testdata/ghstatus_summary.json: captured from the live API.
	body, err := os.ReadFile("testdata/ghstatus_summary.json")
	if err != nil {
		t.Fatalf("testdata fixture missing: %v", err)
	}

	// Parse the fixture to learn the real Actions status (may be degraded during a live outage).
	var summary ghStatusSummary
	if err := json.Unmarshal(body, &summary); err != nil {
		t.Fatalf("fixture is not valid JSON: %v", err)
	}
	var fixtureActionsStatus string
	for _, c := range summary.Components {
		if c.Name == ghActionsName {
			fixtureActionsStatus = c.Status
			break
		}
	}
	if fixtureActionsStatus == "" {
		// ghActionsName did not match any component — the constant is wrong.
		t.Fatalf("component %q not found in real-shape fixture — ghActionsName constant is wrong; found: %v",
			ghActionsName, componentNames(summary))
	}

	// Inject the real-shape fixture and assert the result matches what the fixture says.
	old := ghStatusFetch
	ghStatusFetch = func(string) ([]byte, error) { return body, nil }
	defer func() { ghStatusFetch = old }()

	got := ActionsOutage()
	if fixtureActionsStatus == "operational" {
		if got != "" {
			t.Errorf("operational fixture: expected no warning, got %q", got)
		}
	} else {
		if got == "" {
			t.Errorf("fixture has Actions=%q: expected a warning, got empty — component name constant may be wrong", fixtureActionsStatus)
		}
		if !strings.Contains(got, fixtureActionsStatus) {
			t.Errorf("warning %q does not contain status %q", got, fixtureActionsStatus)
		}
	}
}

func componentNames(s ghStatusSummary) []string {
	names := make([]string, len(s.Components))
	for i, c := range s.Components {
		names[i] = c.Name
	}
	return names
}

type networkErr struct{ s string }

func (e *networkErr) Error() string { return e.s }
