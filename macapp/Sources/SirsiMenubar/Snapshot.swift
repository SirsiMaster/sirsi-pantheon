import AppKit
import SwiftUI

// snapshotMode tells views they are being drawn by ImageRenderer, which renders
// ScrollView viewports as EMPTY (no live scroll host). Views swap a ScrollView
// for a plain stack when set — identical content, no scroll affordance. Never
// set in the real app.
private struct SnapshotModeKey: EnvironmentKey { static let defaultValue = false }
extension EnvironmentValues {
    var snapshotMode: Bool {
        get { self[SnapshotModeKey.self] }
        set { self[SnapshotModeKey.self] = newValue }
    }
}

// Snapshot mode — `SirsiMenubar --snapshot <outDir>` renders the popover's key
// screens to PNGs and exits: headless, display-gated QA proof of what the
// surface really shows, with no Screen Recording TCC and no human at the screen.
//
// How: the harness first runs the SAME CLI calls the live views make
// (`sirsi maat audit --json`, `sirsi net status --json`) and injects the decoded
// CommandResult into the real ResultView, then draws with SwiftUI's
// ImageRenderer. Injection is required because ImageRenderer renders
// synchronously and never runs .task — a self-loading view would render as an
// eternal spinner. (NSHostingView + cacheDisplay was tried first and cannot
// composite SwiftUI's render-server content: captures come back nearly blank.)
@MainActor
func runSnapshotMode(outDir: String) {
    let app = NSApplication.shared
    app.setActivationPolicy(.accessory)
    try? FileManager.default.createDirectory(atPath: outDir, withIntermediateDirectories: true)

    Task { @MainActor in
        let engine = SirsiEngine()
        engine.loadProjectRoot()

        // The real fetches — repo-scoped verbs honor the configured projectRoot
        // exactly as they do when the popover runs them.
        let maat = await SirsiEngine.runResult(args: ["maat", "audit"])
        let net = await SirsiEngine.runResult(args: ["net", "status"])

        let shots: [(name: String, view: AnyView)] = [
            ("home", AnyView(RootView(engine: engine))),
            ("maat-quality", AnyView(ResultView(engine: engine, title: "Ma'at — Quality",
                                                args: ["maat", "audit"], preloaded: maat))),
            ("net-plan", AnyView(ResultView(engine: engine, title: "Net — Plan",
                                            args: ["net", "status"], preloaded: net))),
        ]
        for shot in shots {
            let renderer = ImageRenderer(content: shot.view
                .environment(\.snapshotMode, true)
                .frame(width: 380, height: 520))
            renderer.scale = 2.0
            var wrote = 0
            if let img = renderer.nsImage, let tiff = img.tiffRepresentation,
               let rep = NSBitmapImageRep(data: tiff),
               let png = rep.representation(using: .png, properties: [:]) {
                try? png.write(to: URL(fileURLWithPath: outDir + "/\(shot.name).png"))
                wrote = png.count
            }
            FileHandle.standardOutput.write(Data("snapshot: \(shot.name).png (\(wrote) bytes)\n".utf8))
        }
        exit(0)
    }
    app.run()
}
