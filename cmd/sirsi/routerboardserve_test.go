package main

import "testing"

func TestTailnetMacConsolesListsOnlyOnlineTaggedMacs(t *testing.T) {
	status := []byte(`{"Peer":{
		"a":{"DNSName":"m1.tail.ts.net.","Online":true,"Tags":["tag:sirsi-mac"]},
		"b":{"DNSName":"m5-studio.tail.ts.net.","Online":true,"Tags":["tag:other","tag:sirsi-mac"]},
		"c":{"DNSName":"m5.tail.ts.net.","Online":false,"Tags":["tag:sirsi-mac"]},
		"d":{"DNSName":"iphone.tail.ts.net.","Online":true},
		"e":{"DNSName":"","Online":true,"Tags":["tag:sirsi-mac"]}}}`)
	got, err := tailnetMacConsoles(status)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"m1": "https://m1.tail.ts.net", "m5-studio": "https://m5-studio.tail.ts.net"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
	if _, err := tailnetMacConsoles([]byte("not json")); err == nil {
		t.Fatal("bad status parsed without error")
	}
}
