package namespec

import "testing"

// TestParse covers the grammar in both directions (A35): well-formed names parse
// to the right slots, and malformed names are rejected. The task-with-hyphens
// cases are the SSA C1 fixtures (a grammar that accepted `[a-z0-9]+` slots but
// showed `fw-r02`/`mercury-releases` task examples was internally inconsistent).
func TestParse(t *testing.T) {
	ok := []struct {
		in   string
		want Name
	}{
		{"claude-finalwishes-m1", Name{"claude", "finalwishes", "m1", ""}},
		{"ra-router-m1", Name{"ra", "router", "m1", ""}},
		{"codex-finalwishes-m5", Name{"codex", "finalwishes", "m5", ""}},
		{"claude-home-m1", Name{"claude", "home", "m1", ""}},
		{"sirsi-governance-m5", Name{"sirsi", "governance", "m5", ""}},
		// task present, single segment
		{"claude-finalwishes-m1-web", Name{"claude", "finalwishes", "m1", "web"}},
		// task with internal hyphens (C1): everything after the 3rd hyphen
		{"claude-finalwishes-m1-fw-r02", Name{"claude", "finalwishes", "m1", "fw-r02"}},
		{"mercury-mercury-m5-mercury-releases", Name{"mercury", "mercury", "m5", "mercury-releases"}},
		{"codex-finalwishes-m5-fw-r02-hotfix", Name{"codex", "finalwishes", "m5", "fw-r02-hotfix"}},
	}
	for _, c := range ok {
		got, err := Parse(c.in)
		if err != nil {
			t.Errorf("Parse(%q) unexpected error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("Parse(%q) = %+v, want %+v", c.in, got, c.want)
		}
		// round-trip: String(Parse(x)) == x
		if got.String() != c.in {
			t.Errorf("round-trip: %q.String() = %q", c.in, got.String())
		}
	}

	bad := []string{
		"",                        // empty
		"claude",                  // one slot
		"claude-finalwishes",      // two slots (no machine)
		"Claude-finalwishes-m1",   // uppercase agent
		"claude-Finalwishes-m1",   // uppercase project
		"claude-finalwishes-M1",   // uppercase machine
		"claude--m1",              // empty project slot
		"-finalwishes-m1",         // empty agent slot
		"claude-fin_wishes-m1",    // underscore in slot
		"claude-finalwishes-m1-",  // trailing hyphen (empty task)
		"claude-finalwishes-m1-A", // uppercase task
		"claude-finalwishes-m1-a_b",
	}
	for _, s := range bad {
		if _, err := Parse(s); err == nil {
			t.Errorf("Parse(%q) = nil error, want rejection", s)
		}
	}
}

func TestConstruct(t *testing.T) {
	n, err := Construct("claude", "finalwishes", "m1", "fw-r02")
	if err != nil {
		t.Fatalf("Construct valid: %v", err)
	}
	if n.String() != "claude-finalwishes-m1-fw-r02" {
		t.Errorf("Construct.String() = %q", n.String())
	}
	if _, err := Construct("claude", "final_wishes", "m1", ""); err == nil {
		t.Errorf("Construct with invalid project must error")
	}
	if _, err := Construct("claude", "finalwishes", "m1", "bad_task"); err == nil {
		t.Errorf("Construct with invalid task must error")
	}
}

func TestGleanFrom(t *testing.T) {
	ok := map[string]string{
		"M1.local":    "m1",
		"m5":          "m5",
		"M5.LOCAL":    "m5",
		"Foo.bar.baz": "foo",
		"host123":     "host123",
	}
	for host, want := range ok {
		got, err := gleanFrom(host)
		if err != nil {
			t.Errorf("gleanFrom(%q): %v", host, err)
			continue
		}
		if got != want {
			t.Errorf("gleanFrom(%q) = %q, want %q", host, got, want)
		}
	}
	for _, host := range []string{"", ".local", "has space", "under_score"} {
		if _, err := gleanFrom(host); err == nil {
			t.Errorf("gleanFrom(%q) = nil error, want rejection", host)
		}
	}
}
