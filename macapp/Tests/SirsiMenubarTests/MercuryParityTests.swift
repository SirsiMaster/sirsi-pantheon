import Foundation
import XCTest
@testable import SirsiMenubar

final class MercuryParityTests: XCTestCase {
    private func projection(_ state: String) throws -> MercuryMonitorProjection {
        try JSONDecoder().decode(MercuryMonitorProjection.self, from: JSONSerialization.data(withJSONObject: [
            "state": state, "message": "fixture", "snapshot": ["generated_at": 100, "nodes": [], "links": []]
        ]))
    }

    func testReadStatesAreNotAllRenamedStale() throws {
        for state in ["offline", "invalid", "permission_required", "unconfigured", "empty", "stale"] {
            XCTAssertEqual(try projection(state).displayState(readSucceeded: true, readCompletedAt: 100, elapsed: 0), state.uppercased())
        }
        XCTAssertEqual(try projection("partial").displayState(readSucceeded: true, readCompletedAt: 100, elapsed: 0), "PARTIAL")
        XCTAssertEqual(try projection("partial").displayState(readSucceeded: true, readCompletedAt: 100, elapsed: 7), "STALE")
        XCTAssertEqual(try projection("ok").displayState(readSucceeded: false, readCompletedAt: 100, elapsed: 0), "READ FAILED")
    }

    // Request-start plus enclosing duration is a completion bound, not a new
    // producer timestamp or a claim that the request measured the hardware.
    func testCompletionBoundIncludesRequestLatencyAndDoesNotTrustBehindDisplayClock() throws {
        let start = "1970-01-01T00:01:40Z"
        XCTAssertEqual(try XCTUnwrap(mercuryReadCompletionBound(readAt: start, wallCompletion: 98, requestElapsed: 2)), 102.001, accuracy: 0.00001)
        XCTAssertEqual(mercuryReadCompletionBound(readAt: start, wallCompletion: 105, requestElapsed: 2), 105)
        XCTAssertNil(mercuryReadCompletionBound(readAt: nil, wallCompletion: 100, requestElapsed: 0))
        XCTAssertNil(mercuryReadCompletionBound(readAt: "invalid", wallCompletion: 100, requestElapsed: 0))
        XCTAssertNil(mercuryReadCompletionBound(readAt: start, wallCompletion: 100, requestElapsed: -1))
    }

