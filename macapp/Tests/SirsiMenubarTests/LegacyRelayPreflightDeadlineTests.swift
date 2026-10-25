import XCTest
@testable import SirsiMenubar

@MainActor final class LegacyRelayPreflightDeadlineTests: XCTestCase {
    // Intentionally ignores cancellation until explicitly released. The bridge
    // must return without joining this read-only probe; late completion must not
    // continue the authorization path. Every fixture releases its own probe.
    @MainActor private final class StalledProbe {
        let entered: XCTestExpectation
        var continuation: CheckedContinuation<String, Never>?
        init(_ entered: XCTestExpectation) { self.entered = entered }
        func read() async -> String {
            await withCheckedContinuation { continuation in
                self.continuation = continuation
                entered.fulfill()
            }
        }
        func release() {
            let pending = continuation
            continuation = nil
            pending?.resume(returning: String(repeating: "a", count: 64))
        }
    }

    private let args = ["launchd", "legacy-maintenance", "status"]

    func testStalledFirstQualificationReturnsNotStartedBeforeProbeRelease() async {
        let entered = expectation(description: "first qualification entered")
        let returned = expectation(description: "deadline returned without probe completion")
        let stall = StalledProbe(entered)
        var invocations = 0
        let bridge = LegacyRelayPrivilegedBridge(qualify: { await stall.read() },
            identity: { nil }, compatibility: {}, preflightTimeout: 0.05) { _, _ in
                invocations += 1
                return CommandExecution(childState: .exitUnknown)
            }
        let call = Task { @MainActor in
            let result = await bridge.capture(args: self.args, stdin: nil, authorize: { true })
            returned.fulfill()
            return result
        }
        await fulfillment(of: [entered, returned], timeout: 2)
        XCTAssertNotNil(stall.continuation, "Timeout must not depend on releasing the probe")
        stall.release()
        let result = await call.value
        XCTAssertEqual(result.childState, .notStarted)
        XCTAssertNotNil(result.launchError)
        for _ in 0..<8 { await Task.yield() }
        XCTAssertEqual(invocations, 0, "Late completion cannot submit authorization")
    }

    func testLateSecondQualificationCannotSubmit() async {
        let entered = expectation(description: "second qualification entered")
        let returned = expectation(description: "second qualification expired")
        let stall = StalledProbe(entered)
        var probes = 0, invocations = 0
        let bridge = LegacyRelayPrivilegedBridge(qualify: {
            probes += 1
            if probes == 2 { return await stall.read() }
            return String(repeating: "a", count: 64)
        }, identity: { nil }, compatibility: {}, preflightTimeout: 0.05) { _, _ in
            invocations += 1
            return CommandExecution(childState: .exitUnknown)
        }
        let call = Task { @MainActor in
            let result = await bridge.capture(args: self.args, stdin: nil, authorize: { true })
            returned.fulfill()
            return result
        }
        await fulfillment(of: [entered, returned], timeout: 2)
        XCTAssertNotNil(stall.continuation)
        stall.release()
        let result = await call.value
        XCTAssertEqual(result.childState, .notStarted)
        for _ in 0..<8 { await Task.yield() }
        XCTAssertEqual(probes, 2)
        XCTAssertEqual(invocations, 0)
    }

    func testCancellationReturnsBeforeUncooperativeProbeAndNeverSubmits() async {
        let entered = expectation(description: "qualification entered")
        let returned = expectation(description: "cancellation returned")
        let stall = StalledProbe(entered)
        var invocations = 0
        let bridge = LegacyRelayPrivilegedBridge(qualify: { await stall.read() },
            identity: { nil }, compatibility: {}, preflightTimeout: 30) { _, _ in
                invocations += 1
                return CommandExecution(childState: .exitUnknown)
            }
        let call = Task { @MainActor in
            let result = await bridge.capture(args: self.args, stdin: nil, authorize: { true })
            returned.fulfill()
            return result
        }
        await fulfillment(of: [entered], timeout: 2)
        call.cancel()
        await fulfillment(of: [returned], timeout: 2)
        XCTAssertNotNil(stall.continuation)
        stall.release()
        let result = await call.value
        XCTAssertEqual(result.childState, .notStarted)
        for _ in 0..<8 { await Task.yield() }
        XCTAssertEqual(invocations, 0)
    }

    func testConsentConsumedDuringFinalProbePreventsSubmission() async {
        var probes = 0, invocations = 0
        var approved = true
        let bridge = LegacyRelayPrivilegedBridge(qualify: {
            probes += 1
            if probes == 2 { approved = false }
            return String(repeating: "a", count: 64)
        }, identity: { nil }, compatibility: {}) { _, _ in
            invocations += 1
            return CommandExecution(childState: .exitUnknown)
        }
        let result = await bridge.capture(args: args, stdin: nil, authorize: { approved })
        XCTAssertEqual(probes, 2)
        XCTAssertEqual(result.childState, .notStarted)
        XCTAssertNotNil(result.launchError)
        XCTAssertEqual(invocations, 0)
    }
}
