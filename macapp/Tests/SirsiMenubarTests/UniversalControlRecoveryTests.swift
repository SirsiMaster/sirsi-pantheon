import XCTest
@testable import SirsiMenubar

@MainActor
private final class FakePeerAdapter: UniversalControlPeerAdapter {
    var snapshots: [[UniversalControlPeer]]
    var selections: [String] = []
    var expectedStates: [Bool] = []
    var selectionError: Error?
    var observationError: Error?
    var onObservation: (() -> Void)?
    init(_ snapshots: [[UniversalControlPeer]]) { self.snapshots = snapshots }
    func peers() async throws -> [UniversalControlPeer] {
        if let observationError { throw observationError }
        let result = snapshots.removeFirst()
        onObservation?()
        return result
    }
    func selectPeer(id: String, expectedLinked: Bool) async throws {
        selections.append(id)
        expectedStates.append(expectedLinked)
        if let selectionError { throw selectionError }
    }
}

@MainActor
private final class PausingPeerAdapter: UniversalControlPeerAdapter {
    var started = false
    var continuation: CheckedContinuation<Void, Never>?
    func peers() async throws -> [UniversalControlPeer] {
        started = true
        await withCheckedContinuation { continuation = $0 }
        return []
    }
    func selectPeer(id: String, expectedLinked: Bool) async throws {
        XCTFail("No selection should be made")
    }
}

final class UniversalControlRecoveryTests: XCTestCase {
    private let testLockPath = NSTemporaryDirectory() + "sirsi-uc-test-" + UUID().uuidString

    override func tearDown() {
        try? FileManager.default.removeItem(atPath: testLockPath)
        super.tearDown()
    }

    private let disconnected = UniversalControlPeer(id: "studio", linked: false, selectable: true)
    private let linked = UniversalControlPeer(id: "studio", linked: true, selectable: true)

    @MainActor func testReconnectsOnlySelectedPeerAndRequiresInputVerification() async {
        let adapter = FakePeerAdapter([[linked], [disconnected], [linked]])
        let result = await UniversalControlRecovery(lockPath: testLockPath).reconnect(id: "studio", adapter: adapter)
        XCTAssertEqual(result, .linkedNeedsInputVerification)
        XCTAssertEqual(adapter.selections, ["studio", "studio"])
        XCTAssertEqual(adapter.expectedStates, [true, false])
    }

    @MainActor func testAmbiguousTargetNeverActs() async {
        let adapter = FakePeerAdapter([[linked, linked]])
        let result = await UniversalControlRecovery(lockPath: testLockPath).reconnect(id: "studio", adapter: adapter)
        XCTAssertEqual(result, .targetAmbiguous)
        XCTAssertTrue(adapter.selections.isEmpty)
    }

    @MainActor func testUnavailablePeerNeverActs() async {
        let unavailable = UniversalControlPeer(id: "studio", linked: false, selectable: false)
        let adapter = FakePeerAdapter([[unavailable]])
        let result = await UniversalControlRecovery(lockPath: testLockPath).reconnect(id: "studio", adapter: adapter)
        XCTAssertEqual(result, .targetUnavailable)
        XCTAssertTrue(adapter.selections.isEmpty)
    }

    @MainActor func testUnknownConnectionStateNeverActs() async {
        let unknown = UniversalControlPeer(id: "studio", linked: nil, selectable: true)
        let adapter = FakePeerAdapter([[unknown]])
        let result = await UniversalControlRecovery(lockPath: testLockPath).reconnect(id: "studio", adapter: adapter)
        XCTAssertEqual(result, .connectionStateUnknown)
        XCTAssertTrue(adapter.selections.isEmpty)
    }

