package routerstore

import (
	"fmt"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/schedule"
	"path/filepath"
	"testing"
	"time"
)

func delegationReq(s *SQLiteStore) schedule.Delegation {
	return schedule.Delegation{Pod: "test-pod", AuthorityMachine: "authority-host", Resources: []string{"studio"}, IssuedAt: s.clock().Format(time.RFC3339), NotAfter: s.clock().Add(time.Hour).Format(time.RFC3339)}
}

// openBackendStore runs these same fixtures on real PG when the existing
// admitted SIRSI_TEST_PG_DSN is provided; otherwise isolated test SQLite.
func TestMaatDelegationCanonicalTransactionOrdering(t *testing.T) {
	for _, first := range []string{"reserve", "delegate"} {
		t.Run(first, func(t *testing.T) {
			s := openBackendStore(t, filepath.Join(t.TempDir(), "test.db"))
			defer s.Close()
			ready, finish := make(chan struct{}), make(chan struct{})
			firstDone := make(chan error, 1)
			go func() {
				firstDone <- s.withMaatTransaction(func(_ *txHandle, l *schedule.Ledger) error {
					if _, err := l.GetDelegation("test-pod"); err != nil {
						return err
					}
					close(ready)
					<-finish
					if first == "reserve" {
						_, err := l.Reserve(maatRequest(s, "canonical", "studio", "lane"), false)
						return err
					}
					_, err := l.Delegate(delegationReq(s))
					return err
				})
			}()
			select {
			case <-ready:
			case <-time.After(3 * time.Second):
				t.Fatal("first transaction never acquired lock")
			}
			secondDone := make(chan error, 1)
			started := make(chan struct{})
			go func() {
				close(started)
				if first == "reserve" {
					_, err := s.MaatDelegation("delegate", delegationReq(s))
					secondDone <- err
				} else {
					_, err := s.MaatReserve(maatRequest(s, "canonical", "studio", "lane"), false)
					secondDone <- err
				}
			}()
			<-started
			select {
			case err := <-secondDone:
				close(finish)
				t.Fatalf("competing transaction passed held reservation lock: %v", err)
			case <-time.After(20 * time.Millisecond):
			}
			close(finish)
			if err := <-firstDone; err != nil {
				t.Fatal(err)
			}
			if err := <-secondDone; err == nil {
				t.Fatal("both canonical and pod authority admitted same resource")
			}
		})
	}
}
func TestMaatDelegationReplayBatchRollbackAndIdempotence(t *testing.T) {
	s := openBackendStore(t, filepath.Join(t.TempDir(), "test.db"))
	defer s.Close()
	d, err := s.MaatDelegation("delegate", delegationReq(s))
	if err != nil {
		t.Fatal(err)
	}
	req := maatRequest(s, "pod-one", "studio", "lane")
	req.HeartbeatAt = s.clock().Format(time.RFC3339)
	req.Created = req.HeartbeatAt
	req.Authority = "pod:test-pod"
	req.Epoch = d.Epoch
	req.Seq = 1
	req.Status = schedule.StatusActive
	bad := req
	bad.ID = "gap"
	bad.Seq = 3
	if _, err := s.MaatReconcile("authority-host", d.Pod, d.Epoch, []schedule.Reservation{req, bad}); err == nil {
		t.Fatal("gap batch accepted")
	}
	got, err := s.MaatGetDelegation(d.Pod)
	if err != nil || got.AckedSeq != 0 {
		t.Fatalf("failed batch acked: %+v %v", got, err)
	}
	rows, err := s.MaatRead("status", "studio")
	if err != nil || len(rows) != 0 {
		t.Fatalf("partial batch committed: %v %v", rows, err)
	}
	for i := 0; i < 2; i++ {
		ack, err := s.MaatReconcile("authority-host", d.Pod, d.Epoch, []schedule.Reservation{req})
		if err != nil || ack != 1 {
			t.Fatalf("replay%d: %d %v", i, ack, err)
		}
	}
	if _, err := s.MaatReconcile("other-host", d.Pod, d.Epoch, []schedule.Reservation{req}); err == nil {
		t.Fatal("foreign machine replay accepted")
	}
	if _, err := s.MaatReconcile("authority-host", d.Pod, d.Epoch+1, []schedule.Reservation{req}); err == nil {
		t.Fatal("stale epoch replay accepted")
	}
	if _, err := s.MaatMutate("release", req.ID, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.MaatReconcile("authority-host", d.Pod, d.Epoch, []schedule.Reservation{req}); err != nil {
		t.Fatal("duplicate history replay should acknowledge without resurrecting", err)
	}
	rows, err = s.MaatRead("status", "studio")
	if err != nil || len(rows) != 1 || rows[0].Status != schedule.StatusReleased {
		t.Fatalf("replay resurrected: %v %v", rows, err)
	}
}
func TestMaatDelegationLegacyUndelegatedCompatibility(t *testing.T) {
	s := openBackendStore(t, filepath.Join(t.TempDir(), "test.db"))
	defer s.Close()
	if _, err := s.MaatDelegation("delegate", delegationReq(s)); err != nil {
		t.Fatal(err)
	}
	raw, exists, err := s.GetState(maatReservationsKey)
	if err != nil {
		t.Fatal(err)
	}
	safe := fmt.Sprintf(`{"reservations":[{"id":"legacy","resource":"m1","holder":"lane","status":"active"}]}`)
	if ok, err := s.CompareAndSwapState(maatReservationsKey, raw, exists, safe); err != nil || !ok {
		t.Fatalf("undelegated legacy write refused %v %v", ok, err)
	}
	raw, exists, _ = s.GetState(maatReservationsKey)
	if ok, err := s.CompareAndSwapState(maatReservationsKey, raw, exists, `{"reservations":[{"id":"attack","resource":"studio","holder":"lane","status":"active"}]}`); err == nil || ok {
		t.Fatal("legacy delegated-resource write bypassed fence")
	}
	if err := s.SetState("maat:delegations", `{"delegations":[]}`); err == nil {
		t.Fatal("generic authority overwrite accepted")
	}
}
