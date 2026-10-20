package main_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"testing"
	"time"
)

// Exercise real CLI entry points: the former singleton let the first client
// initialize but closed the second client's transport before its handshake.
func TestMCPConcurrentClients(t *testing.T) {
	for _, pair := range []struct {
		name          string
		first, second []string
	}{
		{"root/root", []string{"mcp"}, []string{"mcp"}},
		{"root/legacy", []string{"mcp"}, []string{"seshat", "mcp"}},
		{"legacy/root", []string{"seshat", "mcp"}, []string{"mcp"}},
		{"legacy/legacy", []string{"seshat", "mcp"}, []string{"seshat", "mcp"}},
	} {
		t.Run(pair.name, func(t *testing.T) {
			start := func(args []string) (func(string, int), func()) {
				t.Helper()
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				t.Cleanup(cancel)
				cmd := exec.CommandContext(ctx, testBinary, args...)
				var stderr bytes.Buffer
				cmd.Stderr = &stderr
				stdin, err := cmd.StdinPipe()
				if err != nil {
					t.Fatal(err)
				}
				stdout, err := cmd.StdoutPipe()
				if err != nil {
					t.Fatal(err)
				}
				if err := cmd.Start(); err != nil {
					t.Fatal(err)
				}
				closed := false
				stop := func() {
					if closed {
						return
					}
					closed = true
					_ = stdin.Close()
					if err := cmd.Wait(); err != nil {
						t.Errorf("%v exit: %v; stderr: %s", args, err, &stderr)
					}
				}
				t.Cleanup(stop)
				reader := bufio.NewReader(stdout)
				request := func(method string, id int) {
					t.Helper()
					if _, err := fmt.Fprintf(stdin, "{\"jsonrpc\":\"2.0\",\"id\":%d,\"method\":%q}\n", id, method); err != nil {
						t.Fatal(err)
					}
					line, err := reader.ReadBytes('\n')
					if err != nil {
						t.Fatalf("%v %s response: %v", args, method, err)
					}
					var response struct {
						ID     int             `json:"id"`
						Result json.RawMessage `json:"result"`
						Error  json.RawMessage `json:"error"`
					}
					if err := json.Unmarshal(line, &response); err != nil {
						t.Fatalf("%v stdout is not JSON-RPC: %q: %v", args, line, err)
					}
					if response.ID != id || len(response.Result) == 0 || len(response.Error) != 0 {
						t.Fatalf("%v unexpected response: %s", args, line)
					}
				}
				return request, stop
			}
			first, stopFirst := start(pair.first)
			first("initialize", 1)
			second, _ := start(pair.second)
			second("initialize", 2)
			first("ping", 3)
			second("tools/list", 4)
			stopFirst()
			second("ping", 5) // One client closing must not end the other.
		})
	}
}
