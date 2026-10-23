//go:build darwin || linux

package schedule

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestPodNativeMutualTLSEndpointOwnLifecycle(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	pub, key, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}}
	der, e := x509.CreateCertificate(rand.Reader, template, template, pub, key)
	if e != nil {
		t.Fatal(e)
	}
	cert, e := x509.ParseCertificate(der)
	if e != nil {
		t.Fatal(e)
	}
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	pair := tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
	digest := sha256.Sum256(der)
	var handler http.Handler
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { handler.ServeHTTP(w, r) }))
	d := mkDelegation("studio-m5", "native-studio", now.Add(-time.Minute).Format(time.RFC3339), now.Add(time.Hour).Format(time.RFC3339), "studio")
	d.Epoch = 1
	d.AuthorityAddrs = []string{"https://" + srv.Listener.Addr().String()}
	j, e := OpenPodJournal(filepath.Join(t.TempDir(), "native-events"), "native-studio", d)
	if e != nil {
		t.Fatal(e)
	}
	defer j.Close()
	handler, e = PodHTTPHandler(j, map[string]string{hex.EncodeToString(digest[:]): "ra"})
	if e != nil {
		t.Fatal(e)
	}
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{pair}, ClientCAs: pool, ClientAuth: tls.RequireAndVerifyClientCert, MinVersion: tls.VersionTLS13}
	srv.StartTLS()
	defer srv.Close()
	client, e := NewHTTPPodTransport(srv.URL, &tls.Config{RootCAs: pool, Certificates: []tls.Certificate{pair}})
	if e != nil {
		t.Fatal(e)
	}
	req := mkReq("studio", "ra", now.Format(time.RFC3339), now.Add(20*time.Minute).Format(time.RFC3339), RegimeQuiet)
	req.ID = "native-progress"
	cache, e := FetchFiniteGrantCache(&podClientFixture{grant: &d}, d.Pod)
	if e != nil {
		t.Fatal(e)
	}
	routed := PodClient{Pod: d.Pod, Canonical: &podClientFixture{grant: &d}, Transport: client, Cache: cache}
	consumer := NewLedger(&PodRoutingStore{Client: routed})
	out, e := consumer.Reserve(req, false)
	if e != nil || !out.Granted {
		t.Fatalf("authenticated grant %+v %v", out, e)
	}
	var clock atomic.Int64
	clock.Store(now.Unix())
	j.now = func() time.Time { return time.Unix(clock.Load(), 0) }
	for i := 0; i < 4; i++ {
		clock.Add(90)
		if _, e = consumer.Heartbeat(out.Reservation.ID); e != nil {
			t.Fatal(e)
		}
	}
	if _, e = consumer.Release(out.Reservation.ID); e != nil {
		t.Fatal(e)
	}
	if len(j.Replay(0)) != 6 {
		t.Fatal("TLS mutations lost sequence events")
	}
	req.ID = "foreign"
	req.Holder = "foreign"
	if _, e = client.ReservePod(context.Background(), d, req, false); e == nil {
		t.Fatal("certificate principal overridden by payload")
	}
	if _, e = NewHTTPPodTransport(srv.URL, &tls.Config{RootCAs: pool}); e == nil {
		t.Fatal("anonymous client accepted")
	}
}
