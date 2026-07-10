package main

import "testing"

// TestUnderProtected guards the Rule A1 safety of `ghosts clean`: a residual at
// or under any protected prefix must never be eligible for trashing.
func TestUnderProtected(t *testing.T) {
	prefixes := []string{"/System", "/usr", "/Library/Apple"}
	cases := []struct {
		path string
		want bool
	}{
		{"/System", true},
		{"/System/Library/Foo", true},
		{"/usr/bin/thing", true},
		{"/Library/Apple/whatever", true},
		{"/Users/me/Library/HTTPStorages/com.google.GoogleUpdater", false},
		{"/Users/me/Library/Caches/Sky", false},
		{"/Systemsomething/notprotected", false}, // prefix must be a path boundary
		{"/usrlocal/x", false},
	}
	for _, c := range cases {
		if got := underProtected(c.path, prefixes); got != c.want {
			t.Errorf("underProtected(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}
