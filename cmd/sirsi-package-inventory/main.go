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
	var app, version, build, infoPath, pkgInfoPath, launchAgentPath, brandLogoPath, apolloCapability, apolloProfilePath, expectedInfoSHA256, expectedProfileSHA256, frozenDigestOutput string
	var requireCodeSignature, requireFrozenDigests bool
	flags.StringVar(&app, "app", "", "Pantheon.app path")
	flags.StringVar(&version, "version", "", "expected CFBundleShortVersionString")
	flags.StringVar(&build, "build", "", "expected CFBundleVersion")
	flags.StringVar(&infoPath, "info-plist", "", "canonical Info.plist path")
	flags.StringVar(&pkgInfoPath, "pkg-info", "", "canonical PkgInfo path")
	flags.StringVar(&launchAgentPath, "launch-agent", "", "canonical LaunchAgent plist path")
	flags.StringVar(&brandLogoPath, "brand-logo", "", "canonical Sirsi application-mark PNG path")
	flags.StringVar(&apolloCapability, "apollo-capability", "", "required independent Apollo native capability expectation")
	flags.StringVar(&apolloProfilePath, "apollo-profile", "", "exact protected Apollo native profile input")
	flags.StringVar(&expectedInfoSHA256, "expected-info-sha256", "", "frozen pre-sign Info.plist digest")
	flags.StringVar(&expectedProfileSHA256, "expected-apollo-profile-sha256", "", "frozen pre-sign Apollo profile digest or absent")
	flags.StringVar(&frozenDigestOutput, "write-frozen-digests", "", "write the validated digest pair to a new private create-only file")
	flags.BoolVar(&requireCodeSignature, "require-code-signature", false, "require the _CodeSignature payload")
	flags.BoolVar(&requireFrozenDigests, "require-frozen-digests", false, "require the pre-sign digest pair for release verification")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(stderr, "sirsi-package-inventory: positional arguments are not accepted")
		return 2
	}
	if app == "" || version == "" || build == "" || infoPath == "" || pkgInfoPath == "" || launchAgentPath == "" || brandLogoPath == "" {
		fmt.Fprintln(stderr, "sirsi-package-inventory: --app, --version, --build, --info-plist, --pkg-info, --launch-agent, and --brand-logo are required")
		return 2
	}
	report, digests, err := packageinventorycmd.VerifyWithFrozenDigests(packageinventorycmd.Inputs{
		App: app, Version: version, Build: build, InfoPlist: infoPath,
		PkgInfo: pkgInfoPath, LaunchAgent: launchAgentPath, BrandLogo: brandLogoPath,
		ApolloCapability: apolloCapability, ApolloProfile: apolloProfilePath,
		ExpectedInfoSHA256: expectedInfoSHA256, ExpectedProfileSHA256: expectedProfileSHA256,
		RequireCodeSignature: requireCodeSignature,
		RequireFrozenDigests: requireFrozenDigests,
	})
	if err != nil {
		return fail(stderr, "app", err)
	}
	if frozenDigestOutput != "" {
		if err := packageinventorycmd.WriteFrozenDigests(frozenDigestOutput, digests); err != nil {
			return fail(stderr, "frozen digests", err)
		}
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
