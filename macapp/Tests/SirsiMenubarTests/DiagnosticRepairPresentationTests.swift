import AppKit
import SwiftUI
import XCTest
@testable import SirsiMenubar

final class DiagnosticRepairPresentationTests: XCTestCase {
    @MainActor
    func testTypedAndLegacyResultSheetsWaitForTheirFrozenApprovalItem() async throws {
        for route in [DiagnosticRepairPresentation.Route.typed, .legacy] {
            let presented = expectation(description: "native repair sheet receives frozen approval")
            let observation = RepairSheetObservation { item in
                XCTAssertEqual(item.approval.args, ["relieve", "--memory", "--json"])
                XCTAssertEqual(item.approval.directory.path, "/owned/project")
                switch (route, item.route) {
                case (.typed, .typed), (.legacy, .legacy): break
                default: XCTFail("The native sheet received a different repair route")
                }
                presented.fulfill()
            }
            let item = DiagnosticRepairPresentation(approval: try approval(), route: route)
            let window = NSWindow(contentRect: NSRect(x: 0, y: 0, width: 420, height: 180),
                                  styleMask: [.titled], backing: .buffered, defer: false)
            window.contentView = NSHostingView(rootView: RepairSheetHarness(item: item, observation: observation))
            window.makeKeyAndOrderFront(nil)
            await fulfillment(of: [presented], timeout: 2)
            XCTAssertNotNil(window.attachedSheet)
            if let sheet = window.attachedSheet { window.endSheet(sheet) }
            window.close()
        }
    }

    private func approval() throws -> DiagnosticRepairApproval {
        let finding = try JSONDecoder().decode(DiagFinding.self, from: JSONSerialization.data(withJSONObject: [
            "check": "Top Memory Consumers", "severity": 2, "message": "Memory pressure",
            "fix": "sirsi relieve --memory", "fixKind": "relief"
        ]))
        return DiagnosticRepairApproval(
            snapshot: FindingObservationSnapshot(current: true, finding: finding),
            args: ["relieve", "--memory", "--json"],
            directory: URL(fileURLWithPath: "/owned/project"))
    }
}

@MainActor
private final class RepairSheetObservation {
    private let onPresentation: (DiagnosticRepairPresentation) -> Void
    private var recorded = false

    init(onPresentation: @escaping (DiagnosticRepairPresentation) -> Void) {
        self.onPresentation = onPresentation
    }

    func record(_ item: DiagnosticRepairPresentation) {
        guard !recorded else { return }
        recorded = true
        onPresentation(item)
    }
}

@MainActor
private struct RepairSheetHarness: View {
    @State private var item: DiagnosticRepairPresentation?
    let scheduledItem: DiagnosticRepairPresentation
    let observation: RepairSheetObservation

    init(item: DiagnosticRepairPresentation, observation: RepairSheetObservation) {
        scheduledItem = item
        self.observation = observation
    }

    var body: some View {
        Color.clear
            .modifier(DiagnosticRepairResultSheet(item: $item) { presented in
                observation.record(presented)
                Text(presented.approval.args.joined(separator: " "))
            })
            .task {
                await Task.yield()
                item = scheduledItem
            }
    }
}
