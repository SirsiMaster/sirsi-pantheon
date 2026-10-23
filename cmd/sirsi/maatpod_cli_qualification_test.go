//go:build darwin || linux

package main_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/schedule"
	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

// This opt-in test executes an explicitly supplied, already-built CLI. It does
// not build a payload or use the installed owner's router, identities or keys.
// For this exact test alone, MAAT_SUBMIT_HELPER=1 selects the existing TestMain
// m.Run fast path, avoiding its otherwise redundant integration CLI build.
func TestNativeCLIPodAuthorityQualification(t *testing.T) {
	cli, optedIn := os.LookupEnv("PANTHEON_NATIVE_CLI")
	if !optedIn {
		t.Skip("set PANTHEON_NATIVE_CLI to an absolute existing built CLI path")
	}
	if !filepath.IsAbs(cli) {
		t.Fatal("PANTHEON_NATIVE_CLI must be an absolute built CLI path")
	}
	cli, err := filepath.EvalSymlinks(cli)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(cli)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
		t.Fatal("PANTHEON_NATIVE_CLI must select an existing executable regular file")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	home, repo, scratch := filepath.Join(root, "home"), filepath.Join(root, "repo"), filepath.Join(root, "tmp")
	registry := filepath.Join(repo, ".agents", "idea-router")
	for _, dir := range []string{home, registry, scratch} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	// The fixture is deliberately nongit: FindRepoRoot must resolve this registry
	// by its cwd walk, rather than a real checkout's Git common directory.
	t.Setenv("HOME", home)
	if err := router.SaveRegistry(registry, &router.Registry{Agents: map[string]router.AgentConfig{
		"ra": {ID: "ra", Cwd: repo, Repo: repo},
	}}); err != nil {
		t.Fatal(err)
	}
	machine := router.MachineID()
	if machine == "" {
		t.Fatal("actual native machine identity is unavailable")
	}
	now := time.Now().UTC().Truncate(time.Second)
	suffix := nativeCLIQualificationNonce(t)[:16]
	pod, resource, thread := "cli-proof-"+suffix, "native-cli-fixture-"+suffix, "native-cli-thread-"+suffix
	backend, err := routerstore.OpenPath(filepath.Join(root, "canonical.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := backend.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := backend.UpsertThreads([]routerstore.ThreadRecord{{ThreadID: thread, Agent: "ra", Host: machine,
		Status: "active", LastSeenAt: now.Format(time.RFC3339Nano), Payload: []byte(`{}`)}}); err != nil {
		t.Fatal(err)
	}
	token := nativeCLIQualificationNonce(t)
	handler, err := routerstore.Handler(backend, routerstore.ServerOptions{
		Token: token, RuleOfRa: "enforce", RecipientRegistry: filepath.Join(registry, "agents.json"), CallTimeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	var partition atomic.Bool
	var blocked, reconciles atomic.Int64
	canonical := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if partition.Load() {
			blocked.Add(1)
			http.Error(w, "fixture canonical partition", http.StatusServiceUnavailable)
			return
		}
		handler.ServeHTTP(w, r)
		if r.URL.Path == "/v1/call/MaatReconcile" {
			reconciles.Add(1)
		}
	}))
	t.Cleanup(canonical.Close)

	cert, key, ca, clientCert, clientKey, fingerprint, tlsConfig := nativeCLIQualificationTLS(t, root, now)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	address := listener.Addr().String()
	delegation, err := backend.MaatDelegation("delegate", schedule.Delegation{Pod: pod, AuthorityMachine: machine,
		AuthorityAddrs: []string{"https://" + address}, Resources: []string{resource},
		IssuedAt: now.Add(-time.Minute).Format(time.RFC3339), NotAfter: now.Add(15 * time.Minute).Format(time.RFC3339)})
	if err != nil {
		t.Fatal(err)
	}
	journal := filepath.Join(root, "native.journal")
	authorityCache, clientCache := filepath.Join(root, "authority.grant"), filepath.Join(root, "client.grant")
	// Build the child environment from an empty allowlist. In particular, no
	// inherited SIRSI_*, MAAT_*, GIT_*, proxies, HOME or credential selectors survive.
	env := []string{"PATH=/usr/bin:/bin:/usr/sbin:/sbin", "HOME=" + home, "TMPDIR=" + scratch, "GIT_CEILING_DIRECTORIES=" + root,
		"PWD=" + repo, "XDG_CONFIG_HOME=" + filepath.Join(home, ".config"), "XDG_CACHE_HOME=" + filepath.Join(home, ".cache"),
		"XDG_DATA_HOME=" + filepath.Join(home, ".local", "share"), "NO_COLOR=1", "SIRSI_NO_TUI=1",
		"SIRSI_ROUTER_URL=" + canonical.URL, "SIRSI_ROUTER_TOKEN=" + token,
		"SIRSI_AGENT_ID=ra", "SIRSI_THREAD_ID=" + thread, "SIRSI_ROUTER_USE_MACHINE_ID=1",
		"SIRSI_MAAT_POD=" + pod, "SIRSI_MAAT_CLIENT_CERT=" + clientCert, "SIRSI_MAAT_CLIENT_KEY=" + clientKey,
		"SIRSI_MAAT_CLIENT_CA=" + ca, "SIRSI_MAAT_GRANT_CACHE=" + clientCache}
	authorityArgs := []string{"maat", "authority-serve", "--pod", pod, "--journal", journal, "--listen", address,
		"--cert", cert, "--key", key, "--client-ca", ca, "--client", fingerprint + "=ra", "--grant-cache", authorityCache}
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	authority := nativeCLIQualificationStart(t, ctx, cli, repo, env, authorityArgs...)
	nativeCLIQualificationReady(t, authority, address, tlsConfig)
	reserve := func(nonce, work string) []string {
		return []string{"maat", "reserve", resource, "--holder", "ra", "--work", work, "--regime", "quiet",
			"--start", now.Format(time.RFC3339), "--est-end", now.Add(5 * time.Minute).Format(time.RFC3339),
			"--lease-ttl", "120", "--operation-nonce", nonce, "--json"}
	}
	var first, retry, second schedule.ReserveResult
	firstNonce, secondNonce := nativeCLIQualificationNonce(t), nativeCLIQualificationNonce(t)
	nativeCLIQualificationRun(t, ctx, cli, repo, env, &first, reserve(firstNonce, "online-fixture")...)
	if !first.Granted || first.Reservation == nil || first.Reservation.ID != "pod:"+pod+":"+strconv.Itoa(delegation.Epoch)+":"+firstNonce || first.Reservation.Seq != 1 {
		t.Fatalf("online reserve did not return the exact initial pod grant: %+v", first)
	}
	nativeCLIQualificationRun(t, ctx, cli, repo, env, &retry, reserve(firstNonce, "online-fixture")...)
	if !reflect.DeepEqual(first, retry) {
		t.Fatal("separate-process reserve retry changed the original receipt")
	}
	mutate := func(action, id string, out *schedule.Reservation) {
		nativeCLIQualificationRun(t, ctx, cli, repo, env, out, "maat", action, id,
			"--operation-nonce", nativeCLIQualificationNonce(t), "--json")
	}
	var onlineHeartbeat, onlineReleased schedule.Reservation
	mutate("heartbeat", first.Reservation.ID, &onlineHeartbeat)
	mutate("release", first.Reservation.ID, &onlineReleased)
	if onlineHeartbeat.ID != first.Reservation.ID || onlineHeartbeat.Seq != 2 || onlineHeartbeat.Status != schedule.StatusActive ||
		onlineReleased.ID != first.Reservation.ID || onlineReleased.Seq != 3 || onlineReleased.Status != schedule.StatusReleased {
		t.Fatal("online lifecycle changed identity, status or event sequence")
	}
	nativeCLIQualificationRun(t, ctx, cli, repo, env, &second, reserve(secondNonce, "partition-fixture")...)
	if !second.Granted || second.Reservation == nil || second.Reservation.Seq != 4 ||
		second.Reservation.ID != "pod:"+pod+":"+strconv.Itoa(delegation.Epoch)+":"+secondNonce {
		t.Fatalf("second exact pod grant absent: %+v", second)
	}
	for _, path := range []string{authorityCache, clientCache} {
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() || info.Size() == 0 || info.Mode().Perm() != 0600 {
			t.Fatal("online CLI did not persist its distinct private signed grant caches")
		}
	}
	authority.stop(t)
	partition.Store(true)
	authority = nativeCLIQualificationStart(t, ctx, cli, repo, env, authorityArgs...)
	nativeCLIQualificationReady(t, authority, address, tlsConfig)
	var own []schedule.Reservation
	nativeCLIQualificationRun(t, ctx, cli, repo, env, &own, "maat", "status", resource, "--json")
	if len(own) != 2 {
		t.Fatalf("offline own status returned %d rows, want exactly two", len(own))
	}
	seen := map[string]schedule.Status{}
	for _, row := range own {
		if row.Holder != "ra" || row.Resource != resource {
			t.Fatal("offline own status escaped the authenticated fixture holder/resource")
		}
		seen[row.ID] = row.Status
	}
	if seen[first.Reservation.ID] != schedule.StatusReleased || seen[second.Reservation.ID] != schedule.StatusActive {
		t.Fatal("offline restart lost original lifecycle identities/status")
	}
	nativeCLIQualificationRun(t, ctx, cli, repo, env, &retry, reserve(secondNonce, "partition-fixture")...)
	if !reflect.DeepEqual(second, retry) {
		t.Fatal("offline native/client restart lost the durable reserve receipt")
	}
	var offlineHeartbeat, offlineReleased schedule.Reservation
	mutate("heartbeat", second.Reservation.ID, &offlineHeartbeat)
	mutate("release", second.Reservation.ID, &offlineReleased)
	if offlineHeartbeat.ID != second.Reservation.ID || offlineHeartbeat.Seq != 5 || offlineHeartbeat.Status != schedule.StatusActive ||
		offlineReleased.ID != second.Reservation.ID || offlineReleased.Seq != 6 || offlineReleased.Status != schedule.StatusReleased {
		t.Fatal("partitioned lifecycle changed identity, status or sequence")
	}
	if blocked.Load() == 0 {
		t.Fatal("partition gate did not observe any actual canonical request")
	}
	reconcilesBeforeRestore := reconciles.Load()
	partition.Store(false)
	// The production authority owns its existing five-second relay. Observe its
	// acknowledgement within the overall 45-second budget, including the sleep
	// plus the bounded network attempt; never replay the journal from the test.
	for {
		ack, err := backend.MaatGetDelegation(pod)
		if err != nil {
			t.Fatal(err)
		}
		if ack != nil && ack.AckedSeq == 6 && reconciles.Load() > reconcilesBeforeRestore {
			break
		}
		select {
		case <-ctx.Done():
			authority.logFailureStderr(t)
			t.Fatal("native relay did not reconcile and acknowledge exactly six events within the overall fixture deadline")
		case <-authority.done:
			authority.logFailureStderr(t)
			t.Fatal("native authority exited before canonical catch-up")
		case <-time.After(25 * time.Millisecond):
		}
	}
	rows, err := backend.MaatRead("status", resource)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("canonical status has %d rows, want exactly two without duplicates", len(rows))
	}
	want := map[string]schedule.Reservation{onlineReleased.ID: onlineReleased, offlineReleased.ID: offlineReleased}
	for _, row := range rows {
		expected, ok := want[row.ID]
		if !ok || !reflect.DeepEqual(row, expected) {
			t.Fatalf("canonical row changed native receipt/fence: id=%s seq=%d status=%s", row.ID, row.Seq, row.Status)
		}
		delete(want, row.ID)
	}
	if len(want) != 0 {
		t.Fatal("canonical readback omitted an exact native reservation ID")
	}
	authority.stop(t)
	t.Logf("canonical qualification: blocked_requests=%d restored_reconciles=%d rows=2 acked_seq=6", blocked.Load(), reconciles.Load()-reconcilesBeforeRestore)
}

