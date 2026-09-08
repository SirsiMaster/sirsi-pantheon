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
	values, err := parseRootDictionary(decoder)
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
	return nil
}

func parseRootDictionary(decoder *xml.Decoder) (map[string]string, error) {
	rootToken, err := nextSignificantToken(decoder)
	if err != nil {
		return nil, errors.New("Info.plist root is missing")
	}
	root, ok := rootToken.(xml.StartElement)
	if !ok || root.Name.Local != "plist" {
		return nil, errors.New("Info.plist root must be plist")
	}
	dictToken, err := nextSignificantToken(decoder)
	if err != nil {
		return nil, errors.New("Info.plist dictionary is missing")
	}
	dict, ok := dictToken.(xml.StartElement)
	if !ok || dict.Name.Local != "dict" {
		return nil, errors.New("Info.plist root must contain one dict")
	}
	values, err := parseDictionary(decoder, dict)
	if err != nil {
		return nil, err
	}
	endRoot, err := nextSignificantToken(decoder)
	if err != nil {
		return nil, errors.New("Info.plist root is incomplete")
	}
	end, ok := endRoot.(xml.EndElement)
	if !ok || end.Name.Local != "plist" {
		return nil, errors.New("Info.plist must close its plist root")
	}
	if trailing, err := nextSignificantToken(decoder); err != io.EOF {
		if err != nil {
			return nil, errors.New("Info.plist trailing XML is invalid")
		}
		_ = trailing
		return nil, errors.New("Info.plist contains trailing XML")
	}
	return values, nil
}

func parseDictionary(decoder *xml.Decoder, start xml.StartElement) (map[string]string, error) {
	values := make(map[string]string)
	for {
		token, err := nextSignificantToken(decoder)
		if err != nil {
			return nil, errors.New("Info.plist dictionary is incomplete")
		}
		if end, ok := token.(xml.EndElement); ok && end.Name.Local == start.Name.Local {
			return values, nil
		}
		keyStart, ok := token.(xml.StartElement)
		if !ok || keyStart.Name.Local != "key" {
			return nil, errors.New("Info.plist dictionary contains a non-key entry")
		}
		var key string
		if err := decoder.DecodeElement(&key, &keyStart); err != nil || strings.TrimSpace(key) == "" {
			return nil, errors.New("Info.plist contains an invalid key")
		}
		valueToken, err := nextSignificantToken(decoder)
		if err != nil {
			return nil, errors.New("Info.plist key has no value")
		}
		valueStart, ok := valueToken.(xml.StartElement)
		if !ok {
			return nil, errors.New("Info.plist key has an invalid value")
		}
		if key != "CFBundleIdentifier" && key != "CFBundleShortVersionString" && key != "CFBundleVersion" {
			if err := decoder.Skip(); err != nil {
				return nil, errors.New("Info.plist contains an invalid value")
			}
			continue
		}
		if valueStart.Name.Local != "string" {
			return nil, fmt.Errorf("Info.plist target %s must be a string", key)
		}
		if _, exists := values[key]; exists {
			return nil, fmt.Errorf("Info.plist contains duplicate target key %s", key)
		}
		var value string
		if err := decoder.DecodeElement(&value, &valueStart); err != nil {
			return nil, errors.New("Info.plist contains an invalid string")
		}
		values[key] = value
	}
}

func nextSignificantToken(decoder *xml.Decoder) (xml.Token, error) {
	for {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		if chars, ok := token.(xml.CharData); ok && strings.TrimSpace(string(chars)) == "" {
			continue
		}
		if _, ok := token.(xml.Comment); ok {
			continue
		}
		if _, ok := token.(xml.ProcInst); ok {
			continue
		}
		if _, ok := token.(xml.Directive); ok {
			continue
		}
		return token, nil
	}
}

func sameIdentity(a, b unix.Stat_t) bool {
	return a.Dev == b.Dev && a.Ino == b.Ino && a.Mode == b.Mode && a.Nlink == b.Nlink && a.Size == b.Size
}
