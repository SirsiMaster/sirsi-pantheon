package maat

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ReleaseContractPreflight is Ma'at's local, non-executing observation of the
// source-level commercial-release boundary. It is evidence about a checkout,
// not evidence that a DMG, PKG, notarization, tag, cask, or installation exists.
type ReleaseContractPreflight struct {
	SchemaVersion int                  `json:"schema_version"`
	Root          string               `json:"root"`
	Fingerprint   string               `json:"fingerprint"`
	Files         []ObservedSourceFile `json:"files"`
	Verdict       MaatVerdict          `json:"verdict"`
}

// ObservedSourceFile is a stable local source snapshot. The path is relative
// to the resolved root so a user can inspect the same artifact that Ma'at
// assessed without treating an ambient absolute path as portable authority.
type ObservedSourceFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256,omitempty"`
	Size   int64  `json:"size,omitempty"`
	Error  string `json:"error,omitempty"`
}

type releaseContractRequirement struct {
	ID        string
	Path      string
	Needles   []string
	Forbidden []string
	Hint      string
}

var releaseContractRequirements = []releaseContractRequirement{
	{
		ID: "dmg-explicit-artifact-mode", Path: "scripts/build-dmg.sh",
		Needles: []string{"--development", "--release", "DEVELOPER_ID_APPLICATION APPLE_ID APPLE_TEAM_ID APPLE_APP_PASSWORD", "SirsiPantheon-${VERSION}-dev-${ARCH}.dmg", "xcrun notarytool submit", "xcrun stapler validate"},
		Hint:    "Restore the explicit development/release DMG contract, then re-run this Ma'at preflight.",
	},
	{
		ID: "pkg-explicit-artifact-mode", Path: "scripts/build-pkg.sh",
		Needles: []string{"--development", "--release", "DEVELOPER_ID_INSTALLER APPLE_ID APPLE_TEAM_ID APPLE_APP_PASSWORD", "SirsiPantheon-${VERSION}-dev-${ARCH}.pkg", "xcrun notarytool submit", "xcrun stapler validate"},
		Hint:    "Restore the explicit development/release PKG contract, then re-run this Ma'at preflight.",
	},
	{
		ID: "tag-workflow-commercial-mode", Path: ".github/workflows/release.yml",
		Needles: []string{"scripts/build-dmg.sh --release", "scripts/build-pkg.sh --release"},
		Hint:    "Route tag publication only through --release packaging mode, then re-run this Ma'at preflight.",
	},
	{
		ID: "installer-identity-keychain-preflight", Path: ".github/workflows/release.yml",
		Needles: []string{
			"Preflight imported Developer ID Installer identity",
			"DEVELOPER_ID_INSTALLER is required for the commercial Pantheon PKG",
			"Temporary signing keychain does not contain a usable Team 9D382WV988 Developer ID Installer identity",
			"Configured Developer ID Installer identity does not match the identity imported into the temporary signing keychain",
		},
		Hint: "Provide the matching Team 9D382WV988 Developer ID Installer identity in the protected signing certificate bundle, then re-run this Ma'at preflight.",
	},
	{
		ID: "canonical-cask-publication-workflow", Path: ".github/workflows/release.yml",
		Needles: []string{"Publish canonical Homebrew Cask", "cask-release render", "cask-release verify"},
		Forbidden: []string{
			"Bump Homebrew Cask in tap",
			"perl -0pi",
			"git clone --depth 1",
		},
		Hint: "Publish the cask through the one canonical render, readback, and verify route; remove every mutable duplicate publisher.",
	},
	{
		ID: "make-target-separation", Path: "Makefile",
		Needles: []string{"dmg-dev:", "pkg-dev:", "release-dmg:", "release-pkg:", "macapp/Package.swift", "StackLab"},
		Hint:    "Keep local and commercial targets separate while composing the native menubar, CLI, and Stack Lab payload together.",
	},
	{
		ID: "stacklab-release-recipe", Path: "contracts/stacklab/pantheon-release-artifact-recipe-v1.json",
		Needles: []string{"stacklab.recipe.pantheon-release-artifact", "release-artifact-class-contract", "release-native-payload-composition", "commercial-sign-notary-publication-route", "canonical-cask-publication"},
		Hint:    "Restore the Stack Lab release-artifact recipe so the release boundary remains inspectable and replaceable.",
	},
	{
		ID: "canonical-cask-renderer", Path: "internal/caskrelease/cask.go",
		Needles: []string{"func Render", "func Verify", "exact canonical rendering"},
		Hint:    "Restore the side-effect-free canonical cask renderer and exact-byte verifier before re-running this Ma'at preflight.",
	},
	{
		ID: "canonical-cask-command", Path: "cmd/sirsi/cask_release.go",
		Needles: []string{"cask-release", "caskrelease.Render", "caskrelease.Verify"},
		Hint:    "Restore the typed cask render/verify command used by the tagged release workflow.",
	},
	{
		ID: "commercial-update-payload-eligibility", Path: "internal/updater/updater.go",
		Needles: []string{"ErrNoCompleteCommercialRelease", "IsCompleteCommercialRelease", "assetless or partial record"},
		Hint:    "Restore complete commercial-payload eligibility so a bare or partial GitHub release record cannot become an update.",
	},
	{
		ID: "unified-update-install-handoff", Path: "cmd/sirsi/update.go",
		Needles: []string{"Commercial Pantheon updates ship one app payload", "unified app installer", "complete arm64 Pantheon app payload"},
		Hint:    "Restore the one unified app-update handoff; do not advertise a standalone CLI replacement outside the commercial payload.",
	},
	{
		ID: "static-contract-verifier", Path: "scripts/verify-commercial-release-contract.sh",
		Needles: []string{"commercial release contract: pass", "pantheon-release-artifact-recipe-v1.json"},
		Hint:    "Restore the release contract verifier and its Stack Lab recipe binding, then re-run this Ma'at preflight.",
	},
}

