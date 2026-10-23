package stacklab

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type lifecyclePublication struct {
	Status    string `json:"status"`
	Canonical struct {
		Path, Commit, SHA256, URL string
	} `json:"canonical"`
	Desktop struct {
		Path, SHA256 string
	} `json:"desktop"`
	Workspace struct {
		DocumentID string `json:"document_id"`
		URL        string `json:"url"`
		RevisionID string `json:"revision_id"`
	} `json:"workspace"`
}

// This validates source-to-mirror binding, not remote document availability or
// installed service behavior. Live publication readback is a separate receipt.
func validateLifecyclePublication(p lifecyclePublication, source []byte) error {
	digest := sha256.Sum256(source)
	if p.Canonical.Path != "docs/runbooks/SCOPED-SERVICE-LIFECYCLE.md" || p.Canonical.SHA256 != hex.EncodeToString(digest[:]) {
		return fmt.Errorf("publication no longer binds the current runbook bytes")
	}
	commit, err := hex.DecodeString(p.Canonical.Commit)
	if err != nil || len(commit) != 20 {
		return fmt.Errorf("publication requires an exact source commit")
	}
	wantURL := "https://github.com/SirsiMaster/sirsi-pantheon/blob/" + p.Canonical.Commit + "/" + p.Canonical.Path
	if p.Canonical.URL != wantURL || p.Status != "SOURCE_CANDIDATE_MIRRORED_NOT_RELEASED_OR_INSTALLED" {
		return fmt.Errorf("publication must preserve exact source and candidate scope")
	}
	if !strings.Contains(p.Desktop.Path, "/Desktop/Sirsi/00 Owner Reading Room/Pantheon/") || len(p.Desktop.SHA256) != 64 {
		return fmt.Errorf("publication omits the canonical owner mirror identity")
	}
	if p.Workspace.DocumentID == "" || p.Workspace.RevisionID == "" || p.Workspace.URL != "https://docs.google.com/document/d/"+p.Workspace.DocumentID+"/edit?usp=drivesdk" {
		return fmt.Errorf("publication omits native Workspace document readback identity")
	}
	return nil
}

func TestScopedLifecyclePublicationBindsCurrentSource(t *testing.T) {
	root := filepath.Join("..", "..")
	raw, err := os.ReadFile(filepath.Join(root, "docs/runbooks/SCOPED-SERVICE-LIFECYCLE.publication.json"))
	if err != nil {
		t.Fatal(err)
	}
	var p lifecyclePublication
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatal(err)
	}
	// Never turn editable publication metadata into an arbitrary file read.
	source, err := os.ReadFile(filepath.Join(root, "docs/runbooks/SCOPED-SERVICE-LIFECYCLE.md"))
	if err != nil {
		t.Fatal(err)
	}
	if err := validateLifecyclePublication(p, source); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*lifecyclePublication){
		"wrong hash":        func(v *lifecyclePublication) { v.Canonical.SHA256 = strings.Repeat("0", 64) },
		"floating source":   func(v *lifecyclePublication) { v.Canonical.Commit = "main" },
		"unrelated source":  func(v *lifecyclePublication) { v.Canonical.URL = "https://example.com/source" },
		"release overclaim": func(v *lifecyclePublication) { v.Status = "RELEASED" },
		"missing readback":  func(v *lifecyclePublication) { v.Workspace.RevisionID = "" },
	} {
		t.Run(name, func(t *testing.T) {
			bad := p
			change(&bad)
			if err := validateLifecyclePublication(bad, source); err == nil {
				t.Fatal("invalid publication accepted")
			}
		})
	}
	if err := validateLifecyclePublication(p, append(append([]byte(nil), source...), '\n')); err == nil {
		t.Fatal("changed source accepted without a new mirror binding")
	}
}