    @MainActor func testPermissionAndLockedConsoleRefuseBeforeActing() async {
        let adapter = FakePeerAdapter([])
        adapter.observationError = UniversalControlAdapterError.accessibilityPermissionRequired
        let permission = await UniversalControlRecovery(lockPath: testLockPath).reconnect(id: "studio", adapter: adapter)
        XCTAssertEqual(permission, .accessibilityPermissionRequired)
        adapter.observationError = UniversalControlAdapterError.consoleLocked
        let locked = await UniversalControlRecovery(lockPath: testLockPath).reconnect(id: "studio", adapter: adapter)
        XCTAssertEqual(locked, .consoleLocked)
        XCTAssertTrue(adapter.selections.isEmpty)
    }

    @MainActor func testDoesNotReconnectAfterUnexpectedDisconnectState() async {
        let adapter = FakePeerAdapter([[linked], [linked]])
        let result = await UniversalControlRecovery(lockPath: testLockPath).reconnect(id: "studio", adapter: adapter)
        XCTAssertEqual(result, .stateChangedAfterPossibleDisconnect)
        XCTAssertEqual(adapter.selections, ["studio"])
    }

    @MainActor func testUnknownStateAfterDisconnectDoesNotSelectAgain() async {
        let unknown = UniversalControlPeer(id: "studio", linked: nil, selectable: true)
        let adapter = FakePeerAdapter([[linked], [unknown]])
        let result = await UniversalControlRecovery(lockPath: testLockPath).reconnect(id: "studio", adapter: adapter)
        XCTAssertEqual(result, .stateChangedAfterPossibleDisconnect)
        XCTAssertEqual(adapter.selections, ["studio"])
    }

    @MainActor func testSelectionFailureWarnsThatThePeerMayBeDisconnected() async {
        let adapter = FakePeerAdapter([[linked]])
        adapter.selectionError = NSError(domain: "test", code: 1)
        let result = await UniversalControlRecovery(lockPath: testLockPath).reconnect(id: "studio", adapter: adapter)
        XCTAssertEqual(result, .failedAfterPossibleDisconnect)
        XCTAssertEqual(adapter.selections, ["studio"])
    }

    @MainActor func testLockDuringSelectionKeepsOutcomeUncertain() async {
        let adapter = FakePeerAdapter([[linked]])
        adapter.selectionError = UniversalControlAdapterError.consoleLocked
        let result = await UniversalControlRecovery(lockPath: testLockPath).reconnect(id: "studio", adapter: adapter)
        XCTAssertEqual(result, .failedAfterPossibleDisconnect)
        XCTAssertEqual(adapter.selections, ["studio"])
    }

    @MainActor func testCancellationAfterSelectionWarnsOfPossibleDisconnect() async {
        let adapter = FakePeerAdapter([[linked]])
        adapter.selectionError = CancellationError()
        let result = await UniversalControlRecovery(lockPath: testLockPath).reconnect(id: "studio", adapter: adapter)
        XCTAssertEqual(result, .cancelledAfterPossibleDisconnect)
        XCTAssertEqual(adapter.selections, ["studio"])
    }

    @MainActor func testFailedConnectReportsPossibleConnection() async {
        let adapter = FakePeerAdapter([[disconnected]])
        adapter.selectionError = NSError(domain: "test", code: 1)
        let result = await UniversalControlRecovery(lockPath: testLockPath).reconnect(id: "studio", adapter: adapter)
        XCTAssertEqual(result, .failedAfterPossibleConnection)
        XCTAssertEqual(adapter.expectedStates, [false])
    }

    @MainActor func testCancelledConnectReportsPossibleConnection() async {
        let adapter = FakePeerAdapter([[disconnected]])
        adapter.selectionError = CancellationError()
        let result = await UniversalControlRecovery(lockPath: testLockPath).reconnect(id: "studio", adapter: adapter)
        XCTAssertEqual(result, .cancelledAfterPossibleConnection)
        XCTAssertEqual(adapter.expectedStates, [false])
    }