    // Main exports this file with TestExportMixedFreshnessParityFixture, then
    // supplies the exact bytes to Swift. No hand-authored projected substitute.
    func testActualGoMixedFreshnessWireKeepsEligibleDirectionsAndUnknownAggregates() throws {
        guard let path = ProcessInfo.processInfo.environment["PANTHEON_TEST_MERCURY_MIXED_JSON"] else {
            throw XCTSkip("Supply the actual Go mixed-freshness export for cross-language wire qualification")
        }
        XCTAssertTrue(path.hasPrefix("/"))
        let data = try Data(contentsOf: URL(fileURLWithPath: path))
        XCTAssertLessThanOrEqual(data.count, 1024 * 1024)
        let p = try JSONDecoder().decode(MercuryMonitorProjection.self, from: data)
        let snapshot = try XCTUnwrap(p.snapshot), completedAt = try XCTUnwrap(snapshot.generated_at)
        XCTAssertEqual(p.state, "partial")
        XCTAssertEqual(p.displayState(readSucceeded: true, readCompletedAt: completedAt, elapsed: 0), "PARTIAL")
        let freshNodes = snapshot.nodes.filter { $0.state == "live" }
        XCTAssertFalse(freshNodes.isEmpty)
        for node in freshNodes {
            XCTAssertTrue(p.isCurrent(node, readCompletedAt: completedAt, elapsed: 0))
            XCTAssertEqual(p.cpuBusy(for: node, readSucceeded: true, readCompletedAt: completedAt, elapsed: 0), node.cpu.busy_pct)
            XCTAssertEqual(p.gpuWork(for: node, readSucceeded: true, readCompletedAt: completedAt, elapsed: 0), node.gpu.work_pct)
        }
        let cable = try XCTUnwrap(p.scene?.cables.first { c in
            c.directions.contains { $0.rate == "— MB/s" } && c.directions.contains { $0.rate != "— MB/s" }
        })
        let withheld = try XCTUnwrap(cable.directions.first { $0.rate == "— MB/s" })
        let eligible = try XCTUnwrap(cable.directions.first { $0.rate != "— MB/s" })
        XCTAssertEqual(p.currentRate(for: withheld, cable: cable, readSucceeded: true, readCompletedAt: completedAt, elapsed: 0), "Unknown")
        XCTAssertEqual(p.currentRate(for: eligible, cable: cable, readSucceeded: true, readCompletedAt: completedAt, elapsed: 0), eligible.rate)
        XCTAssertEqual(p.currentRate(for: eligible, cable: cable, readSucceeded: true, readCompletedAt: completedAt, elapsed: 7), "Unknown")
        XCTAssertEqual(p.currentRate(for: eligible, cable: cable, readSucceeded: false, readCompletedAt: completedAt, elapsed: 0), "Unknown")
        let raw = try XCTUnwrap(JSONSerialization.jsonObject(with: data) as? [String: Any])
        XCTAssertTrue(raw["total_gbs"] is NSNull)
        XCTAssertTrue(raw["incoming_gbs"] is NSNull)
        let headline = try XCTUnwrap(raw["headline"] as? [String: Any])
        XCTAssertEqual((headline["wire"] as? [String: Any])?["number"] as? String, "—")
        XCTAssertEqual((headline["compressed"] as? [String: Any])?["number"] as? String, "—")
        XCTAssertEqual((headline["gain"] as? [String: Any])?["number"] as? String, "—")
        let scene = try XCTUnwrap(raw["scene"] as? [String: Any])
        let cables = try XCTUnwrap(scene["cables"] as? [[String: Any]])
        let mixed = try XCTUnwrap(cables.first { $0["id"] as? String == cable.id })
        XCTAssertEqual((mixed["rate"] as? [String: Any])?["number"] as? String, "—")
    }

    @MainActor func testSupersededReadCannotReplaceNewerFailure() async throws {
        let monitor = MercuryMonitor()
        let good = SirsiEngine.JSONCommandRead(data: Data(#"{"state":"ok","message":"older"}"#.utf8), exitStatus: 0, timedOut: false, launchError: nil)
        let failed = SirsiEngine.JSONCommandRead(data: Data(), exitStatus: 1, timedOut: false, launchError: "new failure")
        await monitor.refresh {
            await monitor.refresh { failed }
            return good
        }
        XCTAssertNil(monitor.projection)
        XCTAssertNotNil(monitor.error)
        XCTAssertFalse(monitor.reading)
    }

    @MainActor func testOwnedCancellationNeverPublishesReadAndRefreshHasOneTaskIdentity() async {
        let monitor = MercuryMonitor()
        let task = Task { @MainActor in
            await monitor.refresh {
                withUnsafeCurrentTask { $0?.cancel() }
                return SirsiEngine.JSONCommandRead(data: Data(#"{"state":"ok","message":"cancelled"}"#.utf8), exitStatus: 0, timedOut: false, launchError: nil)
            }
        }
        await task.value
        XCTAssertNil(monitor.projection)
        XCTAssertFalse(monitor.reading)
        let token = UUID()
        XCTAssertNotEqual(MercuryRefreshIdentity(visible: true, request: token), MercuryRefreshIdentity(visible: false, request: token))
        XCTAssertNotEqual(MercuryRefreshIdentity(visible: true, request: token), MercuryRefreshIdentity(visible: true, request: UUID()))
        let root = URL(fileURLWithPath: #filePath).deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent()
        let source = try? String(contentsOf: root.appendingPathComponent("Sources/SirsiMenubar/MercuryWorkspaceView.swift"), encoding: .utf8)
        XCTAssertTrue(source?.contains("Button { refreshRequest = UUID() }") == true)
        XCTAssertTrue(source?.contains(".task(id: MercuryRefreshIdentity(visible: engine.workspaceVisible, request: refreshRequest))") == true)
        XCTAssertFalse(source?.contains("Button { Task { await refresh() }") == true)
    }
}
