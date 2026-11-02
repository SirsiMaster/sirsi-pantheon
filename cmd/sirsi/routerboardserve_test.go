package main

import (
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
)

func TestMacConsoleURLsFromTheMachineRegistry(t *testing.T) {
	got, err := macConsoleURLs([]router.Machine{{ID: "M1"}, {ID: "M5"}, {ID: "M5-Studio"}}, "tail.ts.net")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"M1": "https://m1.tail.ts.net", "M5": "https://m5.tail.ts.net", "M5-Studio": "https://m5-studio.tail.ts.net"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestMacConsoleURLsFailClosedOnCollisionsAndBadNames(t *testing.T) {
	for _, tc := range []struct {
		ids  []string
		want string
	}{
		{[]string{"M5", "m5"}, "both map to m5.tail.ts.net"},
		{[]string{"M1", "M1 Pro"}, "not a valid tailnet name"},
		{[]string{"m1.evil.com"}, "not a valid tailnet name"},
	} {
		var ms []router.Machine
		for _, id := range tc.ids {
			ms = append(ms, router.Machine{ID: id})
		}
		if got, err := macConsoleURLs(ms, "tail.ts.net"); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%v: got %v, %v; want error %q", tc.ids, got, err, tc.want)
		}
	}
}
