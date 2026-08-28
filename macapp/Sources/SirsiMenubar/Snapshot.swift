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

// Snapshot mode — `SirsiMenubar --snapshot <outDir> [--appearance light|dark]`
// renders the popover's key screens to PNGs and exits: headless, display-gated QA proof of what the
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
func runSnapshotMode(outDir: String, width: CGFloat = 380, appearance: ColorScheme = .dark) {
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
        await engine.diagnose(force: true)
        engine.refresh()
        // Self-loading screens draw from ENGINE state — load it here so the
        // harness renders their REAL content, not an eternal loading shell.
        await engine.loadRouterBoard()
        await engine.loadThreads()
        await engine.fetchAutonomous()
        await engine.fetchVitals()
        let insightRaw = await SirsiEngine.run(args: ["insight", "--json", "--no-ai"], stdin: nil)
        let insight = InsightView.decode(insightRaw)
        // Live round-trip proof for the Ask Sirsi query path: exercise the
        // REAL askLocalAI (board endpoint, quirk handling), print the answer,
        // and preload it into the rendered Ask Sirsi screen so visual QA proves
        // the final answer path, not an empty prompt shell.
        let askSirsiProbe = await engine.askLocalAIKnowledgeReport()
        try? askSirsiProbe.write(toFile: outDir + "/ask-sirsi-learned-report.txt",
                                 atomically: true, encoding: .utf8)
        FileHandle.standardOutput.write(Data("ask-sirsi learned report: \(askSirsiProbe.prefix(240))\n".utf8))

        // EVERY top-level drill-in renders here — a screen the harness doesn't
        // draw is a screen that can ship broken past a "walked" sign-off (the
        // #221 RTK lesson). Self-loading views (.task-driven) render their
        // loading shell; ResultViews get real preloaded output.
        var shots: [(name: String, view: AnyView)] = [
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
            ("ask-sirsi", AnyView(AskSirsiView(engine: engine, preloadedAnswer: askSirsiProbe))),
            ("sne-models-engine", AnyView(SNEControlView(preloaded: .snapshotFixture))),
        ]
        // Owner-gated screens render only when the live board has items — the
        // detail view gets its body preloaded (ImageRenderer never runs .task).
        shots.append(("owner-actions", AnyView(OwnerActionsListView(engine: engine))))
        if let first = engine.ownerGatedItems.first {
            let body = await SirsiEngine.ownerItemBody(id: first.id)
            shots.append(("owner-action-detail",
                          AnyView(OwnerActionView(engine: engine, itemID: first.id, preloadedBody: body))))
        }

        for shot in shots {
            let height: CGFloat
            switch shot.name {
            case "ask-sirsi": height = 760
            case "sne-models-engine": height = 980
            default: height = 520
            }
            let renderer = ImageRenderer(content: shot.view
                .environmentObject(Nav())
                .environment(\.snapshotMode, true)
                .environment(\.colorScheme, appearance)
                // Width is a parameter so the harness can prove RESPONSIVE
                // behaviour headlessly: render the same screen at the panel's
                // minSize and at a wide size and diff them. Verifying type
                // scaling by driving the live panel needs an unlocked screen;
                // this does not.
                .frame(width: width, height: height)
                .background(appearance == .light ? Color.white : Color.black)
                .environment(\.sirsiTypeScale, typeScale(forWidth: width)))
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

// This renderer is intentionally fixture-only: unlike --snapshot it executes
// no Sirsi command, network request, or model action. It produces durable
// native visual evidence for the owner-action states without touching a host.
@MainActor
func runPrefixCachePressureFixtureSnapshot(outDir: String, width: CGFloat = 380, appearance: ColorScheme = .dark) {
    let app = NSApplication.shared
    app.setActivationPolicy(.accessory)
    try? FileManager.default.createDirectory(atPath: outDir, withIntermediateDirectories: true)

    Task { @MainActor in
        let requestID = "pressure-fixture-20260828"
        let observationSHA = String(repeating: "a", count: 64)
        let base = SNEReadViewState.snapshotFixture
        func decodeView(_ state: String, confirmation: Bool, authorization: Bool) -> SNEPrefixCachePressureViewState {
            let confirmationJSON = confirmation ? ",\"confirmation\":{\"confirm_token\":\"fixture-confirm-token\",\"action_hash\":\"\(String(repeating: "b", count: 64))\",\"expires_at\":\"2026-08-28T12:02:00Z\",\"preview\":\"Fixture owner confirmation.\"}" : ""
            let authorizationJSON = authorization ? ",\"authorization\":{\"state\":\"accepted\",\"request_id\":\"\(requestID)\",\"artifact_sha256\":\"\(observationSHA)\",\"expires_at_unix\":1724846700}" : ""
            let json = "{\"state\":\"\(state)\",\"receipt\":{\"schema\":\"pantheon.sne-prefix-cache-pressure-receipt.v1\",\"observation\":{\"request_id\":\"\(requestID)\",\"host_id\":\"fixture-host\",\"observed_at_unix\":1724846400,\"expires_at_unix\":1724846700,\"total_ram_bytes\":51539607552,\"available_ram_bytes\":25769803776,\"swap_used_bytes\":0,\"swap_limit_bytes\":8589934592,\"pressure\":\"normal\",\"pressure_source\":\"fixture\",\"swap_measured\":true},\"observation_sha256\":\"\(observationSHA)\"}\(confirmationJSON)\(authorizationJSON)}"
            let decoder = JSONDecoder()
            decoder.dateDecodingStrategy = .iso8601
            return try! decoder.decode(SNEPrefixCachePressureViewState.self, from: Data(json.utf8))
        }
        func evidence(_ kind: String, available: Bool) -> SNEPrefixCachePressureEvidenceState {
            let receipt = kind == "retention" ? "{\"removed_request_ids\":[\"pressure-old\"],\"retained_request_ids\":[\"pressure-new\"]}" : "{\"status\":\"failed\",\"error_code\":\"cache_pressure_execution_interrupted\"}"
            let json = available ? "{\"state\":\"available\",\"evidence_type\":\"\(kind)\",\"identity\":\"\(requestID)\",\"receipt\":\(receipt)}" : "{\"state\":\"unavailable\",\"evidence_type\":\"\(kind)\",\"identity\":\"\(requestID)\"}"
            return try! JSONDecoder().decode(SNEPrefixCachePressureEvidenceState.self, from: Data(json.utf8))
        }
        let shots: [(String, SNEControlView)] = [
            ("prefix-pressure-measure", SNEControlView(preloaded: base)),
            ("prefix-pressure-confirmation", SNEControlView(preloaded: base, prefixCachePressure: decodeView("owner-confirmation-required", confirmation: true, authorization: false))),
            ("prefix-pressure-authorized-unavailable", SNEControlView(preloaded: base, prefixCachePressure: decodeView("authorization-accepted", confirmation: false, authorization: true), executionEvidence: evidence("execution", available: false))),
            ("prefix-pressure-terminal-retention", SNEControlView(preloaded: base, executionEvidence: evidence("execution", available: true), retentionEvidence: evidence("retention", available: true))),
        ]
        for (name, view) in shots {
            let renderer = ImageRenderer(content: view
                .environmentObject(Nav())
                .environment(\.snapshotMode, true)
                .environment(\.colorScheme, appearance)
                .frame(width: width, height: 980)
                .background(appearance == .light ? Color.white : Color.black)
                .environment(\.sirsiTypeScale, typeScale(forWidth: width)))
            renderer.scale = 2.0
            guard let image = renderer.nsImage, let tiff = image.tiffRepresentation,
                  let bitmap = NSBitmapImageRep(data: tiff), let png = bitmap.representation(using: .png, properties: [:]) else {
                FileHandle.standardError.write(Data("fixture snapshot failed: \(name)\n".utf8))
                exit(1)
            }
            try! png.write(to: URL(fileURLWithPath: outDir + "/\(name).png"))
            FileHandle.standardOutput.write(Data("fixture snapshot: \(name).png (\(png.count) bytes)\n".utf8))
        }
        exit(0)
    }
    app.run()
}
