package brand

import (
	"regexp"
	"strings"
	"testing"
)

var hexRe = regexp.MustCompile(`^#[0-9a-f]{6}$`)

// Every role must resolve to a valid 6-digit hex in BOTH schemes — a missing or
// malformed token would render as a broken color on some surface.
func TestEveryRoleResolves(t *testing.T) {
	for _, s := range []Scheme{Dark, Light} {
		p := For(s)
		for _, r := range Roles() {
			h := p.Hex(r)
			if !hexRe.MatchString(h) {
				t.Errorf("scheme=%d role=%q hex=%q is not a #rrggbb value", s, r.Name(), h)
			}
		}
	}
}

// The identity is emerald + gold. Lock the lead tokens so a careless edit can't
// silently revert Pantheon to the old gold-primary / lapis palette.
func TestBrandIdentityIsEmeraldGold(t *testing.T) {
	cases := []struct {
		s    Scheme
		r    Role
		want string
	}{
		{Dark, Emerald, "#2bd29b"}, {Light, Emerald, "#0f7a54"},
		{Dark, Gold, "#cdad5a"}, {Light, Gold, "#8a6d1f"},
	}
	for _, c := range cases {
		if got := For(c.s).Hex(c.r); got != c.want {
			t.Errorf("scheme=%d %s = %s, want %s", c.s, c.r.Name(), got, c.want)
		}
	}
	// OK is the emerald family (healthy == brand), never gold.
	if For(Dark).Hex(OK) != For(Dark).Hex(Emerald) {
		t.Error("OK must share emerald with the brand identity")
	}
}

// Roles() is the emission contract: 12 roles, Emerald first, in iota order.
func TestRolesOrderStable(t *testing.T) {
	rs := Roles()
	if len(rs) != 12 {
		t.Fatalf("Roles() = %d, want 12", len(rs))
	}
	if rs[0] != Emerald || rs[1] != Gold {
		t.Errorf("order drifted: [0]=%s [1]=%s, want emerald,gold", rs[0].Name(), rs[1].Name())
	}
}

// The CSS emitter must carry the scheme's own values (the dashboard/Nexus read
// this), one custom property per role.
func TestCSSVarsPerScheme(t *testing.T) {
	dark := CSSVars(Dark)
	if !strings.Contains(dark, "--emerald: #2bd29b;") {
		t.Errorf("dark CSS missing emerald token:\n%s", dark)
	}
	light := CSSVars(Light)
	if !strings.Contains(light, "--emerald: #0f7a54;") {
		t.Errorf("light CSS missing emerald token:\n%s", light)
	}
	if n := strings.Count(dark, "--"); n != len(Roles()) {
		t.Errorf("dark CSS has %d vars, want %d", n, len(Roles()))
	}
}

// The Swift emitter carries BOTH schemes so macapp resolves the same hex — and
// is marked generated so no one hand-edits it into drift.
func TestSwiftColorsCarriesBothSchemes(t *testing.T) {
	sw := SwiftColors()
	for _, want := range []string{
		"DO NOT EDIT", "enum Pantheon", "import SwiftUI",
		`case .emerald: return "#0f7a54"`, // light
		`case .emerald: return "#2bd29b"`, // dark
		"init(pantheonHex hex: String)",
	} {
		if !strings.Contains(sw, want) {
			t.Errorf("Swift output missing %q:\n%s", want, sw)
		}
	}
}

// JSON is deterministic and has both schemes for every role.
func TestJSONStable(t *testing.T) {
	j := JSON()
	for _, want := range []string{`"dark"`, `"light"`, `"emerald": "#2bd29b"`, `"emerald": "#0f7a54"`, `"line"`} {
		if !strings.Contains(j, want) {
			t.Errorf("JSON missing %q:\n%s", want, j)
		}
	}
	// Two schemes × 12 roles = 24 quoted hex values.
	if n := strings.Count(j, "#"); n != 2*len(Roles()) {
		t.Errorf("JSON has %d hex values, want %d", n, 2*len(Roles()))
	}
}