func nativeCLIQualificationNonce(t *testing.T) string {
	t.Helper()
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(b)
}

// Output is read only after Wait has joined both exec copy goroutines. The
// child never has an unsynchronized writer racing a readiness/log reader.
type nativeCLIQualificationChild struct {
	cmd            *exec.Cmd
	cancel         context.CancelFunc
	done           chan struct{}
	stdout, stderr nativeCLIQualificationCapture
	waitErr        error
	logOnce        sync.Once
	token          string
}

// exec owns one writer for each distinct stream. Only Wait-joined readers
// access the retained bytes; writes drain excess output without growing memory.
type nativeCLIQualificationCapture struct {
	data      bytes.Buffer
	truncated bool
}

func (b *nativeCLIQualificationCapture) Len() int       { return b.data.Len() }
func (b *nativeCLIQualificationCapture) Bytes() []byte  { return b.data.Bytes() }
func (b *nativeCLIQualificationCapture) String() string { return b.data.String() }

func (b *nativeCLIQualificationCapture) Write(p []byte) (int, error) {
	n := len(p)
	remaining := (64 << 10) - b.Len()
	if len(p) > remaining {
		p = p[:remaining]
		b.truncated = true
	}
	_, _ = b.data.Write(p)
	return n, nil
}

func nativeCLIQualificationStart(t *testing.T, parent context.Context, cli, cwd string, env []string, args ...string) *nativeCLIQualificationChild {
	t.Helper()
	// Authorities are explicitly stopped and joined at each phase boundary;
	// the overall 45-second context also bounds them. One-shot calls below
	// have their own eight-second deadline, independent of the relay lifetime.
	ctx, cancel := context.WithCancel(parent)
	child := &nativeCLIQualificationChild{cancel: cancel, done: make(chan struct{})}
	for _, entry := range env {
		if value, ok := strings.CutPrefix(entry, "SIRSI_ROUTER_TOKEN="); ok {
			child.token = value
		}
	}
	child.cmd = exec.CommandContext(ctx, cli, args...)
	child.cmd.Dir, child.cmd.Env = cwd, env
	child.cmd.WaitDelay = 250 * time.Millisecond
	child.cmd.Stdout, child.cmd.Stderr = &child.stdout, &child.stderr
	t.Logf("child argv=%q cwd=%q env=fixture-only (values redacted)", append([]string{cli}, args...), cwd)
	if err := child.cmd.Start(); err != nil {
		cancel()
		t.Fatal(err)
	}
	go func() { child.waitErr = child.cmd.Wait(); close(child.done) }()
	t.Cleanup(func() { child.stop(t) })
	return child
}

