import Foundation
import Darwin
import XCTest
@testable import SirsiMenubar

final class UniversalControlProcessLockTests: XCTestCase {
    private let path = NSTemporaryDirectory() + "sirsi-uc-lock-test-" + UUID().uuidString

    override func tearDown() {
        try? FileManager.default.removeItem(atPath: path)
        super.tearDown()
    }

    func testOtherProcessExcludesAndCrashAllowsRetry() throws {
        XCTAssertTrue(FileManager.default.createFile(atPath: path, contents: nil, attributes: [.posixPermissions: 0o600]))
        let child = Process()
        let input = Pipe()
        let output = Pipe()
        child.executableURL = URL(fileURLWithPath: "/usr/bin/python3")
        child.arguments = ["-c", "import fcntl,sys; f=open(sys.argv[1], 'a+'); fcntl.flock(f, fcntl.LOCK_EX); print('ready', flush=True); sys.stdin.readline()", path]
        child.standardInput = input
        child.standardOutput = output
        try child.run()
        defer {
            input.fileHandleForWriting.closeFile()
            if child.isRunning { child.terminate(); child.waitUntilExit() }
        }
        let ready = output.fileHandleForReading.availableData
        XCTAssertEqual(String(data: ready, encoding: .utf8), "ready\n")
        XCTAssertThrowsError(try UniversalControlProcessLock(path: path)) {
            guard case UniversalControlLockError.busy = $0 else { return XCTFail("expected busy: \($0)") }
        }
        XCTAssertEqual(kill(child.processIdentifier, SIGKILL), 0)
        child.waitUntilExit()
        XCTAssertEqual(child.terminationReason, .uncaughtSignal)
        XCTAssertEqual(child.terminationStatus, SIGKILL)
        XCTAssertNoThrow(try UniversalControlProcessLock(path: path))
    }

    func testNormalReleaseAllowsRetry() throws {
        var lock: UniversalControlProcessLock? = try UniversalControlProcessLock(path: path)
        XCTAssertNotNil(lock)
        lock = nil
        XCTAssertNoThrow(try UniversalControlProcessLock(path: path))
    }

    func testSymlinkAndLoosePermissionsAreRejected() throws {
        try FileManager.default.createSymbolicLink(atPath: path, withDestinationPath: "/dev/null")
        XCTAssertThrowsError(try UniversalControlProcessLock(path: path))
        try FileManager.default.removeItem(atPath: path)
        XCTAssertTrue(FileManager.default.createFile(atPath: path, contents: nil, attributes: [.posixPermissions: 0o644]))
        try FileManager.default.setAttributes([.posixPermissions: 0o644], ofItemAtPath: path)
        XCTAssertThrowsError(try UniversalControlProcessLock(path: path))
    }
}
