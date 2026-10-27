//go:build darwin && cgo

package setup

import (
	"strings"
	"testing"
)

func TestMaintenanceNativeSystemRowsAcceptDarwinClosedForms(t *testing.T) {
	// Native row syntax supplied by the independently captured complete system
	// inventory; no host inventory or private environment is committed here.
	rows := []string{"0 - com.apple.fixture.idle", "705 - com.apple.fixture.running", "0 (pe) com.apple.fixture.parked", "47547 (pe) com.apple.fixture.active", "0 0 com.apple.fixture.exited", "- 0 com.apple.fixture.legacy", "123 -9 com.apple.fixture.signal"}
	raw := []byte("system = {\nservices = {\n" + strings.Join(rows, "\n") + "\n}\n}\n")
	if found, err := maintenanceSystemJobListed(raw); err != nil || found {
		t.Fatalf("valid native inventory refused: found=%v err=%v", found, err)
	}
}

func TestMaintenanceNativeSystemSelectedZeroIsStillLoaded(t *testing.T) {
	for _, row := range []string{"0 - ", "0 (pe) ", "0 0 ", "123 (pe) ", "- 0 "} {
		raw := []byte("system = {\nservices = {\n" + row + rootRelayLabel + "\n}\n}\n")
		if found, err := maintenanceSystemJobListed(raw); err != nil || !found {
			t.Fatalf("selected native row %q misclassified absent: found=%v err=%v", row, found, err)
		}
	}
}

func TestMaintenanceNativeSystemRowsRejectAmbiguousOrUnknown(t *testing.T) {
	for _, rows := range []string{
		"-1 - other.job", "bad - other.job", "0 (?) other.job", "0 pe other.job", "0 unknown other.job", "0 - other.job extra",
		"0 - other.job\n0 (pe) other.job", "0 - " + rootRelayLabel + "\n123 0 " + rootRelayLabel,
	} {
		raw := []byte("system = {\nservices = {\n" + rows + "\n}\n}\n")
		if found, err := maintenanceSystemJobListed(raw); err == nil {
			t.Fatalf("ambiguous native inventory accepted: %q found=%v", rows, found)
		}
	}
	for _, raw := range []string{"system = {\nservices = {\n0 - other.job\n}\n", "system = {\nservices = {\n}\nservices = {\n}\n}\n", "gui/501 = {\nservices = {\n0 - other.job\n}\n}\n"} {
		if found, err := maintenanceSystemJobListed([]byte(raw)); err == nil {
			t.Fatalf("incomplete/foreign inventory accepted: found=%v raw=%q", found, raw)
		}
	}
}
