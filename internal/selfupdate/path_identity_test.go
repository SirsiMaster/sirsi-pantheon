package selfupdate

import (
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/version"
)

func TestBuildReportAcceptsMatchingReleaseAtDifferentPath(t *testing.T) {
	self := version.Info{Binary: "sirsi", Version: "v0.23.9-beta", Commit: "abc123", Path: "/Applications/Pantheon.app/Contents/MacOS/sirsi"}
	path := "/Users/test/.local/bin/sirsi"
	siblings := []Sibling{{Info: version.Info{Binary: "sirsi", Version: self.Version, Commit: self.Commit, Path: path}}}

	report := BuildReport(self, siblings, path)
	if !report.Healthy || report.D3PathBin != "" {
		t.Fatalf("matching stamped PATH copy must be healthy: %#v", report)
	}
}

func TestBuildReportRejectsDifferentReleaseAtDifferentPath(t *testing.T) {
	self := version.Info{Binary: "sirsi", Version: "v0.23.9-beta", Commit: "abc123", Path: "/Applications/Pantheon.app/Contents/MacOS/sirsi"}
	path := "/Users/test/.local/bin/sirsi"
	siblings := []Sibling{{Info: version.Info{Binary: "sirsi", Version: "v0.23.8-beta", Commit: "def456", Path: path}}}

	report := BuildReport(self, siblings, path)
	if report.Healthy || report.D3PathBin != path {
		t.Fatalf("different PATH release must remain drift: %#v", report)
	}
}
