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
        let monitorJSON = """
        {"state":"ok","message":"Mercury snapshot · fixture-mac","snapshot":{"nodes":[{"id":"fixture-mac","name":"fixture-mac","chip":"Visual fixture silicon","state":"live","observed_at":\(Date().timeIntervalSince1970),"gpu":{"work_pct":12},"cpu":{"busy_pct":30}}]},"scene":{"cables":[]}}
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
}
