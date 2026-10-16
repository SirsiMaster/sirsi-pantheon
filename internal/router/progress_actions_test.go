package router

import (
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/work"
)

// A run that closes, acknowledges or claims a lease made progress even if inbox depth is unchanged;
// mail arriving is not the run's work and must not count.
func TestConsumerRunObserveCountsOnlyDurableActions(t *testing.T) {
	a, b := work.Item{ID: "a"}, work.Item{ID: "b"}
	cases := []struct {
		name   string
		before []work.Item
		bl     int
		after  []work.Item
		al     int
		acted  bool
	}{
		{"nothing changed", []work.Item{a}, 0, []work.Item{a}, 0, false},
		{"arrival only", []work.Item{a}, 0, []work.Item{a, b}, 0, false},
		{"close", []work.Item{a, b}, 0, []work.Item{a}, 0, true},
		{"acknowledge", []work.Item{a}, 0, []work.Item{{ID: "a", AckedAt: "t"}}, 0, true},
		{"lease claimed", []work.Item{a}, 0, []work.Item{a}, 2, true},
		{"lease expired", []work.Item{a}, 2, []work.Item{a}, 0, false},
		{"close masked by arrival", []work.Item{a}, 0, []work.Item{b}, 0, true},
	}
	for _, c := range cases {
		r := &consumerRun{}
		r.observe(c.before, TaskLoad{Leased: c.bl})
		if r.acted {
			t.Fatalf("%s: first snapshot set acted", c.name)
		}
		r.observe(c.after, TaskLoad{Leased: c.al})
		if r.acted != c.acted {
			t.Errorf("%s: acted = %v, want %v", c.name, r.acted, c.acted)
		}
	}
}
