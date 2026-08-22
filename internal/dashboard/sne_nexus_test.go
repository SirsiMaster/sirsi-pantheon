package dashboard

import (
	"net/url"
	"strings"
	"testing"
	"time"
)

func releaseSupportedNexusModel() SNEReadModel {
	return SNEReadModel{
		Ready:          true,
		ActiveModel:    "gemma-4-12b",
		GeneratedAt:    time.Unix(1, 0),
		Lifecycle:      SNELifecycleState{State: "ready", ModelID: "gemma-4-12b", RuntimeID: "sne-v2", RuntimeSHA256: strings.Repeat("a", 64), ModelManifestSHA256: strings.Repeat("b", 64), Profile: "interactive"},
		RuntimeCatalog: SNERuntimeCatalogStatus{State: "verified", SignedRequired: true},
		Catalog:        []SNECatalogItem{{ModelID: "gemma-4-12b", RuntimeID: "sne-v2", Active: true, SupportStatus: "release-supported"}},
	}
}

func TestOpenNexusForModelUsesFragmentOnly(t *testing.T) {
	var opened string
	err := openNexusForModel(releaseSupportedNexusModel(), "private-capability", func(value string) error { opened = value; return nil })
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(opened)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Scheme != "https" || parsed.Host != "sirsi.ai" || parsed.Path != "/local-ai" || parsed.Query().Get("sne_capability") != "" {
		t.Fatalf("unsafe Nexus launch URL: %s", opened)
	}
	fragment, err := url.ParseQuery(parsed.Fragment)
	if err != nil || fragment.Get("sne_capability") != "private-capability" {
		t.Fatalf("missing fragment capability: %s", opened)
	}
}

func TestOpenNexusForModelFailsClosed(t *testing.T) {
	model := releaseSupportedNexusModel()
	model.Catalog[0].SupportStatus = "pilot-candidate"
	if err := openNexusForModel(model, "private-capability", func(string) error { t.Fatal("opened unqualified Nexus"); return nil }); err == nil {
		t.Fatal("unqualified tuple was admitted")
	}
	if err := openNexusForModel(releaseSupportedNexusModel(), "", func(string) error { return nil }); err == nil {
		t.Fatal("missing capability was admitted")
	}
}
