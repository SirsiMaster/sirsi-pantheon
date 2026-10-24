package routermcp

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

// The golden is the released 0fd8463 standalone contract, not a manifest built
// from the new registrations. Exercise actual protocol framing and every schema.
func TestReleasedBareManifestParity(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	s := newServer("", t.TempDir()) // no registration or store access
	var out bytes.Buffer
	input := `{"jsonrpc":"2.0","id":1,"method":"initialize"}` + "\n" +
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}` + "\n" +
		`{"jsonrpc":"2.0","id":3,"method":"resources/list"}` + "\n"
	if err := s.Run(context.Background(), io.NopCloser(strings.NewReader(input)), &out); err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(&out)
	var got map[string]interface{}
	for i := 0; i < 3; i++ {
		var response struct {
			Result map[string]interface{} `json:"result"`
			Error  interface{}            `json:"error"`
		}
		if err := decoder.Decode(&response); err != nil {
			t.Fatal(err)
		}
		if response.Error != nil {
			t.Fatalf("protocol error: %+v", response.Error)
		}
		if i == 0 {
			got = response.Result
		} else {
			for k, v := range response.Result {
				got[k] = v
			}
		}
	}
	var extra interface{}
	if err := decoder.Decode(&extra); err != io.EOF {
		t.Fatalf("non-JSON protocol output/trailing response: %v", err)
	}
	b, err := os.ReadFile("testdata/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var want map[string]interface{}
	if err := json.Unmarshal(b, &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("released manifest changed:\ngot=%v\nwant=%v", got, want)
	}
}
