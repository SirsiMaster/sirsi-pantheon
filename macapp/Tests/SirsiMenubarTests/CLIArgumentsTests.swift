import XCTest
@testable import SirsiMenubar

final class CLIArgumentsTests: XCTestCase {
    func testLaunchHasNoFlags() throws {
        XCTAssertEqual(try MenubarCommandParser.parse(["SirsiMenubar"]), .launch)
    }

    func testHelpNeverLaunches() throws {
        XCTAssertEqual(try MenubarCommandParser.parse(["SirsiMenubar", "--help"]), .help)
        XCTAssertEqual(try MenubarCommandParser.parse(["SirsiMenubar", "-h"]), .help)
    }

    func testSnapshotCapturesAllOptions() throws {
        XCTAssertEqual(
            try MenubarCommandParser.parse(["SirsiMenubar", "--snapshot", "/tmp/out", "--width", "500", "--appearance", "light"]),
            .snapshot(directory: "/tmp/out", width: 500, appearance: .light)
        )
    }

    func testMalformedOrOrphanFlagsFailClosed() {
        XCTAssertThrowsError(try MenubarCommandParser.parse(["SirsiMenubar", "--snapshot"]))
        XCTAssertThrowsError(try MenubarCommandParser.parse(["SirsiMenubar", "--snapshot", "/tmp/out", "--width", "wide"]))
        XCTAssertThrowsError(try MenubarCommandParser.parse(["SirsiMenubar", "--appearance", "light"]))
        XCTAssertThrowsError(try MenubarCommandParser.parse(["SirsiMenubar", "--nonsense"]))
    }
}
