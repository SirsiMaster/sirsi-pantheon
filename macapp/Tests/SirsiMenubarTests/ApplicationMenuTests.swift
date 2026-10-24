import AppKit
import XCTest
@testable import SirsiMenubar

@MainActor
private final class QuitMenuTarget: NSObject {
    var calls = 0
    var sender: Any?

    @objc func terminate(_ sender: Any?) {
        calls += 1
        self.sender = sender
    }
}

final class ApplicationMenuTests: XCTestCase {
    @MainActor func testQuitUsesStandardApplicationMenuAndCommandQ() throws {
        let target = QuitMenuTarget()
        let menu = makePantheonMainMenu(quitTarget: target)
        XCTAssertEqual(menu.items.count, 1)
        let application = try XCTUnwrap(menu.items.first)
        XCTAssertEqual(application.title, "Pantheon")
        let submenu = try XCTUnwrap(application.submenu)
        XCTAssertEqual(submenu.title, "Pantheon")
        XCTAssertEqual(submenu.items.count, 1)
        let quit = try XCTUnwrap(submenu.items.first)
        XCTAssertEqual(quit.title, "Quit Pantheon")
        XCTAssertEqual(quit.keyEquivalent, "q")
        XCTAssertEqual(quit.keyEquivalentModifierMask, [.command])
        XCTAssertEqual(quit.action, #selector(NSApplication.terminate(_:)))
        XCTAssertTrue(quit.target === target)
        XCTAssertTrue(quit.isEnabled)
        XCTAssertEqual(target.calls, 0, "Factory must not invoke Quit")
    }

    @MainActor func testQuitActionInvokesOnlyInjectedFakeTarget() throws {
        let target = QuitMenuTarget()
        let menu = makePantheonMainMenu(quitTarget: target)
        let quit = try XCTUnwrap(menu.items.first?.submenu?.items.first)
        let action = try XCTUnwrap(quit.action)
        let actualTarget = try XCTUnwrap(quit.target as? QuitMenuTarget)
        XCTAssertTrue(actualTarget === target)
        XCTAssertTrue(actualTarget.responds(to: action))
        // Invoke the actual factory selector on its fake target. No event is
        // posted, no app is launched, and no real termination target is used.
        _ = actualTarget.perform(action, with: quit)
        XCTAssertEqual(target.calls, 1)
        XCTAssertTrue((target.sender as? NSMenuItem) === quit)
    }

    func testLaunchInstallsFactoryMenuBeforeRun() throws {
        let package = URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent()
        let source = try String(contentsOf: package.appendingPathComponent("Sources/SirsiMenubar/main.swift"), encoding: .utf8)
        let launch = try XCTUnwrap(source.range(of: "case .launch:"))
        let body = source[launch.upperBound...]
        let assignment = try XCTUnwrap(body.range(of: "app.mainMenu = makePantheonMainMenu(quitTarget: app)"))
        let run = try XCTUnwrap(body.range(of: "app.run()"))
        XCTAssertLessThan(assignment.lowerBound, run.lowerBound)
    }
}
