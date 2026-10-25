//go:build darwin

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/setup"
	"github.com/SirsiMaster/sirsi-pantheon/internal/updater"
	"github.com/spf13/cobra"
	"golang.org/x/sys/unix"
)

type maintenanceInstallerPlan struct {
	Schema         int    `json:"schema"`
	Release        string `json:"release"`
	Package        string `json:"package"`
	SHA256         string `json:"sha256"`
	Device         uint64 `json:"device"`
	Inode          uint64 `json:"inode"`
	Bytes          int64  `json:"bytes"`
	Implementation string `json:"implementation"`
}
type maintenanceInstallerOps struct {
	release  func() (*updater.Release, error)
	download func(context.Context, string, string) error
	inspect  func(context.Context, string, string) (setup.LegacyAppCapability, error)
}

var maintenanceVersionRE = regexp.MustCompile(`^0\.[0-9]+\.[0-9]+$`)

func maintenancePackageAsset(current string, rel *updater.Release) (*updater.Asset, error) {
	if rel == nil {
		return nil, errors.New("No complete supported release is published. Recheck releases; the installed app is unchanged.")
	}
	candidate := strings.TrimPrefix(rel.TagName, "v")
	current = strings.TrimPrefix(current, "v")
	if !maintenanceVersionRE.MatchString(candidate) || !maintenanceVersionRE.MatchString(current) {
		return nil, errors.New("An official app release is required before Installer preparation.")
	}
	if candidate != current {
		if err := updater.ValidateUpdateVersion(current, candidate); err != nil {
			return nil, err
		}
	}
	a := updater.CommercialPKGAsset(rel)
	if a == nil || !updater.IsCompleteCommercialRelease(rel) || a.Size <= 0 || a.Size > 1<<30 {
		return nil, errors.New("Published release lacks the exact complete app/PKG pair; recheck releases.")
	}
	u, err := url.Parse(a.BrowserDownloadURL)
	want := "/SirsiMaster/sirsi-pantheon/releases/download/" + rel.TagName + "/" + a.Name
	digest := strings.TrimPrefix(a.Digest, "sha256:")
	if err != nil || u.Scheme != "https" || u.Host != "github.com" || u.User != nil || u.Path != want || u.RawQuery != "" || u.Fragment != "" || !strings.HasPrefix(a.Digest, "sha256:") || len(digest) != 64 {
		return nil, errors.New("Published PKG provenance/hash metadata is unavailable; recheck the normal release.")
	}
	if raw, err := hex.DecodeString(digest); err != nil || hex.EncodeToString(raw) != digest {
		return nil, errors.New("Published package digest is malformed.")
	}
	return a, nil
}

func maintenanceDownload(ctx context.Context, endpoint, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 90 * time.Second}
	response, err := client.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("Package download HTTP%d; recheck releases", response.StatusCode)
	}
	f, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	n, writeErr := io.Copy(f, io.LimitReader(response.Body, (1<<30)+1))
	syncErr := f.Sync()
	closeErr := f.Close()
	if n > 1<<30 {
		return errors.New("Package download exceeds bound; private staging retained")
	}
	return errors.Join(writeErr, syncErr, closeErr)
}

