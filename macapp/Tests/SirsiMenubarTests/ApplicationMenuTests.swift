import AppKit
import SwiftUI
import XCTest
@testable import SirsiMenubar

private final class MenuTargetDouble: NSObject {}

final class ApplicationMenuTests: XCTestCase {
    @MainActor
    func testMainMenuProvidesNativeMenusAndUsesSuppliedTargets() throws {
        let target = MenuTargetDouble()
        let servicesMenu = NSMenu(title: "Services")
        let menu = makePantheonMainMenu(target: target, servicesMenu: servicesMenu)
        XCTAssertEqual(menu.items.compactMap(\.title), ["Pantheon", "File", "Edit", "View", "Window", "Help"])

        let application = try XCTUnwrap(submenu("Pantheon", in: menu))
        XCTAssertNotNil(application.items.first { $0.title == "About Pantheon" && $0.action == #selector(NSApplication.orderFrontStandardAboutPanel(_:)) })
        XCTAssertTrue(try XCTUnwrap(application.items.first { $0.title == "Services" }?.submenu) === servicesMenu)
        let quit = try XCTUnwrap(application.items.first { $0.title == "Quit Pantheon" })
        XCTAssertEqual(quit.keyEquivalent, "q")
        XCTAssertEqual(quit.keyEquivalentModifierMask, [.command])
        XCTAssertNil(quit.target, "Application commands resolve through AppKit's responder chain")

        let file = try XCTUnwrap(submenu("File", in: menu))
        let choose = try XCTUnwrap(file.items.first { $0.title == "Choose Project…" })
        XCTAssertEqual(choose.keyEquivalent, "o")
        XCTAssertTrue(choose.target === target)
        XCTAssertEqual(file.items.first { $0.title == "Close Window" }?.action, #selector(NSWindow.performClose(_:)))

        let edit = try XCTUnwrap(submenu("Edit", in: menu))
        for title in ["Undo", "Redo", "Cut", "Copy", "Paste", "Select All"] {
            XCTAssertNotNil(edit.items.first { $0.title == title }, "Missing standard Edit command: \(title)")
        }
        XCTAssertTrue(edit.items.allSatisfy { $0.isSeparatorItem || $0.target == nil }, "Edit actions must use the responder chain")

        let view = try XCTUnwrap(submenu("View", in: menu))
        for title in ["Larger Text", "Smaller Text", "Default Text Size", "Search Workspaces…", "Back", "Forward", "Home"] {
            XCTAssertNotNil(view.items.first { $0.title == title }, "Missing View command: \(title)")
        }
        XCTAssertEqual(view.items.first { $0.title == "Search Workspaces…" }?.keyEquivalent, "k")
        let forward = try XCTUnwrap(view.items.first { $0.title == "Forward" })
        XCTAssertEqual(forward.action, #selector(AppDelegate.goForward(_:)))
        XCTAssertTrue(forward.target === target)
        XCTAssertEqual(forward.keyEquivalent, "]")
        XCTAssertEqual(forward.keyEquivalentModifierMask, [.command])

        let window = try XCTUnwrap(submenu("Window", in: menu))
        for title in ["Show Pantheon", "Minimize", "Zoom", "Bring All to Front"] {
            XCTAssertNotNil(window.items.first { $0.title == title }, "Missing Window command: \(title)")
        }
        XCTAssertNotNil(submenu("Help", in: menu)?.items.first { $0.title == "Pantheon Help & Tools" })
    }

    @MainActor
    func testMenuAvailabilityDisablesUnavailableActions() {
        func available(_ action: Selector, size: Int, back: Bool = false,
                       forward: Bool = false, choose: Bool = false) -> Bool {
            PantheonMenuAvailability.isEnabled(action: action, textSizePercent: size,
                                               canGoBack: back, canGoForward: forward,
                                               canChooseProject: choose) ?? true
        }

        XCTAssertFalse(available(#selector(AppDelegate.increaseTextSize(_:)), size: 150))
        XCTAssertTrue(available(#selector(AppDelegate.increaseTextSize(_:)), size: 140))
        XCTAssertFalse(available(#selector(AppDelegate.decreaseTextSize(_:)), size: 100))
        XCTAssertTrue(available(#selector(AppDelegate.decreaseTextSize(_:)), size: 110))
        XCTAssertFalse(available(#selector(AppDelegate.resetTextSize(_:)), size: 100))
        XCTAssertTrue(available(#selector(AppDelegate.resetTextSize(_:)), size: 120))
        XCTAssertFalse(available(#selector(AppDelegate.goBack(_:)), size: 100, back: false))
        XCTAssertTrue(available(#selector(AppDelegate.goBack(_:)), size: 100, back: true))
        XCTAssertFalse(available(#selector(AppDelegate.goForward(_:)), size: 100, forward: false))
        XCTAssertTrue(available(#selector(AppDelegate.goForward(_:)), size: 100, forward: true))
        XCTAssertFalse(available(#selector(AppDelegate.chooseProject(_:)), size: 100, choose: false))
        XCTAssertTrue(available(#selector(AppDelegate.chooseProject(_:)), size: 100, choose: true))
    }

    func testTextSizePreferenceNormalizesAndClamps() {
        let suite = "PantheonTextSizePreferenceTests.\(UUID().uuidString)"
        let defaults = UserDefaults(suiteName: suite)!
        defer { defaults.removePersistentDomain(forName: suite) }

        XCTAssertEqual(PantheonTextSizePreference.current(defaults: defaults), 100)
        defaults.set(125, forKey: PantheonTextSizePreference.key)
        XCTAssertEqual(PantheonTextSizePreference.current(defaults: defaults), 100)
        XCTAssertEqual(defaults.integer(forKey: PantheonTextSizePreference.key), 100)

        PantheonTextSizePreference.set(130, defaults: defaults)
        XCTAssertEqual(PantheonTextSizePreference.current(defaults: defaults), 130)
        PantheonTextSizePreference.adjust(by: 10, defaults: defaults)
        XCTAssertEqual(PantheonTextSizePreference.current(defaults: defaults), 140)
        PantheonTextSizePreference.set(150, defaults: defaults)
        PantheonTextSizePreference.adjust(by: 10, defaults: defaults)
        XCTAssertEqual(PantheonTextSizePreference.current(defaults: defaults), 150)
        PantheonTextSizePreference.set(90, defaults: defaults)
        XCTAssertEqual(PantheonTextSizePreference.current(defaults: defaults), 100)
    }

    @MainActor
    func testBridgeNavigationDispatchResolvesTheCurrentlyActiveWorkspace() {
        let suite = "ApplicationMenuNavigationTests.\(UUID().uuidString)"
        let defaults = UserDefaults(suiteName: suite)!
        defer { defaults.removePersistentDomain(forName: suite) }
        let session = PantheonWorkspaceSession(defaults: defaults)
        let ra = session.navigation(for: .ra)
        let maat = session.navigation(for: .maat)
        ra.push(Text("Ra detail"))
        ra.push(Text("Ra finding"))
        let raFindingID = ra.stack.last!.id
        ra.pop()
        maat.push(Text("Ma'at detail"))
        maat.push(Text("Ma'at finding"))
        let maatFindingID = maat.stack.last!.id
        maat.pop()

        let bridge = PantheonMenuBridge()
        bridge.bindWorkspaceNavigation(session: session)

        session.select(.maat)
        bridge.perform(PantheonMenuAction.forward)
        XCTAssertEqual(maat.stack.last?.id, maatFindingID)
        bridge.perform(PantheonMenuAction.back)
        XCTAssertEqual(maat.stack.count, 1)

        session.select(.ra)
        bridge.perform(PantheonMenuAction.forward)
        XCTAssertEqual(ra.stack.last?.id, raFindingID)
        XCTAssertEqual(maat.stack.count, 1, "menu dispatch must not mutate an inactive workspace")
    }

    func testSupportingTextFloorScalesWithThePreference() {
        XCTAssertEqual(pantheonScaledPointSize(10, scale: 1), 12)
        XCTAssertEqual(pantheonScaledPointSize(10, scale: 1.5), 18)
        XCTAssertEqual(pantheonScaledPointSize(15, scale: 1.5), 22.5)
    }

    @MainActor
    func testChangingTextPreferencePreservesWorkspaceAndNavigationState() {
        let suite = "PantheonSessionContinuityTests.\(UUID().uuidString)"
        let defaults = UserDefaults(suiteName: suite)!
        defer { defaults.removePersistentDomain(forName: suite) }
        let session = PantheonWorkspaceSession(defaults: defaults)
        session.select(.apollo)
        session.navigation(for: .apollo).push(Text("unfinished plan"))

        PantheonTextSizePreference.set(140, defaults: defaults)

        XCTAssertEqual(session.section, .apollo)
        XCTAssertEqual(session.navigation(for: .apollo).stack.count, 1)
        XCTAssertEqual(PantheonTextSizePreference.current(defaults: defaults), 140)
    }

    @MainActor
    func testProjectChoiceRequestFromRetainedReleaseDetailKeepsItsStack() {
        let suite = "PantheonRetainedReleaseChooserTests.\(UUID().uuidString)"
        let defaults = UserDefaults(suiteName: suite)!
        defer { defaults.removePersistentDomain(forName: suite) }
        let session = PantheonWorkspaceSession(defaults: defaults)
        session.select(.release)
        session.navigation(for: .release).push(Text("release draft"))
        let bridge = PantheonMenuBridge()
        var chooserSurfaceRequests = 0
        bridge.presentProjectChooser = { chooserSurfaceRequests += 1 }

        bridge.chooseProject()

        XCTAssertEqual(chooserSurfaceRequests, 1)
        XCTAssertEqual(session.section, .release)
        XCTAssertEqual(session.navigation(for: .release).stack.count, 1)
        XCTAssertNotNil(bridge.pendingProjectChoiceID)
    }

    @MainActor
    func testDeferredProjectChoiceIsCancelledWhenChooserUnregisters() throws {
        let bridge = PantheonMenuBridge()
        bridge.presentProjectChooser = {}
        bridge.canGoForward = true
        bridge.chooseProject()
        let requestID = try XCTUnwrap(bridge.pendingProjectChoiceID)
        let chooserID = UUID()
        var chooserCalls = 0
        bridge.registerProjectChooser(id: chooserID) { chooserCalls += 1 }
        bridge.unregisterProjectChooser(id: chooserID)

        bridge.deliverProjectChoice(chooserID: chooserID, requestID: requestID)

        XCTAssertEqual(chooserCalls, 0)
        XCTAssertNil(bridge.pendingProjectChoiceID)
    }

    @MainActor
    func testDeferredProjectChoiceIsCancelledOnRootTeardown() throws {
        let bridge = PantheonMenuBridge()
        bridge.presentProjectChooser = {}
        bridge.chooseProject()
        let requestID = try XCTUnwrap(bridge.pendingProjectChoiceID)
        let chooserID = UUID()
        var chooserCalls = 0
        bridge.registerProjectChooser(id: chooserID) { chooserCalls += 1 }
        bridge.tearDown()

        bridge.deliverProjectChoice(chooserID: chooserID, requestID: requestID)

        XCTAssertEqual(chooserCalls, 0)
        XCTAssertFalse(bridge.canChooseProject)
        XCTAssertFalse(bridge.canGoBack)
        XCTAssertFalse(bridge.canGoForward)
    }

    @MainActor
    func testSheetDismissalCancelsOnlyItsPendingChooserRegistration() throws {
        let bridge = PantheonMenuBridge()
        bridge.presentProjectChooser = {}
        bridge.actionHandlers[PantheonMenuAction.home] = {}
        bridge.chooseProject()
        let requestID = try XCTUnwrap(bridge.pendingProjectChoiceID)
        let chooserID = UUID()
        var chooserCalls = 0
        bridge.registerProjectChooser(id: chooserID) { chooserCalls += 1 }

        bridge.cancelPendingProjectChoice()
        bridge.unregisterProjectChooser(id: chooserID)
        bridge.deliverProjectChoice(chooserID: chooserID, requestID: requestID)

        XCTAssertEqual(chooserCalls, 0)
        XCTAssertNil(bridge.pendingProjectChoiceID)
        XCTAssertTrue(bridge.canChooseProject, "The window-level fallback remains registered")
        XCTAssertNotNil(bridge.actionHandlers[PantheonMenuAction.home], "Sheet dismissal must not tear down root actions")
    }

    @MainActor
    private func submenu(_ title: String, in menu: NSMenu) -> NSMenu? {
        menu.items.first { $0.title == title }?.submenu
    }
}
