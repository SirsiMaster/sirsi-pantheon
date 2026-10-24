package routermcp

import (
	"context"
	"io"
	"testing"
	"time"
)

func TestServerCancellationAfterMalformedInputJoins(t *testing.T) {
	s, _, closed, _ := inertServer(t, "owned-malformed")
	reader, writer := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	exited := make(chan struct{})
	var runErr error
	go func() { defer close(exited); runErr = s.Run(ctx, reader, io.Discard) }()
	t.Cleanup(func() {
		cancel()
		_ = reader.Close()
		_ = writer.Close()
		select {
		case <-exited:
		case <-time.After(2 * time.Second):
			t.Error("malformed-input server did not join")
		}
	})
	if _, err := writer.Write([]byte("!\n")); err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case <-exited:
		if runErr != nil {
			t.Fatal(runErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("sticky decoder error prevented cancellation")
	}
	if closed.Load() != 1 {
		t.Fatal("registered malformed connection not closed")
	}
}