// PreflightReleaseContract observes the local source files that define the
// release boundary. It does not invoke make, a shell script, a compiler, an
// external process, a package tool, signing, notarization, or a network API.
// A failed observation returns a block verdict with an evidence-linked repair
// hint rather than leaving an operator with an unexplained status.
func PreflightReleaseContract(root string) (ReleaseContractPreflight, error) {
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return ReleaseContractPreflight{}, fmt.Errorf("maat release preflight: resolve root: %w", err)
	}
	info, err := os.Stat(resolvedRoot)
	if err != nil {
		return ReleaseContractPreflight{}, fmt.Errorf("maat release preflight: inspect root: %w", err)
	}
	if !info.IsDir() {
		return ReleaseContractPreflight{}, fmt.Errorf("maat release preflight: root is not a directory")
	}

	observed := make(map[string]ObservedSourceFile, len(releaseContractRequirements))
	checks := make([]FloorCheck, 0, len(releaseContractRequirements))
	findings := make([]ScreenFinding, 0)
	for _, requirement := range releaseContractRequirements {
		snapshot := observeReleaseSource(resolvedRoot, requirement.Path)
		file := snapshot.File
		observed[requirement.Path] = file
		if file.Error != "" {
			checks = append(checks, FloorCheck{Name: requirement.ID, Passed: false, Detail: file.Error})
			findings = append(findings, releaseContractFinding(requirement, file.Error, file.SHA256))
			continue
		}

		missing := missingReleaseNeedles(string(snapshot.Raw), requirement.Needles)
		if len(missing) > 0 {
			detail := "missing required source contract: " + strings.Join(missing, ", ")
			checks = append(checks, FloorCheck{Name: requirement.ID, Passed: false, Detail: detail})
			findings = append(findings, releaseContractFinding(requirement, detail, file.SHA256))
			continue
		}
		forbidden := presentReleaseForbidden(string(snapshot.Raw), requirement.Forbidden)
		if len(forbidden) > 0 {
			detail := "forbidden source contract present: " + strings.Join(forbidden, ", ")
			checks = append(checks, FloorCheck{Name: requirement.ID, Passed: false, Detail: detail})
			findings = append(findings, releaseContractFinding(requirement, detail, file.SHA256))
			continue
		}
		checks = append(checks, FloorCheck{Name: requirement.ID, Passed: true, Detail: requirement.Path + ":sha256=" + file.SHA256})
	}

	files := make([]ObservedSourceFile, 0, len(observed))
	for _, file := range observed {
		files = append(files, file)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	fingerprint := releaseContractFingerprint(files)
	floorPassed := len(findings) == 0
	weight := 100
	if !floorPassed {
		weight = 35
	}
	verdict, err := Screen(SystemOneScreen{
		Subject: VerdictSubject{
			Kind: "commit", Repo: filepath.Base(resolvedRoot), Ref: "release-artifact-contract",
			HeadSHA: "release-contract:sha256=" + fingerprint, Boundary: "delivery",
		},
		FeatherWeight: weight,
		Confidence:    1,
		Findings:      findings,
		Floor:         FloorResult{Passed: floorPassed, Checks: checks},
		Model:         ModelStamp{Provider: "local:maat-release-contract", Version: "v1", Local: true, LatencyMS: 0},
	})
	if err != nil {
		return ReleaseContractPreflight{}, fmt.Errorf("maat release preflight: construct verdict: %w", err)
	}
	return ReleaseContractPreflight{
		SchemaVersion: SystemOneSchemaVersion,
		Root:          resolvedRoot,
		Fingerprint:   "sha256=" + fingerprint,
		Files:         files,
		Verdict:       verdict,
	}, nil
}

