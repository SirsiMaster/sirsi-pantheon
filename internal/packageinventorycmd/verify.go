// Package packageinventorycmd provides the one CLI-facing adapter for the
// descriptor-rooted Pantheon payload inventory authority.
package packageinventorycmd

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
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
const maxCanonicalHashInput = 1 << 30
const maxCanonicalApolloProfile = 64 << 10
const ApolloProfileAbsent = "absent"

type Inputs struct {
	App                   string
	Version               string
	Build                 string
	InfoPlist             string
	PkgInfo               string
	LaunchAgent           string
	BrandLogo             string
	ApolloCapability      string
	ApolloProfile         string
	ExpectedInfoSHA256    string
	ExpectedProfileSHA256 string
	RequireCodeSignature  bool
	RequireFrozenDigests  bool
}

func Verify(inputs Inputs) (packageinventory.Report, error) {
	report, _, err := VerifyWithFrozenDigests(inputs)
	return report, err
}

type FrozenDigests struct {
	InfoPlist     string
	ApolloProfile string
}

type frozenDigestReceiptFile interface {
	Write([]byte) (int, error)
	Close() error
}

type frozenDigestReceiptOpen func(string, int, os.FileMode) (frozenDigestReceiptFile, error)

// WriteFrozenDigests creates a private, create-only receipt for one successful
// inventory pass. Incomplete receipts are retained as explicit reconciliation
// debt; the writer never removes a path after a write or close failure.
func WriteFrozenDigests(path string, digests FrozenDigests) error {
	return writeFrozenDigestsWith(path, digests, func(path string, flags int, mode os.FileMode) (frozenDigestReceiptFile, error) {
		return os.OpenFile(path, flags, mode)
	})
}

func writeFrozenDigestsWith(path string, digests FrozenDigests, open frozenDigestReceiptOpen) error {
	if path == "" || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return errors.New("frozen digest receipt requires a canonical absolute path")
	}
	if open == nil {
		return errors.New("frozen digest receipt opener is required")
	}
	if !canonicalSHA256(digests.InfoPlist) ||
		(digests.ApolloProfile != ApolloProfileAbsent && !canonicalSHA256(digests.ApolloProfile)) {
		return errors.New("frozen digest receipt values are invalid")
	}
	file, err := open(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create-only frozen digest receipt %q: %w", path, err)
	}
	contents := []byte(fmt.Sprintf("info_sha256=%s\napollo_profile_sha256=%s\n", digests.InfoPlist, digests.ApolloProfile))
	written, writeErr := file.Write(contents)
	if writeErr == nil && written != len(contents) {
		writeErr = io.ErrShortWrite
	}
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		return fmt.Errorf("frozen digest receipt %q may be incomplete and was retained for reconciliation: %w", path, errors.Join(writeErr, closeErr))
	}
	return nil
}

