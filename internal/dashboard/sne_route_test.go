package dashboard

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSNEDirectRouteLoadsSNEView(t *testing.T) {
	server := New(Config{})
	request := httptest.NewRequest(http.MethodGet, "/sne", nil)
	response := httptest.NewRecorder()
	server.handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	if body := response.Body.String(); !strings.Contains(body, "location.pathname==='/sne'?'sne':'home'") {
		t.Fatal("direct SNE route does not select the SNE view")
	}
	body := response.Body.String()
	for _, required := range []string{"license_id", "license_url", "Review terms", "verified license terms are unavailable",
		"aria-labelledby','sne-license-title", "Review model terms", "Review '+licenseID+' in a new window",
		"I reviewed and accept these terms", "install.disabled=true", "beginSNEInstall(catalogEntry)",
		"accept_license:true", "allow_research:false"} {
		if !strings.Contains(body, required) {
			t.Fatalf("SNE view omitted license disclosure contract %q", required)
		}
	}
}

func TestSNELicenseTermsRegistryFailsClosed(t *testing.T) {
	if got := sneLicenseTermsURL("gemma-terms"); got != "https://ai.google.dev/gemma/terms" {
		t.Fatalf("Gemma license URL = %q", got)
	}
	if got := sneLicenseTermsURL("unknown-license"); got != "" {
		t.Fatalf("unknown license unexpectedly resolved to %q", got)
	}
}
