//go:build !darwin

package maat

import "testing"

func TestInstalledLifecycleAuthorityRefusesUnsupportedPlatform(t *testing.T) {
	authority, err := VerifyCurrentInstalledLifecycleAuthority()
	if err == nil || authority.Valid() {
		t.Fatalf("unsupported platform received installed authority: valid=%v err=%v", authority.Valid(), err)
	}
}
