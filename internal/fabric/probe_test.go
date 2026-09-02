package fabric

import (
	"context"
	"testing"
	"time"
)

func TestProbe_Loopback(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	addr, err := Serve(ctx, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	r, err := Probe(addr.String(), ProbeOptions{Pings: 20, BulkBytes: 4 << 20, Timeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if r.Transport != "tcp" || r.RDMA == "" {
		t.Errorf("transport=%q rdma=%q — both must be stated", r.Transport, r.RDMA)
	}
	if len(r.RTT) != 2 || r.RTT[1].PayloadBytes != PayloadPipeline || r.RTT[1].Samples != 20 {
		t.Fatalf("rtt = %+v", r.RTT)
	}
	for _, x := range r.RTT {
		if !(x.MinUs <= x.P50us && x.P50us <= x.P95us && x.P95us <= x.P99us) || x.MinUs <= 0 {
			t.Errorf("percentiles disordered or zero: %+v", x)
		}
	}
	if r.BulkBytes != 4<<20 || r.BulkSeconds <= 0 || r.GoodputMBps <= 0 {
		t.Errorf("bulk = %d B in %v s → %v MB/s", r.BulkBytes, r.BulkSeconds, r.GoodputMBps)
	}
	if len(r.Caveats) == 0 || r.CollectedAt.IsZero() {
		t.Error("receipt must carry caveats and its own clock")
	}
}

// Negative control: no server ⇒ error and NO receipt. A partial receipt
// would be a number without its conditions.
func TestProbe_NoServer_NoReceipt(t *testing.T) {
	r, err := Probe("127.0.0.1:1", ProbeOptions{Pings: 1, BulkBytes: 1, Timeout: 500 * time.Millisecond})
	if err == nil || r != nil {
		t.Fatalf("want error and nil receipt, got err=%v r=%+v", err, r)
	}
}