type observedReleaseSource struct {
	File ObservedSourceFile
	Raw  []byte
}

func observeReleaseSource(root, relative string) observedReleaseSource {
	file := ObservedSourceFile{Path: relative}
	path := filepath.Join(root, filepath.FromSlash(relative))
	info, err := os.Lstat(path)
	if err != nil {
		file.Error = "inspect source: " + err.Error()
		return observedReleaseSource{File: file}
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		file.Error = "source must be a regular non-symlink file"
		return observedReleaseSource{File: file}
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		file.Error = "read source: " + err.Error()
		return observedReleaseSource{File: file}
	}
	post, err := os.Lstat(path)
	if err != nil || post.Mode()&os.ModeSymlink != 0 || !post.Mode().IsRegular() || !os.SameFile(info, post) || post.Size() != info.Size() {
		file.Error = "source identity changed during observation"
		return observedReleaseSource{File: file}
	}
	sum := sha256.Sum256(raw)
	file.SHA256 = hex.EncodeToString(sum[:])
	file.Size = info.Size()
	return observedReleaseSource{File: file, Raw: raw}
}

func missingReleaseNeedles(raw string, needles []string) []string {
	missing := make([]string, 0)
	for _, needle := range needles {
		if !strings.Contains(raw, needle) {
			missing = append(missing, needle)
		}
	}
	return missing
}

func presentReleaseForbidden(raw string, forbidden []string) []string {
	present := make([]string, 0)
	for _, needle := range forbidden {
		if strings.Contains(raw, needle) {
			present = append(present, needle)
		}
	}
	return present
}

func releaseContractFinding(requirement releaseContractRequirement, claim, observedHash string) ScreenFinding {
	evidence := requirement.Path
	if observedHash != "" {
		evidence += ":sha256=" + observedHash
	}
	return ScreenFinding{
		ID: requirement.ID, Severity: "block", Category: "release-contract", File: requirement.Path,
		Claim: claim, Evidence: evidence, Confidence: 1, FixHint: requirement.Hint,
	}
}

func releaseContractFingerprint(files []ObservedSourceFile) string {
	hash := sha256.New()
	for _, file := range files {
		fmt.Fprintf(hash, "%s\x00%d\x00%s\x00%s\n", file.Path, file.Size, file.SHA256, file.Error)
	}
	return hex.EncodeToString(hash.Sum(nil))
}
