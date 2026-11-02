import AppKit
import SwiftUI
import XCTest
@testable import SirsiMenubar

final class DiagnosticRepairActionTests: XCTestCase {
    func testMemoryPreviewEntryNeverTreatsOtherCommandsAsReadOnly() {
        XCTAssertTrue(diagnosticOpensMemoryPreview(["relieve", "--memory"]))
        for command in [["relieve", "--memory", "--confirm"], ["relieve"],
                        ["clean", "--confirm"], ["maat", "repair", "liveness-watch"],
                        ["relieve", "--memory", "--force"], ["relieve", "--memory", "--quiet"], []] {
            XCTAssertFalse(diagnosticOpensMemoryPreview(command), "Unexpected preview authority: \(command)")
        }
    }

    func testLegacyMemoryFindingUsesOnlyTheClosedReadOnlyPreviewRoute() throws {
        let args = try XCTUnwrap(legacyDiagnosticRepairArgs(check: "Top Memory Consumers", severity: 2))
        XCTAssertEqual(args, ["relieve", "--memory"])
        XCTAssertTrue(diagnosticOpensMemoryPreview(args))
        XCTAssertNil(legacyDiagnosticRepairArgs(check: "Top Memory Consumers", severity: 1))
        XCTAssertNil(legacyDiagnosticRepairArgs(check: "unregistered diagnostic", severity: 3))
        XCTAssertFalse(diagnosticOpensMemoryPreview(["relieve", "--memory", "--confirm"]))
    }

    func testLegacyRepairReviewProjectsTheFrozenRegisteredArgvAndDirectory() throws {
        let registered = try XCTUnwrap(legacyDiagnosticRepairArgs(check: "binary-drift", severity: 3))
        XCTAssertEqual(registered, ["self-update"])
        XCTAssertFalse(diagnosticOpensMemoryPreview(registered))

        let frozen = remediationArguments(registered)
        let directory = URL(fileURLWithPath: "/owned/project with spaces")
        let scope = actionReviewScope(arguments: frozen, directory: directory)
        let exactLine = try XCTUnwrap(scope.components(separatedBy: "\n\n").first { $0.hasPrefix("Exact arguments: ") })
        let encoded = Data(exactLine.dropFirst("Exact arguments: ".count).utf8)

        XCTAssertEqual(try JSONDecoder().decode([String].self, from: encoded), frozen)
        XCTAssertTrue(scope.contains("Command: sirsi self-update --json"))
        XCTAssertTrue(scope.contains("Project: " + directory.path))
    }

    func testReviewPreservesArgumentBoundariesAndProducingDirectory() throws {
        let args = ["clean", "a path with spaces", "line\nbreak", "--confirm"]
        let directory = URL(fileURLWithPath: "/owned/project with spaces")
        let scope = actionReviewScope(arguments: args, directory: directory)
        let exactLine = try XCTUnwrap(scope.components(separatedBy: "\n\n").first { $0.hasPrefix("Exact arguments: ") })
        let json = Data(exactLine.dropFirst("Exact arguments: ".count).utf8)
        XCTAssertEqual(try JSONDecoder().decode([String].self, from: json), args)
        XCTAssertTrue(scope.contains("Project: " + directory.path))
    }

    @MainActor
    func testTwoLineActionGrowsWithTextSizeWithoutSingleLineBezelClipping() throws {
        let regular = try render(width: 520, scale: 1)
        let enlarged = try render(width: 520, scale: 1.5)
        XCTAssertGreaterThanOrEqual(regular.size.height, 60)
        XCTAssertGreaterThanOrEqual(enlarged.size.height, 72)
        XCTAssertGreaterThan(enlarged.size.height, regular.size.height)
    }

    @MainActor
    func testNarrowEnlargedActionWrapsLongCommandAndTitle() throws {
        let wide = try render(width: 720, scale: 1.5, long: true)
        let narrow = try render(width: 300, scale: 1.5, long: true)
        XCTAssertGreaterThan(narrow.size.height, wide.size.height)
        XCTAssertLessThanOrEqual(narrow.size.width, 300)
    }

    // Render only the inert production label/style. No engine, process,
    // confirmation, live repair or installed-app interaction is performed.
    @MainActor
    private func render(width: CGFloat, scale: CGFloat, long: Bool = false) throws -> NSImage {
        let title = long ? "Restore managed services and verify their recovery" : "Relieve the live cause"
        let command = long ? "sirsi maat repair launchd-disabled --confirm" : "sirsi relieve --memory"
        let content = Button {} label: {
            DiagnosticRepairActionLabel(title: title, command: command, systemImage: "info.circle")
        }
        .buttonStyle(DiagnosticRepairButtonStyle())
        .environment(\.sirsiTypeScale, scale)
        .environment(\.colorScheme, .dark)
        .frame(width: width)
        .fixedSize(horizontal: false, vertical: true)
        .background(PantheonTheme.canvas)
        let renderer = ImageRenderer(content: content)
        renderer.scale = 2
        let image = try XCTUnwrap(renderer.nsImage)
        if let directory = ProcessInfo.processInfo.environment["PANTHEON_REPAIR_LAYOUT_CAPTURE_DIR"] {
            let root = URL(fileURLWithPath: directory, isDirectory: true)
            try FileManager.default.createDirectory(at: root, withIntermediateDirectories: true)
            let bitmap = try XCTUnwrap(image.tiffRepresentation.flatMap(NSBitmapImageRep.init(data:)))
            let data = try XCTUnwrap(bitmap.representation(using: .png, properties: [:]))
            try data.write(to: root.appendingPathComponent("repair-\(Int(width))-\(Int(scale * 100))-\(long ? "long" : "short").png"))
        }
        return image
    }
}
