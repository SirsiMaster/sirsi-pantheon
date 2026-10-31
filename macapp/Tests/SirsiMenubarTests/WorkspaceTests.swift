import AppKit
import SwiftUI
import XCTest
@testable import SirsiMenubar

final class WorkspaceTests: XCTestCase {
    func testWorkspaceSearchFindsResponsibilitiesAndRecoversFromEmptyQuery() {
        XCTAssertEqual(PantheonWorkspace.matching("transport"), [.mercury])
        XCTAssertEqual(PantheonWorkspace.matching("MA’AT"), [.maat])
        XCTAssertTrue(PantheonWorkspace.matching("no-such-destination").isEmpty)
        XCTAssertEqual(PantheonWorkspace.matching(" \n ").count, PantheonWorkspace.allCases.count)
    }

    @MainActor func testMountedMaatRootScrollSurvivesSectionUnmountAndRemount() throws {
        _ = NSApplication.shared
        guard let screen = NSScreen.main else {
            throw XCTSkip("A native screen is required to host the AppKit scroll viewport")
        }
        let defaultsName = "MountedMaatScrollTests." + UUID().uuidString
        let defaults = try XCTUnwrap(UserDefaults(suiteName: defaultsName))
        defer { defaults.removePersistentDomain(forName: defaultsName) }

        let session = PantheonWorkspaceSession(defaults: defaults)
        session.select(.maat)
        let engine = SirsiEngine()
        let root = PantheonDesktopView(
            engine: engine,
            session: session,
            maatCasebookPreview: .snapshotPreview,
            loadsInitialWorkspaceData: false
        ).environment(\.snapshotMode, false)
        let hosting = NSHostingView(rootView: root)
        let size = NSSize(width: 1180, height: 760)
        let visible = screen.visibleFrame
        let origin = NSPoint(x: visible.minX + max(0, (visible.width - size.width) / 2),
                             y: visible.minY + max(0, (visible.height - size.height) / 2))
        hosting.sizingOptions = []
        hosting.autoresizingMask = [.width, .height]
        hosting.frame = NSRect(origin: .zero, size: size)
        let window = NSPanel(contentRect: NSRect(origin: origin, size: size),
                             styleMask: [.titled, .closable, .resizable, .nonactivatingPanel],
                             backing: .buffered, defer: false)
        window.isFloatingPanel = false
        window.hidesOnDeactivate = false
        window.becomesKeyOnlyIfNeeded = true
        window.isReleasedWhenClosed = false
        window.contentView = hosting
        window.orderFront(nil)
        defer { window.close() }

        let firstProbe = try XCTUnwrap(waitForMaatScrollProbe(in: hosting))
        let firstScrollView = try XCTUnwrap(firstProbe.enclosingScrollView)
        let firstClip = firstScrollView.contentView
        let document = try XCTUnwrap(firstScrollView.documentView)
        let maximumOffset = document.frame.height - firstClip.bounds.height
        XCTAssertGreaterThan(maximumOffset, 40, "the mounted System One pane must have real vertical overflow")

        let targetOffset = min(80, maximumOffset)
        firstClip.scroll(to: CGPoint(x: firstClip.bounds.origin.x, y: targetOffset))
        firstScrollView.reflectScrolledClipView(firstClip)
        NotificationCenter.default.post(name: NSView.boundsDidChangeNotification, object: firstClip)
        XCTAssertTrue(waitUntil { abs((session.maatPrimaryRootScrollPosition(for: session.navigation(for: .maat))?.wrappedValue ?? 0) - targetOffset) < 1 })
        let staleRootPosition = try XCTUnwrap(
            session.maatPrimaryRootScrollPosition(for: session.navigation(for: .maat))
        )

        session.select(.horus)
        // Model a final AppKit bounds callback already queued by the old
        // mounted observer. The current section must reject its stale write.
        staleRootPosition.wrappedValue = 0
        XCTAssertEqual(staleRootPosition.wrappedValue, targetOffset, accuracy: 1)

        XCTAssertNil(session.maatPrimaryRootScrollPosition(for: session.navigation(for: .horus)))
        XCTAssertTrue(waitUntil { findMaatScrollProbe(in: hosting) == nil },
                      "the selected workspace subtree should unmount before returning")
        session.select(.maat)
        staleRootPosition.wrappedValue = 0
        XCTAssertEqual(staleRootPosition.wrappedValue, targetOffset, accuracy: 1,
                       "a callback from the prior mount must remain stale after returning")
        let restoredProbe = try XCTUnwrap(waitForMaatScrollProbe(in: hosting))
        XCTAssertFalse(restoredProbe === firstProbe, "switching sections must remount the Ma'at subtree")
        let restoredScrollView = try XCTUnwrap(restoredProbe.enclosingScrollView)
        XCTAssertTrue(waitUntil { abs(restoredScrollView.contentView.bounds.origin.y - targetOffset) < 1 },
                      "the real AppKit viewport must restore the root offset after remount")
        XCTAssertEqual(staleRootPosition.wrappedValue, targetOffset, accuracy: 1)
    }

