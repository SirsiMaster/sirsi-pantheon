package liveness

import (
	"os"
	"path/filepath"
	"testing"
)

func TestApolloProvidesGemmaMarkerKinds(t *testing.T) {
	// "absent" = no Apollo.app at all (the only not-provided, no-error case).
	// "app-only" and "missing" = app present but incomplete: fail closed.
	for _, kind := range []string{"absent", "app-only", "missing", "executable", "non-executable", "directory", "symlink", "dangling-symlink"} {
		t.Run(kind, func(t *testing.T) {
			home := t.TempDir()
			path := ApolloRouterPath(home)
			var err error
			switch kind {
			case "absent":
			case "app-only":
				err = os.MkdirAll(ApolloAppPath(home), 0o755)
			default:
				err = os.MkdirAll(filepath.Dir(path), 0o755)
			}
			if err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "executable":
				err = os.WriteFile(path, []byte("fixture"), 0o755)
			case "non-executable":
				err = os.WriteFile(path, []byte("fixture"), 0o644)
			case "directory":
				err = os.Mkdir(path, 0o755)
			case "symlink":
				target := filepath.Join(home, "other-executable")
				if err = os.WriteFile(target, []byte("fixture"), 0o755); err == nil {
					err = os.Symlink(target, path)
				}
			case "dangling-symlink":
				err = os.Symlink(filepath.Join(home, "absent"), path)
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := ApolloProvidesGemma(home); got != (kind == "executable") {
				t.Fatalf("kind=%s provided=%v", kind, got)
			}
			provided, checkErr := CheckApolloGemmaProvider(home)
			if provided != (kind == "executable") || (checkErr != nil) != (kind != "absent" && kind != "executable") {
				t.Fatalf("kind=%s provided=%v error=%v", kind, provided, checkErr)
			}
		})
	}
}

func TestApolloProviderEmptyHomeFailsClosed(t *testing.T) {
	if provided, err := CheckApolloGemmaProvider(""); provided || err == nil {
		t.Fatalf("provided=%v err=%v", provided, err)
	}
}
