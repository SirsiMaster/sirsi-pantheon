import XCTest
@testable import SirsiMenubar

@MainActor final class MercuryWorkspaceRefreshTests: XCTestCase {
    private struct Result: Equatable { let data: String; let error: String? }
    @MainActor private final class HeldRead {
        let entered: XCTestExpectation
        private var continuation: CheckedContinuation<Result, Never>?
        private var result: Result?
        init(_ entered: XCTestExpectation) { self.entered = entered }
        func read() async -> Result {
            entered.fulfill()
            return await withCheckedContinuation { continuation in
                if let result { continuation.resume(returning: result) }
                else { self.continuation = continuation }
            }
        }
        func release(_ value: Result) {
            guard result == nil else { return }
            result = value
            let waiting = continuation; continuation = nil
            waiting?.resume(returning: value)
        }
    }

    func testCanceledOlderRefreshCannotClearReplacementLoadingOrPublish() async {
        let owner = MercuryWorkspaceRefresh(loading: false)
        let a = HeldRead(expectation(description: "A pending"))
        let b = HeldRead(expectation(description: "B pending"))
        let prior = Result(data: "retained", error: nil)
        var observed = prior
        let old = Task { @MainActor in
            await owner.run(read: { await a.read() }, publish: { observed = $0 })
        }
        await fulfillment(of: [a.entered], timeout: 2)
        old.cancel()
        let replacement = Task { @MainActor in
            await owner.run(read: { await b.read() }, publish: { observed = $0 })
        }
        await fulfillment(of: [b.entered], timeout: 2)
        a.release(Result(data: "late A", error: "canceled A"))
        await old.value
        XCTAssertTrue(owner.loading, "The actual workspace Refresh button must remain disabled while B owns the read")
        XCTAssertEqual(observed, prior, "A cannot overwrite data/error while B is pending")
        let fresh = Result(data: "B", error: nil)
        b.release(fresh)
        await replacement.value
        XCTAssertFalse(owner.loading)
        XCTAssertEqual(observed, fresh)
    }

    func testOlderCompletionCannotOverwriteFinishedNewerRefresh() async {
        let owner = MercuryWorkspaceRefresh(loading: false)
        let a = HeldRead(expectation(description: "A pending"))
        var observed = Result(data: "retained", error: nil)
        let old = Task { @MainActor in
            await owner.run(read: { await a.read() }, publish: { observed = $0 })
        }
        await fulfillment(of: [a.entered], timeout: 2)
        let fresh = Result(data: "B", error: "B's factual error")
        await owner.run(read: { fresh }, publish: { observed = $0 })
        XCTAssertFalse(owner.loading)
        a.release(Result(data: "stale A", error: nil))
        await old.value
        XCTAssertEqual(observed, fresh)
        XCTAssertFalse(owner.loading)
    }

    func testCurrentCancellationClearsItsOwnLoadingWithoutPublishing() async {
        let owner = MercuryWorkspaceRefresh(loading: false)
        let a = HeldRead(expectation(description: "read pending"))
        var publications = 0
        let read = Task { @MainActor in
            await owner.run(read: { await a.read() }, publish: { _ in publications += 1 })
        }
        await fulfillment(of: [a.entered], timeout: 2)
        XCTAssertTrue(owner.loading)
        read.cancel()
        a.release(Result(data: "canceled", error: "not published"))
        await read.value
        XCTAssertFalse(owner.loading)
        XCTAssertEqual(publications, 0)
        await owner.run(read: { Result(data: "fresh retry", error: nil) }, publish: { _ in publications += 1 })
        XCTAssertEqual(publications, 1)
        XCTAssertFalse(owner.loading)
    }
}