func inspectMaintenancePKG(ctx context.Context, path, release string) (setup.LegacyAppCapability, error) {
	var cap setup.LegacyAppCapability
	// These are normal read-only package/signature tools. No installer or
	// script payload is executed. The one helper probe runs unprivileged.
	signature, err := maintenanceInstallerRead(ctx, "/usr/sbin/pkgutil", "--check-signature", path)
	if err != nil || !strings.Contains(string(signature), "Developer ID Installer: Sirsi Technologies Inc. (9D382WV988)") {
		return cap, errors.New("Installer signature is not the canonical Sirsi identity; do not open.")
	}
	assessment, err := maintenanceInstallerRead(ctx, "/usr/sbin/spctl", "--assess", "--type", "install", "--verbose=2", path)
	if err != nil || !strings.Contains(string(assessment), "source=Notarized Developer ID") {
		return cap, errors.New("Normal Installer trust/notarization assessment refused the package.")
	}
	work, err := os.MkdirTemp(filepath.Dir(path), "inspect-")
	if err != nil {
		return cap, err
	}
	expanded := filepath.Join(work, "expanded")
	// All private scratch is retained. No failure removes an unknown replacement.
	if _, err := maintenanceInstallerRead(ctx, "/usr/sbin/pkgutil", "--expand-full", path, expanded); err != nil {
		return cap, err
	}
	if err := maintenancePackageInfo(filepath.Join(expanded, "PackageInfo"), release); err != nil {
		return cap, err
	}
	payload := filepath.Join(expanded, "Payload")
	for dir, want := range map[string][]string{
		payload: {"Applications", "usr"}, filepath.Join(payload, "Applications"): {"Pantheon.app"},
		filepath.Join(payload, "usr"): {"local"}, filepath.Join(payload, "usr/local"): {"libexec"},
		filepath.Join(payload, "usr/local/libexec"): {"sirsi-pantheon"}, filepath.Join(payload, "usr/local/libexec/sirsi-pantheon"): {"sirsi"},
	} {
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) != len(want) {
			return cap, errors.New("Package has an unexpected app/system payload namespace")
		}
		for i, entry := range entries {
			if entry.Name() != want[i] || entry.Type()&os.ModeSymlink != 0 {
				return cap, errors.New("Package payload path differs from the normal Installer contract")
			}
		}
	}
	app := filepath.Join(payload, "Applications/Pantheon.app")
	if _, err := maintenanceInstallerRead(ctx, "/usr/bin/codesign", "--verify", "--strict", "--deep", "-R", `=anchor apple generic and certificate leaf[subject.OU] = "9D382WV988" and identifier "ai.sirsi.pantheon"`, app); err != nil {
		return cap, errors.New("Package app signature refused")
	}
	pkgHelper := filepath.Join(expanded, "Payload/usr/local/libexec/sirsi-pantheon/sirsi")
	appHelper := filepath.Join(expanded, "Payload/Applications/Pantheon.app/Contents/MacOS/sirsi")
	var expected string
	for _, helper := range []string{pkgHelper, appHelper} {
		h, err := maintenanceInstallerFileHash(helper, false)
		if err != nil {
			return cap, err
		}
		if expected != "" && h.SHA256 != expected {
			return cap, errors.New("Package app and protected helper are not the same payload.")
		}
		expected = h.SHA256
		if _, err := maintenanceInstallerRead(ctx, "/usr/bin/codesign", "--verify", "--strict", "-R", `=anchor apple generic and certificate leaf[subject.OU] = "9D382WV988" and identifier "sirsi"`, helper); err != nil {
			return cap, errors.New("Package helper signature refused.")
		}
		raw, err := maintenanceInstallerRead(ctx, helper, "launchd", "legacy-maintenance-app-capabilities")
		if err != nil {
			return cap, errors.New("Published package does not contain the maintenance bridge; recheck after a compatible release.")
		}
		var next setup.LegacyAppCapability
		if json.Unmarshal(raw, &next) != nil || next.Schema != 1 || !next.Native || next.Protocol != "root-relay-local-app-v1" || next.Release != release || len(next.Implementation) != 64 {
			return cap, errors.New("Package helper is not compatible with this maintenance workflow.")
		}
		if cap.Schema != 0 && cap != next {
			return cap, errors.New("Package helper contracts differ.")
		}
		cap = next
	}
	if cap != legacyAppCapability() {
		return cap, errors.New("The exact normal PKG does not contain this app's helper protocol; no older release is offered")
	}
	current, err := setup.CurrentPackagedMaatEngine()
	if err != nil {
		return cap, err
	}
	currentBytes, err := maintenanceInstallerFileHash(current, false)
	if err != nil || currentBytes.SHA256 != expected {
		return cap, errors.New("The exact PKG and running canonical app do not carry the same engine; no alternate installer offered")
	}
	return cap, nil
}

func maintenanceInstallerRead(ctx context.Context, exe string, args ...string) ([]byte, error) {
	// rootRelayRead is setup-private; use the existing command capture helper's
	// same bounded direct-exec pattern, never a shell or a caller command.
	bounded, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := newMaintenanceInstallerProcess(bounded, exe, args...)
	var out legacyAppOutput
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("package metadata check failed: %s: %w", filepath.Base(exe), err)
	}
	return out.Bytes(), nil
}

func maintenanceInstallerFileHash(path string, stage bool) (maintenanceInstallerPlan, error) {
	var result maintenanceInstallerPlan
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return result, errors.New("Canonical package path required")
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil || parent != filepath.Dir(path) {
		return result, errors.New("Linked package path refused")
	}
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return result, err
	}
	f := os.NewFile(uintptr(fd), path)
	defer f.Close()
	var before, after, named unix.Stat_t
	if unix.Fstat(fd, &before) != nil || before.Mode&unix.S_IFMT != unix.S_IFREG || before.Nlink != 1 || (before.Uid != uint32(os.Getuid()) && (stage || before.Uid != 0)) || before.Size <= 0 || before.Size > 1<<30 {
		return result, errors.New("Owned bounded regular package/helper required")
	}
	if stage {
		p, err := os.Lstat(parent)
		if err != nil || p.Mode().Perm() != 0700 || !strings.HasPrefix(filepath.Base(parent), "sirsi-maintenance-installer-") {
			return result, errors.New("Private normal updater staging required")
		}
	}
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(f, (1<<30)+1))
	if err != nil || n != before.Size || unix.Fstat(fd, &after) != nil || unix.Lstat(path, &named) != nil || before.Dev != after.Dev || before.Ino != after.Ino || after.Dev != named.Dev || after.Ino != named.Ino || before.Size != after.Size || before.Mtim != after.Mtim {
		return result, errors.New("Package changed during readback")
	}
	result.Package = path
	result.SHA256 = hex.EncodeToString(h.Sum(nil))
	result.Device = uint64(after.Dev)
	result.Inode = after.Ino
	result.Bytes = after.Size
	return result, nil
}

