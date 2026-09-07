import AppKit
import SwiftUI

// Programmatic NSApplication entry — no Storyboard, no Xcode project. `.accessory`
// activation policy = LSUIElement (menubar agent, no Dock icon). Built with
// `swift build`; packaged into a .app bundle with a stable CFBundleIdentifier
// (ai.sirsi.pantheon) so macOS TCC keys Full Disk Access on it across reinstalls.
// Top-level main.swift runs on the main thread; assert main-actor isolation so
// we can touch the @MainActor AppDelegate / NSApplication APIs. app.run() blocks.
MainActor.assumeIsolated {
    let command: MenubarCommand
    do {
        command = try MenubarCommandParser.parse(CommandLine.arguments)
    } catch let error as MenubarCommandError {
        FileHandle.standardError.write(Data("SirsiMenubar: \(error.message)\n\(MenubarCommandParser.usage)\n".utf8))
        exit(2)
    } catch {
        FileHandle.standardError.write(Data("SirsiMenubar: invalid command\n\(MenubarCommandParser.usage)\n".utf8))
        exit(2)
    }

    switch command {
    case .help:
        print(MenubarCommandParser.usage)
        exit(0)
    case let .snapshot(directory, width, appearance):
        let colorScheme: ColorScheme = appearance == .light ? .light : .dark
        runSnapshotMode(outDir: directory, width: CGFloat(width), appearance: colorScheme)
    case .launch:
        // A snapshot-only flag with no `--snapshot` is a mistyped command, not a
        // request to open the surface. Launching here is what made the original
        // bug so hard to see: the app did something plausible instead of failing.
        let app = NSApplication.shared
        let delegate = AppDelegate()
        app.delegate = delegate
        app.setActivationPolicy(.accessory)
        app.run()
    }
}
