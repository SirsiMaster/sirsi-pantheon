package guard

import (
	"strings"
	"testing"
)

// gateSwapOnPressure: swap may never alarm higher than CURRENT RAM pressure,
// because macOS keeps swap allocated after using it (residue, not thrashing).
func TestGateSwapOnPressure(t *testing.T) {
	swapDetail := "total = 14336.00M  used = 12986.31M  free = 1349.69M  (encrypted)"

	cases := []struct {
		name        string
		pressureSev DiagnosticSeverity
		swapSev     DiagnosticSeverity
		wantSwapSev DiagnosticSeverity
		wantMsgHas  string // substring the resulting message must contain
	}{
		{"normal pressure caps heavy swap to OK", SeverityOK, SeverityCritical, SeverityOK, "memory pressure is normal"},
		{"warn pressure caps heavy swap to warn", SeverityWarn, SeverityCritical, SeverityWarn, "under some pressure"},
		{"critical pressure keeps genuine thrashing", SeverityCritical, SeverityCritical, SeverityCritical, "thrashing"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			findings := []DiagnosticFinding{
				{Check: "RAM Pressure", Severity: c.pressureSev, Message: "ram"},
				{Check: "Swap Usage", Severity: c.swapSev, Detail: swapDetail, Message: "Heavy swapping (12.7 GB) — system is thrashing"},
			}
			gateSwapOnPressure(findings)
			if got := findings[1].Severity; got != c.wantSwapSev {
				t.Fatalf("swap severity = %v, want %v", got, c.wantSwapSev)
			}
			if !strings.Contains(findings[1].Message, c.wantMsgHas) {
				t.Fatalf("resulting message %q missing %q", findings[1].Message, c.wantMsgHas)
			}
			// A demoted swap must NOT keep the false "thrashing" claim.
			if c.wantSwapSev < SeverityCritical && strings.Contains(findings[1].Message, "thrashing") {
				t.Fatalf("demoted swap still says 'thrashing': %q", findings[1].Message)
			}
		})
	}
}

// No RAM Pressure reading → swap is left exactly as-is (don't guess).
func TestGateSwapOnPressure_NoPressureFindingLeavesSwapAlone(t *testing.T) {
	findings := []DiagnosticFinding{
		{Check: "Swap Usage", Severity: SeverityCritical, Detail: "used = 12986.31M", Message: "Heavy swapping"},
	}
	gateSwapOnPressure(findings)
	if findings[0].Severity != SeverityCritical {
		t.Fatalf("with no RAM Pressure finding, swap must be untouched, got %v", findings[0].Severity)
	}
}

func TestParseSwapUsedGB(t *testing.T) {
	gb := parseSwapUsedGB("total = 14336.00M  used = 12986.31M  free = 1349.69M  (encrypted)")
	if gb < 12.6 || gb > 12.8 {
		t.Fatalf("parseSwapUsedGB = %.2f, want ~12.68", gb)
	}
}