// VerifyWithFrozenDigests returns hashes of the exact trusted Info.plist and
// profile bytes read by this successful inventory pass. Callers can persist
// this small digest pair before signing without exposing profile contents.
func VerifyWithFrozenDigests(inputs Inputs) (packageinventory.Report, FrozenDigests, error) {
	if inputs.App == "" || inputs.Version == "" || inputs.Build == "" || inputs.InfoPlist == "" || inputs.PkgInfo == "" || inputs.LaunchAgent == "" || inputs.BrandLogo == "" {
		return packageinventory.Report{}, FrozenDigests{}, errors.New("app, version, build, info-plist, pkg-info, launch-agent, and brand-logo are required")
	}
	if inputs.ApolloCapability == "" {
		return packageinventory.Report{}, FrozenDigests{}, errors.New("explicit --apollo-capability is required")
	}
	frozenDigests := inputs.ExpectedInfoSHA256 != "" || inputs.ExpectedProfileSHA256 != ""
	if frozenDigests && (inputs.ExpectedInfoSHA256 == "" || inputs.ExpectedProfileSHA256 == "") {
		return packageinventory.Report{}, FrozenDigests{}, errors.New("frozen Info.plist and Apollo profile digest expectations must be supplied together")
	}
	if inputs.RequireFrozenDigests && !inputs.RequireCodeSignature {
		return packageinventory.Report{}, FrozenDigests{}, errors.New("frozen digest enforcement requires code signature verification")
	}
	if inputs.RequireFrozenDigests && !frozenDigests {
		return packageinventory.Report{}, FrozenDigests{}, errors.New("release inventory requires frozen Info.plist and Apollo profile digests")
	}
	info, infoErr := ReadCanonicalFile(inputs.InfoPlist)
	if infoErr != nil {
		return packageinventory.Report{}, FrozenDigests{}, infoErr
	}
	if err := verifyFrozenDigest("Info.plist", info, inputs.ExpectedInfoSHA256); err != nil {
		return packageinventory.Report{}, FrozenDigests{}, err
	}
	var apolloProfile []byte
	if inputs.ApolloProfile != "" {
		if !filepath.IsAbs(inputs.ApolloProfile) || filepath.Clean(inputs.ApolloProfile) != inputs.ApolloProfile {
			return packageinventory.Report{}, FrozenDigests{}, errors.New("Apollo profile requires a canonical absolute path")
		}
		var err error
		apolloProfile, err = ReadProtectedApolloProfile(inputs.ApolloProfile)
		if err != nil {
			return packageinventory.Report{}, FrozenDigests{}, fmt.Errorf("read protected Apollo profile: %w", err)
		}
		if len(apolloProfile) == 0 || len(apolloProfile) > maxCanonicalApolloProfile {
			return packageinventory.Report{}, FrozenDigests{}, errors.New("Apollo profile exceeds bounded canonical input")
		}
	}
	if err := verifyFrozenProfileDigest(apolloProfile, inputs.ApolloProfile != "", inputs.ExpectedProfileSHA256); err != nil {
		return packageinventory.Report{}, FrozenDigests{}, err
	}
	if validateErr := validateInfoPlist(info, inputs.Version, inputs.Build, inputs.ApolloCapability, len(apolloProfile) != 0); validateErr != nil {
		return packageinventory.Report{}, FrozenDigests{}, validateErr
	}
	pkgInfo, pkgInfoErr := ReadCanonicalFile(inputs.PkgInfo)
	if pkgInfoErr != nil {
		return packageinventory.Report{}, FrozenDigests{}, pkgInfoErr
	}
	launchAgent, launchAgentErr := ReadCanonicalFile(inputs.LaunchAgent)
	if launchAgentErr != nil {
		return packageinventory.Report{}, FrozenDigests{}, launchAgentErr
	}
	brandLogo, brandLogoErr := ReadCanonicalFile(inputs.BrandLogo)
	if brandLogoErr != nil {
		return packageinventory.Report{}, FrozenDigests{}, brandLogoErr
	}
	report, err := packageinventory.Verify(inputs.App, packageinventory.Expectations{
		Version: inputs.Version, Build: inputs.Build, InfoPlist: info,
		PkgInfo: pkgInfo, LaunchAgent: launchAgent, BrandLogo: brandLogo,
		ApolloCapability: inputs.ApolloCapability, ApolloProfile: apolloProfile,
		RequireCodeSignature: inputs.RequireCodeSignature,
	})
	if err != nil {
		return packageinventory.Report{}, FrozenDigests{}, err
	}
	profileDigest := ApolloProfileAbsent
	if len(apolloProfile) != 0 {
		profileDigest = fmt.Sprintf("%x", sha256.Sum256(apolloProfile))
	}
	return report, FrozenDigests{InfoPlist: fmt.Sprintf("%x", sha256.Sum256(info)), ApolloProfile: profileDigest}, nil
}

func ReadCanonicalFile(path string) ([]byte, error) {
	return readCanonicalFile(path, maxCanonicalInput, nil)
}

// ReadProtectedApolloProfile reads the build reference or installed profile
// only when its descriptor is owned by root/current user and not writable by
// group or other. The profile remains read-only data; no caller executes it.
func ReadProtectedApolloProfile(path string) ([]byte, error) {
	return readCanonicalFile(path, maxCanonicalApolloProfile, func(st unix.Stat_t) error {
		if !apolloProfileDescriptorAllowed(st.Uid, uint32(st.Mode), uint32(os.Getuid())) {
			return errors.New("Apollo profile owner or write permissions are unsafe")
		}
		return nil
	})
}