    @MainActor func testConnectingAnUnlinkedPeerDoesNotFirstDisconnectIt() async {
        let adapter = FakePeerAdapter([[disconnected], [linked]])
        let result = await UniversalControlRecovery(lockPath: testLockPath).reconnect(id: "studio", adapter: adapter)
        XCTAssertEqual(result, .linkedNeedsInputVerification)
        XCTAssertEqual(adapter.expectedStates, [false])
    }

    @MainActor func testCancellationAfterObservedLinkPreservesObservedResult() async {
        let adapter = FakePeerAdapter([[disconnected], [linked]])
        var observations = 0
        adapter.onObservation = {
            observations += 1
            if observations == 2 { withUnsafeCurrentTask { $0?.cancel() } }
        }
        let task = Task { await UniversalControlRecovery(lockPath: testLockPath).reconnect(id: "studio", adapter: adapter) }
        let result = await task.value
        XCTAssertEqual(result, .linkedNeedsInputVerification)
        XCTAssertEqual(adapter.expectedStates, [false])
    }

    @MainActor func testSecondRecoveryIsRefusedEvenFromAnotherInstance() async {
        let pending = PausingPeerAdapter()
        let first = Task { await UniversalControlRecovery(lockPath: testLockPath).reconnect(id: "studio", adapter: pending) }
        while !pending.started { await Task.yield() }
        let other = FakePeerAdapter([])
        let second = await UniversalControlRecovery(lockPath: testLockPath).reconnect(id: "m1", adapter: other)
        XCTAssertEqual(second, .alreadyRunning)
        XCTAssertTrue(other.selections.isEmpty)
        pending.continuation?.resume()
        let firstResult = await first.value
        XCTAssertEqual(firstResult, .targetUnavailable)
    }

    @MainActor func testProcessLockRefusesBeforeReadingOrSelectingPeer() async throws {
        let held = try UniversalControlProcessLock(path: testLockPath)
        let adapter = FakePeerAdapter([])
        let result = await UniversalControlRecovery(lockPath: testLockPath).reconnect(id: "studio", adapter: adapter)
        XCTAssertEqual(result, .alreadyRunning)
        XCTAssertTrue(adapter.selections.isEmpty)
        withExtendedLifetime(held) {}
    }

    @MainActor func testUnsafeLockRefusesBeforeReadingOrSelectingPeer() async throws {
        try FileManager.default.createSymbolicLink(atPath: testLockPath, withDestinationPath: "/dev/null")
        let adapter = FakePeerAdapter([])
        let result = await UniversalControlRecovery(lockPath: testLockPath).reconnect(id: "studio", adapter: adapter)
        XCTAssertEqual(result, .lockUnavailable)
        XCTAssertTrue(adapter.selections.isEmpty)
    }

    @MainActor func testReplacedLockRefusesBeforeSelectingPeer() async {
        let adapter = FakePeerAdapter([[linked]])
        adapter.onObservation = {
            try? FileManager.default.removeItem(atPath: self.testLockPath)
            _ = FileManager.default.createFile(atPath: self.testLockPath, contents: nil, attributes: [.posixPermissions: 0o600])
        }
        let result = await UniversalControlRecovery(lockPath: testLockPath).reconnect(id: "studio", adapter: adapter)
        XCTAssertEqual(result, .lockUnavailable)
        XCTAssertTrue(adapter.selections.isEmpty)
    }

    @MainActor func testReplacedLockAfterDisconnectNeverReconnectsBlindly() async {
        let adapter = FakePeerAdapter([[linked], [disconnected]])
        var observations = 0
        adapter.onObservation = {
            observations += 1
            if observations == 2 {
                try? FileManager.default.removeItem(atPath: self.testLockPath)
                _ = FileManager.default.createFile(atPath: self.testLockPath, contents: nil, attributes: [.posixPermissions: 0o600])
            }
        }
        let result = await UniversalControlRecovery(lockPath: testLockPath).reconnect(id: "studio", adapter: adapter)
        XCTAssertEqual(result, .stateChangedAfterPossibleDisconnect)
        XCTAssertEqual(adapter.selections, ["studio"])
    }
}
