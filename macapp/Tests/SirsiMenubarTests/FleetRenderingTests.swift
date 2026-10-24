import AppKit
import SwiftUI
import XCTest
@testable import SirsiMenubar

@MainActor
final class FleetRenderingTests: XCTestCase {
    func testDecodedUppercaseAndUnknownFixtureRendersRealFleetRows() throws {
        // Synthetic producer-shaped data, not production lane-state evidence.
        // Render the actual row without FleetView's ScrollView or live CLI task.
        let lanes = try JSONDecoder().decode([FleetLane].self, from: Data(#"""
        [
          {"agent":"fixture-working","state":"WORKING","open":16,"active":1,"stalled":0,"blocked":15},
          {"agent":"fixture-assigned","state":"ASSIGNED","open":1,"active":0,"stalled":0,"blocked":0},
          {"agent":"fixture-idle","state":"IDLE_WITH_WORK","open":6,"active":1,"stalled":0,"blocked":5},
          {"agent":"fixture-blocked","state":"BLOCKED","open":1,"active":0,"stalled":0,"blocked":1},
          {"agent":"fixture-unroutable","state":"UNROUTABLE","open":1,"active":0,"stalled":0,"blocked":0},
          {"agent":"fixture-complete","state":"COMPLETE","open":0,"active":0,"stalled":0,"blocked":0},
          {"agent":"fixture-unknown","state":"FUTURE_STATE","open":2,"active":0,"stalled":0,"blocked":0}
        ]
        """#.utf8))
        XCTAssertEqual(lanes.map { FleetStatePresentation.label($0.state) }, [
            "WORKING", "ASSIGNED", "IDLE — WORK WAITING", "BLOCKED",
            "UNROUTABLE", "COMPLETE", "UNKNOWN — REVIEW"
        ])
        let colors: [Color] = [.green, .blue, .orange, .orange, .orange, .secondary, .orange]
        XCTAssertEqual(lanes.map { FleetStatePresentation.color($0.state) }, colors)
        XCTAssertEqual(lanes.first?.open, 16)
        XCTAssertEqual(lanes.first?.active, 1)
        XCTAssertEqual(lanes.last?.open, 2)

        func render(_ lane: FleetLane) throws -> NSBitmapImageRep {
            let renderer = ImageRenderer(content:
                FleetLaneRow(lane: lane)
                    .padding(14).frame(width: 900)
                    .foregroundStyle(.white).background(Color.black)
                    .environment(\.colorScheme, .dark))
            renderer.scale = 2
            let image = try XCTUnwrap(renderer.nsImage, "No row image for \(lane.state)")
            let tiff = try XCTUnwrap(image.tiffRepresentation)
            return try XCTUnwrap(NSBitmapImageRep(data: tiff))
        }

        for lane in lanes {
            let bitmap = try render(lane)
            XCTAssertGreaterThan(bitmap.pixelsWide, 0)
            XCTAssertGreaterThan(bitmap.pixelsHigh, 0)
            // A non-nil empty canvas is not rendering proof. Require actual
            // visible row content against the fixture's black background.
            var hasVisibleContent = false
            for y in 0..<bitmap.pixelsHigh {
                for x in 0..<bitmap.pixelsWide {
                    if let color = bitmap.colorAt(x: x, y: y)?.usingColorSpace(.deviceRGB),
                       max(color.redComponent, max(color.greenComponent, color.blueComponent)) > 0.2 {
                        hasVisibleContent = true
                        break
                    }
                }
                if hasVisibleContent { break }
            }
            XCTAssertTrue(hasVisibleContent, "Blank rendered Fleet row for \(lane.state)")
            let repeatBitmap = try render(lane)
            XCTAssertEqual(
                try XCTUnwrap(bitmap.representation(using: .png, properties: [:])),
                try XCTUnwrap(repeatBitmap.representation(using: .png, properties: [:])),
                "Identical state must render identically before comparing state changes")
            // Hold the identity/counts constant: the real row must actually
            // render its state, not merely produce agent/count text while a
            // disconnected presentation-helper assertion passes separately.
            let alternate = FleetLane(
                agent: lane.agent, state: lane.state == "WORKING" ? "COMPLETE" : "WORKING",
                open: lane.open, active: lane.active, stalled: lane.stalled,
                blocked: lane.blocked, touchedAgo: lane.touchedAgo)
            let alternateBitmap = try render(alternate)
            XCTAssertNotEqual(
                try XCTUnwrap(bitmap.representation(using: .png, properties: [:])),
                try XCTUnwrap(alternateBitmap.representation(using: .png, properties: [:])),
                "Changing only state must change the actual rendered Fleet row for \(lane.state)")
        }
    }
}
