package report

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestAppendLoadRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "conduit-report.json")
	yes := true
	if err := Append(p, Run{TS: "2026-07-23T13:00:00Z", Source: "supervisor", Outcome: OutcomeGreen, APIReachable: &yes}); err != nil {
		t.Fatal(err)
	}
	if err := Append(p, Run{TS: "2026-07-23T13:05:00Z", Source: "supervisor", Outcome: OutcomeHealed, Heals: []string{"local AI restarted (bounded)"}}); err != nil {
		t.Fatal(err)
	}
	f, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Runs) != 2 || f.Runs[0].Outcome != OutcomeHealed {
		t.Fatalf("newest-first violated: %+v", f.Runs)
	}
	if f.SchemaVersion != SchemaVersion {
		t.Errorf("schema version %q", f.SchemaVersion)
	}
}

func TestAppendTrimsToMaxRuns(t *testing.T) {
	p := filepath.Join(t.TempDir(), "r.json")
	for i := 0; i < MaxRuns+7; i++ {
		if err := Append(p, Run{TS: "2026-07-23T13:00:00Z", Source: "s", Outcome: OutcomeGreen}); err != nil {
			t.Fatal(err)
		}
	}
	f, _ := Load(p)
	if len(f.Runs) != MaxRuns {
		t.Fatalf("want %d runs, got %d", MaxRuns, len(f.Runs))
	}
}

func TestLoadMissingIsEmpty(t *testing.T) {
	f, err := Load(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil || len(f.Runs) != 0 {
		t.Fatalf("missing file should be empty report: %v %v", f, err)
	}
}

func TestSentence(t *testing.T) {
	no := false
	cases := []struct {
		r    Run
		want []string // substrings
	}{
		{Run{TS: "2026-07-23T13:00:00Z", Outcome: OutcomeGreen}, []string{"all green"}},
		{Run{TS: "2026-07-23T13:00:00Z", Outcome: OutcomeHealed, Heals: []string{"local AI restarted"}}, []string{"local AI restarted"}},
		{Run{TS: "2026-07-23T13:00:00Z", Outcome: OutcomeDegraded, Escalations: []string{"broker down"}}, []string{"needs attention", "broker down"}},
		{Run{TS: "2026-07-23T13:00:00Z", Outcome: OutcomeGreen, APIReachable: &no}, []string{"cloud unreachable", "local AI holding the fort"}},
	}
	for _, c := range cases {
		got := Sentence(c.r)
		for _, w := range c.want {
			if !strings.Contains(got, w) {
				t.Errorf("Sentence(%+v) = %q, missing %q", c.r, got, w)
			}
		}
	}
}
