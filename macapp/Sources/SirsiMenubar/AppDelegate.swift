import AppKit
import SwiftUI

// AppDelegate owns the NSStatusItem and the NSPopover. This is the durable
// surface ADR-030 specifies: an NSStatusItem.button anchors an NSPopover whose
// content is a SwiftUI NavigationStack — the same architecture as Bartender /
// Fantastical / CleanMyMac MenuBar. The panel STAYS OPEN, drills in, and goes
// back; nothing is rendered into a dropdown that closes on click.
@MainActor
final class AppDelegate: NSObject, NSApplicationDelegate {
    private var statusItem: NSStatusItem!
    private let popover = NSPopover()
    private let engine = SirsiEngine()
    private var refreshTimer: Timer?

    func applicationDidFinishLaunching(_ notification: Notification) {
        // Proactively register with TCC so "Sirsi Menubar" already has a row in
        // the Full Disk Access list before the user ever clicks the Grant button.
        // A TCC-denied open() is what puts an app in that list (see Views.swift).
        registerForFullDiskAccess()

        statusItem = NSStatusBar.system.statusItem(withLength: NSStatusItem.variableLength)
        if let button = statusItem.button {
            button.title = "𓁢"
            button.action = #selector(togglePopover(_:))
            button.target = self
        }

        popover.behavior = .transient                  // closes when you click away…
        popover.animates = true
        popover.contentSize = NSSize(width: 380, height: 520)
        popover.contentViewController = NSHostingController(
            rootView: RootView(engine: engine)
        )

        engine.onTitle = { [weak self] label in
            self?.statusItem.button?.title = label
        }
        engine.refresh()

        // Cheap periodic re-read of the persisted scan so the label tracks reality
        // (≥60s — never a tight tick; A27 forbids flooding the registry/Spotlight).
        refreshTimer = Timer.scheduledTimer(withTimeInterval: 90, repeats: true) { [weak self] _ in
            Task { @MainActor in self?.engine.refresh() }
        }
    }

    @objc private func togglePopover(_ sender: Any?) {
        guard let button = statusItem.button else { return }
        if popover.isShown {
            popover.performClose(sender)
        } else {
            engine.refresh()
            popover.show(relativeTo: button.bounds, of: button, preferredEdge: .minY)
            // Bring the popover to the front so text fields / scrolling get events.
            popover.contentViewController?.view.window?.makeKeyAndOrderFront(nil)
            NSApp.activate(ignoringOtherApps: true)
        }
    }
}
