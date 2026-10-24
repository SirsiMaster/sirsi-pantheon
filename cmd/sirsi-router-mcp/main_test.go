package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestLegacyLauncherReleasedManifest(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SIRSI_AGENT_ID", "")
	var out bytes.Buffer
	input := `{"jsonrpc":"2.0","id":1,"method":"initialize"}` + "\n" +
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}` + "\n" +
		`{"jsonrpc":"2.0","id":3,"method":"resources/list"}` + "\n"
	if err := run(context.Background(), io.NopCloser(strings.NewReader(input)), &out); err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(&out)
	got := map[string]interface{}{}
	for i := 0; i < 3; i++ {
		var response struct {
			Result map[string]interface{} `json:"result"`
			Error  interface{}            `json:"error"`
		}
		if err := decoder.Decode(&response); err != nil {
			t.Fatal(err)
		}
		if response.Error != nil {
			t.Fatal(response.Error)
		}
		for k, v := range response.Result {
			got[k] = v
		}
	}
	var extra interface{}
	if err := decoder.Decode(&extra); err != io.EOF {
		t.Fatalf("stdout not JSON-only: %v", err)
	}
	b, err := os.ReadFile("../../internal/routermcp/testdata/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var want map[string]interface{}
	if err := json.Unmarshal(b, &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("legacy launcher manifest differs: %v", got)
	}
}
