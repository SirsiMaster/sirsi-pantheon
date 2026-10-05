// Package packageinventorycmd provides the one CLI-facing adapter for the
// descriptor-rooted Pantheon payload inventory authority.
package packageinventorycmd

import (
	"encoding/xml"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/SirsiMaster/sirsi-pantheon/internal/packageinventory"
	"golang.org/x/sys/unix"
)

const maxCanonicalInput = 32 << 20

type Inputs struct {
	App                  string
	Version              string
	Build                string
	InfoPlist            string
	PkgInfo              string
	LaunchAgent          string
	BrandLogo            string
	RequireCodeSignature bool
}

func Verify(inputs Inputs) (packageinventory.Report, error) {
	if inputs.App == "" || inputs.Version == "" || inputs.Build == "" || inputs.InfoPlist == "" || inputs.PkgInfo == "" || inputs.LaunchAgent == "" || inputs.BrandLogo == "" {
		return packageinventory.Report{}, errors.New("app, version, build, info-plist, pkg-info, launch-agent, and brand-logo are required")
	}
	info, infoErr := ReadCanonicalFile(inputs.InfoPlist)
	if infoErr != nil {
		return packageinventory.Report{}, infoErr
	}
	if validateErr := validateInfoPlist(info, inputs.Version, inputs.Build); validateErr != nil {
		return packageinventory.Report{}, validateErr
	}
	pkgInfo, pkgInfoErr := ReadCanonicalFile(inputs.PkgInfo)
	if pkgInfoErr != nil {
		return packageinventory.Report{}, pkgInfoErr
	}
	launchAgent, launchAgentErr := ReadCanonicalFile(inputs.LaunchAgent)
	if launchAgentErr != nil {
		return packageinventory.Report{}, launchAgentErr
	}
	brandLogo, brandLogoErr := ReadCanonicalFile(inputs.BrandLogo)
	if brandLogoErr != nil {
		return packageinventory.Report{}, brandLogoErr
	}
	return packageinventory.Verify(inputs.App, packageinventory.Expectations{
		Version: inputs.Version, Build: inputs.Build, InfoPlist: info,
		PkgInfo: pkgInfo, LaunchAgent: launchAgent, BrandLogo: brandLogo,
		RequireCodeSignature: inputs.RequireCodeSignature,
	})
}

