import AppKit

// Programmatic NSApplication entry — no Storyboard, no Xcode project. `.accessory`
// activation policy = LSUIElement (menubar agent, no Dock icon). Built with
// `swift build`; packaged into a .app bundle with a stable CFBundleIdentifier
// (ai.sirsi.pantheon) so macOS TCC keys Full Disk Access on it across reinstalls.
// Top-level main.swift runs on the main thread; assert main-actor isolation so
// we can touch the @MainActor AppDelegate / NSApplication APIs. app.run() blocks.
MainActor.assumeIsolated {
    // `--snapshot <dir>` renders the popover's key screens to PNGs and exits —
    // headless QA proof of what the surface really shows (see Snapshot.swift).
    let argv = CommandLine.arguments
    if let i = argv.firstIndex(of: "--snapshot"), i + 1 < argv.count {
        var w: CGFloat = 380
        if let j = argv.firstIndex(of: "--width"), j + 1 < argv.count, let v = Double(argv[j + 1]) {
            w = CGFloat(v)
        }
        runSnapshotMode(outDir: argv[i + 1], width: w)   // configures NSApp and runs it
    } else {
        let app = NSApplication.shared
        let delegate = AppDelegate()
        app.delegate = delegate
        app.setActivationPolicy(.accessory)
        app.run()
    }
}
