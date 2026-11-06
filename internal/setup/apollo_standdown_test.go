package setup

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/liveness"
)

func TestApolloStandDownPreservesSelectedRoute(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("darwin only")
	}
	for _, port := range []string{"8520\n", "8521\n", "custom-host-route\n", ""} {
		t.Run(port, func(t *testing.T) {
			home := apolloHome(t)
			dir := filepath.Join(home, ".sirsi")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "gemma-server.port")
			if port != "" {
				if err := os.WriteFile(path, []byte(port), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			profile := filepath.Join(dir, "selected-profile.json")
			const body = `{"endpoint":"http://127.0.0.1:8521/v1","model":"unchanged"}`
			if err := os.WriteFile(profile, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			if res := InstallGemmaBroker(); res.Status != StatusSkipped {
				t.Fatalf("result=%+v", res)
			}
			if fileExists(GemmaBrokerPlistPath()) {
				t.Fatal("legacy broker plist created")
			}
			got, err := os.ReadFile(path)
			if port == "" {
				if !os.IsNotExist(err) {
					t.Fatalf("absent route was created: %q, %v", got, err)
				}
			} else if err != nil || string(got) != port {
				t.Fatalf("selected route changed: %q, %v", got, err)
			}
			if got, err := os.ReadFile(profile); err != nil || string(got) != body {
				t.Fatalf("profile changed: %q, %v", got, err)
			}
		})
	}
}

func TestApolloStandDownInvalidMarkerFailsClosed(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("darwin only")
	}
	for _, kind := range []string{"incomplete-app", "missing-helper", "directory", "non-executable", "symlink", "dangling-symlink"} {
		t.Run(kind, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			fakeSNERuntime(t, home)
			path := liveness.ApolloRouterPath(home)
			mk := filepath.Dir(path)
			if kind == "incomplete-app" {
				mk = liveness.ApolloAppPath(home)
			}
			if err := os.MkdirAll(mk, 0o755); err != nil {
				t.Fatal(err)
			}
			var err error
			switch kind {
			case "directory":
				err = os.Mkdir(path, 0o755)
			case "non-executable":
				err = os.WriteFile(path, []byte("fixture"), 0o644)
			case "symlink":
				target := filepath.Join(home, "external-router")
				if err = os.WriteFile(target, []byte("fixture"), 0o755); err == nil {
					err = os.Symlink(target, path)
				}
			case "dangling-symlink":
				err = os.Symlink(filepath.Join(home, "absent"), path)
			}
			if err != nil {
				t.Fatal(err)
			}
			dir := filepath.Join(home, ".sirsi")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			port := filepath.Join(dir, "gemma-server.port")
			if err := os.WriteFile(port, []byte("8521\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if res := InstallGemmaBroker(); res.Status != StatusFailed {
				t.Fatalf("result=%+v", res)
			}
			if fileExists(GemmaBrokerPlistPath()) {
				t.Fatal("invalid marker caused legacy broker installation")
			}
			if got, err := os.ReadFile(port); err != nil || string(got) != "8521\n" {
				t.Fatalf("route changed: %q %v", got, err)
			}
		})
	}
}
