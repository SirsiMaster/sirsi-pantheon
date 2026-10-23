package schedule

import (
	"context"
	"errors"
	"testing"
	"time"
)

type podClientFixture struct {
	grant               *Delegation
	err                 error
	ordinary, transport int
}

func (f *podClientFixture) MaatDelegation(string, Delegation) (Delegation, error) {
	return Delegation{}, errors.New("unused")
}
func (f *podClientFixture) MaatGetDelegation(string) (*Delegation, error) { return f.grant, f.err }
func (f *podClientFixture) MaatAuthorityReserve(string, Reservation) (ReserveResult, error) {
	return ReserveResult{}, errors.New("unused")
}
func (f *podClientFixture) MaatReconcile(string, string, int, []Reservation) (int64, error) {
	return 0, errors.New("unused")
}
func (f *podClientFixture) Reserve(r Reservation, _ bool) (ReserveResult, error) {
	f.ordinary++
	return ReserveResult{Granted: true, Reservation: &r}, nil
}
func (f *podClientFixture) ReservePod(_ context.Context, _ Delegation, r Reservation, _ bool) (ReserveResult, error) {
	f.transport++
	return ReserveResult{}, errors.New("authority down")
}

func TestPodClientUndelegatedAndPartitionRouting(t *testing.T) {
	now := time.Now()
	d := mkDelegation("studio-m5", "native-studio", now.Add(-time.Minute).Format(time.RFC3339), now.Add(time.Hour).Format(time.RFC3339), "studio")
	d.Epoch = 1
	f := &podClientFixture{grant: &d}
	cache, e := FetchFiniteGrantCache(f, d.Pod)
	if e != nil {
		t.Fatal(e)
	}
	c := PodClient{Pod: d.Pod, Canonical: f, Ordinary: f, Transport: f, Cache: cache}
	if _, e := c.Reserve(context.Background(), Reservation{Resource: "m1"}, false); e != nil || f.ordinary != 1 {
		t.Fatal("undelegated path changed", e)
	}
	if _, e := c.Reserve(context.Background(), Reservation{Resource: "studio"}, false); e == nil || f.ordinary != 1 || f.transport != 1 {
		t.Fatal("authority failure fell back")
	}
	f.err = errors.New("cloud partition")
	if _, e := c.Reserve(context.Background(), Reservation{Resource: "studio"}, false); e == nil || f.transport != 2 || f.ordinary != 1 {
		t.Fatal("cached finite delegation not routed")
	}
	f.err = nil
	d.Revoked = true
	if _, e := c.Reserve(context.Background(), Reservation{Resource: "studio"}, false); e == nil || f.transport != 2 {
		t.Fatal("revoked cache routed")
	}
	c.Pod = ""
	if _, e := c.Reserve(context.Background(), Reservation{Resource: "m1"}, false); e != nil || f.ordinary != 2 {
		t.Fatal("unconfigured old client changed", e)
	}
}

func TestFiniteGrantCacheCannotBeMutatedOrExtendedLocally(t *testing.T) {
	now := time.Now()
	d := mkDelegation("studio-m5", "native", now.Add(-time.Minute).Format(time.RFC3339), now.Add(time.Hour).Format(time.RFC3339), "studio")
	d.Epoch = 3
	f := &podClientFixture{grant: &d}
	cache, e := FetchFiniteGrantCache(f, d.Pod)
	if e != nil {
		t.Fatal(e)
	}
	expiry := d.NotAfter
	d.NotAfter = now.Add(12 * time.Hour).Format(time.RFC3339)
	d.Resources[0] = "m5"
	snapshot, e := cache.Snapshot()
	if e != nil {
		t.Fatal(e)
	}
	if snapshot.NotAfter != expiry || snapshot.Resources[0] != "studio" {
		t.Fatal("source alias mutated cache")
	}
	snapshot.Resources[0] = "foreign"
	snapshot.NotAfter = d.NotAfter
	again, e := cache.Snapshot()
	if e != nil || again.Resources[0] != "studio" || again.NotAfter != expiry {
		t.Fatal("snapshot alias extended grant")
	}
}
