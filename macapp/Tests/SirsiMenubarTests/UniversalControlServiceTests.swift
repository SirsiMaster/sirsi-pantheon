import XCTest
@testable import SirsiMenubar

final class UniversalControlServiceTests: XCTestCase {
    private let running = """
    gui/501/com.apple.ensemble = {
    \tpath = /System/Library/LaunchAgents/com.apple.ensemble.plist
    \tstate = running
    \tprogram = /System/Library/CoreServices/UniversalControl.app/Contents/MacOS/UniversalControl
    \t\tstate = active
    }
    """

    func testRunningIsOnlyLocalServiceObservation() {
        XCTAssertEqual(UniversalControlService.parse(running, complete: true, exitCode: 0), .running)
        XCTAssertEqual(UniversalControlService.parse(running.replacingOccurrences(of: "\tstate = running", with: "\tstate = waiting"), complete: true, exitCode: 0), .notRunning)
    }

    func testFailureAndAmbiguousOrUnexpectedOutputRemainUnknown() {
        XCTAssertEqual(UniversalControlService.parse(running, complete: false, exitCode: 0), .unknown)
        XCTAssertEqual(UniversalControlService.parse(running, complete: true, exitCode: 1), .unknown)
        XCTAssertEqual(UniversalControlService.parse(running.replacingOccurrences(of: "com.apple.ensemble.plist", with: "other.plist"), complete: true, exitCode: 0), .unknown)
        XCTAssertEqual(UniversalControlService.parse(running + "\n\tstate = waiting", complete: true, exitCode: 0), .unknown)
        XCTAssertEqual(UniversalControlService.parse("", complete: true, exitCode: 0), .unknown)
    }
}