func stageMaintenanceInstaller(ctx context.Context, current string, ops maintenanceInstallerOps) (maintenanceInstallerPlan, error) {
	var result maintenanceInstallerPlan
	rel, err := ops.release()
	if err != nil {
		return result, err
	}
	asset, err := maintenancePackageAsset(current, rel)
	if err != nil {
		return result, err
	}
	stage, err := os.MkdirTemp("", "sirsi-maintenance-installer-")
	if err != nil {
		return result, err
	}
	stage, err = filepath.EvalSymlinks(stage)
	if err != nil {
		return result, err
	}
	path := filepath.Join(stage, asset.Name)
	if err := ops.download(ctx, asset.BrowserDownloadURL, path); err != nil {
		return result, fmt.Errorf("Private staging retained at %s: %w", stage, err)
	}
	if err := updater.VerifyChecksum(path, asset.Name, []byte(strings.TrimPrefix(asset.Digest, "sha256:")+"  "+asset.Name+"\n")); err != nil {
		return result, err
	}
	capability, err := ops.inspect(ctx, path, strings.TrimPrefix(rel.TagName, "v"))
	if err != nil {
		return result, err
	}
	result, err = maintenanceInstallerFileHash(path, true)
	if err != nil {
		return result, err
	}
	if result.SHA256 != strings.TrimPrefix(asset.Digest, "sha256:") || result.Bytes != asset.Size {
		return result, errors.New("Inspected package differs from the selected published bytes; no Installer handoff")
	}
	result.Schema = 1
	result.Release = capability.Release
	result.Implementation = capability.Implementation
	return result, nil
}

