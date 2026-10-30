package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/SirsiMaster/sirsi-pantheon/internal/packageinventory"
	"github.com/SirsiMaster/sirsi-pantheon/internal/packageinventorycmd"
	"github.com/SirsiMaster/sirsi-pantheon/internal/setup"
)

const managedPantheonApp = "/Applications/Pantheon.app"

type installedApolloBundle struct {
	InfoPlist    string
	Profile      string
	Capability   string
	ProfileBytes []byte
}

type apolloBundleRead func(string) ([]byte, error)
type apolloBundleHash func(string) (string, error)
type apolloBundleLstat func(string) (os.FileInfo, error)

// resolveApolloBundle accepts only the app-embedded CLI or the canonical
// managed-PKG CLI. The managed copy may use app resources only when both
// executables are the same bytes; it never searches PATH or a caller location.
func resolveApolloBundle(executable, managedExecutable, managedApp string, read apolloBundleRead, hash apolloBundleHash, lstat apolloBundleLstat) (installedApolloBundle, error) {
	var result installedApolloBundle
	if !filepath.IsAbs(executable) || filepath.Clean(executable) != executable || read == nil || hash == nil || lstat == nil {
		return result, errors.New("installed Apollo bundle identity unavailable")
	}
	bundle := ""
	if executable == managedExecutable {
		bundle = managedApp
		appEngine := filepath.Join(bundle, "Contents", "MacOS", "sirsi")
		selfHash, selfErr := hash(executable)
		appHash, appErr := hash(appEngine)
		if selfErr != nil || appErr != nil || selfHash == "" || selfHash != appHash {
			return result, errors.New("managed Apollo CLI does not match the installed app engine")
		}
	} else {
		if filepath.Base(executable) != "sirsi" || filepath.Base(filepath.Dir(executable)) != "MacOS" || filepath.Base(filepath.Dir(filepath.Dir(executable))) != "Contents" {
			return result, errors.New("installed Apollo bundle layout unavailable")
		}
		bundle = filepath.Dir(filepath.Dir(filepath.Dir(executable)))
		if filepath.Base(bundle) != "Pantheon.app" || filepath.Join(bundle, "Contents", "MacOS", "sirsi") != executable {
			return result, errors.New("installed Apollo bundle layout unavailable")
		}
	}
	contents := filepath.Join(bundle, "Contents")
	result.InfoPlist = filepath.Join(contents, "Info.plist")
	result.Profile = filepath.Join(contents, "Resources", "Apollo", "native-profile.json")
	info, err := read(result.InfoPlist)
	if err != nil {
		return installedApolloBundle{}, fmt.Errorf("installed Apollo Info.plist unavailable: %w", err)
	}
	result.Capability, err = packageinventorycmd.ApolloCapabilityFromInfoPlist(info)
	if err != nil {
		return installedApolloBundle{}, err
	}
	profileInfo, profileErr := lstat(result.Profile)
	if result.Capability == packageinventory.ApolloCapabilityDisabled {
		if profileErr == nil || !errors.Is(profileErr, os.ErrNotExist) {
			return installedApolloBundle{}, errors.New("installed Apollo capability conflicts with profile presence")
		}
		return result, nil
	}
	if profileErr != nil || profileInfo == nil || !profileInfo.Mode().IsRegular() || profileInfo.Mode()&os.ModeSymlink != 0 {
		return installedApolloBundle{}, errors.New("enabled installed Apollo profile unavailable or unsafe")
	}
	result.ProfileBytes, err = read(result.Profile)
	if err != nil || len(result.ProfileBytes) == 0 || len(result.ProfileBytes) > 64<<10 {
		return installedApolloBundle{}, errors.New("enabled installed Apollo profile unavailable or outside its size bound")
	}
	return result, nil
}

func currentApolloBundle() (installedApolloBundle, error) {
	executable, err := os.Executable()
	if err != nil {
		return installedApolloBundle{}, errors.New("locate installed sirsi executable")
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return installedApolloBundle{}, errors.New("canonicalize installed sirsi executable")
	}
	return resolveApolloBundle(executable, setup.LegacyAppEngine, managedPantheonApp,
		packageinventorycmd.ReadCanonicalFile, apolloNativeFileSHA256, os.Lstat)
}

// apolloNativeFileSHA256 binds file bytes through the package-inventory
// component walker. This is file identity evidence, not later process-image
// launch attestation.
func apolloNativeFileSHA256(path string) (string, error) {
	return packageinventorycmd.HashCanonicalFile(path)
}
