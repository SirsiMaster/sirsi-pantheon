//go:build unix

package stacklab

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLocalStackLabOffDefaultEnableDisableAndProviderRoundtrip(t *testing.T) {
	home := t.TempDir()
	s, err := InspectLocal(home)
	if err != nil || s.Enabled || s.Provider != "local" {
		t.Fatalf("default %+v %v", s, err)
	}
	if _, err = os.Stat(filepath.Join(home, ".sirsi")); !os.IsNotExist(err) {
		t.Fatal("read-only default created state")
	}
	for _, provider := range []string{"local", "frontier", "self-hosted"} {
		want, err := SetLocal(home, true, provider)
		if err != nil {
			t.Fatal(err)
		}
		got, err := InspectLocal(home)
		if err != nil || got != want {
			t.Fatalf("readback %+v %v", got, err)
		}
		got, err = SetLocal(home, false, provider)
		if err != nil || got.Enabled || got.Provider != provider {
			t.Fatal(got, err)
		}
	}
}

func TestLocalStackLabInvalidProviderAndCorruptOrLinkedStateRefused(t *testing.T) {
	home := t.TempDir()
	if _, err := SetLocal(home, true, "unknown"); err == nil {
		t.Fatal("invalid provider")
	}
	if _, err := os.Stat(filepath.Join(home, ".sirsi")); !os.IsNotExist(err) {
		t.Fatal("invalid request wrote state")
	}
	if _, err := SetLocal(home, true, "local"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, ".sirsi", "stacklab", "state.json")
	if err := os.WriteFile(path, []byte("{corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := InspectLocal(home); err == nil {
		t.Fatal("corrupt state read as disabled")
	}
	if _, err := SetLocal(home, false, "local"); err == nil {
		t.Fatal("corrupt state overwritten")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(t.TempDir(), "foreign")
	if err := os.WriteFile(foreign, []byte("private sentinel"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(foreign, path); err != nil {
		t.Fatal(err)
	}
	if _, err := SetLocal(home, true, "local"); err == nil {
		t.Fatal("linked state followed")
	}
	data, _ := os.ReadFile(foreign)
	if string(data) != "private sentinel" {
		t.Fatal("other root changed")
	}
}

func TestLocalStackLabMissingAliasNullAndDuplicateStateRemainUnknown(t *testing.T) {
	for _, data := range []string{`{"schema":"sirsi.stacklab.local.v1","provider":"local"}`, `{"schema":"sirsi.stacklab.local.v1","enabled":null,"provider":"local"}`, `{"Schema":"sirsi.stacklab.local.v1","enabled":true,"provider":"local"}`, `{"schema":"sirsi.stacklab.local.v1","enabled":false,"enabled":true,"provider":"local"}`} {
		home := t.TempDir()
		if _, err := SetLocal(home, true, "local"); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(home, ".sirsi", "stacklab", "state.json")
		if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := InspectLocal(home); err == nil {
			t.Fatalf("ambiguous state accepted: %s", data)
		}
	}
}