func (c *nativeCLIQualificationChild) stop(t *testing.T) {
	c.cancel()
	<-c.done
	c.logOnce.Do(func() {
		t.Logf("child pid=%d status=%v stdout_bytes=%d stderr_bytes=%d", c.cmd.Process.Pid, c.waitErr, c.stdout.Len(), c.stderr.Len())
	})
}

func nativeCLIQualificationRun(t *testing.T, ctx context.Context, cli, cwd string, env []string, out any, args ...string) {
	t.Helper()
	call, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	child := nativeCLIQualificationStart(t, call, cli, cwd, env, args...)
	<-child.done
	child.stop(t)
	if child.waitErr != nil {
		child.logFailureStderr(t)
		t.Fatalf("CLI %q failed: %v", args, child.waitErr)
	}
	if child.stdout.truncated {
		t.Fatal("CLI JSON exceeded the 64 KiB fixture capture bound")
	}
	if err := json.Unmarshal(child.stdout.Bytes(), out); err != nil {
		child.logFailureStderr(t)
		t.Fatalf("CLI %q returned invalid JSON: %v", args, err)
	}
}

func (c *nativeCLIQualificationChild) logFailureStderr(t *testing.T) {
	// Cancellation and Wait precede every read, including readiness failures.
	c.stop(t)
	text := c.stderr.String()
	if c.stderr.truncated {
		// Drop the truncated final line, which could contain a partial token.
		if at := strings.LastIndexByte(text, '\n'); at >= 0 {
			text = text[:at]
		} else {
			text = ""
		}
	}
	if c.token != "" {
		text = strings.ReplaceAll(text, c.token, "[redacted fixture token]")
	}
	var safe strings.Builder
	insidePEM := false
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, "-----BEGIN ") {
			insidePEM = true
			safe.WriteString("[redacted PEM material]\n")
		}
		if insidePEM {
			if strings.Contains(line, "-----END ") {
				insidePEM = false
			}
			continue
		}
		safe.WriteString(line + "\n")
	}
	text = safe.String()
	if len(text) > 4096 {
		text = text[:4096] + "\n[diagnostic truncated]"
	}
	t.Logf("child stderr (bounded, redacted; capture_truncated=%t):\n%s", c.stderr.truncated, text)
}

