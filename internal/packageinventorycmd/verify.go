// Package packageinventorycmd provides the one CLI-facing adapter for the
// descriptor-rooted Pantheon payload inventory authority.
package packageinventorycmd

import (
	"encoding/xml"
	"errors"
	"fmt"
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
	ApolloCapability     string
	ApolloProfile        string
	RequireCodeSignature bool
}

func Verify(inputs Inputs) (packageinventory.Report, error) {
	if inputs.App == "" || inputs.Version == "" || inputs.Build == "" || inputs.InfoPlist == "" || inputs.PkgInfo == "" || inputs.LaunchAgent == "" || inputs.BrandLogo == "" {
		return packageinventory.Report{}, errors.New("app, version, build, info-plist, pkg-info, launch-agent, and brand-logo are required")
	}
	if inputs.ApolloCapability == "" {
		return packageinventory.Report{}, errors.New("explicit --apollo-capability is required")
	}
	info, infoErr := ReadCanonicalFile(inputs.InfoPlist)
	if infoErr != nil {
		return packageinventory.Report{}, infoErr
	}
	var apolloProfile []byte
	if inputs.ApolloProfile != "" {
		if !filepath.IsAbs(inputs.ApolloProfile) || filepath.Clean(inputs.ApolloProfile) != inputs.ApolloProfile {
			return packageinventory.Report{}, errors.New("Apollo profile requires a canonical absolute path")
		}
		var err error
		apolloProfile, err = ReadCanonicalFile(inputs.ApolloProfile)
		if err != nil {
			return packageinventory.Report{}, fmt.Errorf("read protected Apollo profile: %w", err)
		}
		if len(apolloProfile) == 0 || len(apolloProfile) > 64<<10 {
			return packageinventory.Report{}, errors.New("Apollo profile exceeds bounded canonical input")
		}
	}
	if validateErr := validateInfoPlist(info, inputs.Version, inputs.Build, inputs.ApolloCapability, len(apolloProfile) != 0); validateErr != nil {
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
		ApolloCapability: inputs.ApolloCapability, ApolloProfile: apolloProfile,
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

func validateInfoPlist(data []byte, version, build, expectedApolloCapability string, profilePresent bool) error {
	values, err := infoPlistStringValues(data)
	if err != nil {
		return err
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
	actualCapability, declared := values[packageinventory.ApolloCapabilityInfoKey]
	switch expectedApolloCapability {
	case packageinventory.ApolloCapabilityEnabled:
		if !declared || actualCapability != packageinventory.ApolloCapabilityEnabled || !profilePresent {
			return errors.New("Info.plist Apollo capability/profile mismatch for enabled package")
		}
	case packageinventory.ApolloCapabilityDisabled:
		if !declared || actualCapability != packageinventory.ApolloCapabilityDisabled || profilePresent {
			return errors.New("Info.plist Apollo capability/profile mismatch for disabled package")
		}
	case packageinventory.ApolloCapabilityLegacyDevelopment:
		if declared && ((actualCapability != packageinventory.ApolloCapabilityEnabled && actualCapability != packageinventory.ApolloCapabilityDisabled) ||
			(actualCapability == packageinventory.ApolloCapabilityEnabled) != profilePresent) {
			return errors.New("legacy development Info.plist Apollo capability/profile mismatch")
		}
		if !declared && !profilePresent {
			return nil
		}
		if !declared && profilePresent {
			// Historical local builds could include the profile before the signed
			// capability key existed. Keep that development-only layout readable.
			return nil
		}
	default:
		return errors.New("expected Apollo capability must be enabled, disabled, or legacy-development")
	}
	return nil
}

// ApolloCapabilityFromInfoPlist returns only an explicit signed capability.
// Missing metadata remains unavailable; it is never interpreted as disabled.
func ApolloCapabilityFromInfoPlist(data []byte) (string, error) {
	values, err := infoPlistStringValues(data)
	if err != nil {
		return "", err
	}
	if values["CFBundleIdentifier"] != "ai.sirsi.pantheon" {
		return "", errors.New("installed Apollo bundle identity is unavailable or malformed")
	}
	capability, ok := values[packageinventory.ApolloCapabilityInfoKey]
	if !ok || (capability != packageinventory.ApolloCapabilityEnabled && capability != packageinventory.ApolloCapabilityDisabled) {
		return "", errors.New("installed Apollo capability metadata is unavailable or malformed")
	}
	return capability, nil
}

func infoPlistStringValues(data []byte) (map[string]string, error) {
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
			return nil, errors.New("Info.plist is not valid XML")
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "key" {
			continue
		}
		if keyErr := decoder.DecodeElement(&key, &start); keyErr != nil || key == "" {
			return nil, errors.New("Info.plist contains an invalid key")
		}
		if _, exists := seen[key]; exists {
			return nil, errors.New("Info.plist contains duplicate keys")
		}
		seen[key] = struct{}{}
		valueStart, valueStartErr := nextPlistValueStart(decoder)
		if valueStartErr != nil {
			return nil, valueStartErr
		}
		if valueStart.Name.Local != "string" {
			if _, needed := required[key]; needed || key == packageinventory.ApolloCapabilityInfoKey {
				return nil, errors.New("Info.plist required identity/capability value is not a string")
			}
			if skipErr := decoder.Skip(); skipErr != nil {
				return nil, errors.New("Info.plist contains an invalid typed value")
			}
			continue
		}
		var value string
		if valueErr := decoder.DecodeElement(&value, &valueStart); valueErr != nil {
			return nil, errors.New("Info.plist contains an invalid string")
		}
		values[key] = value
	}
	return values, nil
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
