package main

import "testing"

func TestFirstMeaningfulLine(t *testing.T) {
	cases := map[string]string{
		"":                              "",
		"\n\n  \n":                      "",
		"hello":                         "hello",
		"\n\n  first real line\nsecond": "first real line",
		"\x1b[32mgreen result\x1b[0m":   "green result", // ANSI stripped
	}
	for in, want := range cases {
		if got := firstMeaningfulLine(in); got != want {
			t.Errorf("firstMeaningfulLine(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestStripANSI(t *testing.T) {
	if got := stripANSI("\x1b[1;33m𓆄 Ma'at\x1b[0m audit OK"); got != "𓆄 Ma'at audit OK" {
		t.Errorf("stripANSI = %q", got)
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("  short  ", 140); got != "short" {
		t.Errorf("truncate trims+keeps short = %q", got)
	}
	long := "0123456789"
	if got := truncate(long, 5); got != "0123…" {
		t.Errorf("truncate(%q,5) = %q, want 0123…", long, got)
	}
}