func nativeCLIQualificationReady(t *testing.T, child *nativeCLIQualificationChild, address string, config *tls.Config) {
	t.Helper()
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	for {
		connection, err := tls.DialWithDialer(&net.Dialer{Timeout: 100 * time.Millisecond}, "tcp", address, config)
		if err == nil {
			_ = connection.Close()
			return
		}
		select {
		case <-child.done:
			child.logFailureStderr(t)
			t.Fatal("native CLI authority exited before mutual TLS readiness")
		case <-deadline.C:
			child.logFailureStderr(t)
			t.Fatal("native CLI authority did not become mutually authenticated within two seconds")
		case <-time.After(25 * time.Millisecond):
		}
	}
}

func nativeCLIQualificationTLS(t *testing.T, root string, now time.Time) (cert, key, ca, clientCert, clientKey, fingerprint string, config *tls.Config) {
	t.Helper()
	caPublic, caPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	issuer := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: now.Add(-time.Minute), NotAfter: now.Add(20 * time.Minute),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature}
	caDER, err := x509.CreateCertificate(rand.Reader, issuer, issuer, caPublic, caPrivate)
	if err != nil {
		t.Fatal(err)
	}
	ca = filepath.Join(root, "test-ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}), 0600); err != nil {
		t.Fatal(err)
	}
	leaf := func(name string, serial int64, usage x509.ExtKeyUsage) (string, string, []byte) {
		public, private, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		template := &x509.Certificate{SerialNumber: big.NewInt(serial), NotBefore: issuer.NotBefore, NotAfter: issuer.NotAfter,
			KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{usage}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}}
		der, err := x509.CreateCertificate(rand.Reader, template, issuer, public, caPrivate)
		if err != nil {
			t.Fatal(err)
		}
		privateDER, err := x509.MarshalPKCS8PrivateKey(private)
		if err != nil {
			t.Fatal(err)
		}
		certPath, keyPath := filepath.Join(root, name+"-cert.pem"), filepath.Join(root, name+"-key.pem")
		for path, block := range map[string]*pem.Block{certPath: {Type: "CERTIFICATE", Bytes: der}, keyPath: {Type: "PRIVATE KEY", Bytes: privateDER}} {
			if err := os.WriteFile(path, pem.EncodeToMemory(block), 0600); err != nil {
				t.Fatal(err)
			}
		}
		return certPath, keyPath, der
	}
	cert, key, _ = leaf("test-authority", 2, x509.ExtKeyUsageServerAuth)
	var clientDER []byte
	clientCert, clientKey, clientDER = leaf("test-client", 3, x509.ExtKeyUsageClientAuth)
	digest := sha256.Sum256(clientDER)
	fingerprint = hex.EncodeToString(digest[:])
	pair, err := tls.LoadX509KeyPair(clientCert, clientKey)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})) {
		t.Fatal("test CA could not be loaded")
	}
	config = &tls.Config{RootCAs: pool, Certificates: []tls.Certificate{pair}, MinVersion: tls.VersionTLS13}
	return
}
