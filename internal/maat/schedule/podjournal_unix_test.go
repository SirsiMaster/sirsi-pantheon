//go:build darwin || linux

package schedule

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPodNonceSurvivesRestartAndRejectsChangedOperation(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	d := mkDelegation("pod", "native", now.Add(-time.Minute).Format(time.RFC3339), now.Add(time.Hour).Format(time.RFC3339), "studio")
	d.Epoch = 1
	path := filepath.Join(t.TempDir(), "nonces")
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
	p := filepath.Join(t.TempDir(), "pod.journal")
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
	p := filepath.Join(t.TempDir(), "journal")
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

func TestPodJournalProgressPastTTLRetainsEveryMutation(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	d := mkDelegation("studio-m5", "native-studio", now.Add(-time.Minute).Format(time.RFC3339), now.Add(time.Hour).Format(time.RFC3339), "studio")
	d.Epoch = 1
	p := filepath.Join(t.TempDir(), "events")
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
