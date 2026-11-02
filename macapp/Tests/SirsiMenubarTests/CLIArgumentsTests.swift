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

    func testExactVersionJSONProbeNeverFallsThroughToLaunch() throws {
        XCTAssertEqual(try MenubarCommandParser.parse(["SirsiMenubar", "version", "--json"]), .versionJSON)
        for arguments in [
            ["SirsiMenubar", "version"],
            ["SirsiMenubar", "version", "--json", "extra"],
            ["SirsiMenubar", "--json", "version"],
            ["SirsiMenubar", "--version", "--json"]
        ] {
            XCTAssertThrowsError(try MenubarCommandParser.parse(arguments), "Unexpectedly accepted \(arguments)")
        }
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
        XCTAssertThrowsError(try MenubarCommandParser.parse(["SirsiMenubar", "--snapshot", "/tmp/out", "--width", "0"]))
        XCTAssertThrowsError(try MenubarCommandParser.parse(["SirsiMenubar", "--snapshot", "/tmp/out", "--width", "nan"]))
        XCTAssertThrowsError(try MenubarCommandParser.parse(["SirsiMenubar", "--snapshot", "/tmp/out", "--width", "inf"]))
        XCTAssertThrowsError(try MenubarCommandParser.parse(["SirsiMenubar", "--appearance", "light"]))
        XCTAssertThrowsError(try MenubarCommandParser.parse(["SirsiMenubar", "--snapshot", "/tmp/out", "--snapshot", "/tmp/other"]))
        XCTAssertThrowsError(try MenubarCommandParser.parse(["SirsiMenubar", "--nonsense"]))
    }
}
