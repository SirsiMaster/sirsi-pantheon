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

    // The Eye of Horus mark for the menu bar — SF Symbol "eye.fill" rendered as a
    // template (so contentTintColor drives its color and it adapts to light/dark).
    // Guaranteed to render on every macOS ≥13, unlike a hieroglyph in the menu-bar
    // font. accessibilityDescription names it for VoiceOver.
    static let eyeImage: NSImage? = {
        let cfg = NSImage.SymbolConfiguration(pointSize: 14, weight: .semibold)
        let img = NSImage(systemSymbolName: "eye.fill", accessibilityDescription: "Sirsi Pantheon — system health")?
            .withSymbolConfiguration(cfg)
        img?.isTemplate = true
        return img
    }()

    // tint maps the health band to the Eye's color. Healthy → nil = native
    // monochrome (the icon sits quiet until something actually needs attention).
    static func tint(for status: String) -> NSColor? {
        switch status {
        case "red":   return .systemRed
        case "amber": return .systemYellow
        default:      return nil
        }
    }

    func applicationDidFinishLaunching(_ notification: Notification) {
        // Proactively register with TCC so "Sirsi Menubar" already has a row in
        // the Full Disk Access list before the user ever clicks the Grant button.
        // A TCC-denied open() is what puts an app in that list (see Views.swift).
        registerForFullDiskAccess()

        statusItem = NSStatusBar.system.statusItem(withLength: NSStatusItem.variableLength)
        if let button = statusItem.button {
            // Branded, guaranteed-to-render mark: the Eye (Horus, the watchful
            // protector) as an SF Symbol TEMPLATE image — vector, adapts to the
            // menu-bar height + light/dark, and tintable. Its TINT carries health
            // (monochrome when healthy; amber/red only when something needs you),
            // so Pantheon is no longer "just a colored dot." ADR-030.
            button.image = Self.eyeImage
            button.imagePosition = .imageOnly
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
            guard let self = self, let button = self.statusItem.button else { return }
            // The Eye is always the icon; the waste figure (≥1 GB) rides beside it.
            button.title = label.isEmpty ? "" : " \(label)"
            button.imagePosition = label.isEmpty ? .imageOnly : .imageLeading
            // Tint = health band. nil → native menu-bar monochrome (healthy).
            button.contentTintColor = Self.tint(for: self.engine.titleStatus)
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
