// Package packageinventorycmd provides the one CLI-facing adapter for the
// descriptor-rooted Pantheon payload inventory authority.
package packageinventorycmd

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
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
	infoFile, err := captureCanonicalFile(inputs.InfoPlist)
	if err != nil {
		return packageinventory.Report{}, err
	}
	defer infoFile.close()
	if err := validateInfoPlist(infoFile.data, inputs.Version, inputs.Build); err != nil {
		return packageinventory.Report{}, err
	}
	pkgInfoFile, err := captureCanonicalFile(inputs.PkgInfo)
	if err != nil {
		return packageinventory.Report{}, err
	}
	defer pkgInfoFile.close()
	if err := validatePkgInfo(pkgInfoFile.data); err != nil {
		return packageinventory.Report{}, err
	}
	launchAgentFile, err := captureCanonicalFile(inputs.LaunchAgent)
	if err != nil {
		return packageinventory.Report{}, err
	}
	defer launchAgentFile.close()
	if err := validateLaunchAgent(launchAgentFile.data); err != nil {
		return packageinventory.Report{}, err
	}
	return packageinventory.VerifyWithFinalCheck(inputs.App, packageinventory.Expectations{
		Version: inputs.Version, Build: inputs.Build, InfoPlist: infoFile.data,
		PkgInfo: pkgInfoFile.data, LaunchAgent: launchAgentFile.data,
		RequireCodeSignature: inputs.RequireCodeSignature,
	}, func() error {
		for _, file := range []*canonicalFile{infoFile, pkgInfoFile, launchAgentFile} {
			if err := file.revalidate(); err != nil {
				return fmt.Errorf("canonical input %s: %w", file.path, err)
			}
		}
		return nil
	})
}

func validatePkgInfo(data []byte) error {
	if string(data) != "APPL????\n" {
		return errors.New("PkgInfo must contain the canonical Pantheon APPL signature")
	}
	return nil
}

func validateLaunchAgent(data []byte) error {
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	rootToken, err := nextSignificantToken(decoder)
	if err != nil {
		return errors.New("LaunchAgent plist root is missing")
	}
	root, ok := rootToken.(xml.StartElement)
	if !ok || root.Name.Local != "plist" || !hasPlistAttributes(root, map[string]string{"version": "1.0"}) {
		return errors.New("LaunchAgent root must be plist")
	}
	dictToken, err := nextSignificantToken(decoder)
	if err != nil {
		return errors.New("LaunchAgent dictionary is missing")
	}
	dict, ok := dictToken.(xml.StartElement)
	if !ok || dict.Name.Local != "dict" || !hasPlistAttributes(dict, nil) {
		return errors.New("LaunchAgent root must contain one dictionary")
	}
	values := make(map[string]any, 5)
	for {
		token, err := nextSignificantToken(decoder)
		if err != nil {
			return errors.New("LaunchAgent dictionary is incomplete")
		}
		if end, ok := token.(xml.EndElement); ok {
			if end.Name.Local != "dict" {
				return errors.New("LaunchAgent dictionary has an invalid end tag")
			}
			break
		}
		keyStart, ok := token.(xml.StartElement)
		if !ok || keyStart.Name.Local != "key" || !hasPlistAttributes(keyStart, nil) {
			return errors.New("LaunchAgent dictionary contains a non-key entry")
		}
		key, err := decodePlistString(decoder, keyStart)
		if err != nil || strings.TrimSpace(key) == "" {
			return errors.New("LaunchAgent contains an invalid key")
		}
		if _, duplicate := values[key]; duplicate {
			return fmt.Errorf("LaunchAgent contains duplicate key %q", key)
		}
		valueToken, err := nextSignificantToken(decoder)
		if err != nil {
			return fmt.Errorf("LaunchAgent key %q has no value", key)
		}
		valueStart, ok := valueToken.(xml.StartElement)
		if !ok || !hasPlistAttributes(valueStart, nil) {
			return fmt.Errorf("LaunchAgent key %q has an invalid value", key)
		}
		switch key {
		case "Label", "ProcessType":
			if valueStart.Name.Local != "string" {
				return fmt.Errorf("LaunchAgent key %q must be a string", key)
			}
			value, err := decodePlistString(decoder, valueStart)
			if err != nil {
				return fmt.Errorf("LaunchAgent key %q has an invalid string", key)
			}
			values[key] = value
		case "ProgramArguments":
			if valueStart.Name.Local != "array" {
				return errors.New("LaunchAgent ProgramArguments must be an array")
			}
			arguments, err := decodeLaunchAgentArguments(decoder)
			if err != nil {
				return err
			}
			values[key] = arguments
		case "KeepAlive", "RunAtLoad":
			if valueStart.Name.Local != "true" {
				return fmt.Errorf("LaunchAgent key %q must be true", key)
			}
			if err := consumeEmptyPlistElement(decoder, valueStart); err != nil {
				return fmt.Errorf("LaunchAgent key %q must be an empty true element", key)
			}
			values[key] = true
		default:
			return fmt.Errorf("LaunchAgent contains unexpected key %q", key)
		}
	}
	rootEnd, err := nextSignificantToken(decoder)
	if err != nil {
		return errors.New("LaunchAgent plist root is incomplete")
	}
	end, ok := rootEnd.(xml.EndElement)
	if !ok || end.Name.Local != "plist" {
		return errors.New("LaunchAgent plist root is malformed")
	}
	if _, err := nextSignificantToken(decoder); err != io.EOF {
		return errors.New("LaunchAgent contains trailing content")
	}
	if len(values) != 5 || values["Label"] != "ai.sirsi.pantheon" || values["ProcessType"] != "Interactive" || values["KeepAlive"] != true || values["RunAtLoad"] != true {
		return errors.New("LaunchAgent does not match the canonical Pantheon service contract")
	}
	arguments, ok := values["ProgramArguments"].([]string)
	if !ok || len(arguments) != 1 || arguments[0] != "/Applications/Pantheon.app/Contents/MacOS/sirsi-menubar" {
		return errors.New("LaunchAgent ProgramArguments do not target the canonical menu-bar executable")
	}
	return nil
}

