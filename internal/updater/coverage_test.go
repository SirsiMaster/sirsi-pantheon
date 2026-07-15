package updater

import (
	"testing"
)

func TestNewClient(t *testing.T) {
	client := NewClient()
	if client == nil {
		t.Fatal("NewClient returned nil")
	}
	if client.ReleasesURL != GitHubReleasesAPI {
		t.Errorf("ReleasesURL = %q, want %q", client.ReleasesURL, GitHubReleasesAPI)
	}
	if client.AdvisoryURL != AdvisoryURL {
		t.Errorf("AdvisoryURL = %q, want %q", client.AdvisoryURL, AdvisoryURL)
	}
	if client.HTTPClient == nil {
		t.Error("HTTPClient should not be nil")
	}
}

// TestCheck_DevVersion (placebo — asserted nothing) was replaced by
// TestCheck_DevVersionNeverUpdates in updater_failure_test.go, which actually
// exercises the dev short-circuit against an httptest server.
