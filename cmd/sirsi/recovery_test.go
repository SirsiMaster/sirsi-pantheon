package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
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

func TestRecoveryPublicKeysRejectMalformedAndAcceptExactEd25519(t *testing.T) {
	if _, err := recoveryPublicKeys(map[string]string{"issuer": "not-base64"}); err == nil {
		t.Fatal("accepted malformed recovery public key")
	}
	public, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	keys, err := recoveryPublicKeys(map[string]string{"issuer": base64.RawURLEncoding.EncodeToString(public)})
	if err != nil || string(keys["issuer"]) != string(public) {
		t.Fatalf("recovery public key parse = %#v, %v", keys, err)
	}
}
