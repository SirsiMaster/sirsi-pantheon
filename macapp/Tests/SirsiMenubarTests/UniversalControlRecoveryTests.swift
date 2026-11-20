import XCTest
@testable import SirsiMenubar

@MainActor
private final class FakePeerAdapter: UniversalControlPeerAdapter {
    var snapshots: [[UniversalControlPeer]]
    var selections: [String] = []
    var expectedStates: [Bool] = []
    var selectionError: Error?
    init(_ snapshots: [[UniversalControlPeer]]) { self.snapshots = snapshots }
    func peers() async throws -> [UniversalControlPeer] { snapshots.removeFirst() }
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
    private let disconnected = UniversalControlPeer(id: "studio", linked: false, selectable: true)
    private let linked = UniversalControlPeer(id: "studio", linked: true, selectable: true)

    @MainActor func testReconnectsOnlySelectedPeerAndRequiresInputVerification() async {
        let adapter = FakePeerAdapter([[linked], [disconnected], [linked]])
        let result = await UniversalControlRecovery().reconnect(id: "studio", adapter: adapter)
        XCTAssertEqual(result, .linkedNeedsInputVerification)
        XCTAssertEqual(adapter.selections, ["studio", "studio"])
        XCTAssertEqual(adapter.expectedStates, [true, false])
    }

    @MainActor func testAmbiguousTargetNeverActs() async {
        let adapter = FakePeerAdapter([[linked, linked]])
        let result = await UniversalControlRecovery().reconnect(id: "studio", adapter: adapter)
        XCTAssertEqual(result, .targetUnavailable)
        XCTAssertTrue(adapter.selections.isEmpty)
    }

    @MainActor func testUnavailablePeerNeverActs() async {
        let unavailable = UniversalControlPeer(id: "studio", linked: false, selectable: false)
        let adapter = FakePeerAdapter([[unavailable]])
        let result = await UniversalControlRecovery().reconnect(id: "studio", adapter: adapter)
        XCTAssertEqual(result, .targetUnavailable)
        XCTAssertTrue(adapter.selections.isEmpty)
    }

    @MainActor func testDoesNotReconnectAfterUnexpectedDisconnectState() async {
        let adapter = FakePeerAdapter([[linked], [linked]])
        let result = await UniversalControlRecovery().reconnect(id: "studio", adapter: adapter)
        XCTAssertEqual(result, .stateChangedAfterPossibleDisconnect)
        XCTAssertEqual(adapter.selections, ["studio"])
    }

    @MainActor func testSelectionFailureWarnsThatThePeerMayBeDisconnected() async {
        let adapter = FakePeerAdapter([[linked]])
        adapter.selectionError = NSError(domain: "test", code: 1)
        let result = await UniversalControlRecovery().reconnect(id: "studio", adapter: adapter)
        XCTAssertEqual(result, .failedAfterPossibleDisconnect)
        XCTAssertEqual(adapter.selections, ["studio"])
    }

    @MainActor func testCancellationAfterSelectionWarnsOfPossibleDisconnect() async {
        let adapter = FakePeerAdapter([[linked]])
        adapter.selectionError = CancellationError()
        let result = await UniversalControlRecovery().reconnect(id: "studio", adapter: adapter)
        XCTAssertEqual(result, .cancelledAfterPossibleDisconnect)
        XCTAssertEqual(adapter.selections, ["studio"])
    }

    @MainActor func testConnectingAnUnlinkedPeerDoesNotFirstDisconnectIt() async {
        let adapter = FakePeerAdapter([[disconnected], [linked]])
        let result = await UniversalControlRecovery().reconnect(id: "studio", adapter: adapter)
        XCTAssertEqual(result, .linkedNeedsInputVerification)
        XCTAssertEqual(adapter.expectedStates, [false])
    }

    @MainActor func testSecondRecoveryIsRefusedEvenFromAnotherInstance() async {
        let pending = PausingPeerAdapter()
        let first = Task { await UniversalControlRecovery().reconnect(id: "studio", adapter: pending) }
        while !pending.started { await Task.yield() }
        let other = FakePeerAdapter([])
        let second = await UniversalControlRecovery().reconnect(id: "m1", adapter: other)
        XCTAssertEqual(second, .alreadyRunning)
        XCTAssertTrue(other.selections.isEmpty)
        pending.continuation?.resume()
        let firstResult = await first.value
        XCTAssertEqual(firstResult, .targetUnavailable)
    }
}