func decodeLaunchAgentArguments(decoder *xml.Decoder) ([]string, error) {
	token, err := nextSignificantToken(decoder)
	if err != nil {
		return nil, errors.New("LaunchAgent ProgramArguments is incomplete")
	}
	argument, ok := token.(xml.StartElement)
	if !ok || argument.Name.Local != "string" || !hasPlistAttributes(argument, nil) {
		return nil, errors.New("LaunchAgent must declare exactly one executable argument")
	}
	value, err := decodePlistString(decoder, argument)
	if err != nil {
		return nil, errors.New("LaunchAgent executable argument is malformed")
	}
	endToken, err := nextSignificantToken(decoder)
	if err != nil {
		return nil, errors.New("LaunchAgent ProgramArguments is incomplete")
	}
	end, ok := endToken.(xml.EndElement)
	if !ok || end.Name.Local != "array" {
		return nil, errors.New("LaunchAgent must declare exactly one executable argument")
	}
	return []string{value}, nil
}

func decodePlistString(decoder *xml.Decoder, start xml.StartElement) (string, error) {
	if !hasPlistAttributes(start, nil) {
		return "", errors.New("attributes or namespace in plist string")
	}
	var value strings.Builder
	for {
		token, err := decoder.Token()
		if err != nil {
			return "", err
		}
		switch item := token.(type) {
		case xml.CharData:
			value.Write(item)
		case xml.EndElement:
			if item.Name != start.Name {
				return "", errors.New("mismatched plist string end tag")
			}
			return value.String(), nil
		default:
			return "", errors.New("nested markup in plist string")
		}
	}
}

func hasPlistAttributes(start xml.StartElement, expected map[string]string) bool {
	if start.Name.Space != "" || len(start.Attr) != len(expected) {
		return false
	}
	for _, attr := range start.Attr {
		if attr.Name.Space != "" || expected[attr.Name.Local] != attr.Value {
			return false
		}
	}
	return true
}

