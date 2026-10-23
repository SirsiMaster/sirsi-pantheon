//go:build darwin || linux

package schedule

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// Only canonicalize our newly allocated test directory. Production inputs
// must still traverse every parent through retained no-follow descriptors.
// macOS commonly returns /var/... here, whose first component is a symlink.
func canonicalPodTestDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestPodNonceSurvivesRestartAndRejectsChangedOperation(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	d := mkDelegation("pod", "native", now.Add(-time.Minute).Format(time.RFC3339), now.Add(time.Hour).Format(time.RFC3339), "studio")
	d.Epoch = 1
	path := filepath.Join(canonicalPodTestDir(t), "nonces")
	j, e := OpenPodJournal(path, "native", d)
	if e != nil {
		t.Fatal(e)
	}
	r := mkReq("studio", "ra", now.Format(time.RFC3339), now.Add(time.Minute).Format(time.RFC3339), RegimeQuiet)
	r.ID = "nonce-grant"
	reserve := podRequest{Pod: d.Pod, Epoch: 1, Nonce: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Action: "reserve", Reservation: r}
	result, e := j.ExecutePod("ra", reserve)
	if e != nil {
		t.Fatal(e)
	}
	var grant ReserveResult
	if e = json.Unmarshal(result, &grant); e != nil {
		t.Fatal(e)
	}
	hb := podRequest{Pod: d.Pod, Epoch: 1, Nonce: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Action: "heartbeat", ID: grant.Reservation.ID}
	first, e := j.ExecutePod("ra", hb)
	if e != nil {
		t.Fatal(e)
	}
	j.Close()
	d.AckedSeq = 2
	j, e = OpenPodJournal(path, "native", d)
	if e != nil {
		t.Fatal(e)
	}
	defer j.Close()
	again, e := j.ExecutePod("ra", hb)
	if e != nil || !bytes.Equal(first, again) || len(j.Replay(0)) != 2 {
		t.Fatal("lost reply replayed mutation after restart", e)
	}
	changed := hb
	changed.Action = "release"
	if _, e = j.ExecutePod("ra", changed); e == nil {
		t.Fatal("nonce reused with different operation")
	}
	if _, e = j.ExecutePod("foreign", hb); e == nil {
		t.Fatal("nonce borrowed by foreign holder")
	}
	changed.Nonce = "cccccccccccccccccccccccccccccccc"
	if _, e = j.ExecutePod("ra", changed); e != nil {
		t.Fatal(e)
	}
	if _, e = j.ExecutePod("ra", reserve); e == nil {
		t.Fatal("historical receipt revived released grant")
	}
}

func TestPodJournalDurabilityFiniteFenceAndExclusiveOwner(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	d := mkDelegation("studio-m5", "native-studio", now.Add(-time.Minute).Format(time.RFC3339), now.Add(time.Hour).Format(time.RFC3339), "studio")
	d.Epoch = 1
	p := filepath.Join(canonicalPodTestDir(t), "pod.journal")
	j, e := OpenPodJournal(p, "native-studio", d)
	if e != nil {
		t.Fatal(e)
	}
	if other, e := OpenPodJournal(p, "native-studio", d); e == nil {
		other.Close()
		t.Fatal("second writer admitted")
	}
	req := mkReq("studio", "ra", now.Format(time.RFC3339), now.Add(time.Minute).Format(time.RFC3339), RegimeQuiet)
	got, e := j.Reserve(req, false)
	if e != nil || !got.Granted {
		t.Fatalf("grant: %+v %v", got, e)
	}
	if got.Reservation.Seq != 1 || got.Reservation.Epoch != 1 || got.Reservation.Authority != "pod:studio-m5" {
		t.Fatalf("missing fence %+v", got.Reservation)
	}
	j.Close()
	j, e = OpenPodJournal(p, "native-studio", d)
	if e != nil {
		t.Fatal(e)
	}
	defer j.Close()
	rows := j.Replay(0)
	if len(rows) != 1 || rows[0].ID != got.Reservation.ID {
		t.Fatalf("restart lost grant %+v", rows)
	}
	if len(j.Replay(1)) != 0 {
		t.Fatal("acked grant replayed")
	}
	req.Resource = "m5"
	if _, e = j.Reserve(req, false); e == nil {
		t.Fatal("foreign resource admitted")
	}
	req.Resource = "studio"
	req.EstEnd = now.Add(2 * time.Hour).Format(time.RFC3339)
	if _, e = j.Reserve(req, false); e == nil {
		t.Fatal("unbounded grant admitted")
	}
	j.now = func() time.Time { return now.Add(2 * time.Hour) }
	if _, e = j.Reserve(req, false); e == nil {
		t.Fatal("expired authority admitted")
	}
}

