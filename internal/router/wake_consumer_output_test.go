package router

import (
	"strings"
	"testing"
	"time"
)

// A consumer that fails must say WHY. Before this, dispatchConsumer left
// cmd.Stdout/cmd.Stderr nil, exec.Cmd wired both to /dev/null, and 94% of
// fleet-wide dispatches (3843/4082) exited 1 carrying no cause at all.
func TestDispatchConsumerCapturesOutputTail(t *testing.T) {
	run, err := dispatchConsumer(&ResolvedConsumer{
		// The lingering grandchild is the point: it inherits the output fd and
		// outlives the consumer. With an io.Writer sink, exec's copier goroutine
		// would hold cmd.Wait open until that grandchild died — running() would
		// stay true and re-dispatch would stop fleet-wide. An *os.File sink is
		// passed straight through and started no copier, so Wait is unaffected.
		Argv: []string{"/bin/sh", "-c", "sleep 30 & echo to-stdout; echo to-stderr >&2; exit 3"},
		Cwd:  t.TempDir(),
	})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	select {
	case <-run.done:
	case <-time.After(10 * time.Second):
		t.Fatal("consumer never completed — Wait is blocked on an output pipe it must not own")
	}
	if run.err == nil {
		t.Fatal("want a non-nil exit error for exit 3")
	}
	// Both streams: the fatal line lands on either one depending on the consumer.
	// Poll — the copier is a separate goroutine from Wait, by design.
	var got string
	for i := 0; i < 100; i++ {
		got = run.tail.String()
		if strings.Contains(got, "to-stdout") && strings.Contains(got, "to-stderr") {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("captured tail missing output: %q", got)
}

// The ring is what keeps a 1.4 MB transcript out of the log while keeping the
// end of it, which is where the fatal line lives.
func TestRingTailKeepsLastBytesAndNamesSilence(t *testing.T) {
	tail := &ringTail{max: 8}
	if got := tail.String(); got != "(no output)" {
		t.Fatalf("empty tail = %q, want the silence marker — silence is itself a finding", got)
	}
	if got := (*ringTail)(nil).String(); got != "(not captured)" {
		t.Fatalf("nil tail = %q", got)
	}
	for _, chunk := range []string{"aaaa", "bbbb", "cccc"} {
		if n, err := tail.Write([]byte(chunk)); n != len(chunk) || err != nil {
			t.Fatalf("Write(%q) = %d, %v", chunk, n, err)
		}
	}
	if got := tail.String(); got != "bbbbcccc" {
		t.Fatalf("tail = %q, want the LAST 8 bytes %q", got, "bbbbcccc")
	}
}
