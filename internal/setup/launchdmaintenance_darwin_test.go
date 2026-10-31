//go:build darwin

package setup

import (
	"context"
	"reflect"
	"testing"
)

func TestLegacyMaintenanceNativeFixedLifecycleArgv(t *testing.T) {
	var got [][]string
	b := nativeServiceBackend{relay: &rootRelayDependencies{read: func(ctx context.Context, stdin []byte, program string, args ...string) ([]byte, error) {
		if len(stdin) != 0 {
			t.Fatal("unexpected stdin")
		}
		got = append(got, append([]string{program}, args...))
		if reflect.DeepEqual(args, []string{"print", "system"}) {
			return []byte("system = {\nservices = {\n}\n}\n"), nil
		}
		return nil, nil
	}}}
	d := b.maintenanceOperations()
	if err := d.retire(context.Background(), fixtureMaintenancePlan()); err != nil {
		t.Fatal(err)
	}
	if err := d.bootstrap(context.Background(), MaintenanceStatus{}); err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"/bin/launchctl", "bootout", "system/ai.sirsi.router.relay"}, {"/bin/launchctl", "print", "system"}, {"/bin/launchctl", "bootstrap", "system", "/Library/LaunchDaemons/ai.sirsi.router.relay.plist"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected selected lifecycle arguments %v", got)
	}
}

func TestLegacyMaintenanceRequiresCompleteSystemInventory(t *testing.T) {
	// Synthetic closed grammar fixture; native output compatibility remains an
	// independent owner-run read-only qualification, not credited by this data.
	for _, tc := range []struct {
		raw   string
		found bool
		bad   bool
	}{
		{"system = {\nservices = {\n123 0 other.job\n}\n}\n", false, false},
		{"system = {\nservices = {\n- 0 ai.sirsi.router.relay\n}\n}\n", true, false},
		{"system = {\nservices = {\n123 0 ai.sirsi.router.relay\n123 0 ai.sirsi.router.relay\n}\n}\n", false, true},
		{"system = {\nservices = {\nmalformed\n}\n}\n", false, true},
		{"gui/501 = {\nservices = {\n}\n}\n", false, true},
		{"system = {\nservices = {\n", false, true},
		{"", false, true},
	} {
		found, err := maintenanceSystemJobListed([]byte(tc.raw))
		if (err != nil) != tc.bad || (!tc.bad && found != tc.found) {
			t.Fatalf("%q found%v err%v", tc.raw, found, err)
		}
	}
}

func TestLegacyMaintenancePreservesDisabledOrUnknownIntent(t *testing.T) {
	for _, tc := range []struct {
		raw string
		bad bool
	}{
		{"disabled services = {\n}\n", false},
		{"disabled services = {\n\"ai.sirsi.router.relay\" => false\n}\n", false},
		{"disabled services = {\n\"unrelated.job\" => true\n}\n", false},
		{"disabled services = {\n\"ai.sirsi.router.relay\" => true\n}\n", true},
		{"disabled services = {\n\"ai.sirsi.router.relay\" => false\n\"ai.sirsi.router.relay\" => true\n}\n", true},
		{"disabled services = {\nunknown\n}\n", true},
		{"", true},
	} {
		if err := maintenanceDisabledIntent([]byte(tc.raw)); (err != nil) != tc.bad {
			t.Fatalf("%q %v", tc.raw, err)
		}
	}
}
