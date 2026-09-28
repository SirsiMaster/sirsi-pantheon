package caskrelease

import (
	"strings"
	"testing"
)

const testSHA = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestRenderAndVerifyCanonicalCask(t *testing.T) {
	in := Input{Version: "0.24.14-beta.1", DMGSHA256: testSHA}
	bytes, err := Render(in)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(bytes), `version "0.24.14-beta.1"`) || !strings.Contains(string(bytes), `sha256 "`+testSHA+`"`) {
		t.Fatalf("rendered cask did not bind release tuple:\n%s", bytes)
	}
	if !strings.Contains(string(bytes), `binary "#{appdir}/Pantheon.app/Contents/MacOS/sirsi", target: "sirsi"`) {
		t.Fatalf("rendered cask does not expose the CLI from the packaged Pantheon.app:\n%s", bytes)
	}
	if err := Verify(bytes, in); err != nil {
		t.Fatalf("Verify() rejected canonical bytes: %v", err)
	}
}

func TestVerifyRejectsAnyCaskDrift(t *testing.T) {
	in := Input{Version: "0.24.14", DMGSHA256: testSHA}
	bytes, err := Render(in)
	if err != nil {
		t.Fatal(err)
	}
	for _, drift := range [][]byte{
		append(append([]byte(nil), bytes...), []byte("# trailing code\n")...),
		[]byte(strings.Replace(string(bytes), `version "0.24.14"`, `version "9.9.9"`, 1)),
		[]byte(strings.Replace(string(bytes), `SirsiPantheon-#{version}-arm64.dmg`, `other.dmg`, 1)),
	} {
		if err := Verify(drift, in); err == nil {
			t.Fatal("Verify() accepted drifted cask bytes")
		}
	}
}

func TestRejectsAmbiguousReleaseInputs(t *testing.T) {
	for _, in := range []Input{
		{Version: "v0.24.14", DMGSHA256: testSHA},
		{Version: "0.24", DMGSHA256: testSHA},
		{Version: "0.24.14", DMGSHA256: strings.ToUpper(testSHA)},
		{Version: "0.24.14", DMGSHA256: testSHA[:63]},
	} {
		if _, err := Render(in); err == nil {
			t.Fatalf("Render(%+v) succeeded", in)
		}
	}
}
