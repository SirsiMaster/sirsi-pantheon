package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/stacklab"
)

func TestFindStacklabCatalogRootUsesNearestSelectedCheckout(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "contracts", "stacklab"), 0o755); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "nested", "review")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	oldGetwd := stacklabCatalogGetwd
	t.Cleanup(func() { stacklabCatalogGetwd = oldGetwd })
	stacklabCatalogGetwd = func() (string, error) { return nested, nil }
	got, err := findStacklabCatalogRoot()
	if err != nil {
		t.Fatal(err)
	}
	if same, err := sameDirPaths(got, root); err != nil || !same {
		t.Fatalf("catalog root = %q, want %q (same=%v, err=%v)", got, root, same, err)
	}
}

func TestStacklabDoctorDiagnosticCodeIsClosedAndWrapped(t *testing.T) {
	err := fmt.Errorf("read failed: %w", stacklab.MissingGitHubCLIError{})
	if got := stacklabDoctorDiagnosticCode(err); got != "missing_gh" {
		t.Fatalf("diagnostic code = %q, want missing_gh", got)
	}
	if got := stacklabDoctorDiagnosticCode(fmt.Errorf("private service detail")); got != "" {
		t.Fatalf("unrecognized diagnostic code = %q, want empty", got)
	}
}

func TestFindStacklabCatalogRootRejectsMissingContracts(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	oldGetwd := stacklabCatalogGetwd
	t.Cleanup(func() { stacklabCatalogGetwd = oldGetwd })
	stacklabCatalogGetwd = func() (string, error) { return root, nil }
	if _, err := findStacklabCatalogRoot(); err == nil {
		t.Fatal("catalog root accepted a directory without Stack Lab contracts")
	}
}

func sameDirPaths(a, b string) (bool, error) {
	ai, err := os.Stat(a)
	if err != nil {
		return false, err
	}
	bi, err := os.Stat(b)
	if err != nil {
		return false, err
	}
	return os.SameFile(ai, bi), nil
}
