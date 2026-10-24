package routermcp

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/mcp"
)

type responseCounter struct{ lines int }

func (w *responseCounter) Write(p []byte) (int, error) {
	w.lines += bytes.Count(p, []byte("\n"))
	return len(p), nil
}

func TestServerFiniteMalformedInputClosesWithoutCancel(t *testing.T) {
	for _, input := range []string{"!", `{"jsonrpc":`} {
		t.Run(input, func(t *testing.T) {
			s, _, closed, _ := inertServer(t, "owned-finite-malformed")
			var out responseCounter
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if err := s.Run(ctx, io.NopCloser(strings.NewReader(input)), &out); err == nil {
				t.Fatal("malformed input succeeded")
			}
			if ctx.Err() != nil {
				t.Fatal("malformed input needed cancellation to unwind")
			}
			if closed.Load() != 1 || out.lines != 1 {
				t.Fatalf("cleanup/response count: %d %d", closed.Load(), out.lines)
			}
		})
	}
}

type failingReader struct{ err error }

func (r failingReader) Read([]byte) (int, error) { return 0, r.err }

type failingWriter struct {
	err   error
	short bool
}

func (w failingWriter) Write(p []byte) (int, error) {
	if w.short {
		return len(p) - 1, nil
	}
	return 0, w.err
}

func TestServerTerminalIOUnwindsRegistration(t *testing.T) {
	failure := errors.New("owned I/O failure")
	requests := `{"jsonrpc":"2.0","id":1,"method":"ping"}` + "\n" + `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"owned-after-failure","arguments":{}}}`
	for _, tc := range []struct {
		name string
		in   io.Reader
		out  io.Writer
		want error
	}{
		{"read", failingReader{failure}, io.Discard, failure},
		{"write", strings.NewReader(requests), failingWriter{err: failure}, failure},
		{"short-write", strings.NewReader(requests), failingWriter{short: true}, io.ErrShortWrite},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, _, closed, _ := inertServer(t, "owned-io")
			calls := 0
			s.protocol.RegisterTool(mcp.Tool{Name: "owned-after-failure"}, func(map[string]interface{}) (*mcp.ToolResult, error) { calls++; return &mcp.ToolResult{}, nil })
			in := &countedInput{ReadCloser: io.NopCloser(tc.in)}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if err := s.Run(ctx, in, tc.out); !errors.Is(err, tc.want) {
				t.Fatalf("terminal I/O error = %v, want %v", err, tc.want)
			}
			if ctx.Err() != nil {
				t.Fatal("I/O failure needed cancellation to unwind")
			}
			if closed.Load() != 1 || in.closes.Load() != 1 {
				t.Fatal("terminal I/O did not close exactly once")
			}
			if calls != 0 {
				t.Fatal("handler ran after terminal response failure")
			}
			if _, err := s.reg.gate(); err == nil {
				t.Fatal("failed connection retained authority")
			}
		})
	}
}
