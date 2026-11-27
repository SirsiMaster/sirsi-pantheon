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
}
