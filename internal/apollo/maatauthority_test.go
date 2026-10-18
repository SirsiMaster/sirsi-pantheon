package apollo

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/schedule"
)

type memoryReservationStore struct{ state map[string]string }

func (s *memoryReservationStore) GetState(key string) (string, bool, error) {
	v, ok := s.state[key]
	return v, ok, nil
}
func (s *memoryReservationStore) SetState(key, value string) error {
	s.state[key] = value
	return nil
}

func nativeAuthorityFixture(t *testing.T) (*MaatAuthority, NativeAdmissionRequest) {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Second)
	ledger := schedule.NewLedger(&memoryReservationStore{state: map[string]string{}})
	a, err := NewMaatAuthority(ledger)
	if err != nil {
		t.Fatal(err)
	}
	a.now = func() time.Time { return now }
	return a, NativeAdmissionRequest{
		ExecutionID: "0123456789abcdef0123456789abcdef", PlanSHA256: strings.Repeat("a", 64), Epoch: 7,
		Holder: "local:501:test", ExpiresAt: now.Add(90 * time.Second),
		Ranks: []NativeRankRequest{
			{Rank: 0, HostID: "m5", Resource: "mem.m5", MemoryBytes: 2 << 30},
			{Rank: 1, HostID: "m1", Resource: "mem.m1", MemoryBytes: 1 << 30},
		},
	}
}

func TestMaatAuthorityAdmitsVerifiesAndReleasesEveryRank(t *testing.T) {
	a, req := nativeAuthorityFixture(t)
	admission, err := a.Admit(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if len(admission.Ranks) != 2 || admission.Ranks[0].Fence == "" || admission.Ranks[0].Generation == "" {
		t.Fatalf("incomplete admission: %#v", admission)
	}
	if err := a.Verify(context.Background(), admission); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if err := a.Release(context.Background(), admission); err != nil {
		t.Fatalf("release: %v", err)
	}
	if err := a.Verify(context.Background(), admission); err == nil {
		t.Fatal("released admission verified")
	}
}

func TestMaatAuthorityAdmitsVerifiesAndReleasesOneLocalRank(t *testing.T) {
	a, req := nativeAuthorityFixture(t)
	req.Ranks = req.Ranks[:1]
	admission, err := a.Admit(context.Background(), req)
	if err != nil {
		t.Fatalf("single local rank admission: %v", err)
	}
	if len(admission.Ranks) != 1 || admission.Ranks[0].HostID != "m5" {
		t.Fatalf("single local rank admission mismatch: %#v", admission)
	}
	if err := a.Verify(context.Background(), admission); err != nil {
		t.Fatalf("verify single local rank: %v", err)
	}
	if err := a.Release(context.Background(), admission); err != nil {
		t.Fatalf("release single local rank: %v", err)
	}
}

func TestMaatAuthorityRollsBackWhenOneRankIsContended(t *testing.T) {
	a, req := nativeAuthorityFixture(t)
	foreign, err := a.ledger.Reserve(schedule.Reservation{
		Resource: "mem.m1", Holder: "foreign", Work: "other", Regime: schedule.RegimeLoaded,
		Start: a.now().Format(time.RFC3339), EstEnd: req.ExpiresAt.Format(time.RFC3339), LeaseTTLSec: 90,
	}, false)
	if err != nil || foreign.Reservation == nil {
		t.Fatalf("set up foreign hold: %v %#v", err, foreign)
	}
	if _, err := a.Admit(context.Background(), req); err == nil {
		t.Fatal("contended all-rank admission succeeded")
	}
	rows, err := a.ledger.Status("mem.m5")
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.Work == "apollo-native:"+req.ExecutionID && row.Status == schedule.StatusActive {
			t.Fatalf("partial rank leaked after rollback: %#v", row)
		}
	}
}

func TestMaatAuthorityRefusesAlteredFence(t *testing.T) {
	a, req := nativeAuthorityFixture(t)
	admission, err := a.Admit(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	altered := cloneNativeAdmission(admission)
	altered.Ranks[0].Generation = "foreign"
	if err := a.Verify(context.Background(), altered); err == nil {
		t.Fatal("altered fence verified")
	}
	if err := a.Release(context.Background(), admission); err != nil {
		t.Fatal(err)
	}
}
