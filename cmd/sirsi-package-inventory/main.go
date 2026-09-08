// Command sirsi-package-inventory verifies a Pantheon.app payload without
// executing anything from the bundle. It is a bounded bridge for the
// descriptor-rooted internal/packageinventory verifier; signing, notarization,
// and the existing shell identity wrapper remain separate gates.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/SirsiMaster/sirsi-pantheon/internal/packageinventory"
	"golang.org/x/sys/unix"
)

const maxCanonicalInput = 32 << 20

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
	info, err := readCanonicalFile(infoPath)
	if err != nil {
		return fail(stderr, "info-plist", err)
	}
	pkgInfo, err := readCanonicalFile(pkgInfoPath)
	if err != nil {
		return fail(stderr, "pkg-info", err)
	}
	launchAgent, err := readCanonicalFile(launchAgentPath)
	if err != nil {
		return fail(stderr, "launch-agent", err)
	}
	report, err := packageinventory.Verify(app, packageinventory.Expectations{
		Version: version, Build: build, InfoPlist: info, PkgInfo: pkgInfo,
		LaunchAgent: launchAgent, RequireCodeSignature: requireCodeSignature,
	})
	if err != nil {
		return fail(stderr, "app", err)
	}
	encoder := jsonEncoder(stdout)
	if err := encoder.Encode(report); err != nil {
		fmt.Fprintf(stderr, "sirsi-package-inventory: write report: %v\n", err)
		return 1
	}
	return 0
}

type reportEncoder interface {
	Encode(any) error
}

func jsonEncoder(w io.Writer) reportEncoder {
	return newJSONEncoder(w)
}

func fail(stderr io.Writer, field string, err error) int {
	fmt.Fprintf(stderr, "sirsi-package-inventory: %s: %v\n", field, err)
	return 1
}

func readCanonicalFile(path string) ([]byte, error) {
	if path == "" {
		return nil, errors.New("path is required")
	}
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		unix.Close(fd)
		return nil, errors.New("cannot retain descriptor")
	}
	defer file.Close()
	var before unix.Stat_t
	if err := unix.Fstat(fd, &before); err != nil {
		return nil, err
	}
	if before.Mode&unix.S_IFMT != unix.S_IFREG || before.Nlink != 1 || before.Size < 0 || before.Size > maxCanonicalInput {
		return nil, errors.New("expected a regular nlink=1 file within the size limit")
	}
	data, err := io.ReadAll(io.LimitReader(file, maxCanonicalInput+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) != before.Size {
		return nil, errors.New("file changed size during read")
	}
	var after unix.Stat_t
	if err := unix.Fstat(fd, &after); err != nil {
		return nil, err
	}
	if !sameSourceIdentity(before, after) {
		return nil, errors.New("file identity changed during read")
	}
	return data, nil
}

func sameSourceIdentity(a, b unix.Stat_t) bool {
	return a.Dev == b.Dev && a.Ino == b.Ino && a.Mode == b.Mode && a.Nlink == b.Nlink && a.Size == b.Size
}
