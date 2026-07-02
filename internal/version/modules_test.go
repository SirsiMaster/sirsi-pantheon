package version

import "testing"

func TestGet(t *testing.T) {
	tests := []struct {
		module string
		want   string
	}{
		{"maat", "1.1.0"},
		{"stele", "1.0.0"},
		{"no-such-module", "unknown"},
	}
	for _, tt := range tests {
		if got := Get(tt.module); got != tt.want {
			t.Errorf("Get(%q) = %q, want %q", tt.module, got, tt.want)
		}
	}
}

func TestModules_NonEmptyVersions(t *testing.T) {
	for mod, ver := range Modules {
		if ver == "" {
			t.Errorf("module %q has an empty version", mod)
		}
	}
}
