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

	"github.com/SirsiMaster/sirsi-pantheon/internal/routermcp"
)

func TestRouterMCPCommandReleasedManifest(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SIRSI_AGENT_ID", "")
	cmd := newRouterMCPCmd(routermcp.Run)
	cmd.SetArgs([]string{})
	cmd.SetIn(io.NopCloser(strings.NewReader(
		`{"jsonrpc":"2.0","id":1,"method":"initialize"}` + "\n" +
			`{"jsonrpc":"2.0","id":2,"method":"tools/list"}` + "\n" +
			`{"jsonrpc":"2.0","id":3,"method":"resources/list"}` + "\n")))
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)
	if err := cmd.Execute(); err != nil {
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
		t.Fatalf("canonical CLI manifest differs from released bare server: %v", got)
	}
}

func TestRouterMCPCommandRefusesPositionalsBeforeRun(t *testing.T) {
	calls := 0
	cmd := newRouterMCPCmd(func(context.Context, io.ReadCloser, io.Writer) error { calls++; return nil })
	cmd.SetArgs([]string{"ignored"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	if err := cmd.Execute(); err == nil || calls != 0 {
		t.Fatalf("args accepted/run called: %v %d", err, calls)
	}
}