    func testApolloRevisionRetainsTheExactMachineRouteAndEnvelope() {
        XCTAssertNil(plan().revisionError(in: .snapshotPreview))
        XCTAssertNotNil(plan(machine: "removed-peer").revisionError(in: .snapshotPreview))
        XCTAssertNotNil(plan(engine: "other-engine").revisionError(in: .snapshotPreview))
        XCTAssertNotNil(plan(model: "changed-model").revisionError(in: .snapshotPreview))
        XCTAssertNotNil(plan(cores: 1000).revisionError(in: .snapshotPreview))
        XCTAssertNotNil(plan(memory: 256 * 1_073_741_824).revisionError(in: .snapshotPreview))
        XCTAssertNotNil(plan(estates: ["unpublished-estate"]).revisionError(in: .snapshotPreview))
    }

    @MainActor func testDesktopVisualEvidenceWhenRequested() throws {
        guard let directory = ProcessInfo.processInfo.environment["PANTHEON_VISUAL_EVIDENCE"] else {
            throw XCTSkip("Set PANTHEON_VISUAL_EVIDENCE to render native workspace evidence")
        }
        _ = NSApplication.shared
        try FileManager.default.createDirectory(atPath: directory, withIntermediateDirectories: true)
        let engine = SirsiEngine()
        let repo = URL(fileURLWithPath: #filePath).deletingLastPathComponent()
            .deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent()
        let logo = try XCTUnwrap(NSImage(contentsOf: repo.appendingPathComponent("docs/assets/sirsi-logo-white.png")))
        for width in [900.0, 1180.0, 1440.0] {
            let renderer = ImageRenderer(content:
                PantheonDesktopView(engine: engine, previewWorkspace: .command)
                    .environment(\.snapshotMode, true)
                    .environment(\.colorScheme, .dark)
                    .environment(\.pantheonBrandImage, logo)
                    .frame(width: width, height: 1000)
                    .overlay(alignment: .bottomTrailing) {
                        Text("Visual fixture · sources not queried")
                            .font(.system(size: 10)).foregroundStyle(.white)
                            .padding(6).background(Color.black)
                    })
            renderer.scale = 1
            let image = try XCTUnwrap(renderer.nsImage)
            let tiff = try XCTUnwrap(image.tiffRepresentation)
            let bitmap = try XCTUnwrap(NSBitmapImageRep(data: tiff))
            let png = try XCTUnwrap(bitmap.representation(using: .png, properties: [:]))
            try png.write(to: URL(fileURLWithPath: directory + "/overview-\(Int(width)).png"))
        }
        let lane = try decodeMercuryLane(Data(#"[{"agent":"mercury","verdict":"HELD","detail":"Attended session owns this lane","workable":true}]"#.utf8))
        let machines = try decodeMercuryMachines(Data(#"{"machines":[{"id":"fixture-mac","model":"Visual fixture Mac","chip":"Apple silicon","memory_gib":32,"registered_at":"2026-10-23T14:00:00Z","admitted_by":"fixture"}]}"#.utf8)).machines
        let links = try decodeMercuryTransportReceipt(Data(#"{"collected_at":"2026-10-23T14:00:00Z","host":"fixture-mac","macos":"27","ports":[],"port_count_known":true,"thunderbolt_interfaces":[],"rdma":{"family_loaded":"unknown","nvram_enabled":"unknown"},"source":"visual fixture"}"#.utf8))
        let sampleTime = Date().timeIntervalSince1970
        let monitorJSON = """
        {"state":"ok","message":"Mercury snapshot · fixture-mac","snapshot":{"generated_at":\(sampleTime),"nodes":[{"id":"fixture-mac","name":"fixture-mac","chip":"Visual fixture silicon","state":"live","observed_at":\(sampleTime),"age_s":0,"freshness_s":3,"gpu":{"work_pct":12},"cpu":{"busy_pct":30}}],"links":[]},"scene":{"cables":[]}}
        """
        let monitorRead = SirsiEngine.JSONCommandRead(data: Data(monitorJSON.utf8), exitStatus: 0, timedOut: false, launchError: nil)
        let monitor = try decodeMercuryMonitor(read: monitorRead)
        let surfaces: [(PantheonWorkspace, AnyView)] = [
            (.apollo, AnyView(ApolloRunPlannerView(engine: engine, preloadedCatalog: .snapshotPreview))),
            (.mercury, AnyView(MercuryWorkspaceView(engine: engine, preloadedLane: lane, preloadedMachines: machines, preloadedLinks: links, preloadedMonitor: monitor))),
            (.ra, AnyView(RaFabricView(engine: engine))),
            (.maat, AnyView(MaatWorkspaceView(engine: engine, preloadedCasebook: .snapshotPreview, preloadedKnowledge: .snapshotPreview, apolloContext: plan())))
        ]
        for (workspace, content) in surfaces {
            let renderer = ImageRenderer(content:
                PantheonDesktopView(engine: engine, previewWorkspace: workspace, previewContent: content)
                    .environment(\.snapshotMode, true)
                    .environment(\.colorScheme, .dark)
                    .environment(\.pantheonBrandImage, logo)
                    .frame(width: 1180, height: 1400)
                    .overlay(alignment: .bottomTrailing) {
                        Text("Visual fixture · sources not queried")
                            .font(.system(size: 10)).foregroundStyle(.white)
                            .padding(6).background(Color.black)
                    })
            renderer.scale = 1
            let tiff = try XCTUnwrap(renderer.nsImage?.tiffRepresentation)
            let bitmap = try XCTUnwrap(NSBitmapImageRep(data: tiff))
            let png = try XCTUnwrap(bitmap.representation(using: .png, properties: [:]))
            try png.write(to: URL(fileURLWithPath: directory + "/\(workspace.rawValue)-1180.png"))
        }
    }

    @MainActor func testFailedHealthReadPreservesOriginalObservationTimeAndBecomesUnknown() {
        let engine = SirsiEngine()
        let original = Date()
        let good = SirsiEngine.JSONCommandRead(data: Data(#"{"score":100,"findings":[],"status":"green"}"#.utf8), exitStatus: 0, timedOut: false, launchError: nil)
        engine.acceptHealthRead(good, at: original)
        XCTAssertEqual(engine.healthReadAt, original)
        XCTAssertTrue(engine.healthIsCurrent)
        let bad = SirsiEngine.JSONCommandRead(data: Data(), exitStatus: 1, timedOut: false, launchError: nil)
        engine.acceptHealthRead(bad, at: original.addingTimeInterval(10))
        XCTAssertEqual(engine.healthReadAt, original)
        XCTAssertFalse(engine.healthIsCurrent)
        XCTAssertNotNil(engine.healthReadError)
    }

    private func plan(machine: String = "this-mac", engine: String = "apollo-local", model: String = "qualified resident model",
                      cores: Int = 4, memory: Int64 = 8 * 1_073_741_824,
                      estates: [String] = ["cpu", "gpu"]) -> ApolloPlan {
        ApolloPlan(machineID: machine, engineID: engine, residentModel: model,
                   cpuCores: cores, memoryBytes: memory, swapBytes: 0,
                   chipEstates: estates, unavailableEstates: nil)
    }

    @MainActor private func waitForMaatScrollProbe(in root: NSView) -> NSView? {
        let deadline = Date().addingTimeInterval(2)
        repeat {
            root.layoutSubtreeIfNeeded()
            if let probe = findMaatScrollProbe(in: root) { return probe }
            RunLoop.main.run(until: Date().addingTimeInterval(0.02))
        } while Date() < deadline
        return findMaatScrollProbe(in: root)
    }

    @MainActor private func findMaatScrollProbe(in view: NSView) -> NSView? {
        if view.identifier?.rawValue == "maat.system-one.primary-root-scroll-probe" { return view }
        for child in view.subviews {
            if let probe = findMaatScrollProbe(in: child) { return probe }
        }
        return nil
    }

    @MainActor private func waitUntil(_ condition: () -> Bool) -> Bool {
        let deadline = Date().addingTimeInterval(2)
        repeat {
            if condition() { return true }
            RunLoop.main.run(until: Date().addingTimeInterval(0.02))
        } while Date() < deadline
        return condition()
    }
}
