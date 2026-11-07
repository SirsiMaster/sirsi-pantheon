package apollo

import (
	"bufio"
	"os"
	"strings"
	"testing"
)

// The documented 2.2.x route ID stays accepted as input and is always sent as the current route.
func TestInstalledAppLegacyRouteNormalized(t *testing.T) {
	for _, model := range []string{installedAppLegacyRoute, InstalledAppRoute, InstalledAppModel, "qwen3.5-9b"} {
		in := appRequestFixture()
		in.Model = model
		wire, err := appWireRequest(in)
		if err != nil {
			t.Fatalf("%s refused: %v", model, err)
		}
		if wire.Model != InstalledAppRoute {
			t.Fatalf("%s sent as %q, want %q", model, wire.Model, InstalledAppRoute)
		}
	}
	if IsInstalledAppModel(InstalledAppModel + "@apollo:2.1.0") {
		t.Fatal("an undocumented route ID must not select the installed app")
	}
}

// Release-contract control: every pinned artifact hash must equal the contents manifest published with the
// GitHub release apollo-v2.3.0 (testdata is the read-back copy of Apollo-2.3.0-contents.sha256, sha256 23611a31…,
// itself listed in that release's SHA256SUMS). This is sourced independently of installedAppFiles.
func TestInstalledAppFilesMatchPublishedContents(t *testing.T) {
	f, err := os.Open("testdata/Apollo-2.3.0-contents.sha256")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	published := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 2 || strings.HasPrefix(fields[0], "#") {
			continue
		}
		published[strings.TrimPrefix(fields[1], "Contents/")] = fields[0]
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	for path, want := range installedAppFiles {
		if got, ok := published[path]; !ok || got != want {
			t.Fatalf("%s pinned %s, published manifest has %q", path, want, got)
		}
	}
}
