package fabric

// probe.go — the Notebook 4 Phase 0 link probe: two hosts, one link, observed
// numbers. `Serve` on one Mac, `Probe` from the other over the Thunderbolt
// Bridge address (or any address — the receipt records which).
//
// It measures exactly what §10.6 says matters first: round-trip time at the
// 14 KB pipeline-boundary payload, and bulk goodput. It measures over TCP.
// It has NO RDMA path, and says so in the receipt rather than leaving the
// field blank — an RDMA number will come from a separate tool that can be
// named in `transport`.

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sort"
	"time"
)

const (
	opPing = 'p'
	opBulk = 'b'
	// PayloadPipeline is the §10.6 planning payload: hidden 7168 × bf16.
	PayloadPipeline = 7168 * 2
)

// ProbeOptions bounds a run. Zero values take the defaults below.
type ProbeOptions struct {
	Pings     int           // round trips per payload size (default 200)
	BulkBytes int64         // bytes sent for goodput (default 256 MiB)
	Timeout   time.Duration // per-connection deadline (default 60s)
}

func (o ProbeOptions) withDefaults() ProbeOptions {
	if o.Pings <= 0 {
		o.Pings = 200
	}
	if o.BulkBytes <= 0 {
		o.BulkBytes = 256 << 20
	}
	if o.Timeout <= 0 {
		o.Timeout = 60 * time.Second
	}
	return o
}

// RTT is one payload size's round-trip distribution, in microseconds.
type RTT struct {
	PayloadBytes int     `json:"payload_bytes"`
	Samples      int     `json:"samples"`
	P50us        float64 `json:"p50_us"`
	P95us        float64 `json:"p95_us"`
	P99us        float64 `json:"p99_us"`
	MinUs        float64 `json:"min_us"`
}

// ProbeReport is the receipt. Every number is observed on this run; nothing
// is copied from a spec sheet.
type ProbeReport struct {
	CollectedAt time.Time `json:"collected_at"`
	Transport   string    `json:"transport"` // "tcp"
	RDMA        string    `json:"rdma"`      // "not measured: this tool has no RDMA path"
	LocalAddr   string    `json:"local_addr"`
	RemoteAddr  string    `json:"remote_addr"`
	RTT         []RTT     `json:"rtt"`
	BulkBytes   int64     `json:"bulk_bytes"`
	BulkSeconds float64   `json:"bulk_seconds"`
	GoodputMBps float64   `json:"goodput_mb_per_s"` // decimal MB, one direction, application bytes
	GoodputGbps float64   `json:"goodput_gb_per_s"`
	Caveats     []string  `json:"caveats"`
}

// Serve answers probes until ctx is done. It returns the bound address so a
// caller (or test) that passed ":0" learns the port.
func Serve(ctx context.Context, listen string) (net.Addr, error) {
	ln, err := net.Listen("tcp", listen)
	if err != nil {
		return nil, err
	}
	go func() {
		<-ctx.Done()
		ln.Close()
	}()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go serveConn(c)
		}
	}()
	return ln.Addr(), nil
}

func serveConn(c net.Conn) {
	defer c.Close()
	var hdr [5]byte
	if _, err := io.ReadFull(c, hdr[:]); err != nil {
		return
	}
	size := int(binary.BigEndian.Uint32(hdr[1:]))
	switch hdr[0] {
	case opPing:
		buf := make([]byte, size)
		for {
			if _, err := io.ReadFull(c, buf); err != nil {
				return
			}
			if _, err := c.Write(buf); err != nil {
				return
			}
		}
	case opBulk:
		n, _ := io.Copy(io.Discard, c)
		var ack [8]byte
		binary.BigEndian.PutUint64(ack[:], uint64(n))
		_, _ = c.Write(ack[:]) // best effort: the client's timeout is the failure path
	}
}

