package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/stacklab"
)

func TestStackLabLocalActualCommandsNoCheckoutAndOffDefault(t *testing.T) {
	home := t.TempDir()
	recipes := t.TempDir()
	calls := 0
	invoke := func(args ...string) (string, error) {
		cmd := newStacklabLocalCmd(func() (string, error) { return home, nil }, func() (string, error) { calls++; return recipes, nil })
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(args)
		err := cmd.Execute()
		return out.String(), err
	}
	out, err := invoke("inspect", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var state stacklab.LocalState
	if err = json.Unmarshal([]byte(out), &state); err != nil || state.Enabled {
		t.Fatal(out, err)
	}
	if _, err = invoke("catalog", "--json"); err == nil || calls != 0 {
		t.Fatal("disabled catalog read")
	}
	if _, err = invoke("enable", "--provider", "frontier", "--json"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"maat-system-one-recipe-v1.json", "maat-wing-v1.json"} {
		data, err := os.ReadFile(filepath.Join("..", "..", "contracts", "stacklab", name))
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(recipes, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	out, err = invoke("catalog", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var complete stacklab.Catalog
	if err = json.Unmarshal([]byte(out), &complete); err != nil || !complete.Complete() || len(complete.Entries) != 2 || len(complete.Entries[0].Components) == 0 || complete.Entries[0].SourcePath != "StackLab/maat-system-one-recipe-v1.json" {
		t.Fatal(out, err)
	}
	if err = os.WriteFile(filepath.Join(recipes, "unreadable-recipe-v1.json"), []byte(`{`), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err = invoke("catalog", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var catalog stacklab.Catalog
	if err = json.Unmarshal([]byte(out), &catalog); err != nil || len(catalog.Unknown) != 1 || len(catalog.Entries) != 2 || calls != 2 {
		t.Fatal(out, err)
	}
	if _, err = invoke("disable", "--json"); err != nil {
		t.Fatal(err)
	}
	if _, err = invoke("enable", "--provider", "invalid"); err == nil {
		t.Fatal("invalid provider accepted")
	}
}
