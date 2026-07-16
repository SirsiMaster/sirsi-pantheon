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
    // The surface is a real, movable, RESIZABLE floating panel — not a locked
    // NSPopover. An NSPopover cannot be moved or resized by the user, which is
    // wrong for a 20-screen app (owner, 2026-07-09). The panel remembers its
    // frame (position + size) across opens via setFrameAutosaveName.
    private var panel: NSPanel!
    private var panelObserver: Any?
    private let engine = SirsiEngine()
    private var refreshTimer: Timer?

    // retireOlderInstances terminates every OTHER running process with our bundle
    // identifier that launched before us. Uses launchDate (not PID magnitude —
    // PIDs recycle) to decide seniority; falls back to "any other instance" when
    // a launchDate is unreadable. terminate() is the polite AppKit path (runs the
    // peer's teardown); forceTerminate only if the peer ignores it for 3s.
    private func retireOlderInstances() {
        guard let bundleID = Bundle.main.bundleIdentifier else { return }
        let me = NSRunningApplication.current
        let peers = NSRunningApplication.runningApplications(withBundleIdentifier: bundleID)
            .filter { $0.processIdentifier != me.processIdentifier }
        guard !peers.isEmpty else { return }
        let myLaunch = me.launchDate ?? Date()
        for peer in peers {
            let peerLaunch = peer.launchDate ?? .distantPast
            guard peerLaunch <= myLaunch else { continue }  // never retire a newer peer
            peer.terminate()
            let deadline = Date().addingTimeInterval(3)
            DispatchQueue.global().async {
                while !peer.isTerminated && Date() < deadline {
                    usleep(200_000)
                }
                if !peer.isTerminated { peer.forceTerminate() }
            }
        }
    }

    // The Eye of Horus (wedjat) — the watchful protector, and unmistakably Sirsi's
    // mark, not a stock eyeball. Drawn as a vector NSBezierPath into a template
    // NSImage (so contentTintColor drives the health color and it adapts to
    // light/dark + Retina). All in code → guaranteed to render, no bundled asset.
    // Authored in a 100×80 design space, uniform-scaled; pupil filled, rest stroked.
    static func makeEye(_ color: NSColor) -> NSImage {
        // Drawn in the health COLOR directly (white/amber/red), isTemplate = false.
        // A template image's tinting would not engage for this runtime-drawn icon —
        // it rendered literal-black and vanished on a dark menu bar (across both the
        // drawingHandler and lockFocus forms). Baking the colour in guarantees it
        // shows; the icon is redrawn whenever health changes. ~18 pt tall.
        let s: CGFloat = 0.225
        let img = NSImage(size: NSSize(width: 100 * s, height: 80 * s))
        img.lockFocus()
        func P(_ x: CGFloat, _ y: CGFloat) -> NSPoint { NSPoint(x: x * s, y: y * s) }
        func quad(_ path: NSBezierPath, _ from: NSPoint, _ c: NSPoint, _ to: NSPoint) {
            let c1 = NSPoint(x: from.x + 2.0 / 3 * (c.x - from.x), y: from.y + 2.0 / 3 * (c.y - from.y))
            let c2 = NSPoint(x: to.x + 2.0 / 3 * (c.x - to.x), y: to.y + 2.0 / 3 * (c.y - to.y))
            path.curve(to: to, controlPoint1: c1, controlPoint2: c2)
        }
        color.setStroke()
        color.setFill()
        let line = NSBezierPath()
        line.lineWidth = 2.4
        line.lineCapStyle = .round
        line.lineJoinStyle = .round
        line.move(to: P(22, 56)); quad(line, P(22, 56), P(50, 74), P(82, 54))   // eyebrow
        line.move(to: P(16, 40)); quad(line, P(16, 40), P(46, 56), P(78, 42))   // upper lid
        line.move(to: P(16, 40)); quad(line, P(16, 40), P(46, 26), P(78, 42))   // lower lid
        line.move(to: P(78, 42)); line.line(to: P(95, 45))                      // outer corner
        line.move(to: P(40, 28)); line.line(to: P(30, 5))                       // teardrop
        line.move(to: P(60, 30)); quad(line, P(60, 30), P(70, 10), P(84, 14)); quad(line, P(84, 14), P(92, 16), P(86, 26)) // curl
        line.stroke()
        let r: CGFloat = 6.5 * s
        let c = P(45, 41)
        NSBezierPath(ovalIn: NSRect(x: c.x - r, y: c.y - r, width: 2 * r, height: 2 * r)).fill()
        img.unlockFocus()
        img.isTemplate = false
        return img
    }

    // tint maps the health band to the Eye's DRAWN colour: red (live-critical),
    // amber (warnings / 7-day trends), else labelColor (adaptive white-on-dark).
    static func tint(for status: String) -> NSColor {
        switch status {
        case "red":   return .systemRed
        case "amber": return .systemYellow
        // Explicit adaptive color (NOT nil) — a status-bar button with a template
        // image and contentTintColor=nil renders the image's literal black, which
        // vanishes on a dark menu bar. labelColor is white on dark, black on light.
        default:      return .labelColor
        }
    }

    func applicationDidFinishLaunching(_ notification: Notification) {
        // Single-instance guard: a relaunch RETIRES the older instance instead of
        // stacking a second eye in the menu bar (the 2026-07-02 double-icon bug:
        // the LaunchAgent-managed app + a manually-opened copy both ran). Newest
        // PID wins — every older process with our bundle id is terminated. If the
        // retired one was LaunchAgent-managed (KeepAlive), launchd respawns it as
        // the newest and this same guard retires the manual copy — converging to
        // exactly one, agent-managed instance within a bounce.
        retireOlderInstances()

        // Proactively register with TCC so "Sirsi Menubar" already has a row in
        // the Full Disk Access list before the user ever clicks the Grant button.
        // A TCC-denied open() is what puts an app in that list (see Views.swift).
        registerForFullDiskAccess()

        statusItem = NSStatusBar.system.statusItem(withLength: NSStatusItem.variableLength)
        if let button = statusItem.button {
            // Branded mark: the Eye of Horus (the watchful protector), drawn in
            // code in the health colour (makeEye), NOT a template image. Healthy
            // white at launch; onTitle recolours it amber/red as health changes,
            // so Pantheon is no longer "just a colored dot." ADR-030.
            button.image = Self.makeEye(.labelColor)
            button.imagePosition = .imageOnly  // becomes .imageLeading when a waste figure rides beside it
            button.action = #selector(togglePopover(_:))
            button.target = self
        }

        buildPanel()

        engine.onTitle = { [weak self] label in
            guard let self = self, let button = self.statusItem.button else { return }
            // The Eye is always the icon; the waste figure (≥1 GB) rides beside it.
            button.title = label.isEmpty ? "" : " \(label)"
            button.imagePosition = label.isEmpty ? .imageOnly : .imageLeading
            // Redraw the Eye in the health colour so it's ALWAYS visible (no
            // reliance on template tinting, which didn't engage on a dark bar).
            button.image = Self.makeEye(Self.tint(for: self.engine.titleStatus))
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

    // buildPanel constructs the movable, resizable floating panel once. Titled +
    // closable + resizable gives the user a drag handle (transparent title bar,
    // isMovableByWindowBackground) and standard resize edges; the frame is
    // autosaved so position + size persist across opens and relaunches.
    private func buildPanel() {
        let hosting = NSHostingView(rootView: RootView(engine: engine))
        // CRITICAL for resize: NSHostingView otherwise pins Auto Layout
        // constraints to its content's intrinsic (fitting) size, which locks the
        // window at a fixed size no matter the .resizable mask. Clearing
        // sizingOptions and letting it fill via the autoresizing mask lets the
        // user resize freely (the 2026-07-09 'not sizable' report).
        if #available(macOS 13.0, *) { hosting.sizingOptions = [] }
        hosting.translatesAutoresizingMaskIntoConstraints = true
        hosting.autoresizingMask = [.width, .height]

        let p = NSPanel(
            contentRect: NSRect(x: 0, y: 0, width: 400, height: 560),
            styleMask: [.titled, .closable, .resizable, .nonactivatingPanel],
            backing: .buffered, defer: false)
        p.title = "Sirsi Pantheon"
        p.titleVisibility = .hidden
        p.titlebarAppearsTransparent = true
        p.isMovableByWindowBackground = true          // drag from anywhere
        p.isFloatingPanel = true
        p.level = .floating
        p.hidesOnDeactivate = false
        p.collectionBehavior = [.canJoinAllSpaces, .fullScreenAuxiliary]
        let container = NSView(frame: NSRect(x: 0, y: 0, width: 400, height: 560))
        hosting.frame = container.bounds
        container.addSubview(hosting)
        p.contentView = container
        p.minSize = NSSize(width: 360, height: 420)   // resizable, with a sane floor
        p.maxSize = NSSize(width: 900, height: 1400)
        p.isReleasedWhenClosed = false
        p.setFrameAutosaveName("SirsiPantheonPanel")  // persist position + size
        // Yield focus like a real menubar surface: click anywhere else and the
        // panel gets out of the way instead of floating over every window
        // forever (owner, 2026-07-16: "static and never loses focus and blocks
        // other windows"). Sheets (Apply confirmations) become key while the
        // panel resigns — don't hide underneath our own dialog.
        panelObserver = NotificationCenter.default.addObserver(
            forName: NSWindow.didResignKeyNotification, object: p, queue: .main
        ) { _ in
            Task { @MainActor in
                if let key = NSApp.keyWindow, key.sheetParent === p || key.parent === p { return }
                if p.attachedSheet != nil { return }
                p.orderOut(nil)
            }
        }
        panel = p
    }

    @objc private func togglePopover(_ sender: Any?) {
        guard panel != nil else { return }
        if panel.isVisible {
            panel.orderOut(sender)
            return
        }
        engine.refresh()
        engine.reopenTick += 1   // RootView pops to a fresh Home (no stale screens)
        // First open with no saved frame: anchor under the status item. After
        // that, respect wherever the user moved/sized it (autosaved frame).
        if panel.frameAutosaveName.isEmpty || !panel.setFrameUsingName("SirsiPantheonPanel") {
            positionUnderStatusItem()
        } else if let button = statusItem.button, button.window == nil {
            positionUnderStatusItem()
        }
        // If the saved frame would place it off-screen (display changed), recenter.
        if let screen = NSScreen.main, !screen.visibleFrame.intersects(panel.frame) {
            positionUnderStatusItem()
        }
        panel.makeKeyAndOrderFront(sender)
        NSApp.activate(ignoringOtherApps: true)
    }

    // positionUnderStatusItem places the panel just below the menu-bar icon,
    // right-aligned to it (the natural first-open location).
    private func positionUnderStatusItem() {
        guard let button = statusItem.button, let bwin = button.window else {
            if let screen = NSScreen.main { panel.center(); _ = screen } // fallback
            return
        }
        let btnRectScreen = bwin.convertToScreen(button.convert(button.bounds, to: nil))
        var f = panel.frame
        f.origin.x = btnRectScreen.maxX - f.width
        f.origin.y = btnRectScreen.minY - f.height - 6
        if let vis = NSScreen.main?.visibleFrame {
            f.origin.x = min(max(f.origin.x, vis.minX + 8), vis.maxX - f.width - 8)
            f.origin.y = min(max(f.origin.y, vis.minY + 8), vis.maxY - f.height - 8)
        }
        panel.setFrame(f, display: false)
    }
}
