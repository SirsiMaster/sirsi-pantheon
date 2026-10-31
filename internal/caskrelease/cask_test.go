package caskrelease

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testSHA = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestRenderAndVerifyCanonicalCask(t *testing.T) {
	in := Input{Version: "0.24.154-beta.1", DMGSHA256: testSHA}
	bytes, err := Render(in)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(bytes), `version "0.24.154-beta.1"`) || !strings.Contains(string(bytes), `sha256 "`+testSHA+`"`) {
		t.Fatalf("rendered cask did not bind release tuple:\n%s", bytes)
	}
	for _, required := range []string{`depends_on arch: :arm64`, `depends_on macos: :ventura`} {
		if !strings.Contains(string(bytes), required) {
			t.Errorf("rendered cask missing platform eligibility %q:\n%s", required, bytes)
		}
	}
	if !strings.Contains(string(bytes), `binary "#{appdir}/Pantheon.app/Contents/MacOS/sirsi", target: "sirsi"`) {
		t.Fatalf("rendered cask does not expose the CLI from the packaged Pantheon.app:\n%s", bytes)
	}
	if err := Verify(bytes, in); err != nil {
		t.Fatalf("Verify() rejected canonical bytes: %v", err)
	}
}

func TestVerifyRejectsAnyCaskDrift(t *testing.T) {
	in := Input{Version: "0.24.154", DMGSHA256: testSHA}
	bytes, err := Render(in)
	if err != nil {
		t.Fatal(err)
	}
	for _, drift := range [][]byte{
		append(append([]byte(nil), bytes...), []byte("# trailing code\n")...),
		[]byte(strings.Replace(string(bytes), `version "0.24.154"`, `version "9.9.9"`, 1)),
		[]byte(strings.Replace(string(bytes), `SirsiPantheon-#{version}-arm64.dmg`, `other.dmg`, 1)),
		[]byte(strings.Replace(string(bytes), "  depends_on arch: :arm64\n", "", 1)),
		[]byte(strings.Replace(string(bytes), "  depends_on macos: :ventura\n", "", 1)),
		[]byte(strings.Replace(string(bytes), "  depends_on macos: :ventura\n", "  depends_on macos: :monterey\n", 1)),
	} {
		if err := Verify(drift, in); err == nil {
			t.Fatal("Verify() accepted drifted cask bytes")
		}
	}
}

func TestHistoricalCaskBytesRemainCanonical(t *testing.T) {
	legacy, err := os.ReadFile(filepath.Join("testdata", "legacy-v0.24.153.rb"))
	if err != nil {
		t.Fatal(err)
	}
	for _, version := range []string{"0.24.152", "0.24.153", "0.24.153-rc.1"} {
		t.Run(version, func(t *testing.T) {
			expected := []byte(strings.Replace(string(legacy), `version "0.24.153"`, `version "`+version+`"`, 1))
			in := Input{Version: version, DMGSHA256: testSHA}
			actual, err := Render(in)
			if err != nil {
				t.Fatal(err)
			}
			if string(actual) != string(expected) {
				t.Fatalf("historical cask bytes changed for %s", version)
			}
			if err := Verify(expected, in); err != nil {
				t.Fatalf("Verify() rejected historical canonical bytes: %v", err)
			}
		})
	}
}

func TestCaskEligibilityCutoverIsOneCanonicalTemplate(t *testing.T) {
	for _, version := range []string{"0.24.154", "0.24.154-rc.1", "0.25.0"} {
		t.Run(version, func(t *testing.T) {
			in := Input{Version: version, DMGSHA256: testSHA}
			guarded, err := Render(in)
			if err != nil {
				t.Fatal(err)
			}
			for _, required := range []string{"  depends_on arch: :arm64\n", "  depends_on macos: :ventura\n"} {
				if !strings.Contains(string(guarded), required) {
					t.Fatalf("guarded canonical Cask missing %q", required)
				}
			}
			if err := Verify(guarded, in); err != nil {
				t.Fatalf("Verify() rejected guarded canonical bytes: %v", err)
			}
		})
	}

	guarded153 := []byte(strings.Replace(string(mustRender(t, Input{Version: "0.24.154", DMGSHA256: testSHA})), `version "0.24.154"`, `version "0.24.153"`, 1))
	if err := Verify(guarded153, Input{Version: "0.24.153", DMGSHA256: testSHA}); err == nil {
		t.Fatal("Verify() accepted the new guarded template for historical v0.24.153")
	}
}

func mustRender(t *testing.T, in Input) []byte {
	t.Helper()
	got, err := Render(in)
	if err != nil {
		t.Fatal(err)
	}
	return got
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
