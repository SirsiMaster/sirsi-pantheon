import AppKit
import SwiftUI
import XCTest
@testable import SirsiMenubar

final class MercurySceneTests: XCTestCase {
    private func actualGoRead() throws -> MercuryMonitorProjection {
        guard let path = ProcessInfo.processInfo.environment["PANTHEON_TEST_MERCURY_MIXED_JSON"] else {
            throw XCTSkip("Supply the exact Go mixed-freshness read for scene wire qualification")
        }
        let data = try Data(contentsOf: URL(fileURLWithPath: path))
        XCTAssertLessThanOrEqual(data.count, 1024 * 1024)
        return try JSONDecoder().decode(MercuryMonitorProjection.self, from: data)
    }

    func testActualGoSceneRetainsSharedOrderGeometryUnknownHeadlineAndProvenance() throws {
        let p = try actualGoRead(), scene = try XCTUnwrap(p.scene?.geometry)
        let dashboard = try XCTUnwrap(p.snapshot), headline = try XCTUnwrap(p.headline)
        XCTAssertTrue(scene.isUsable)
        XCTAssertEqual(scene.cards.map(\.id), headline.node_order)
        XCTAssertEqual(Set(scene.cards.map(\.id)), Set(dashboard.nodes.map(\.id)))
        XCTAssertEqual(scene.cards.count, 3)
        XCTAssertEqual(scene.cables.count, dashboard.links.count)
        XCTAssertEqual(headline.wire.number, "—")
        XCTAssertFalse(p.aggregateIsCurrent(readCompletedAt: dashboard.generated_at, elapsed: 0))
        XCTAssertNotNil(dashboard.source?.host)
        XCTAssertNotNil(dashboard.source?.build)
        XCTAssertNotNil(dashboard.generation)
        XCTAssertTrue(dashboard.nodes.allSatisfy { $0.gpu.calibration != nil })
        XCTAssertTrue(dashboard.links.flatMap(\.directions).contains { $0.provenance != nil })
        XCTAssertTrue(scene.cables.allSatisfy { $0.directions.allSatisfy { $0.speed >= 0 && $0.speed <= 600 } })
    }

    func testInvalidGeometryIsNotPromotedToAnInventedNativeLayout() throws {
        XCTAssertEqual(mercuryObservationTime(nil), "Unknown")
        XCTAssertEqual(mercuryObservationTime(.infinity), "Unknown")
        XCTAssertTrue(mercuryObservationTime(1e100).contains("outside calendar display range"))
        for (width, duplicate) in [(0, false), (1120, true)] {
            let card: [String: Any] = ["id": "a", "family": "generic", "x": 20, "y": 180, "width": 280, "height": 300]
            let object: [String: Any] = ["width": width, "height": 540, "cards": duplicate ? [card, card] : [card], "cables": []]
            let data = try JSONSerialization.data(withJSONObject: object)
            let geometry = try JSONDecoder().decode(MercurySceneGeometry.self, from: data)
            XCTAssertFalse(geometry.isUsable)
        }
    }

    @MainActor func testRetainedSceneRendersOnlyWhenExplicitFixtureOutputIsRequested() throws {
        guard let path = ProcessInfo.processInfo.environment["PANTHEON_TEST_MERCURY_RENDER_PNG"] else {
            throw XCTSkip("Opt-in fixture-only native scene render on a supported display host")
        }
        XCTAssertTrue(path.hasPrefix("/"))
        let p = try actualGoRead(), at = try XCTUnwrap(p.snapshot?.generated_at)
        XCTAssertTrue(try XCTUnwrap(p.scene?.geometry).isUsable)
        _ = NSApplication.shared
        let view = VStack(alignment: .leading) {
            Text("Historical Go mixed-freshness fixture · not current telemetry")
            MercurySnapshotPresentation(projection: p, readSucceeded: true, readCompletedAt: at, elapsed: 0)
        }
        .environment(\.snapshotMode, true).environment(\.colorScheme, .dark)
        .frame(width: 1500, height: 1250).background(.black)
        let renderer = ImageRenderer(content: view)
        renderer.scale = 1
        let image = try XCTUnwrap(renderer.nsImage)
        let bitmap = try XCTUnwrap(NSBitmapImageRep(data: try XCTUnwrap(image.tiffRepresentation)))
        let png = try XCTUnwrap(bitmap.representation(using: .png, properties: [:]))
        try png.write(to: URL(fileURLWithPath: path), options: .withoutOverwriting)
    }
}
