package signing_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Daemon fixtures run the real sirsi-signd.sh in a temp HOME with fake codesign/spctl. They sign nothing, read no
// keychain and contact no Apple service. Only the PATH line (to reach the fakes) and the timestamp backoff (to keep
// the test fast) are rewritten in the copy under test.
type signdRun struct {
	root, out, audit, receipt string
	calls                     int
}

func runSignd(t *testing.T, codesignFails int, failText string, setup func(home string), env ...string) signdRun {
	t.Helper()
	zsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("signing daemon requires zsh")
	}
	src, err := os.ReadFile("sirsi-signd.sh")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	os.Mkdir(bin, 0o700)
	s := string(src)
	for _, r := range [][2]string{
		{"export PATH=/usr/bin:/bin:/usr/sbin:/sbin:/opt/homebrew/bin", "export PATH=" + bin + ":/usr/bin:/bin:/usr/sbin:/sbin"},
		{"TS_BACKOFF=(10 30)", "TS_BACKOFF=(0 0)"},
	} {
		if !strings.Contains(s, r[0]) {
			t.Fatalf("daemon no longer contains %q: update the fixture", r[0])
		}
		s = strings.Replace(s, r[0], r[1], 1)
	}
	daemon := filepath.Join(root, "sirsi-signd.sh")
	os.WriteFile(daemon, []byte(s), 0o700)
	// fake codesign: the signing call fails `codesignFails` times with failText, then succeeds; -dvv answers like a good signature
	os.WriteFile(filepath.Join(bin, "codesign"), []byte(`#!/bin/sh
case "$1" in -dvv) printf '%s\n' 'Authority=Developer ID Application: Sirsi Technologies Inc. (9D382WV988)' 'TeamIdentifier=9D382WV988' 'Timestamp=fixture' >&2; exit 0;; esac
n=$(cat "$HOME/calls" 2>/dev/null || echo 0); n=$((n+1)); echo $n > "$HOME/calls"
if [ $n -le `+strconv.Itoa(codesignFails)+` ]; then echo "`+failText+`" >&2; exit 1; fi
echo "signed (fixture)"
`), 0o700)
	home := filepath.Join(root, "home")
	job := "11111111-2222-4333-8444-555555555555"
	J := filepath.Join(home, "sirsi-sign", "inbox", job)
	os.MkdirAll(J, 0o700)
	os.WriteFile(filepath.Join(J, "kind"), []byte("codesign\n"), 0o600)
	os.WriteFile(filepath.Join(J, "caller"), []byte("fixture@test\n"), 0o600)
	os.WriteFile(filepath.Join(J, "tool"), []byte("unsigned"), 0o600)
	if setup != nil {
		setup(home)
	}
	cmd := exec.Command(zsh, "-f", daemon)
	cmd.Env = append([]string{"HOME=" + home, "PATH=" + bin + ":/usr/bin:/bin", "TMPDIR=" + root}, env...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("daemon run: %v %s", err, out)
	}
	r := signdRun{root: home}
	b, _ := os.ReadFile(filepath.Join(home, "sirsi-sign", "audit.log"))
	r.audit = string(b)
	b, _ = os.ReadFile(filepath.Join(home, "sirsi-sign", "outbox", job, "receipt.txt"))
	r.receipt = string(b)
	b, _ = os.ReadFile(filepath.Join(home, "calls"))
	r.calls, _ = strconv.Atoi(strings.TrimSpace(string(b)))
	r.out = filepath.Join(home, "sirsi-sign", "outbox", job)
	return r
}

const tsDown = "The timestamp service is not available."

func TestSigndRetriesTimestampOutageThenSucceeds(t *testing.T) {
	r := runSignd(t, 2, tsDown, nil)
	if r.calls != 3 {
		t.Fatalf("want 3 signing tries, got %d", r.calls)
	}
	if _, err := os.Stat(filepath.Join(r.out, "done")); err != nil {
		t.Fatalf("job not done after a recovered timestamp outage: %s", r.receipt)
	}
	if _, err := os.Stat(filepath.Join(r.out, "tool")); err != nil {
		t.Fatal("signed artifact not published")
	}
	if !strings.Contains(r.receipt, "try 1 of 3") || !strings.Contains(r.receipt, "try 2 of 3") || !strings.Contains(r.audit, "result=ok") {
		t.Fatalf("retries not recorded: %s / %s", r.receipt, r.audit)
	}
}

func TestSigndFailsClosedAfterThreeTimestampOutages(t *testing.T) {
	r := runSignd(t, 3, tsDown, nil)
	if r.calls != 3 {
		t.Fatalf("want exactly 3 tries, got %d", r.calls)
	}
	if _, err := os.Stat(filepath.Join(r.out, "failed")); err != nil {
		t.Fatal("job not failed after 3 timestamp outages")
	}
	if _, err := os.Stat(filepath.Join(r.out, "tool")); !os.IsNotExist(err) {
		t.Fatal("an unsigned artifact was published")
	}
}

