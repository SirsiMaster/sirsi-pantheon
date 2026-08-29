cask "sirsi-pantheon" do
  version "0.23.8-beta"
  sha256 "113684229f1d866c1edd267af620455dc9e5d655fd774b5145e6d1413031e221"

  url "https://github.com/SirsiMaster/sirsi-pantheon/releases/download/v#{version}/SirsiPantheon-#{version}-arm64.dmg"
  name "Sirsi Pantheon"
  desc "DevOps intelligence platform — menu bar monitor + CLI"
  homepage "https://github.com/SirsiMaster/sirsi-pantheon"

  depends_on macos: :monterey

  app "Pantheon.app"
  binary "#{appdir}/Pantheon.app/Contents/MacOS/sirsi"

  uninstall launchctl: "ai.sirsi.pantheon",
            quit:      "ai.sirsi.pantheon"

  zap trash: [
    "~/.config/pantheon",
    "~/Library/LaunchAgents/ai.sirsi.pantheon.plist",
  ]

  caveats <<~EOS
    Pantheon.app includes both the menu bar monitor and the sirsi CLI.

    To start the menu bar at login:
      /Applications/Pantheon.app/Contents/MacOS/sirsi surface install gui

    Quick start:
      sirsi scan       Find waste on your machine
      sirsi diagnose   Check system health
      sirsi ghosts     Find remnants of uninstalled apps
  EOS
end
