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

    // The Eye of Horus (wedjat) — the watchful protector, and unmistakably Sirsi's
    // mark, not a stock eyeball. Drawn as a vector NSBezierPath into a template
    // NSImage (so contentTintColor drives the health color and it adapts to
    // light/dark + Retina). All in code → guaranteed to render, no bundled asset.
    // Authored in a 100×80 design space, uniform-scaled; pupil filled, rest stroked.
    static let eyeImage: NSImage? = {
        // Sized to fill the menu bar (≈18–19 pt tall) so the Eye reads clearly —
        // the first cut at 20×16 with thin strokes was too slight. Uniform 0.24 scale.
        let w: CGFloat = 24, h: CGFloat = 19.2
        let img = NSImage(size: NSSize(width: w, height: h), flipped: false) { _ in
            let sx = w / 100, sy = h / 80
            func P(_ x: CGFloat, _ y: CGFloat) -> NSPoint { NSPoint(x: x * sx, y: y * sy) }
            // Append a quadratic (SVG-style) segment as a cubic, since NSBezierPath
            // is cubic-only.
            func quad(_ path: NSBezierPath, _ from: NSPoint, _ c: NSPoint, _ to: NSPoint) {
                let c1 = NSPoint(x: from.x + 2.0 / 3 * (c.x - from.x), y: from.y + 2.0 / 3 * (c.y - from.y))
                let c2 = NSPoint(x: to.x + 2.0 / 3 * (c.x - to.x), y: to.y + 2.0 / 3 * (c.y - to.y))
                path.curve(to: to, controlPoint1: c1, controlPoint2: c2)
            }
            NSColor.black.setStroke()
            NSColor.black.setFill()

            let line = NSBezierPath()
            line.lineWidth = 2.6   // bold — the thin 1.35 stroke read "too slight" in the bar
            line.lineCapStyle = .round
            line.lineJoinStyle = .round
            // eyebrow
            line.move(to: P(22, 56)); quad(line, P(22, 56), P(50, 74), P(82, 54))
            // upper eyelid
            line.move(to: P(16, 40)); quad(line, P(16, 40), P(46, 56), P(78, 42))
            // lower eyelid (closes the almond)
            line.move(to: P(16, 40)); quad(line, P(16, 40), P(46, 26), P(78, 42))
            // the elongated outer corner extending toward the temple
            line.move(to: P(78, 42)); line.line(to: P(95, 45))
            // the descending straight marking (the "teardrop")
            line.move(to: P(40, 28)); line.line(to: P(30, 5))
            // the spiral/curl (the falcon-cheek marking)
            line.move(to: P(60, 30)); quad(line, P(60, 30), P(70, 10), P(84, 14)); quad(line, P(84, 14), P(92, 16), P(86, 26))
            line.stroke()

            // pupil
            let r: CGFloat = 6.5 * sx
            let c = P(45, 41)
            NSBezierPath(ovalIn: NSRect(x: c.x - r, y: c.y - r, width: 2 * r, height: 2 * r)).fill()
            return true
        }
        img.isTemplate = true
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
        // Tint the Eye to REAL health immediately — a health glyph that only colors
        // after you click it is half-useful. diagnose() sets healthStatus → onTitle.
        Task { @MainActor in await engine.diagnose() }

        // Periodic refresh so the Eye tracks reality at a glance: cheap waste re-read
        // + a health diagnose (≥60s — never a tight tick; A27 forbids flooding).
        refreshTimer = Timer.scheduledTimer(withTimeInterval: 90, repeats: true) { [weak self] _ in
            Task { @MainActor in
                self?.engine.refresh()
                await self?.engine.diagnose()
            }
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
