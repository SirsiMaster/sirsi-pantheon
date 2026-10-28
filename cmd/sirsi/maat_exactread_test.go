package main

import (
	"fmt"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/schedule"
	"testing"
)

type exactReadCLIStore struct{ kind, id string }

func (*exactReadCLIStore) GetState(string) (string, bool, error) {
	return "", false, fmt.Errorf("unexpected broad state read")
}
func (*exactReadCLIStore) SetState(string, string) error { return fmt.Errorf("unexpected write") }
func (*exactReadCLIStore) MaatReserve(schedule.Reservation, bool) (schedule.ReserveResult, error) {
	return schedule.ReserveResult{}, fmt.Errorf("unexpected reserve")
}
func (*exactReadCLIStore) MaatMutate(string, string, string) (*schedule.Reservation, error) {
	return nil, fmt.Errorf("unexpected mutation")
}
func (s *exactReadCLIStore) MaatRead(k, id string) ([]schedule.Reservation, error) {
	s.kind, s.id = k, id
	return []schedule.Reservation{{ID: id, Status: schedule.StatusReleased}}, nil
}
func TestMaatStatusExactIDUsesBoundedKind(t *testing.T) {
	old, oldID, oldJSON := maatLedger, maatStatusID, maatJSON
	flag := maatStatusCmd.Flags().Lookup("id")
	oldChanged := flag.Changed
	t.Cleanup(func() { maatLedger, maatStatusID, maatJSON = old, oldID, oldJSON; flag.Changed = oldChanged })
	s := &exactReadCLIStore{}
	l := schedule.NewLedger(s)
	maatLedger = func() (*schedule.Ledger, error) { return l, nil }
	maatStatusID = "805f"
	maatJSON = true
	flag.Changed = true
	if e := maatStatusCmd.RunE(maatStatusCmd, nil); e != nil {
		t.Fatal(e)
	}
	if s.kind != "reservation" || s.id != "805f" {
		t.Fatal(s)
	}
	if e := maatStatusCmd.RunE(maatStatusCmd, []string{"m5"}); e == nil {
		t.Fatal("ambiguous resource accepted")
	}
	if _, e := maatPendingCedes(l, "", true); e != nil {
		t.Fatal("exact mode read cedes", e)
	}
}
