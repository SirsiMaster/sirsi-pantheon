import AppKit

// Construct menu objects only: the caller owns installation and the target's
// lifetime. Tests supply an NSObject double, never NSApplication.shared.
@MainActor
func makePantheonMainMenu(quitTarget: NSObject) -> NSMenu {
    let mainMenu = NSMenu(title: "Pantheon")
    let applicationItem = NSMenuItem(title: "Pantheon", action: nil, keyEquivalent: "")
    let applicationMenu = NSMenu(title: "Pantheon")
    let quit = NSMenuItem(
        title: "Quit Pantheon",
        action: #selector(NSApplication.terminate(_:)),
        keyEquivalent: "q"
    )
    quit.keyEquivalentModifierMask = [.command]
    quit.target = quitTarget
    applicationMenu.addItem(quit)
    applicationItem.submenu = applicationMenu
    mainMenu.addItem(applicationItem)
    return mainMenu
}
