import XCTest
@testable import SirsiMenubar

@MainActor
final class RouterBoardUnverifiedInboxTests: XCTestCase {
    func testUnverifiedInboxDecodeIsOptionalAndPreservesPopulatedRow() throws {
        let decoder = JSONDecoder()

        let legacy = try decoder.decode(RouterBoard.self, from: Data(#"{"schema_version":"1.0.0","stranded_inbox":[]}"#.utf8))
        XCTAssertNil(legacy.unverifiedInbox)

        let populated = try decoder.decode(
            RouterBoard.self,
            from: Data(#"{"schema_version":"1.0.0","unverified_inbox":[{"agent_id":"claude-foreign","open_items":2,"os_scope":"remote"}]}"#.utf8)
        )
        let engine = SirsiEngine()
        engine.routerBoard = populated
        XCTAssertEqual(engine.routerUnverifiedInbox.count, 1)
        XCTAssertEqual(engine.routerUnverifiedInbox[0].id, "claude-foreign")
        XCTAssertEqual(engine.routerUnverifiedInbox[0].openItems, 2)
        XCTAssertEqual(engine.routerUnverifiedInbox[0].osScope, "remote")
        XCTAssertEqual(engine.routerUnverifiedInbox[0].scopeLabel, "Remote machine · unverified here")
    }
}
