import XCTest
@testable import SirsiMenubar

// Reachability test for the ADR-069 spool outbox decoder (PR #931 follow-up,
// router item 20261001-014718): a decoder that models `outbox[]` but never
// surfaces it to a blocker/view path reintroduces the exact false-quiet bug
// the Go side fixed, one layer up. This asserts the field actually REACHES
// routerHasBlockers / routerOutboxBlockers, not just that it decodes.
@MainActor
final class OutboxReachabilityTests: XCTestCase {
    private func decode(_ json: String) -> RouterBoard {
        try! JSONDecoder().decode(RouterBoard.self, from: Data(json.utf8))
    }

    func testUnreadableOutboxIsNotSilentlyEmpty() {
        let board = decode("""
        {"outbox": [{"agent": "codex-pantheon", "unreadable": true, "error": "permission denied"}]}
        """)
        let engine = SirsiEngine()
        engine.routerBoard = board

        XCTAssertEqual(engine.routerOutboxBlockers.count, 1)
        XCTAssertEqual(engine.routerOutboxBlockers.first?.agent, "codex-pantheon")
        XCTAssertTrue(engine.routerHasBlockers, "an unreadable outbox must surface as a blocker")
    }

    func testQueuedButReadableOutboxIsNotABlocker() {
        let board = decode("""
        {"outbox": [{"agent": "codex-pantheon", "queued_for_retry": 3}]}
        """)
        let engine = SirsiEngine()
        engine.routerBoard = board

        XCTAssertTrue(engine.routerOutboxBlockers.isEmpty)
        XCTAssertFalse(engine.routerHasBlockers, "a merely non-empty retry queue is expected transient state")
    }

    func testAbsentOutboxDecodesCleanly() {
        let board = decode("{}")
        XCTAssertNil(board.outbox)
    }
}
