package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRecoveryConfigIsClosedAndLoopbackOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recovery.json")
	if err := os.WriteFile(path, []byte(`{"nodes":[],"allowed_tailnet_logins":["owner@example.test"],"unknown":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadRecoveryConfig(path); err == nil {
		t.Fatal("accepted an unknown recovery configuration field")
	}
	if !isLoopbackListen("127.0.0.1:9188") || !isLoopbackListen("[::1]:9188") {
		t.Fatal("rejected a loopback recovery listener")
	}
	for _, address := range []string{":9188", "0.0.0.0:9188", "100.88.242.95:9188", "m5.example.ts.net:9188"} {
		if isLoopbackListen(address) {
			t.Fatalf("accepted non-loopback recovery listener %q", address)
		}
	}
}
