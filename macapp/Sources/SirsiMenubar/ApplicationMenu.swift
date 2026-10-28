import AppKit
import SwiftUI

enum PantheonMenuAction {
    static let search = "search"
    static let back = "back"
    static let home = "home"
    static let help = "help"
}

@MainActor
final class PantheonMenuBridge {
    private var projectChooserID: UUID?
    private var projectChooser: (() -> Void)?
    private var pendingRequestID: UUID?
    var pendingProjectChoiceID: UUID? { pendingRequestID }
    var presentProjectChooser: (() -> Void)?
    var actionHandlers: [String: () -> Void] = [:]
    var canGoBack = false

    var canChooseProject: Bool { projectChooser != nil || presentProjectChooser != nil }

    func registerProjectChooser(id: UUID, handler: @escaping () -> Void) {
        projectChooserID = id
        projectChooser = handler
        guard let requestID = pendingRequestID else { return }
        Task { @MainActor [weak self] in
            await Task.yield()
            self?.deliverProjectChoice(chooserID: id, requestID: requestID)
        }
    }

    func unregisterProjectChooser(id: UUID) {
        guard projectChooserID == id else { return }
        projectChooserID = nil
        projectChooser = nil
        if pendingRequestID != nil { pendingRequestID = nil }
    }

    func chooseProject() {
        if let projectChooser {
            projectChooser()
        } else if pendingRequestID == nil, let presentProjectChooser {
            pendingRequestID = UUID()
            presentProjectChooser()
        }
    }

    func deliverProjectChoice(chooserID: UUID, requestID: UUID) {
        guard projectChooserID == chooserID,
              pendingRequestID == requestID,
              let projectChooser else { return }
        pendingRequestID = nil
        projectChooser()
    }

    func cancelPendingProjectChoice() {
        pendingRequestID = nil
    }

    func tearDown() {
        projectChooserID = nil
        projectChooser = nil
        pendingRequestID = nil
        presentProjectChooser = nil
        actionHandlers.removeAll()
        canGoBack = false
    }
    func perform(_ action: String) { actionHandlers[action]?() }
}

private struct PantheonMenuBridgeKey: EnvironmentKey {
    static let defaultValue: PantheonMenuBridge? = nil
}

extension EnvironmentValues {
    var pantheonMenuBridge: PantheonMenuBridge? {
        get { self[PantheonMenuBridgeKey.self] }
        set { self[PantheonMenuBridgeKey.self] = newValue }
    }
}

@MainActor
enum PantheonMenuAvailability {
    static func isEnabled(action: Selector, textSizePercent: Int,
                          canGoBack: Bool, canChooseProject: Bool) -> Bool? {
        switch action {
        case #selector(AppDelegate.increaseTextSize(_:)):
            return PantheonTextSizePreference.normalized(textSizePercent) < 150
        case #selector(AppDelegate.decreaseTextSize(_:)):
            return PantheonTextSizePreference.normalized(textSizePercent) > 100
        case #selector(AppDelegate.resetTextSize(_:)):
            return PantheonTextSizePreference.normalized(textSizePercent) != 100
        case #selector(AppDelegate.goBack(_:)):
            return canGoBack
        case #selector(AppDelegate.chooseProject(_:)):
            return canChooseProject
        default:
            return nil
        }
    }
}

@MainActor
func makePantheonMainMenu(target: NSObject, servicesMenu: NSMenu) -> NSMenu {
    let mainMenu = NSMenu(title: "Pantheon")
    mainMenu.addItem(menu(title: "Pantheon", items: [
        item("About Pantheon", #selector(NSApplication.orderFrontStandardAboutPanel(_:))),
        menuItem("Services", submenu: servicesMenu),
        NSMenuItem.separator(),
        item("Hide Pantheon", #selector(NSApplication.hide(_:)), key: "h", modifiers: [.command]),
        item("Hide Others", #selector(NSApplication.hideOtherApplications(_:)), key: "h", modifiers: [.command, .option]),
        item("Show All", #selector(NSApplication.unhideAllApplications(_:))),
        NSMenuItem.separator(),
        item("Quit Pantheon", #selector(NSApplication.terminate(_:)), key: "q", modifiers: [.command])
    ]))
    mainMenu.addItem(menu(title: "File", items: [
        item("Choose Project…", #selector(AppDelegate.chooseProject(_:)), target: target, key: "o", modifiers: [.command]),
        NSMenuItem.separator(),
        item("Close Window", #selector(NSWindow.performClose(_:)), target: nil, key: "w", modifiers: [.command])
    ]))
    mainMenu.addItem(menu(title: "Edit", items: [
        item("Undo", Selector("undo:"), key: "z", modifiers: [.command]),
        item("Redo", Selector("redo:"), key: "z", modifiers: [.command, .shift]),
        NSMenuItem.separator(),
        item("Cut", Selector("cut:"), key: "x", modifiers: [.command]),
        item("Copy", Selector("copy:"), key: "c", modifiers: [.command]),
        item("Paste", Selector("paste:"), key: "v", modifiers: [.command]),
        item("Select All", Selector("selectAll:"), key: "a", modifiers: [.command])
    ]))
    mainMenu.addItem(menu(title: "View", items: [
        item("Larger Text", #selector(AppDelegate.increaseTextSize(_:)), target: target, key: "+", modifiers: [.command]),
        item("Smaller Text", #selector(AppDelegate.decreaseTextSize(_:)), target: target, key: "-", modifiers: [.command]),
        item("Default Text Size", #selector(AppDelegate.resetTextSize(_:)), target: target, key: "0", modifiers: [.command]),
        NSMenuItem.separator(),
        item("Search Workspaces…", #selector(AppDelegate.searchWorkspaces(_:)), target: target, key: "k", modifiers: [.command]),
        item("Back", #selector(AppDelegate.goBack(_:)), target: target, key: "[", modifiers: [.command]),
        item("Home", #selector(AppDelegate.goHome(_:)), target: target, key: "h", modifiers: [.command, .shift])
    ]))
    mainMenu.addItem(menu(title: "Window", items: [
        item("Show Pantheon", #selector(AppDelegate.showWorkspace(_:)), target: target),
        item("Minimize", #selector(NSWindow.performMiniaturize(_:)), target: nil, key: "m", modifiers: [.command]),
        item("Zoom", #selector(NSWindow.performZoom(_:)), target: nil),
        NSMenuItem.separator(),
        item("Bring All to Front", #selector(NSApplication.arrangeInFront(_:)))
    ]))
    mainMenu.addItem(menu(title: "Help", items: [
        item("Pantheon Help & Tools", #selector(AppDelegate.openHelp(_:)), target: target, key: "?", modifiers: [.command])
    ]))
    return mainMenu
}

@MainActor
private func menu(title: String, items: [NSMenuItem]) -> NSMenuItem {
    let root = NSMenuItem(title: title, action: nil, keyEquivalent: "")
    let submenu = NSMenu(title: title)
    items.forEach { submenu.addItem($0) }
    root.submenu = submenu
    return root
}

@MainActor
private func item(_ title: String, _ action: Selector, target: AnyObject? = nil,
                  key: String = "", modifiers: NSEvent.ModifierFlags = []) -> NSMenuItem {
    let result = NSMenuItem(title: title, action: action, keyEquivalent: key)
    result.target = target
    result.keyEquivalentModifierMask = modifiers
    return result
}

@MainActor
private func menuItem(_ title: String, submenu: NSMenu) -> NSMenuItem {
    let result = NSMenuItem(title: title, action: nil, keyEquivalent: "")
    result.submenu = submenu
    return result
}
