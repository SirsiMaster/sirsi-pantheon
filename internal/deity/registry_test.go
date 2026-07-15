package deity

import "testing"

func TestLookup(t *testing.T) {
	tests := []struct {
		key       string
		wantName  string
		wantGlyph string
	}{
		{"horus", "Horus", "𓂀"},
		{"maat", "Ma'at", "𓆄"},
		{"osiris", "Osiris", "𓁹"},
		{"nonexistent", "nonexistent", "⚙"}, // fallback keeps the key as name
	}
	for _, tt := range tests {
		got := Lookup(tt.key)
		if got.Name != tt.wantName || got.Glyph != tt.wantGlyph {
			t.Errorf("Lookup(%q) = {%q %q}, want {%q %q}", tt.key, got.Name, got.Glyph, tt.wantName, tt.wantGlyph)
		}
		if got.Key != tt.key {
			t.Errorf("Lookup(%q).Key = %q, want %q", tt.key, got.Key, tt.key)
		}
	}
}

func TestDisplay(t *testing.T) {
	glyph, name := Display("ra")
	if glyph != "𓇶" || name != "Ra" {
		t.Errorf("Display(ra) = (%q, %q), want (𓇶, Ra)", glyph, name)
	}
}

func TestKeys_RosterOrder(t *testing.T) {
	keys := Keys()
	if len(keys) != len(Roster) {
		t.Fatalf("Keys() len = %d, want %d", len(keys), len(Roster))
	}
	// Hierarchy order (Rule D6): Horus leads, Osiris closes.
	if keys[0] != "horus" {
		t.Errorf("keys[0] = %q, want horus", keys[0])
	}
	if keys[len(keys)-1] != "osiris" {
		t.Errorf("last key = %q, want osiris", keys[len(keys)-1])
	}
	for i, d := range Roster {
		if keys[i] != d.Key {
			t.Errorf("keys[%d] = %q, want %q (roster order)", i, keys[i], d.Key)
		}
	}
}
