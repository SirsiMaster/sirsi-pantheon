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
	RequireCodeSignature bool
}

func Verify(inputs Inputs) (packageinventory.Report, error) {
	if inputs.App == "" || inputs.Version == "" || inputs.Build == "" || inputs.InfoPlist == "" || inputs.PkgInfo == "" || inputs.LaunchAgent == "" {
		return packageinventory.Report{}, errors.New("app, version, build, info-plist, pkg-info, and launch-agent are required")
	}
	info, err := ReadCanonicalFile(inputs.InfoPlist)
	if err != nil {
		return packageinventory.Report{}, err
	}
	if err := validateInfoPlist(info, inputs.Version, inputs.Build); err != nil {
		return packageinventory.Report{}, err
	}
	pkgInfo, err := ReadCanonicalFile(inputs.PkgInfo)
	if err != nil {
		return packageinventory.Report{}, err
	}
	launchAgent, err := ReadCanonicalFile(inputs.LaunchAgent)
	if err != nil {
		return packageinventory.Report{}, err
	}
	return packageinventory.Verify(inputs.App, packageinventory.Expectations{
		Version: inputs.Version, Build: inputs.Build, InfoPlist: info,
		PkgInfo: pkgInfo, LaunchAgent: launchAgent,
		RequireCodeSignature: inputs.RequireCodeSignature,
	})
}

func ReadCanonicalFile(path string) ([]byte, error) {
	if path == "" {
		return nil, errors.New("path is required")
	}
	parentFD, leaf, err := openParent(path)
	if err != nil {
		return nil, err
	}
	defer unix.Close(parentFD)
	var parentBefore unix.Stat_t
	if err := unix.Fstat(parentFD, &parentBefore); err != nil {
		return nil, err
	}
	fd, err := unix.Openat(parentFD, leaf, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
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
	if !sameIdentity(before, after) {
		return nil, errors.New("file identity changed during read")
	}
	var nameAfter unix.Stat_t
	if err := unix.Fstatat(parentFD, leaf, &nameAfter, unix.AT_SYMLINK_NOFOLLOW); err != nil || !sameIdentity(before, nameAfter) {
		return nil, errors.New("file name identity changed during read")
	}
	var parentAfter unix.Stat_t
	if err := unix.Fstat(parentFD, &parentAfter); err != nil || !sameIdentity(parentBefore, parentAfter) {
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
	var key string
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return errors.New("Info.plist is not valid XML")
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "key" {
			continue
		}
		if err := decoder.DecodeElement(&key, &start); err != nil || key == "" {
			return errors.New("Info.plist contains an invalid key")
		}
		valueToken, err := decoder.Token()
		if err != nil {
			return errors.New("Info.plist key has no value")
		}
		valueStart, ok := valueToken.(xml.StartElement)
		if !ok || valueStart.Name.Local != "string" {
			return errors.New("Info.plist value is not a string")
		}
		var value string
		if err := decoder.DecodeElement(&value, &valueStart); err != nil {
			return errors.New("Info.plist contains an invalid string")
		}
		if _, exists := values[key]; exists {
			return errors.New("Info.plist contains duplicate keys")
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

func sameIdentity(a, b unix.Stat_t) bool {
	return a.Dev == b.Dev && a.Ino == b.Ino && a.Mode == b.Mode && a.Nlink == b.Nlink && a.Size == b.Size
}
