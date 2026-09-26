package router

import "testing"

// ownsRecordHost is the security-relevant decision behind the fabric-wide
// registration fix (2026-09-26): SaveThreadRegistry must persist/prune only
// THIS host's records, never another host's — attempting a foreign write and
// then aborting the save on the service's (correct) ADR-067 403 stranded the
// caller's own just-committed registration. This narrows what we WRITE, never
// what the service ALLOWS.
func TestOwnsRecordHost(t *testing.T) {
	const self = "M1.local"
	cases := []struct {
		name       string
		recordHost string
		selfHost   string
		own        bool
	}{
		{"own host matches", "M1.local", self, true},
		{"empty record host is ours (service stamps it)", "", self, true},
		{"another host is NOT ours — the confirmed 403 case", "Mac", self, false},
		{"another host, different name", "M5.local", self, false},
		{"empty self host stays permissive (never over-skip our own)", "M1.local", "", true},
		{"both empty", "", "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ownsRecordHost(c.recordHost, c.selfHost); got != c.own {
				t.Fatalf("ownsRecordHost(%q, %q) = %v, want %v", c.recordHost, c.selfHost, got, c.own)
			}
		})
	}
}
