// Package caskrelease owns Pantheon's canonical Homebrew Cask bytes.
//
// A release DMG is an input to this package only after the signing and
// publication workflow has produced it. Rendering or verifying cask bytes is
// deliberately side-effect free: it neither builds a package nor contacts a
// tap, GitHub, or Homebrew.
package caskrelease

import (
	"fmt"
	"regexp"
)

var (
	versionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z][0-9A-Za-z.-]*)?$`)
	shaPattern     = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// Input is the exact release tuple represented by a Cask. The hash is the
// SHA-256 of the uploaded, signed DMG, never a development package.
type Input struct {
	Version   string
	DMGSHA256 string
}

// Validate rejects ambiguous versions and hashes before any bytes are
// rendered. This keeps the generated URL, version record, and checksum one
// indivisible release tuple.
func (in Input) Validate() error {
	if !versionPattern.MatchString(in.Version) {
		return fmt.Errorf("cask version must be a numeric x.y.z release version, optionally followed by one prerelease suffix")
	}
	if !shaPattern.MatchString(in.DMGSHA256) {
		return fmt.Errorf("DMG SHA-256 must be exactly 64 lowercase hexadecimal characters")
	}
	return nil
}

// Render returns the only cask representation Pantheon may publish for an
// accepted DMG tuple. Exact byte equality is intentional: it rejects duplicate
// Ruby records, conflicting URL behavior, and trailing hand-edited content.
func Render(in Input) ([]byte, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}
	return []byte(fmt.Sprintf(`cask "sirsi-pantheon" do
  version "%s"
  sha256 "%s"

  url "https://github.com/SirsiMaster/sirsi-pantheon/releases/download/v#{version}/SirsiPantheon-#{version}-arm64.dmg"
  name "Sirsi Pantheon"
  desc "DevOps intelligence platform — menu bar monitor + CLI"
  homepage "https://github.com/SirsiMaster/sirsi-pantheon"

  app "Pantheon.app"

  uninstall quit:      "ai.sirsi.pantheon",
            launchctl: "ai.sirsi.pantheon"

  zap trash: [
    "~/.config/pantheon",
    "~/Library/LaunchAgents/ai.sirsi.pantheon.plist",
  ]

  caveats <<~EOS
    Pantheon.app includes both the menu bar monitor and the sirsi CLI.

    To start the menu bar at login:
      cp /Applications/Pantheon.app/Contents/Resources/ai.sirsi.pantheon.plist ~/Library/LaunchAgents/
      launchctl load ~/Library/LaunchAgents/ai.sirsi.pantheon.plist

    Quick start:
      sirsi scan       Find waste on your machine
      sirsi doctor     Check system health
      sirsi ghosts     Find remnants of uninstalled apps
  EOS
end
`, in.Version, in.DMGSHA256)), nil
}

// Verify requires an existing cask to be the exact canonical rendering of the
// supplied release tuple. It is not a permissive Ruby parser: extra records or
// arbitrary code must never acquire release authority by looking similar.
func Verify(actual []byte, in Input) error {
	expected, err := Render(in)
	if err != nil {
		return err
	}
	if string(actual) != string(expected) {
		return fmt.Errorf("cask bytes do not match the canonical Pantheon release rendering")
	}
	return nil
}
