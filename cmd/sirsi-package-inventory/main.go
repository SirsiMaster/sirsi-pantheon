// Command sirsi-package-inventory verifies a Pantheon.app payload without
// executing anything from the bundle. It is a bounded bridge for the
// descriptor-rooted internal/packageinventory verifier; signing, notarization,
// and the existing shell identity wrapper remain separate gates.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/SirsiMaster/sirsi-pantheon/internal/packageinventorycmd"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("sirsi-package-inventory", flag.ContinueOnError)
	flags.SetOutput(stderr)
	var app, version, build, infoPath, pkgInfoPath, launchAgentPath string
	var requireCodeSignature bool
	flags.StringVar(&app, "app", "", "Pantheon.app path")
	flags.StringVar(&version, "version", "", "expected CFBundleShortVersionString")
	flags.StringVar(&build, "build", "", "expected CFBundleVersion")
	flags.StringVar(&infoPath, "info-plist", "", "canonical Info.plist path")
	flags.StringVar(&pkgInfoPath, "pkg-info", "", "canonical PkgInfo path")
	flags.StringVar(&launchAgentPath, "launch-agent", "", "canonical LaunchAgent plist path")
	flags.BoolVar(&requireCodeSignature, "require-code-signature", false, "require the _CodeSignature payload")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(stderr, "sirsi-package-inventory: positional arguments are not accepted")
		return 2
	}
	if app == "" || version == "" || build == "" || infoPath == "" || pkgInfoPath == "" || launchAgentPath == "" {
		fmt.Fprintln(stderr, "sirsi-package-inventory: --app, --version, --build, --info-plist, --pkg-info, and --launch-agent are required")
		return 2
	}
	report, err := packageinventorycmd.Verify(packageinventorycmd.Inputs{
		App: app, Version: version, Build: build, InfoPlist: infoPath,
		PkgInfo: pkgInfoPath, LaunchAgent: launchAgentPath,
		RequireCodeSignature: requireCodeSignature,
	})
	if err != nil {
		return fail(stderr, "app", err)
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		fmt.Fprintf(stderr, "sirsi-package-inventory: write report: %v\n", err)
		return 1
	}
	return 0
}

func fail(stderr io.Writer, field string, err error) int {
	fmt.Fprintf(stderr, "sirsi-package-inventory: %s: %v\n", field, err)
	return 1
}
