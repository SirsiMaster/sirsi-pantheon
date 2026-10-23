//go:build darwin || linux

package schedule

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

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