func TestPodJournalRefusesTornTailAndSymlink(t *testing.T) {
	now := time.Now().UTC()
	d := mkDelegation("studio-m5", "native-studio", now.Add(-time.Minute).Format(time.RFC3339), now.Add(time.Hour).Format(time.RFC3339), "studio")
	d.Epoch = 1
	base := canonicalPodTestDir(t)
	valid, err := OpenPodJournal(filepath.Join(base, "valid-journal"), "native-studio", d)
	if err != nil {
		t.Fatalf("canonical parent positive control: %v", err)
	}
	if err := valid.Close(); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(base, "journal")
	if e := os.WriteFile(p, []byte("{\"seq\":"), 0600); e != nil {
		t.Fatal(e)
	}
	if j, e := OpenPodJournal(p, "native-studio", d); e == nil {
		j.Close()
		t.Fatal("torn tail accepted")
	}
	link := p + "-link"
	if e := os.Symlink(p, link); e != nil {
		t.Fatal(e)
	}
	if j, e := OpenPodJournal(link, "native-studio", d); e == nil {
		j.Close()
		t.Fatal("symlink accepted")
	}
}

func TestPodJournalRejectsSymlinkAncestor(t *testing.T) {
	now := time.Now().UTC()
	d := mkDelegation("pod", "native", now.Add(-time.Minute).Format(time.RFC3339), now.Add(time.Hour).Format(time.RFC3339), "studio")
	d.Epoch = 1
	base := canonicalPodTestDir(t)
	real := filepath.Join(base, "real")
	if err := os.Mkdir(real, 0700); err != nil {
		t.Fatal(err)
	}
	j, err := OpenPodJournal(filepath.Join(real, "events"), "native", d)
	if err != nil {
		t.Fatalf("canonical parent positive control: %v", err)
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(base, "alias")
	if err := os.Symlink(real, alias); err != nil {
		t.Fatal(err)
	}
	if j, err := OpenPodJournal(filepath.Join(alias, "events"), "native", d); err == nil {
		_ = j.Close()
		t.Fatal("symlink ancestor accepted")
	}
}

func TestPodJournalProgressPastTTLRetainsEveryMutation(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	d := mkDelegation("studio-m5", "native-studio", now.Add(-time.Minute).Format(time.RFC3339), now.Add(time.Hour).Format(time.RFC3339), "studio")
	d.Epoch = 1
	p := filepath.Join(canonicalPodTestDir(t), "events")
	j, e := OpenPodJournal(p, "native-studio", d)
	if e != nil {
		t.Fatal(e)
	}
	j.now = func() time.Time { return now }
	r := mkReq("studio", "ra", now.Format(time.RFC3339), now.Add(20*time.Minute).Format(time.RFC3339), RegimeQuiet)
	r.LeaseTTLSec = 120
	out, e := j.Reserve(r, false)
	if e != nil {
		t.Fatal(e)
	}
	id := out.Reservation.ID
	for i := 0; i < 4; i++ {
		now = now.Add(90 * time.Second)
		if _, e = j.Mutate("foreign", "heartbeat", id, ""); e == nil {
			t.Fatal("foreign heartbeat accepted")
		}
		if _, e = j.Mutate("ra", "heartbeat", id, ""); e != nil {
			t.Fatal("healthy progress expired", e)
		}
	}
	if _, e = j.Mutate("ra", "extend", id, now.Add(10*time.Minute).Format(time.RFC3339)); e != nil {
		t.Fatal(e)
	}
	if _, e = j.Mutate("ra", "release", id, ""); e != nil {
		t.Fatal(e)
	}
	if _, e = j.Mutate("ra", "heartbeat", id, ""); e == nil {
		t.Fatal("released reservation resurrected")
	}
	events := j.Replay(0)
	if len(events) != 7 {
		t.Fatalf("lost interleaved events: %d", len(events))
	}
	for i, r := range events {
		if r.Seq != int64(i+1) {
			t.Fatal("sequence gap", events)
		}
	}
	if len(j.Replay(4)) != 3 {
		t.Fatal("partial ack discarded unacked events")
	}
	j.Close()
	j, e = OpenPodJournal(p, "native-studio", d)
	if e != nil {
		t.Fatal(e)
	}
	defer j.Close()
	if events = j.Replay(0); len(events) != 7 || events[6].Status != StatusReleased {
		t.Fatal("recovery lost history", events)
	}
}

type relayRecoveryFixture struct {
	podClientFixture
	calls int
}

func (f *relayRecoveryFixture) MaatGetDelegation(_ string) (*Delegation, error) {
	f.calls++
	if f.calls == 1 {
		return nil, errors.New("fixture canonical outage")
	}
	return f.grant, nil
}
func TestPodRelayReportsOnlyTransitionsAndStops(t *testing.T) {
	now := time.Now().UTC()
	d := mkDelegation("pod", "native", now.Add(-time.Minute).Format(time.RFC3339), now.Add(time.Hour).Format(time.RFC3339), "studio")
	d.Epoch = 1
	j, e := OpenPodJournal(filepath.Join(canonicalPodTestDir(t), "relay"), "native", d)
	if e != nil {
		t.Fatal(e)
	}
	defer j.Close()
	f := &relayRecoveryFixture{podClientFixture: podClientFixture{grant: &d}}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	reports := []bool{}
	j.RunRelay(ctx, f, time.Millisecond, func(err error) {
		reports = append(reports, err != nil)
		if err == nil {
			cancel()
		}
	})
	if f.calls != 2 || len(reports) != 2 || !reports[0] || reports[1] {
		t.Fatalf("relay transition mismatch calls%d reports%v", f.calls, reports)
	}
}

// Two independent processes reproduce a caller crash after durable commit and
// before reply consumption. No prior process memory survives the retry.
func TestPodIndependentProcessRetryHandoff(t *testing.T) {
	if path := os.Getenv("MAAT_FIXTURE_CHILD_JOURNAL"); path != "" {
		now, e := time.Parse(time.RFC3339, os.Getenv("MAAT_FIXTURE_NOW"))
		if e != nil {
			t.Fatal(e)
		}
		d := mkDelegation("pod", "native", now.Add(-time.Minute).Format(time.RFC3339), now.Add(time.Hour).Format(time.RFC3339), "studio")
		d.Epoch = 1
		j, e := OpenPodJournal(path, "native", d)
		if e != nil {
			t.Fatal(e)
		}
		defer j.Close()
		r := mkReq("studio", "ra", now.Format(time.RFC3339), now.Add(20*time.Minute).Format(time.RFC3339), RegimeQuiet)
		r.ID = "process-stable"
		if _, e = j.ExecutePod("ra", podRequest{Nonce: "dddddddddddddddddddddddddddddddd", Pod: d.Pod, Epoch: 1, Action: "reserve", Reservation: r}); e != nil {
			t.Fatal(e)
		}
		return
	}
	now := time.Now().UTC().Truncate(time.Second)
	path := filepath.Join(canonicalPodTestDir(t), "process-journal")
	for i := 0; i < 2; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		c := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestPodIndependentProcessRetryHandoff$")
		c.Env = append(os.Environ(), "MAAT_FIXTURE_CHILD_JOURNAL="+path, "MAAT_FIXTURE_NOW="+now.Format(time.RFC3339))
		out, e := c.CombinedOutput()
		cancel()
		if e != nil {
			t.Fatalf("child %d: %v %s", i, e, out)
		}
	}
	d := mkDelegation("pod", "native", now.Add(-time.Minute).Format(time.RFC3339), now.Add(time.Hour).Format(time.RFC3339), "studio")
	d.Epoch = 1
	j, e := OpenPodJournal(path, "native", d)
	if e != nil {
		t.Fatal(e)
	}
	defer j.Close()
	if len(j.Replay(0)) != 1 {
		t.Fatal("process retry duplicated durable grant")
	}
}
