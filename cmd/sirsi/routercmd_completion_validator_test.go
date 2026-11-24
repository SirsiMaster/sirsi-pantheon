package main

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/completionproof"
)

func TestCompletionValidatorExplicitExportCommand(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "standalone.py")
	cmd := newCompletionValidatorCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"export", path})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := completionproof.ValidatorSHA256()
	if got := fmt.Sprintf("%x", sha256.Sum256(raw)); got != want || strings.TrimSpace(out.String()) != want {
		t.Fatal("command did not export/identify exact embedded bytes")
	}
	second := newCompletionValidatorCommand()
	second.SetArgs([]string{"export", path})
	if err := second.Execute(); err == nil {
		t.Fatal("command overwrote export")
	}
	if after, err := os.ReadFile(path); err != nil || !bytes.Equal(raw, after) {
		t.Fatal("existing export changed")
	}
}