// Probe runs the ping and bulk phases against addr and returns the receipt.
// Any failure returns an error and NO receipt: a partial receipt would be a
// number without its conditions.
func Probe(addr string, opts ProbeOptions) (*ProbeReport, error) {
	o := opts.withDefaults()
	r := &ProbeReport{
		CollectedAt: time.Now().UTC(),
		Transport:   "tcp",
		RDMA:        "not measured: this tool has no RDMA path",
		Caveats: []string{
			"TCP over whatever interface routes to remote_addr; the interface is not identified by this tool — pair with `sirsi hardware links`",
			"goodput is application bytes one direction; line rate is not inferable from it",
			"RTT includes both hosts' kernel and scheduler time; halve for a one-way estimate only with that caveat stated",
		},
	}
	for _, size := range []int{64, PayloadPipeline} {
		rtt, local, remote, err := pingPhase(addr, size, o)
		if err != nil {
			return nil, fmt.Errorf("ping %dB: %w", size, err)
		}
		r.RTT = append(r.RTT, rtt)
		r.LocalAddr, r.RemoteAddr = local, remote
	}
	secs, err := bulkPhase(addr, o)
	if err != nil {
		return nil, fmt.Errorf("bulk: %w", err)
	}
	r.BulkBytes = o.BulkBytes
	r.BulkSeconds = secs
	r.GoodputMBps = float64(o.BulkBytes) / secs / 1e6
	r.GoodputGbps = r.GoodputMBps * 8 / 1000
	return r, nil
}

func dial(addr string, op byte, size int, o ProbeOptions) (net.Conn, error) {
	c, err := net.DialTimeout("tcp", addr, o.Timeout)
	if err != nil {
		return nil, err
	}
	if err := c.SetDeadline(time.Now().Add(o.Timeout)); err != nil {
		c.Close()
		return nil, err
	}
	if tc, ok := c.(*net.TCPConn); ok {
		_ = tc.SetNoDelay(true) // advisory; RTT samples are what they are either way
	}
	var hdr [5]byte
	hdr[0] = op
	binary.BigEndian.PutUint32(hdr[1:], uint32(size))
	if _, err := c.Write(hdr[:]); err != nil {
		c.Close()
		return nil, err
	}
	return c, nil
}

func pingPhase(addr string, size int, o ProbeOptions) (RTT, string, string, error) {
	c, err := dial(addr, opPing, size, o)
	if err != nil {
		return RTT{}, "", "", err
	}
	defer c.Close()
	buf := make([]byte, size)
	for i := range buf {
		buf[i] = byte(i)
	}
	echo := make([]byte, size)
	samples := make([]float64, 0, o.Pings)
	// Two warm-up round trips are excluded: connection setup is not link latency.
	for i := 0; i < o.Pings+2; i++ {
		t0 := time.Now()
		if _, err := c.Write(buf); err != nil {
			return RTT{}, "", "", err
		}
		if _, err := io.ReadFull(c, echo); err != nil {
			return RTT{}, "", "", err
		}
		if i >= 2 {
			samples = append(samples, float64(time.Since(t0).Microseconds()))
		}
	}
	sort.Float64s(samples)
	pct := func(p float64) float64 { return samples[int(p*float64(len(samples)-1))] }
	return RTT{PayloadBytes: size, Samples: len(samples), P50us: pct(0.50), P95us: pct(0.95), P99us: pct(0.99), MinUs: samples[0]},
		c.LocalAddr().String(), c.RemoteAddr().String(), nil
}

func bulkPhase(addr string, o ProbeOptions) (float64, error) {
	c, err := dial(addr, opBulk, 0, o)
	if err != nil {
		return 0, err
	}
	defer c.Close()
	chunk := make([]byte, 1<<20)
	t0 := time.Now()
	var sent int64
	for sent < o.BulkBytes {
		n := int64(len(chunk))
		if o.BulkBytes-sent < n {
			n = o.BulkBytes - sent
		}
		w, err := c.Write(chunk[:n])
		if err != nil {
			return 0, err
		}
		sent += int64(w)
	}
	if tc, ok := c.(*net.TCPConn); ok {
		_ = tc.CloseWrite() // the ack read below is the real failure path
	}
	var ack [8]byte
	if _, err := io.ReadFull(c, ack[:]); err != nil {
		return 0, fmt.Errorf("no byte-count ack from server: %w", err)
	}
	secs := time.Since(t0).Seconds()
	if got := int64(binary.BigEndian.Uint64(ack[:])); got != sent {
		return 0, fmt.Errorf("server counted %d bytes, client sent %d", got, sent)
	}
	return secs, nil
}