func TestSigndDoesNotRetryOtherFailures(t *testing.T) {
	r := runSignd(t, 1, "errSecInternalComponent", nil)
	if r.calls != 1 {
		t.Fatalf("a non-timestamp failure was retried (%d tries)", r.calls)
	}
	if _, err := os.Stat(filepath.Join(r.out, "failed")); err != nil {
		t.Fatal("job not failed")
	}
}

func mkOutboxJob(home, id, marker string) string {
	d := filepath.Join(home, "sirsi-sign", "outbox", id)
	os.MkdirAll(d, 0o700)
	os.WriteFile(filepath.Join(d, "receipt.txt"), []byte("r"), 0o600)
	os.WriteFile(filepath.Join(d, "big.pkg"), []byte("artifact"), 0o600)
	if marker != "" {
		os.WriteFile(filepath.Join(d, marker), nil, 0o600)
	}
	old := time.Now().Add(-400 * 24 * time.Hour)
	os.Chtimes(d, old, old)
	return d
}

// Pruning stays off: SIRSI_SIGND_KEEP_DAYS unset (runSignd passes a closed environment, so nothing is inherited)
// means NO pruning, as do empty, zero and non-numeric values. A year-old finished job keeps its artifact. Turning
// pruning on needs consumer acknowledgment, reader/writer fencing and custodian approval (sirsi-signd.sh header).
func TestSigndNeverPrunesByDefault(t *testing.T) {
	for _, env := range [][]string{nil, {"SIRSI_SIGND_KEEP_DAYS="}, {"SIRSI_SIGND_KEEP_DAYS=0"}, {"SIRSI_SIGND_KEEP_DAYS=abc"}} {
		var d string
		r := runSignd(t, 0, "", func(home string) { d = mkOutboxJob(home, "aaaaaaaa-0000-4000-8000-000000000009", "done") }, env...)
		if _, err := os.Stat(filepath.Join(d, "big.pkg")); err != nil {
			t.Fatalf("artifact pruned with %v", env)
		}
		if strings.Contains(r.audit, "result=pruned") {
			t.Fatalf("prune audited with %v: %s", env, r.audit)
		}
	}
}

// A dead hidden staging dir (.<id>, e.g. left by a late remote writer after the client gave up) is never enqueued.
func TestSigndIgnoresHiddenStaging(t *testing.T) {
	id := "aaaaaaaa-0000-4000-8000-00000000000a"
	r := runSignd(t, 0, "", func(home string) {
		st := filepath.Join(home, "sirsi-sign", "inbox", "."+id)
		os.MkdirAll(st, 0o700)
		os.WriteFile(filepath.Join(st, "kind"), []byte("codesign\n"), 0o600)
		os.WriteFile(filepath.Join(st, "artifact.bin"), []byte("late"), 0o600)
	})
	if _, err := os.Stat(filepath.Join(r.root, "sirsi-sign", "outbox", id)); !os.IsNotExist(err) {
		t.Fatal("hidden staging was processed")
	}
	if strings.Contains(r.audit, id) {
		t.Fatalf("hidden staging reached the audit log: %s", r.audit)
	}
}

func TestSigndPrunesOnlyOldFinishedArtifacts(t *testing.T) {
	old := time.Now().Add(-20 * 24 * time.Hour)
	mk := func(home, id string, marker string) string {
		d := filepath.Join(home, "sirsi-sign", "outbox", id)
		os.MkdirAll(d, 0o700)
		os.WriteFile(filepath.Join(d, "receipt.txt"), []byte("r"), 0o600)
		os.WriteFile(filepath.Join(d, "big.pkg"), []byte("artifact"), 0o600)
		if marker != "" {
			os.WriteFile(filepath.Join(d, marker), nil, 0o600)
		}
		return d
	}
	var oldDone, oldOpen, newDone string
	r := runSignd(t, 0, "", func(home string) {
		oldDone = mk(home, "aaaaaaaa-0000-4000-8000-000000000001", "done")
		oldOpen = mk(home, "aaaaaaaa-0000-4000-8000-000000000002", "")
		newDone = mk(home, "aaaaaaaa-0000-4000-8000-000000000003", "failed")
		for _, d := range []string{oldDone, oldOpen} {
			os.Chtimes(d, old, old)
		}
	}, "SIRSI_SIGND_KEEP_DAYS=14")
	if _, err := os.Stat(filepath.Join(oldDone, "big.pkg")); !os.IsNotExist(err) {
		t.Error("old finished artifact not pruned")
	}
	for _, keep := range []string{filepath.Join(oldDone, "receipt.txt"), filepath.Join(oldDone, "done"), filepath.Join(oldOpen, "big.pkg"), filepath.Join(newDone, "big.pkg")} {
		if _, err := os.Stat(keep); err != nil {
			t.Errorf("pruned what must stay: %s", keep)
		}
	}
	if !strings.Contains(r.audit, "id=aaaaaaaa-0000-4000-8000-000000000001 result=pruned removed=big.pkg") || strings.Contains(r.audit, "000000000002 result=pruned") {
		t.Errorf("prune audit wrong: %s", r.audit)
	}
}