func newMaintenanceInstallerCmd(ops maintenanceInstallerOps) *cobra.Command {
	var verify string
	cmd := &cobra.Command{Use: "maintenance-installer", Args: cobra.NoArgs, PersistentPreRun: func(*cobra.Command, []string) {}, RunE: func(cmd *cobra.Command, _ []string) error {
		ctx, cancel := context.WithTimeout(cmd.Context(), 120*time.Second)
		defer cancel()
		var plan maintenanceInstallerPlan
		var err error
		if os.Getuid() == 0 || os.Geteuid() == 0 {
			return errors.New("Normal Installer preparation must run in the unprivileged app, not as root")
		}
		if verify == "" {
			plan, err = stageMaintenanceInstaller(ctx, version, ops)
		} else {
			raw, e := hex.DecodeString(verify)
			if e != nil || len(raw) > 8192 {
				return errors.New("Bounded captured installer selection required")
			}
			if json.Unmarshal(raw, &plan) != nil || plan.Schema != 1 {
				return errors.New("Malformed installer selection")
			}
			canonical, _ := json.Marshal(plan)
			if !bytes.Equal(raw, append(canonical, '\n')) {
				return errors.New("Exact captured installer selection required")
			}
			current, e := maintenanceInstallerFileHash(plan.Package, true)
			if e != nil {
				return e
			}
			if current.SHA256 != plan.SHA256 || current.Device != plan.Device || current.Inode != plan.Inode || current.Bytes != plan.Bytes {
				return errors.New("Selected installer changed; check the release again")
			}
			cap, e := ops.inspect(ctx, plan.Package, plan.Release)
			if e != nil {
				return e
			}
			if cap.Implementation != plan.Implementation {
				return errors.New("Installer helper protocol changed")
			}
			final, e := maintenanceInstallerFileHash(plan.Package, true)
			if e != nil || final.SHA256 != plan.SHA256 || final.Device != plan.Device || final.Inode != plan.Inode || final.Bytes != plan.Bytes {
				return errors.New("Selected package changed during final trust assessment")
			}
		}
		if err != nil {
			return err
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(plan)
	}}
	cmd.Flags().StringVar(&verify, "verify-selection", "", "Recheck exact previously captured normal Installer selection; never run it")
	return cmd
}

// Same closed observed PackageInfo contract as scripts/package_info_contract.py.
// The downloaded payload is never installed or its scripts executed here.
type maintenanceXMLNode struct {
	XMLName  xml.Name
	Attrs    []xml.Attr           `xml:",any,attr"`
	Text     string               `xml:",chardata"`
	Children []maintenanceXMLNode `xml:",any"`
}

func maintenanceAttrs(n maintenanceXMLNode, want map[string]string) bool {
	if strings.TrimSpace(n.Text) != "" || len(n.Attrs) != len(want) {
		return false
	}
	seen := map[string]bool{}
	for _, a := range n.Attrs {
		expected, known := want[a.Name.Local]
		if a.Name.Space != "" || seen[a.Name.Local] || !known || expected != a.Value {
			return false
		}
		seen[a.Name.Local] = true
	}
	return true
}
func maintenancePackageInfo(path, release string) error {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NONBLOCK|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	f := os.NewFile(uintptr(fd), path)
	defer f.Close()
	var st unix.Stat_t
	if unix.Fstat(fd, &st) != nil || st.Mode&unix.S_IFMT != unix.S_IFREG || st.Size > 64*1024 || st.Nlink != 1 {
		return errors.New("Bounded regular PackageInfo required")
	}
	data, err := io.ReadAll(io.LimitReader(f, 64*1024+1))
	var after, named unix.Stat_t
	if err != nil || len(data) > 64*1024 || int64(len(data)) != st.Size || unix.Fstat(fd, &after) != nil || unix.Lstat(path, &named) != nil ||
		st.Dev != after.Dev || st.Ino != after.Ino || st.Size != after.Size || st.Mtim != after.Mtim || after.Nlink != 1 || after.Dev != named.Dev || after.Ino != named.Ino {
		return errors.Join(errors.New("PackageInfo changed during bounded readback"), err)
	}
	if bytes.Contains(bytes.ToUpper(data), []byte("<!DOCTYPE")) || bytes.Contains(bytes.ToUpper(data), []byte("<!ENTITY")) {
		return errors.New("PackageInfo entities refused")
	}
	var root maintenanceXMLNode
	if xml.Unmarshal(data, &root) != nil || root.XMLName.Local != "pkg-info" || root.XMLName.Space != "" {
		return errors.New("Normal native PackageInfo identity required")
	}
	{
		required := map[string]string{"overwrite-permissions": "true", "relocatable": "false", "identifier": "ai.sirsi.pantheon", "version": release, "install-location": "/", "postinstall-action": "none"}
		seen := map[string]bool{}
		for _, a := range root.Attrs {
			if a.Name.Space != "" || seen[a.Name.Local] {
				return errors.New("Ambiguous PackageInfo attributes")
			}
			seen[a.Name.Local] = true
			if v, ok := required[a.Name.Local]; ok && a.Value != v {
				return errors.New("PackageInfo replacement contract differs")
			}
		}
		for key := range required {
			if !seen[key] {
				return errors.New("Incomplete PackageInfo replacement contract")
			}
		}
	}
	if strings.TrimSpace(root.Text) != "" || len(root.Children) != 8 {
		return errors.New("Unexpected PackageInfo members")
	}
	seen := map[string]bool{}
	for _, node := range root.Children {
		name := node.XMLName.Local
		if node.XMLName.Space != "" || seen[name] {
			return errors.New("Ambiguous PackageInfo members")
		}
		seen[name] = true
		switch name {
		case "bundle":
			if !maintenanceAttrs(node, map[string]string{"path": "./Applications/Pantheon.app", "id": "ai.sirsi.pantheon", "CFBundleShortVersionString": release, "CFBundleVersion": release}) || len(node.Children) != 0 {
				return errors.New("PackageInfo does not replace the one canonical app")
			}
		case "bundle-version", "upgrade-bundle", "strict-identifier":
			if !maintenanceAttrs(node, map[string]string{}) || len(node.Children) != 1 || node.Children[0].XMLName.Local != "bundle" || node.Children[0].XMLName.Space != "" || !maintenanceAttrs(node.Children[0], map[string]string{"id": "ai.sirsi.pantheon"}) || len(node.Children[0].Children) != 0 {
				return errors.New("PackageInfo upgrade membership differs")
			}
		case "update-bundle", "atomic-update-bundle", "relocate":
			if !maintenanceAttrs(node, map[string]string{}) || len(node.Children) != 0 {
				return errors.New("Conflicting PackageInfo update metadata")
			}
		case "payload":
			if len(node.Children) != 0 || strings.TrimSpace(node.Text) != "" {
				return errors.New("Unexpected PackageInfo payload metadata")
			}
		default:
			return errors.New("Unexpected Installer script or component")
		}
	}
	return nil
}
func init() {
	c := updater.NewClient()
	c.HTTPClient = &http.Client{Timeout: 15 * time.Second}
	launchdCmd.AddCommand(newMaintenanceInstallerCmd(maintenanceInstallerOps{func() (*updater.Release, error) { return c.ReleaseForVersion(version) }, maintenanceDownload, inspectMaintenancePKG}))
}
