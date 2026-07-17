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
        let rtk = await SirsiEngine.runResult(args: ["rtk", "stats"])
        let ra = await SirsiEngine.runResult(args: ["ra", "status"])
        let seshat = await SirsiEngine.runResult(args: ["seshat", "list"])
        let vault = await SirsiEngine.runResult(args: ["vault", "stats"])
        await engine.diagnose()
        engine.refresh()
        // Self-loading screens draw from ENGINE state — load it here so the
        // harness renders their REAL content, not an eternal loading shell.
        await engine.loadRouterBoard()
        await engine.loadThreads()
        await engine.fetchAutonomous()
        await engine.fetchVitals()
        let insightRaw = await SirsiEngine.run(args: ["insight", "--json", "--no-ai"], stdin: nil)
        let insight = InsightView.decode(insightRaw)

        // EVERY top-level drill-in renders here — a screen the harness doesn't
        // draw is a screen that can ship broken past a "walked" sign-off (the
        // #221 RTK lesson). Self-loading views (.task-driven) render their
        // loading shell; ResultViews get real preloaded output.
        let shots: [(name: String, view: AnyView)] = [
            ("home", AnyView(RootView(engine: engine))),
            ("insight", AnyView(InsightView(engine: engine, preloaded: insight))),
            ("anubis-hygiene", AnyView(AnubisView(engine: engine))),
            ("horus-ops", AnyView(HorusView(engine: engine))),
            ("maat-quality", AnyView(ResultView(engine: engine, title: "Ma'at — Quality",
                                                args: ["maat", "audit"], preloaded: maat))),
            ("thoth-memory", AnyView(ThothMemoryInfoView(engine: engine))),
            ("ra-agent-fleet", AnyView(ResultView(engine: engine, title: "Ra — Agent Fleet",
                                                  args: ["ra", "status"], preloaded: ra))),
            ("router-fabric", AnyView(RouterView(engine: engine))),
            ("threads-heartbeat", AnyView(ThreadsView(engine: engine))),
            ("risk", AnyView(RiskView(engine: engine))),
            ("seshat-knowledge", AnyView(ResultView(engine: engine, title: "Seshat — Knowledge",
                                                    args: ["seshat", "list"], preloaded: seshat))),
            ("net-plan", AnyView(ResultView(engine: engine, title: "Net — Plan",
                                            args: ["net", "status"], preloaded: net))),
            ("vault-context", AnyView(ResultView(engine: engine, title: "Vault — Context",
                                                 args: ["vault", "stats"], preloaded: vault))),
            ("rtk-output-filter", AnyView(ResultView(engine: engine, title: "RTK — Output Filter",
                                                     args: ["rtk", "stats"], preloaded: rtk))),
            ("activity", AnyView(ActivityView(engine: engine))),
            ("ghosts-leftover-apps", AnyView(GhostsView(engine: engine))),
            ("scan-clean", AnyView(ScanCleanView(engine: engine))),
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