func consumeEmptyPlistElement(decoder *xml.Decoder, start xml.StartElement) error {
	if !hasPlistAttributes(start, nil) {
		return errors.New("attributes or namespace in plist boolean")
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	end, ok := token.(xml.EndElement)
	if !ok || end.Name != start.Name {
		return errors.New("non-empty plist element")
	}
	return nil
}

func ReadCanonicalFile(path string) ([]byte, error) {
	file, err := captureCanonicalFile(path)
	if err != nil {
		return nil, err
	}
	defer file.close()
	if err := file.revalidate(); err != nil {
		return nil, err
	}
	return append([]byte(nil), file.data...), nil
}

type directoryEdge struct {
	parentIndex int
	name        string
	identity    unix.Stat_t
}

type canonicalParent struct {
	fds        []int
	identities []unix.Stat_t
	edges      []directoryEdge
}

type canonicalFile struct {
	path     string
	leaf     string
	fd       int
	identity unix.Stat_t
	data     []byte
	parent   *canonicalParent
}

func captureCanonicalFile(path string) (*canonicalFile, error) {
	return captureCanonicalFileWithHook(path, nil)
}

func captureCanonicalFileWithHook(path string, beforeLeafOpen func()) (*canonicalFile, error) {
	if path == "" || !filepath.IsAbs(path) {
		return nil, errors.New("canonical input path must be absolute")
	}
	clean := filepath.Clean(path)
	if clean != path {
		return nil, errors.New("canonical input path must be normalized")
	}
	leaf := filepath.Base(clean)
	if leaf == "." || leaf == string(filepath.Separator) || leaf == ".." {
		return nil, errors.New("canonical input path has an unsafe leaf")
	}
	parent, err := openDirectoryChain(filepath.Dir(clean))
	if err != nil {
		return nil, err
	}
	file := &canonicalFile{path: clean, leaf: leaf, fd: -1, parent: parent}
	cleanup := func(err error) (*canonicalFile, error) {
		file.close()
		return nil, err
	}
	parentFD := parent.fds[len(parent.fds)-1]
	if beforeLeafOpen != nil {
		beforeLeafOpen()
	}
	fd, err := unix.Openat(parentFD, leaf, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return cleanup(fmt.Errorf("open canonical input %s: %w", clean, err))
	}
	file.fd = fd
	var before, named unix.Stat_t
	if err := unix.Fstat(fd, &before); err != nil {
		return cleanup(fmt.Errorf("stat canonical input %s: %w", clean, err))
	}
	if before.Mode&unix.S_IFMT != unix.S_IFREG || before.Nlink != 1 || before.Size < 0 || before.Size > maxCanonicalInput {
		return cleanup(errors.New("expected a regular nlink=1 file within the size limit"))
	}
	if err := unix.Fstatat(parentFD, leaf, &named, unix.AT_SYMLINK_NOFOLLOW); err != nil || !sameIdentity(before, named) {
		return cleanup(errors.New("canonical input name changed while opening"))
	}
	if err := parent.revalidate(); err != nil {
		return cleanup(fmt.Errorf("canonical input parent: %w", err))
	}
	data, err := readStableDescriptor(fd, before.Size)
	if err != nil {
		return cleanup(fmt.Errorf("read canonical input %s: %w", clean, err))
	}
	var after unix.Stat_t
	if err := unix.Fstat(fd, &after); err != nil || !sameIdentity(before, after) {
		return cleanup(errors.New("canonical input identity changed during read"))
	}
	file.identity = before
	file.data = data
	if err := file.revalidate(); err != nil {
		return cleanup(err)
	}
	return file, nil
}

func openDirectoryChain(dir string) (*canonicalParent, error) {
	if !filepath.IsAbs(dir) || filepath.Clean(dir) != dir {
		return nil, errors.New("canonical input parent must be an absolute normalized path")
	}
	fd, err := unix.Open(string(filepath.Separator), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	var rootIdentity unix.Stat_t
	if err := unix.Fstat(fd, &rootIdentity); err != nil {
		unix.Close(fd)
		return nil, err
	}
	parent := &canonicalParent{fds: []int{fd}, identities: []unix.Stat_t{rootIdentity}}
	cleanup := func(err error) (*canonicalParent, error) {
		parent.close()
		return nil, err
	}
	parts := strings.Split(strings.TrimPrefix(dir, string(filepath.Separator)), string(filepath.Separator))
	if dir == string(filepath.Separator) {
		parts = nil
	}
	for _, component := range parts {
		if component == "" || component == "." || component == ".." {
			return cleanup(errors.New("canonical input parent contains an unsafe component"))
		}
		parentIndex := len(parent.fds) - 1
		parentFD := parent.fds[parentIndex]
		var before unix.Stat_t
		if err := unix.Fstatat(parentFD, component, &before, unix.AT_SYMLINK_NOFOLLOW); err != nil {
			return cleanup(fmt.Errorf("stat canonical input parent %q: %w", component, err))
		}
		if before.Mode&unix.S_IFMT != unix.S_IFDIR {
			return cleanup(fmt.Errorf("canonical input parent is not a real directory: %q", component))
		}
		next, err := unix.Openat(parentFD, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if err != nil {
			return cleanup(fmt.Errorf("open canonical input parent %q: %w", component, err))
		}
		var opened unix.Stat_t
		if err := unix.Fstat(next, &opened); err != nil || !sameDirectoryIdentity(before, opened) {
			unix.Close(next)
			if err != nil {
				return cleanup(fmt.Errorf("fstat canonical input parent %q: %w", component, err))
			}
			return cleanup(fmt.Errorf("canonical input parent substitution at %q", component))
		}
		parent.edges = append(parent.edges, directoryEdge{parentIndex: parentIndex, name: component, identity: opened})
		parent.fds = append(parent.fds, next)
		parent.identities = append(parent.identities, opened)
	}
	return parent, nil
}

func (p *canonicalParent) revalidate() error {
	for i, fd := range p.fds {
		var opened unix.Stat_t
		if err := unix.Fstat(fd, &opened); err != nil || !sameDirectoryIdentity(p.identities[i], opened) {
			return errors.New("retained canonical input parent identity changed")
		}
	}
	for _, edge := range p.edges {
		var named unix.Stat_t
		if err := unix.Fstatat(p.fds[edge.parentIndex], edge.name, &named, unix.AT_SYMLINK_NOFOLLOW); err != nil || !sameDirectoryIdentity(edge.identity, named) {
			return fmt.Errorf("canonical input parent path continuity failed at %q", edge.name)
		}
	}
	return nil
}

func (p *canonicalParent) close() {
	for i := len(p.fds) - 1; i >= 0; i-- {
		_ = unix.Close(p.fds[i])
	}
	p.fds = nil
}

func (f *canonicalFile) revalidate() error {
	if f.fd < 0 {
		return errors.New("canonical input descriptor is closed")
	}
	if err := f.parent.revalidate(); err != nil {
		return err
	}
	var opened, named unix.Stat_t
	if err := unix.Fstat(f.fd, &opened); err != nil || !sameIdentity(f.identity, opened) {
		return errors.New("canonical input descriptor identity changed")
	}
	if err := unix.Fstatat(f.parent.fds[len(f.parent.fds)-1], f.leaf, &named, unix.AT_SYMLINK_NOFOLLOW); err != nil || !sameIdentity(f.identity, named) {
		return errors.New("canonical input name identity changed")
	}
	data, err := readStableDescriptor(f.fd, f.identity.Size)
	if err != nil || !bytes.Equal(data, f.data) {
		return errors.New("canonical input bytes changed after capture")
	}
	var after unix.Stat_t
	if err := unix.Fstat(f.fd, &after); err != nil || !sameIdentity(f.identity, after) {
		return errors.New("canonical input identity changed during revalidation")
	}
	if err := f.parent.revalidate(); err != nil {
		return err
	}
	if err := unix.Fstatat(f.parent.fds[len(f.parent.fds)-1], f.leaf, &named, unix.AT_SYMLINK_NOFOLLOW); err != nil || !sameIdentity(f.identity, named) {
		return errors.New("canonical input name changed during revalidation")
	}
	return nil
}

func (f *canonicalFile) close() {
	if f.fd >= 0 {
		_ = unix.Close(f.fd)
		f.fd = -1
	}
	if f.parent != nil {
		f.parent.close()
		f.parent = nil
	}
}

func readStableDescriptor(fd int, size int64) ([]byte, error) {
	if size < 0 || size > maxCanonicalInput {
		return nil, errors.New("canonical input size is outside the limit")
	}
	read := func() ([]byte, error) {
		data := make([]byte, int(size))
		for offset := 0; offset < len(data); {
			n, err := unix.Pread(fd, data[offset:], int64(offset))
			if err != nil {
				return nil, err
			}
			if n == 0 {
				return nil, io.ErrUnexpectedEOF
			}
			offset += n
		}
		var extra [1]byte
		n, err := unix.Pread(fd, extra[:], size)
		if err != nil {
			return nil, err
		}
		if n != 0 {
			return nil, errors.New("canonical input grew during read")
		}
		return data, nil
	}
	first, err := read()
	if err != nil {
		return nil, err
	}
	second, err := read()
	if err != nil || !bytes.Equal(first, second) {
		return nil, errors.New("canonical input changed during read")
	}
	return first, nil
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

// Retained directory authority is stable across unrelated child additions;
// governed file identity and bytes are checked separately and exactly.
func sameDirectoryIdentity(a, b unix.Stat_t) bool {
	return a.Dev == b.Dev && a.Ino == b.Ino && a.Mode == b.Mode && a.Mode&unix.S_IFMT == unix.S_IFDIR && b.Mode&unix.S_IFMT == unix.S_IFDIR
}
