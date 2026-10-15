package main

import (
	"reflect"
	"testing"
)

func TestParseApolloTokenIDs(t *testing.T) {
	got, err := parseApolloTokenIDs("2, 105,0")
	if err != nil {
		t.Fatal(err)
	}
	if want := []uint32{2, 105, 0}; !reflect.DeepEqual(got, want) {
		t.Fatalf("tokens = %v, want %v", got, want)
	}
	for _, raw := range []string{"", "2,,3", "-1", "4294967296", "word"} {
		if _, err := parseApolloTokenIDs(raw); err == nil {
			t.Fatalf("parseApolloTokenIDs(%q) succeeded", raw)
		}
	}
}

func TestApolloClusterCLIRefusesUnenrolledTransport(t *testing.T) {
	old := apolloClusterEndpoint
	t.Cleanup(func() { apolloClusterEndpoint = old })
	apolloClusterEndpoint = "https://foreign.invalid"
	if client, err := configuredApolloClusterClient(); err == nil || client != nil {
		t.Fatalf("unenrolled client escaped: client=%v err=%v", client, err)
	}
}