func ReadCanonicalFile(path string) ([]byte, error) {
	if path == "" {
		return nil, errors.New("path is required")
	}
	parentFD, leaf, openErr := openParent(path)
	if openErr != nil {
		return nil, openErr
	}
	defer unix.Close(parentFD)
	var parentBefore unix.Stat_t
	if parentStatErr := unix.Fstat(parentFD, &parentBefore); parentStatErr != nil {
		return nil, parentStatErr
	}
	fd, fileOpenErr := unix.Openat(parentFD, leaf, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if fileOpenErr != nil {
		return nil, fileOpenErr
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		unix.Close(fd)
		return nil, errors.New("cannot retain descriptor")
	}
	defer file.Close()
	var before unix.Stat_t
	if fileStatErr := unix.Fstat(fd, &before); fileStatErr != nil {
		return nil, fileStatErr
	}
	if before.Mode&unix.S_IFMT != unix.S_IFREG || before.Nlink != 1 || before.Size < 0 || before.Size > maxCanonicalInput {
		return nil, errors.New("expected a regular nlink=1 file within the size limit")
	}
	data, readErr := io.ReadAll(io.LimitReader(file, maxCanonicalInput+1))
	if readErr != nil {
		return nil, readErr
	}
	if int64(len(data)) != before.Size {
		return nil, errors.New("file changed size during read")
	}
	var after unix.Stat_t
	if afterStatErr := unix.Fstat(fd, &after); afterStatErr != nil {
		return nil, afterStatErr
	}
	if !sameIdentity(before, after) {
		return nil, errors.New("file identity changed during read")
	}
	var nameAfter unix.Stat_t
	if nameStatErr := unix.Fstatat(parentFD, leaf, &nameAfter, unix.AT_SYMLINK_NOFOLLOW); nameStatErr != nil || !sameIdentity(before, nameAfter) {
		return nil, errors.New("file name identity changed during read")
	}
	var parentAfter unix.Stat_t
	if parentAfterErr := unix.Fstat(parentFD, &parentAfter); parentAfterErr != nil || !sameIdentity(parentBefore, parentAfter) {
		return nil, errors.New("file parent identity changed during read")
	}
	return data, nil
}

func openParent(path string) (int, string, error) {
	if !filepath.IsAbs(path) {
		return -1, "", errors.New("canonical input path must be absolute")
	}
	clean := filepath.Clean(path)
	parts := strings.Split(strings.TrimPrefix(clean, string(filepath.Separator)), string(filepath.Separator))
	if len(parts) < 2 || parts[0] == "" {
		return -1, "", errors.New("canonical input path must name a file below a directory")
	}
	fd, err := unix.Open(string(filepath.Separator), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, "", err
	}
	for _, component := range parts[:len(parts)-1] {
		if component == "" || component == "." || component == ".." {
			unix.Close(fd)
			return -1, "", errors.New("canonical input path contains an unsafe component")
		}
		next, err := unix.Openat(fd, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if err != nil {
			unix.Close(fd)
			return -1, "", err
		}
		unix.Close(fd)
		fd = next
	}
	leaf := parts[len(parts)-1]
	if leaf == "" || leaf == "." || leaf == ".." {
		unix.Close(fd)
		return -1, "", errors.New("canonical input path has an unsafe leaf")
	}
	return fd, leaf, nil
}

func validateInfoPlist(data []byte, version, build string) error {
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	values := make(map[string]string)
	seen := make(map[string]struct{})
	required := map[string]struct{}{
		"CFBundleIdentifier":         {},
		"CFBundleShortVersionString": {},
		"CFBundleVersion":            {},
	}
	var key string
	for {
		token, tokenErr := decoder.Token()
		if tokenErr == io.EOF {
			break
		}
		if tokenErr != nil {
			return errors.New("Info.plist is not valid XML")
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "key" {
			continue
		}
		if keyErr := decoder.DecodeElement(&key, &start); keyErr != nil || key == "" {
			return errors.New("Info.plist contains an invalid key")
		}
		if _, exists := seen[key]; exists {
			return errors.New("Info.plist contains duplicate keys")
		}
		seen[key] = struct{}{}
		valueStart, valueStartErr := nextPlistValueStart(decoder)
		if valueStartErr != nil {
			return valueStartErr
		}
		if valueStart.Name.Local != "string" {
			if _, needed := required[key]; needed {
				return errors.New("Info.plist required identity value is not a string")
			}
			if skipErr := decoder.Skip(); skipErr != nil {
				return errors.New("Info.plist contains an invalid typed value")
			}
			continue
		}
		var value string
		if valueErr := decoder.DecodeElement(&value, &valueStart); valueErr != nil {
			return errors.New("Info.plist contains an invalid string")
		}
		values[key] = value
	}
	if values["CFBundleIdentifier"] != "ai.sirsi.pantheon" {
		return errors.New("Info.plist bundle identifier mismatch")
	}
	if values["CFBundleShortVersionString"] != version {
		return errors.New("Info.plist version does not match --version")
	}
	if values["CFBundleVersion"] != build {
		return errors.New("Info.plist build does not match --build")
	}
	return nil
}

// nextPlistValueStart permits XML formatting whitespace between a plist key
// and its value but refuses any other unstructured data. macOS writes
// human-readable plists this way; accepting arbitrary character data here
// would make a malformed dictionary look canonical.
func nextPlistValueStart(decoder *xml.Decoder) (xml.StartElement, error) {
	for {
		token, tokenErr := decoder.Token()
		if tokenErr != nil {
			return xml.StartElement{}, errors.New("Info.plist key has no value")
		}
		switch value := token.(type) {
		case xml.CharData:
			if strings.TrimSpace(string(value)) == "" {
				continue
			}
			return xml.StartElement{}, errors.New("Info.plist key has an invalid value")
		case xml.StartElement:
			return value, nil
		default:
			return xml.StartElement{}, errors.New("Info.plist key has an invalid value")
		}
	}
}

func sameIdentity(a, b unix.Stat_t) bool {
	return a.Dev == b.Dev && a.Ino == b.Ino && a.Mode == b.Mode && a.Nlink == b.Nlink && a.Size == b.Size
}
