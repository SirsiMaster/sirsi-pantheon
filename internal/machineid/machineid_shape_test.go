package machineid

import "testing"

func TestLooksLikeMachineID(t *testing.T) {
	cases := map[string]bool{
		"6BA7B810-9DAD-11D1-80B4-00C04FD430C8": true, // darwin IOPlatformUUID
		"6ba7b8109dad11d180b400c04fd430c8":     true, // linux /etc/machine-id (32 bare hex)
		"":                                     false,
		"Mac":                                  false,
		"M1.local":                             false,
		"MacBook-Pro-2.local":                  false,
		"m1":                                   false,
		"m5":                                   false,
	}
	for in, want := range cases {
		if got := LooksLikeMachineID(in); got != want {
			t.Errorf("LooksLikeMachineID(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestInTransition(t *testing.T) {
	const uuid1 = "6BA7B810-9DAD-11D1-80B4-00C04FD430C8"
	const uuid2 = "6BA7B811-9DAD-11D1-80B4-00C04FD430C8"
	cases := []struct {
		a, b string
		want bool
	}{
		{"Mac", uuid1, true},  // legacy <-> new: the exact migration bridge
		{uuid1, "Mac", true},  // symmetric
		{"m1", "m5", false},   // two DIFFERENT legacy hostnames: NOT a transition — must stay refusable
		{uuid1, uuid2, false}, // two DIFFERENT machine ids: NOT a transition — must stay refusable
		{"Mac", "Mac", false}, // identical values: irrelevant, InTransition is only consulted on mismatch
		{uuid1, uuid1, false},
	}
	for _, c := range cases {
		if got := InTransition(c.a, c.b); got != c.want {
			t.Errorf("InTransition(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}
