import AppKit
import SwiftUI

// Programmatic NSApplication entry — no Storyboard, no Xcode project. Pantheon
// is a normal macOS application: it has a Dock presence and a full workspace
// window. The menu-bar Eye is a companion entry point to that same workspace,
// never a separate, reduced "menubar-only" product. Built with `swift build`;
// packaged into a .app bundle with a stable CFBundleIdentifier
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
    case .versionJSON:
        do {
            var response = try NativeVersionProtocol.response(
                bundleInfo: Bundle.main.infoDictionary ?? [:],
                executablePath: Bundle.main.executableURL?.resolvingSymlinksInPath().path ?? ""
            )
            response.append(0x0A)
            FileHandle.standardOutput.write(response)
            exit(0)
        } catch {
            FileHandle.standardError.write(Data("SirsiMenubar: stamped version identity unavailable\n".utf8))
            exit(2)
        }
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
        app.setActivationPolicy(.regular)
        let servicesMenu = NSMenu(title: "Services")
        app.servicesMenu = servicesMenu
        let mainMenu = makePantheonMainMenu(target: delegate, servicesMenu: servicesMenu)
        app.mainMenu = mainMenu
        app.windowsMenu = mainMenu.items.first { $0.title == "Window" }?.submenu
        app.helpMenu = mainMenu.items.first { $0.title == "Help" }?.submenu
        app.run()
    }
}
