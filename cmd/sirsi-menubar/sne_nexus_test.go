package main

import (
	"net/url"
	"testing"
)

func TestMenubarNexusLaunchUsesGovernedLocalAIFragment(t *testing.T) {
	launch, err := buildMenubarNexusURL("private-capability")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(launch)
	if err != nil {
		t.Fatal(err)
	}
	fragment, err := url.ParseQuery(parsed.Fragment)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Scheme != "https" || parsed.Host != "sirsi.ai" || parsed.Path != "/local-ai" || parsed.RawQuery != "" || fragment.Get("sne_capability") != "private-capability" {
		t.Fatalf("ungoverned Nexus launch URL: %s", launch)
	}
	if _, err := buildMenubarNexusURL(""); err == nil {
		t.Fatal("missing local capability was accepted")
	}
}
