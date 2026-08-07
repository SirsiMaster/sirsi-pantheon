import AppKit
import SwiftUI

// Programmatic NSApplication entry — no Storyboard, no Xcode project. `.accessory`
// activation policy = LSUIElement (menubar agent, no Dock icon). Built with
// `swift build`; packaged into a .app bundle with a stable CFBundleIdentifier
// (ai.sirsi.pantheon) so macOS TCC keys Full Disk Access on it across reinstalls.
// Top-level main.swift runs on the main thread; assert main-actor isolation so
// we can touch the @MainActor AppDelegate / NSApplication APIs. app.run() blocks.
MainActor.assumeIsolated {
    let argv = CommandLine.arguments

    let usage = """
    Usage: SirsiMenubar [--snapshot <dir> [--width <pt>] [--appearance light|dark]]

    With no arguments, launches the menubar surface (accessory app, no Dock icon).

      --snapshot <dir>          render the popover's key screens to PNGs and exit
      --width <pt>              snapshot width in points (default 380)
      --appearance light|dark   snapshot appearance (default dark)
      -h, --help                print this message and exit
    """

    // WHY THIS BLOCK EXISTS, AND WHY IT IS FIRST: argument handling used to be a
    // single `if let i = argv.firstIndex(of: "--snapshot") … else { app.run() }`.
    // Every argument the snapshot parser did not consume therefore fell through
    // to the LAUNCH path, so `SirsiMenubar --help` opened a second Command Deck
    // panel on the owner's screen instead of printing usage.
    //
    // `--help` was only the reported symptom. The same fall-through swallowed
    // `--snapshot` with no directory, and `--width`/`--appearance` passed without
    // `--snapshot` — three more ways to ask for a CLI answer and get a window.
    // The guard is written against the fall-through, not against `--help`, so a
    // flag added later cannot silently re-open the hole.
    let knownFlags: Set<String> = ["--snapshot", "--width", "--appearance"]
    let flags = argv.dropFirst().filter { $0.hasPrefix("-") }

    func usageFailure(_ reason: String) -> Never {
        FileHandle.standardError.write(Data("SirsiMenubar: \(reason)\n\(usage)\n".utf8))
        exit(2)
    }

    if flags.contains("--help") || flags.contains("-h") {
        print(usage)
        exit(0)
    }
    if let unknown = flags.first(where: { !knownFlags.contains($0) }) {
        usageFailure("unknown flag \(unknown)")
    }

    // `--snapshot <dir>` renders the popover's key screens to PNGs and exits —
    // headless QA proof of what the surface really shows (see Snapshot.swift).
    if let i = argv.firstIndex(of: "--snapshot") {
        guard i + 1 < argv.count, !argv[i + 1].hasPrefix("-") else {
            usageFailure("--snapshot requires a directory")
        }
        var w: CGFloat = 380
        if let j = argv.firstIndex(of: "--width") {
            guard j + 1 < argv.count, let v = Double(argv[j + 1]) else {
                usageFailure("--width requires a number")
            }
            w = CGFloat(v)
        }
        var appearance: ColorScheme = .dark
        if let k = argv.firstIndex(of: "--appearance") {
            guard k + 1 < argv.count, ["light", "dark"].contains(argv[k + 1]) else {
                usageFailure("--appearance requires light or dark")
            }
            appearance = argv[k + 1] == "light" ? .light : .dark
        }
        runSnapshotMode(outDir: argv[i + 1], width: w, appearance: appearance)   // configures NSApp and runs it
    } else {
        // A snapshot-only flag with no `--snapshot` is a mistyped command, not a
        // request to open the surface. Launching here is what made the original
        // bug so hard to see: the app did something plausible instead of failing.
        if let orphan = flags.first {
            usageFailure("\(orphan) requires --snapshot")
        }
        let app = NSApplication.shared
        let delegate = AppDelegate()
        app.delegate = delegate
        app.setActivationPolicy(.accessory)
        app.run()
    }
}
