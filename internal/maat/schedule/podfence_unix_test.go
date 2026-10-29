//go:build darwin || linux

package schedule

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPodCanonicalFenceSurvivesRestartAndPreservesCleanup(t *testing.T) {
	for _, change := range []string{"revoked", "missing", "epoch", "machine", "resource", "expiry"} {
		t.Run(change, func(t *testing.T) {
			now := time.Now().UTC().Truncate(time.Second)
			d := mkDelegation("pod", "native", now.Add(-time.Minute).Format(time.RFC3339), now.Add(time.Hour).Format(time.RFC3339), "studio")
			d.Epoch = 1
			path := filepath.Join(canonicalPodTestDir(t), "events")
			j, err := OpenPodJournal(path, "native", d)
			if err != nil {
				t.Fatal(err)
			}
			req := mkReq("studio", "ra", now.Format(time.RFC3339), now.Add(time.Minute).Format(time.RFC3339), RegimeQuiet)
			req.ID = "retained"
			nonce := podRequest{Pod: d.Pod, Epoch: d.Epoch, Nonce: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Action: "reserve", Reservation: req}
			if _, err = j.ExecutePod("ra", nonce); err != nil {
				t.Fatal(err)
			}
			observed := d
			switch change {
			case "revoked":
				observed.Revoked = true
			case "epoch":
				observed.Epoch++
			case "machine":
				observed.AuthorityMachine = "other"
			case "resource":
				observed.Resources = []string{"m5"}
			case "expiry":
				observed.NotAfter = now.Add(-time.Second).Format(time.RFC3339)
			}
			canonical := &podClientFixture{grant: &observed}
			if change == "missing" {
				canonical.grant = nil
			}
			if _, err = j.RelayOnce(context.Background(), canonical); err == nil {
				t.Fatal("lost authority not reported")
			}
			// Reopen with the original still-unexpired cached grant, as during
			// an offline restart. The durable observation must win over it.
			if err = j.Close(); err != nil {
				t.Fatal(err)
			}
			j, err = OpenPodJournal(path, "native", d)
			if err != nil {
				t.Fatal(err)
			}
			defer j.Close()
			if _, err = j.ExecutePod("ra", nonce); err == nil {
				t.Fatal("historical nonce restored liveness")
			}
			if _, err = j.Mutate("ra", "heartbeat", req.ID, ""); err == nil {
				t.Fatal("fenced heartbeat renewed")
			}
			if _, err = j.Mutate("ra", "extend", req.ID, now.Add(2*time.Minute).Format(time.RFC3339)); err == nil {
				t.Fatal("fenced extension admitted")
			}
			if _, err = j.ReadOwn("ra", "studio"); err == nil {
				t.Fatal("fenced guard read became live")
			}
			if _, err = j.Mutate("ra", "release", req.ID, ""); err != nil {
				t.Fatal("owned cleanup refused", err)
			}
			req.ID = "new"
			if _, err = j.Reserve(req, false); err == nil {
				t.Fatal("new grant admitted after fence")
			}
			rows := j.Replay(0)
			if len(rows) != 2 || rows[0].Seq != 1 || rows[1].Seq != 2 || rows[1].Status != StatusReleased {
				t.Fatal("fence changed replay sequence/history", rows)
			}
			// Observing the old live grant again cannot erase a known fence.
			canonical.grant = &d
			if _, err = j.RelayOnce(context.Background(), canonical); err == nil {
				t.Fatal("sticky fence reported healthy")
			}
			if _, err = j.Reserve(req, false); err == nil {
				t.Fatal("old observation cleared fence")
			}
		})
	}
}

func TestPodCanonicalOutageDoesNotRevokeFiniteAuthority(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	d := mkDelegation("pod", "native", now.Add(-time.Minute).Format(time.RFC3339), now.Add(time.Hour).Format(time.RFC3339), "studio")
	d.Epoch = 1
	path := filepath.Join(canonicalPodTestDir(t), "events")
	j, err := OpenPodJournal(path, "native", d)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	canonical := &podClientFixture{err: errors.New("fixture offline")}
	if _, err = j.RelayOnce(context.Background(), canonical); err == nil {
		t.Fatal("outage hidden")
	}
	req := mkReq("studio", "ra", now.Format(time.RFC3339), now.Add(time.Minute).Format(time.RFC3339), RegimeQuiet)
	if result, err := j.Reserve(req, false); err != nil || !result.Granted {
		t.Fatal("transport outage revoked finite grant", err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	canonical.grant, canonical.err = &d, nil
	canonical.grant.AckedSeq = 1 // Acknowledgement is not a grant change.
	if _, err = j.RelayOnce(context.Background(), canonical); err != nil {
		t.Fatal("ack-only observation fenced authority", err)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(before) != string(after) {
		t.Fatal("healthy observation wrote a fence", err)
	}
}

func TestPodCanonicalFenceWriteFailureRefusesAdmission(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	d := mkDelegation("pod", "native", now.Add(-time.Minute).Format(time.RFC3339), now.Add(time.Hour).Format(time.RFC3339), "studio")
	d.Epoch = 1
	j, err := OpenPodJournal(filepath.Join(canonicalPodTestDir(t), "events"), "native", d)
	if err != nil {
		t.Fatal(err)
	}
	// Deliberately close only this fixture's owned file descriptor. No host
	// service, authority or shared journal is involved.
	if err = j.f.Close(); err != nil {
		t.Fatal(err)
	}
	d.Revoked = true
	if _, err = j.RelayOnce(context.Background(), &podClientFixture{grant: &d}); err == nil {
		t.Fatal("durability failure hidden")
	}
	req := mkReq("studio", "ra", now.Format(time.RFC3339), now.Add(time.Minute).Format(time.RFC3339), RegimeQuiet)
	if _, err = j.Reserve(req, false); err == nil {
		t.Fatal("failed fence write allowed admission")
	}
}

func TestPodCanonicalFenceRejectsInvalidRetainedFrames(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	d := mkDelegation("pod", "native", now.Add(-time.Minute).Format(time.RFC3339), now.Add(time.Hour).Format(time.RFC3339), "studio")
	d.Epoch = 1
	for _, bad := range []map[string]any{
		{"grant": d, "fenced": true, "seq": 1},
		{"grant": d, "fenced": true, "seq": 0, "rows": []Reservation{{ID: "foreign"}}},
		{"grant": d, "fenced": true, "seq": 0, "operation": "unproved"},
	} {
		path := filepath.Join(canonicalPodTestDir(t), "bad-fence")
		body, err := json.Marshal(bad)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, append(body, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
		if j, err := OpenPodJournal(path, "native", d); err == nil {
			j.Close()
			t.Fatal("malformed fence admitted")
		}
	}
}
