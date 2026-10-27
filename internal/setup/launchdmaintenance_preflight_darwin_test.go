//go:build darwin

package setup

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func maintenancePreflightInventory(rows ...string) []byte {
	return []byte("system = {\nservices = {\n" + strings.Join(rows, "\n") + "\n}\n}\n")
}

func maintenancePreflightRead(raw []byte, readErr error, calls *[][]string) func(context.Context, []byte, string, ...string) ([]byte, error) {
	return func(ctx context.Context, stdin []byte, program string, args ...string) ([]byte, error) {
		*calls = append(*calls, append([]string{program}, args...))
		if len(stdin) != 0 {
			return nil, errors.New("unexpected inventory stdin")
		}
		if program != "/bin/launchctl" || !reflect.DeepEqual(args, []string{"print", "system"}) {
			return nil, errors.New("unexpected inventory command")
		}
		return raw, readErr
	}
}

func TestMaintenancePreRetirementInventoryRequiresCompleteSelectedSystemJob(t *testing.T) {
	validSelected := "0 (pe) " + rootRelayLabel
	cases := []struct {
		name    string
		raw     []byte
		readErr error
	}{
		{name: "truncated after selected", raw: []byte("system = {\nservices = {\n" + validSelected + "\n")},
		{name: "duplicate selected after selected", raw: maintenancePreflightInventory(validSelected, validSelected)},
		{name: "duplicate other row after selected", raw: maintenancePreflightInventory(validSelected, "5 - com.apple.fixture", "7 (pe) com.apple.fixture")},
		{name: "malformed row after selected", raw: maintenancePreflightInventory(validSelected, "not-a-pid - com.apple.fixture")},
		{name: "unknown status after selected", raw: maintenancePreflightInventory(validSelected, "5 (?) com.apple.fixture")},
		{name: "unknown row after selected", raw: maintenancePreflightInventory(validSelected, "unknown inventory row")},
		{name: "selected job absent", raw: maintenancePreflightInventory("0 - com.apple.fixture")},
		{name: "read error", raw: maintenancePreflightInventory(validSelected), readErr: errors.New("system inventory unavailable")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var calls [][]string
			err := maintenancePreRetirementInventory(context.Background(), maintenancePreflightRead(tc.raw, tc.readErr, &calls))
			if err == nil {
				t.Fatal("incomplete, absent, or unreadable inventory accepted")
			}
			want := [][]string{{"/bin/launchctl", "print", "system"}}
			if !reflect.DeepEqual(calls, want) {
				t.Fatalf("inventory reads %v, want %v", calls, want)
			}
		})
	}
}

func TestMaintenancePreRetirementInventoryAcceptsNativeSelectedRows(t *testing.T) {
	for _, selected := range []string{"0 - " + rootRelayLabel, "0 (pe) " + rootRelayLabel} {
		t.Run(strings.TrimPrefix(selected, "0 "), func(t *testing.T) {
			raw := maintenancePreflightInventory(
				"123 456 com.apple.fixture.numeric",
				"0 (pe) com.apple.fixture.parked",
				selected,
				"789 0 com.apple.fixture.running",
			)
			var calls [][]string
			if err := maintenancePreRetirementInventory(context.Background(), maintenancePreflightRead(raw, nil, &calls)); err != nil {
				t.Fatalf("valid complete inventory refused: %v", err)
			}
			if !reflect.DeepEqual(calls, [][]string{{"/bin/launchctl", "print", "system"}}) {
				t.Fatalf("inventory reads %v", calls)
			}
		})
	}
}

func TestMaintenancePreRetirementInventoryChecksCancellationBeforeRead(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var calls [][]string
	err := maintenancePreRetirementInventory(ctx, maintenancePreflightRead(nil, nil, &calls))
	if !errors.Is(err, context.Canceled) || len(calls) != 0 {
		t.Fatalf("canceled preflight: err=%v calls=%v", err, calls)
	}
}

func TestLegacyMaintenancePreflightRefusalLeavesPreparedStateAndNeverRetires(t *testing.T) {
	selected := "0 (pe) " + rootRelayLabel
	cases := []struct {
		name    string
		raw     []byte
		readErr error
	}{
		{name: "truncated after selected", raw: []byte("system = {\nservices = {\n" + selected + "\n")},
		{name: "duplicate selected after selected", raw: maintenancePreflightInventory(selected, selected)},
		{name: "duplicate other row after selected", raw: maintenancePreflightInventory(selected, "5 - com.apple.fixture", "7 (pe) com.apple.fixture")},
		{name: "malformed row after selected", raw: maintenancePreflightInventory(selected, "not-a-pid - com.apple.fixture")},
		{name: "unknown row after selected", raw: maintenancePreflightInventory(selected, "unknown inventory row")},
		{name: "selected job absent", raw: maintenancePreflightInventory("0 - com.apple.fixture")},
		{name: "read error", raw: maintenancePreflightInventory(selected), readErr: errors.New("system inventory unavailable")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, c := newMaintenanceFixture(t)
			var calls [][]string
			read := maintenancePreflightRead(tc.raw, tc.readErr, &calls)
			f.d.revalidate = func(ctx context.Context, _ MaintenancePlan, original bool) error {
				if !original {
					return nil
				}
				return maintenancePreRetirementInventory(ctx, read)
			}
			state, err := c.ConfirmLegacyMaintenance(context.Background(), f.plan)
			if err == nil || state.Phase != "prepared" || f.state.Phase != "prepared" {
				t.Fatalf("preflight refusal did not retain prepared state: result=%+v journal=%+v err=%v", state, f.state, err)
			}
			if countMaintenanceEvent(f, "retire") != 0 || countMaintenanceEvent(f, "persist:retirement_requested") != 0 {
				t.Fatalf("refusal crossed retirement boundary: %v", f.events)
			}
			if !reflect.DeepEqual(calls, [][]string{{"/bin/launchctl", "print", "system"}}) {
				t.Fatalf("inventory reads %v", calls)
			}
		})
	}
}

func TestNativeMaintenanceRevalidatePreflightsBeforeAnyOtherOperation(t *testing.T) {
	var calls [][]string
	read := maintenancePreflightRead(
		maintenancePreflightInventory("0 (pe) "+rootRelayLabel, "unknown inventory row"),
		nil,
		&calls,
	)
	b := nativeServiceBackend{relay: &rootRelayDependencies{read: read}}
	err := b.maintenanceOperations().revalidate(context.Background(), fixtureMaintenancePlan(), true)
	if err == nil {
		t.Fatal("invalid system inventory accepted")
	}
	want := [][]string{{"/bin/launchctl", "print", "system"}}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("native revalidation performed reads %v, want only %v", calls, want)
	}
}