func readCanonicalFile(path string, limit int64, policy func(unix.Stat_t) error) ([]byte, error) {
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
	fd, fileOpenErr := unix.Openat(parentFD, leaf, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
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
	if before.Mode&unix.S_IFMT != unix.S_IFREG || before.Nlink != 1 || before.Size < 0 || before.Size > limit {
		return nil, errors.New("expected a regular nlink=1 file within the size limit")
	}
	if policy != nil {
		if err := policy(before); err != nil {
			return nil, err
		}
	}
	data, readErr := io.ReadAll(io.LimitReader(file, limit+1))
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
	identityMatches := sameIdentity(before, after)
	if policy != nil {
		identityMatches = sameHashSnapshot(before, after)
	}
	if !identityMatches {
		return nil, errors.New("file identity changed during read")
	}
	var nameAfter unix.Stat_t
	nameErr := unix.Fstatat(parentFD, leaf, &nameAfter, unix.AT_SYMLINK_NOFOLLOW)
	nameMatches := nameErr == nil && sameIdentity(before, nameAfter)
	if policy != nil {
		nameMatches = nameErr == nil && sameHashSnapshot(before, nameAfter)
	}
	if !nameMatches {
		return nil, errors.New("file name identity changed during read")
	}
	var parentAfter unix.Stat_t
	parentErr := unix.Fstat(parentFD, &parentAfter)
	parentMatches := parentErr == nil && sameIdentity(parentBefore, parentAfter)
	if policy != nil {
		parentMatches = parentErr == nil && sameHashSnapshot(parentBefore, parentAfter)
	}
	if !parentMatches {
		return nil, errors.New("file parent identity changed during read")
	}
	if policy != nil {
		if err := policy(after); err != nil {
			return nil, err
		}
	}
	return data, nil
}

func apolloProfileDescriptorAllowed(owner, mode, current uint32) bool {
	return (owner == 0 || owner == current) && mode&0o0022 == 0
}

func verifyFrozenDigest(label string, content []byte, expected string) error {
	if expected == "" {
		return nil
	}
	if !canonicalSHA256(expected) || fmt.Sprintf("%x", sha256.Sum256(content)) != expected {
		return fmt.Errorf("%s differs from its frozen pre-sign digest", label)
	}
	return nil
}

func verifyFrozenProfileDigest(content []byte, present bool, expected string) error {
	if expected == "" {
		return nil
	}
	if expected == ApolloProfileAbsent {
		if present {
			return errors.New("Apollo profile presence differs from frozen pre-sign inventory")
		}
		return nil
	}
	if !present || !canonicalSHA256(expected) || fmt.Sprintf("%x", sha256.Sum256(content)) != expected {
		return errors.New("Apollo profile differs from its frozen pre-sign digest")
	}
	return nil
}

func canonicalSHA256(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && hex.EncodeToString(decoded) == value
}

// HashCanonicalFile hashes one bounded regular file through the same retained,
// no-follow component walk as ReadCanonicalFile. Two descriptor reads and
// before/after file, name, and parent checks reject a changing hash source.
func HashCanonicalFile(path string) (string, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return "", errors.New("canonical hash input path must be absolute and clean")
	}
	parentFD, leaf, err := openParent(path)
	if err != nil {
		return "", err
	}
	defer unix.Close(parentFD)
	var parentBefore unix.Stat_t
	if err := unix.Fstat(parentFD, &parentBefore); err != nil {
		return "", err
	}
	fd, err := unix.Openat(parentFD, leaf, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return "", err
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		unix.Close(fd)
		return "", errors.New("cannot retain canonical hash descriptor")
	}
	defer file.Close()
	var before unix.Stat_t
	if err := unix.Fstat(fd, &before); err != nil || before.Mode&unix.S_IFMT != unix.S_IFREG || before.Nlink != 1 || before.Size <= 0 || before.Size > maxCanonicalHashInput {
		return "", errors.New("canonical hash input must be a bounded singly-linked regular file")
	}
	first := sha256.New()
	n, err := io.Copy(first, io.LimitReader(file, maxCanonicalHashInput+1))
	if err != nil || n != before.Size || n > maxCanonicalHashInput {
		return "", errors.New("canonical hash input changed size or could not be read")
	}
	if _, err := unix.Seek(fd, 0, 0); err != nil {
		return "", err
	}
	second := sha256.New()
	confirmed, err := io.Copy(second, io.LimitReader(file, maxCanonicalHashInput+1))
	if err != nil || confirmed != n || !bytes.Equal(first.Sum(nil), second.Sum(nil)) {
		return "", errors.New("canonical hash input changed during confirmation read")
	}
	var after, named, parentAfter unix.Stat_t
	if unix.Fstat(fd, &after) != nil || !sameHashSnapshot(before, after) ||
		unix.Fstatat(parentFD, leaf, &named, unix.AT_SYMLINK_NOFOLLOW) != nil || !sameHashSnapshot(before, named) ||
		unix.Fstat(parentFD, &parentAfter) != nil || !sameHashSnapshot(parentBefore, parentAfter) {
		return "", errors.New("canonical hash input or retained parent changed during inspection")
	}
	return hex.EncodeToString(first.Sum(nil)), nil
}

func sameHashSnapshot(a, b unix.Stat_t) bool {
	return a.Dev == b.Dev && a.Ino == b.Ino && a.Mode == b.Mode && a.Nlink == b.Nlink &&
		a.Uid == b.Uid && a.Gid == b.Gid && a.Size == b.Size && a.Mtim == b.Mtim && a.Ctim == b.Ctim
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
