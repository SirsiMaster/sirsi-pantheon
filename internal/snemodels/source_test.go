package snemodels

import (
	"strings"
	"testing"
)

func TestSourceEntryRejectsRevisionPathInjection(t *testing.T) {
	entry := sourceFixture()
	for _, revision := range []string{"../main", "refs/heads/main", "%2e%2e", "main?download=1"} {
		entry.Revision = revision
		if err := entry.validate(); err == nil || !strings.Contains(err.Error(), "revision") {
			t.Fatalf("revision %q error = %v", revision, err)
		}
	}
}

func TestSourceEntryResolvesFirstPartyDerivative(t *testing.T) {
	entry := sourceFixture()
	entry.Provider = "sirsi"
	entry.BaseURL = "https://models.sirsi.ai/v1"
	entry.Repository = "gemma4/derived/26b-a4b-nvfp4"
	if err := entry.validate(); err != nil {
		t.Fatal(err)
	}
	got, err := entry.ResolveURL(entry.BaseURL, "weights/model.safetensors")
	if err != nil {
		t.Fatal(err)
	}
	want := "https://models.sirsi.ai/v1/gemma4/derived/26b-a4b-nvfp4/revision/weights/model.safetensors"
	if got != want {
		t.Fatalf("url = %q, want %q", got, want)
	}
}

func sourceFixture() SourceEntry {
	return SourceEntry{CatalogEntry: "test", Provider: "huggingface", Repository: "owner/repo", Revision: "revision", LicenseID: "terms", Files: []SourceFile{{Path: "model.bin", SHA256: strings.Repeat("a", 64), SizeBytes: 1}}}
}
