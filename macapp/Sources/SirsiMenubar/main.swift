import AppKit

// Programmatic NSApplication entry — no Storyboard, no Xcode project. `.accessory`
// activation policy = LSUIElement (menubar agent, no Dock icon). Built with
// `swift build`; packaged into a .app bundle with a stable CFBundleIdentifier
// (ai.sirsi.pantheon) so macOS TCC keys Full Disk Access on it across reinstalls.
// Top-level main.swift runs on the main thread; assert main-actor isolation so
// we can touch the @MainActor AppDelegate / NSApplication APIs. app.run() blocks.
MainActor.assumeIsolated {
    let app = NSApplication.shared
    let delegate = AppDelegate()
    app.delegate = delegate
    app.setActivationPolicy(.accessory)
    app.run()
}
