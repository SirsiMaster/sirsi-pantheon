import XCTest
@testable import SirsiMenubar

@MainActor
final class RouterBoardReadTests: XCTestCase {
    private let valid = Data(#"{"schema_version":"1.0.0","total_pending":7,"live_thread_count":2}"#.utf8)

    func testActualRefreshUsesBoundedAuthProbeAndAcceptsLiveBoard() async {
        let engine = SirsiEngine()
        var calls: [[String]] = []
        let accepted = await engine.loadRouterBoard(force: true, readJSON: { args in
            calls.append(args)
            return .init(data: self.valid, exitStatus: 0, timedOut: false, launchError: nil)
        })
        XCTAssertEqual(calls, [["router", "node-status", "--json", "--probe-timeout-ms", "1000"]])
        XCTAssertTrue(accepted)
        XCTAssertEqual(engine.routerBoard?.totalPending, 7)
        XCTAssertEqual(engine.routerBoard?.liveThreadCount, 2)
        XCTAssertNil(engine.routerReadError)
        XCTAssertNotNil(engine.routerReadAt)
        XCTAssertFalse(engine.routerLoading)
    }

    func testFailedInitialReadsShowFailureRatherThanEmptyFabric() async {
        let reads: [(SirsiEngine.JSONCommandRead, String)] = [
            (.init(data: valid, exitStatus: 0, timedOut: true, launchError: nil), "12-second"),
            (.init(data: Data(), exitStatus: nil, timedOut: false, launchError: "fixture"), "could not be started"),
            (.init(data: valid, exitStatus: 4, timedOut: false, launchError: nil), "exit 4"),
            (.init(data: Data("not JSON".utf8), exitStatus: 0, timedOut: false, launchError: nil), "invalid or missing")
        ]
        for (read, reason) in reads {
            let engine = SirsiEngine()
            let accepted = await engine.loadRouterBoard(force: true, readJSON: { _ in read })
            XCTAssertFalse(accepted)
            XCTAssertNil(engine.routerBoard)
            XCTAssertNil(engine.routerReadAt)
            XCTAssertEqual(engine.routerEmptyStateTitle, "Ra refresh unavailable")
            XCTAssertTrue(engine.routerEmptyStateMessage.contains(reason))
            XCTAssertTrue(engine.routerEmptyStateMessage.contains("Refresh to retry"))
            XCTAssertTrue(engine.routerEmptyStateMessage.contains("Ma'at"))
            XCTAssertFalse(engine.routerLoading)
        }
    }

    func testFailedRefreshRetainsOldBoardWithoutRenewingObservation() async throws {
        let engine = SirsiEngine()
        let oldDate = Date(timeIntervalSince1970: 1_792_760_000)
        engine.routerBoard = try JSONDecoder().decode(RouterBoard.self, from: valid)
        engine.routerReadAt = oldDate
        let accepted = await engine.loadRouterBoard(force: true, readJSON: { _ in
            .init(data: Data(), exitStatus: nil, timedOut: true, launchError: nil)
        })
        XCTAssertFalse(accepted)
        XCTAssertEqual(engine.routerBoard?.totalPending, 7)
        XCTAssertEqual(engine.routerReadAt, oldDate)
        XCTAssertNotNil(engine.routerReadError)
        XCTAssertFalse(engine.routerObservationIsCurrent(at: oldDate))
    }

    func testRetryCanRecoverFromFailureWithoutInventingCachedData() async {
        let engine = SirsiEngine()
        _ = await engine.loadRouterBoard(force: true, readJSON: { _ in
            .init(data: Data(), exitStatus: 3, timedOut: false, launchError: nil)
        })
        XCTAssertNil(engine.routerBoard)
        let accepted = await engine.loadRouterBoard(force: true, readJSON: { _ in
            .init(data: self.valid, exitStatus: 0, timedOut: false, launchError: nil)
        })
        XCTAssertTrue(accepted)
        XCTAssertNil(engine.routerReadError)
        XCTAssertEqual(engine.routerBoard?.totalPending, 7)
    }

    func testInflightRefreshDoesNotStartAnotherReader() async {
        let engine = SirsiEngine()
        engine.routerLoading = true
        var calls = 0
        let accepted = await engine.loadRouterBoard(force: true, readJSON: { _ in
            calls += 1
            return .init(data: self.valid, exitStatus: 0, timedOut: false, launchError: nil)
        })
        XCTAssertFalse(accepted)
        XCTAssertEqual(calls, 0)
        XCTAssertTrue(engine.routerLoading)
    }

    func testNeverReadAndLoadingStatesRemainDistinctFromFailure() {
        let engine = SirsiEngine()
        XCTAssertEqual(engine.routerEmptyStateTitle, "No fabric data yet on this Mac.")
        engine.routerLoading = true
        XCTAssertEqual(engine.routerEmptyStateTitle, "Reading the fabric…")
    }
}
