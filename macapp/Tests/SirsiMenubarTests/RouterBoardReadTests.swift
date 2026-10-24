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

    func testCancelledRefreshDoesNotPublishLaunchFailureAndFreshRetryWorks() async throws {
        let engine = SirsiEngine()
        let oldDate = Date(timeIntervalSince1970: 1_792_760_000)
        engine.routerBoard = try JSONDecoder().decode(RouterBoard.self, from: valid)
        engine.routerReadAt = oldDate
        let entered = expectation(description: "actual board loader awaiting read")
        let held = HeldRouterRead(entered: entered)
        let reading = Task { @MainActor in
            await engine.loadRouterBoard(force: true, readJSON: { args in await held.read(args) })
        }
        await fulfillment(of: [entered], timeout: 2)
        XCTAssertTrue(engine.routerLoading)
        reading.cancel()
        // Use the actual capture-to-read adapter implicated by the installed
        // symptom, not a synthetic unrelated error classification.
        let cancelledRead = CommandExecution(cancelled: true).jsonRead
        XCTAssertNotNil(cancelledRead.launchError)
        held.finish(cancelledRead)
        let accepted = await reading.value // join before state assertions/retry
        XCTAssertFalse(accepted)
        XCTAssertEqual(held.arguments, [SirsiEngine.routerBoardReadArguments])
        XCTAssertEqual(engine.routerBoard?.totalPending, 7)
        XCTAssertEqual(engine.routerReadAt, oldDate)
        XCTAssertNil(engine.routerReadError, "cancelled navigation published a reader launch failure")
        XCTAssertFalse(engine.routerLoading)

        let fresh = Data(#"{"schema_version":"1.0.0","total_pending":9,"live_thread_count":3}"#.utf8)
        let retried = await engine.loadRouterBoard(force: true, readJSON: { _ in
            .init(data: fresh, exitStatus: 0, timedOut: false, launchError: nil)
        })
        XCTAssertTrue(retried)
        XCTAssertEqual(engine.routerBoard?.totalPending, 9)
        XCTAssertEqual(engine.routerBoard?.liveThreadCount, 3)
        XCTAssertNotEqual(engine.routerReadAt, oldDate)
        XCTAssertNotNil(engine.routerReadAt)
        XCTAssertNil(engine.routerReadError)
        XCTAssertFalse(engine.routerLoading)
    }

    func testCancelledLateSuccessPreservesPriorBoardTimeAndError() async throws {
        let engine = SirsiEngine()
        let oldDate = Date(timeIntervalSince1970: 1_792_760_000)
        let oldError = "Previous refresh failed; retained observation."
        engine.routerBoard = try JSONDecoder().decode(RouterBoard.self, from: valid)
        engine.routerReadAt = oldDate
        engine.routerReadError = oldError
        let entered = expectation(description: "non-cooperative board reader entered")
        let held = HeldRouterRead(entered: entered)
        let reading = Task { @MainActor in
            await engine.loadRouterBoard(force: true, readJSON: { args in await held.read(args) })
        }
        await fulfillment(of: [entered], timeout: 2)
        reading.cancel()
        held.finish(.init(data: Data(#"{"schema_version":"1.0.0","total_pending":999}"#.utf8),
                          exitStatus: 0, timedOut: false, launchError: nil))
        let accepted = await reading.value
        XCTAssertFalse(accepted, "cancelled task accepted a late successful board")
        XCTAssertEqual(engine.routerBoard?.totalPending, 7, "cancelled read overwrote prior board")
        XCTAssertEqual(engine.routerReadAt, oldDate)
        XCTAssertEqual(engine.routerReadError, oldError)
        XCTAssertFalse(engine.routerLoading)
    }

    func testAlreadyCancelledRefreshDoesNotInvokeReaderOrChangeObservation() async throws {
        let engine = SirsiEngine()
        let oldDate = Date(timeIntervalSince1970: 1_792_760_000)
        engine.routerBoard = try JSONDecoder().decode(RouterBoard.self, from: valid)
        engine.routerReadAt = oldDate
        engine.routerReadError = "Retained previous failure."
        var calls = 0
        let reading = Task { @MainActor in
            await engine.loadRouterBoard(force: true, readJSON: { _ in
                calls += 1
                return .init(data: Data(), exitStatus: 4, timedOut: false, launchError: nil)
            })
        }
        reading.cancel() // same actor, before the queued task can enter
        let accepted = await reading.value
        XCTAssertFalse(accepted)
        XCTAssertEqual(calls, 0, "already-cancelled refresh invoked the reader")
        XCTAssertEqual(engine.routerBoard?.totalPending, 7)
        XCTAssertEqual(engine.routerReadAt, oldDate)
        XCTAssertEqual(engine.routerReadError, "Retained previous failure.")
        XCTAssertFalse(engine.routerLoading)
    }
}

// A deliberately cancellation-insensitive fixture read proves that publication
// is fenced at the real loadRouterBoard entry, not merely by a cooperative
// transport. finish also works before read enters, so a timed-out expectation
// cannot strand a subsequently scheduled fixture continuation.
@MainActor
private final class HeldRouterRead {
    let entered: XCTestExpectation
    private var pending: CheckedContinuation<SirsiEngine.JSONCommandRead, Never>?
    private var finished: SirsiEngine.JSONCommandRead?
    private(set) var arguments: [[String]] = []

    init(entered: XCTestExpectation) { self.entered = entered }

    func read(_ args: [String]) async -> SirsiEngine.JSONCommandRead {
        arguments.append(args)
        entered.fulfill()
        if let finished { return finished }
        return await withCheckedContinuation { pending = $0 }
    }

    func finish(_ result: SirsiEngine.JSONCommandRead) {
        finished = result
        pending?.resume(returning: result)
        pending = nil
    }
}
