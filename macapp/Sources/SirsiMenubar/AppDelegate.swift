import AppKit
import SwiftUI
import UserNotifications

// AppDelegate owns the NSStatusItem and the NSPopover. This is the durable
// surface ADR-030 specifies: an NSStatusItem.button anchors an NSPopover whose
// content is a SwiftUI NavigationStack — the same architecture as Bartender /
// Fantastical / CleanMyMac MenuBar. The panel STAYS OPEN, drills in, and goes
// back; nothing is rendered into a dropdown that closes on click.
@MainActor
final class AppDelegate: NSObject, NSApplicationDelegate {
    private var statusItem: NSStatusItem!
    // Pantheon is a full desktop application. The Eye is only a companion
    // affordance; both open this same normal, movable, resizable NSWindow.
    // Keeping one window means no second, reduced menu-bar-only workflow can
    // drift away from the application the user is actually operating.
    private var window: NSWindow!
    private let engine = SirsiEngine()
    private var refreshTimer: Timer?
    private var instanceLease: MenubarInstanceLease?

    // retireOlderInstances terminates every OTHER running process with our bundle
    // identifier that launched before us. Uses launchDate (not PID magnitude —
    // PIDs recycle) to decide seniority; falls back to "any other instance" when
    // a launchDate is unreadable. terminate() is the polite AppKit path (runs the
    // peer's teardown); forceTerminate only if the peer ignores it for 3s.
    // peerInstances finds the OTHER live copies of this surface. Bundle identity
    // alone is not enough: the normal dev path runs the raw binary
    // (.build/release/SirsiMenubar), which has NO bundle identifier at all — so
    // the old `guard let bundleID = … else { return }` read as "no peers found"
    // when it actually meant "cannot tell", and two panels coexisted with
    // neither able to retire the other. Executable NAME is matched alongside the
    // bundle id specifically so a bundled .app and a raw-binary run can see each
    // other; keying on the full path would let those two coexist again, because
    // Contents/MacOS/SirsiMenubar and .build/release/SirsiMenubar are genuinely
    // different files. The historical Go menubar was named `sirsi-menubar`; it
    // shares the lease in new builds, and recognizing it here retires an older
    // copy during an upgrade rather than leaving two icons behind.
    private static let pantheonMenubarExecutables: Set<String> = ["SirsiMenubar", "sirsi-menubar"]

    private func peerInstances() -> [NSRunningApplication] {
        let me = NSRunningApplication.current
        let bundleID = Bundle.main.bundleIdentifier
        return NSWorkspace.shared.runningApplications.filter { peer in
            guard peer.processIdentifier != me.processIdentifier else { return false }
            if let bundleID, peer.bundleIdentifier == bundleID { return true }
            if let name = peer.executableURL?.lastPathComponent,
               Self.pantheonMenubarExecutables.contains(name) { return true }
            return false
        }
    }

    private func retireOlderInstances() {
        let me = NSRunningApplication.current
        let peers = peerInstances()
        guard !peers.isEmpty else { return }
        let myLaunch = me.launchDate ?? Date()
        for peer in peers {
            let peerLaunch = peer.launchDate ?? .distantPast
            // A newer modern peer is allowed to finish its own lease hand-off.
            // The retired Go binary cannot share that hand-off, however, so it
            // must always yield to the native product if it appears later.
            let isLegacyGoPeer = peer.executableURL?.lastPathComponent == "sirsi-menubar"
            guard isLegacyGoPeer || peerLaunch <= myLaunch else { continue }
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

    // The status item is a compact crop of the canonical Sirsi application mark.
    // It is deliberately an actual bundled brand asset—not a hand-drawn stand-in.
    // The lower half contains the multicolor Sirsi loop, which remains legible at
    // status-bar scale without trying to squeeze the whole wordmark into 18 points.
    static func makeSirsiStatusMark() -> NSImage {
        guard let url = Bundle.main.url(forResource: "sirsi-logo-white", withExtension: "png"),
              let source = NSImage(contentsOf: url) else {
            return NSImage(systemSymbolName: "circle.hexagonpath.fill", accessibilityDescription: "Sirsi") ?? NSImage()
        }
        let image = NSImage(size: NSSize(width: 27, height: 16))
        image.lockFocus()
        source.draw(
            in: NSRect(x: 0, y: 0, width: 27, height: 16),
            from: NSRect(x: 0, y: 0, width: source.size.width, height: source.size.height * 0.55),
            operation: .sourceOver,
            fraction: 1
        )
        image.unlockFocus()
        image.isTemplate = false
        return image
    }

    func applicationDidFinishLaunching(_ notification: Notification) {
        // A second modern instance activates the current product surface and
        // exits before it can add another status item. Older binaries did not
        // hold the lease; after this app obtains it, the legacy peer pass below
        // still converges that transitional case to one process.
        guard let lease = MenubarInstanceLease.acquire() else {
            peerInstances().first?.activate(options: [.activateAllWindows])
            NSApplication.shared.terminate(nil)
            return
        }
        instanceLease = lease

        // Single-instance guard: a relaunch RETIRES the older instance instead of
        // stacking a second eye in the menu bar (the 2026-07-02 double-icon bug:
        // the LaunchAgent-managed app + a manually-opened copy both ran). Newest
        // PID wins — every older process with our bundle id is terminated. If the
        // retired one was LaunchAgent-managed (KeepAlive), launchd respawns it as
        // the newest and this same guard retires the manual copy — converging to
        // exactly one, agent-managed instance within a bounce.
        retireOlderInstances()

        // Never probe protected folders just to register a Full Disk Access row.
        // The user may invoke that explicitly from the guided recovery only.

        // Claim a RIGHT-side menu-bar slot from the first launch (owner reports
        // 2026-07-17): macOS hides the LEFTMOST status items when the bar fills,
        // and a newly-created item spawns leftmost — so the Eye (recreated on
        // every app relaunch) was perpetually first to vanish behind Outlook's
        // transient notification item. A Cmd-drag anchor doesn't work against a
        // transient neighbor, so seed the position PROGRAMMATICALLY: macOS reads
        // the item's saved slot from the "NSStatusItem Preferred Position
        // <autosaveName>" default (points from the RIGHT edge of the status
        // area) BEFORE placing it. Seeding a small value pins the Eye next to
        // the system items — right of every transient third-party icon — and is
        // written only when absent, so the owner's own drag always wins after.
        let posKey = "NSStatusItem Preferred Position ai.sirsi.pantheon.eye"
        if UserDefaults.standard.object(forKey: posKey) == nil {
            UserDefaults.standard.set(120.0, forKey: posKey)
        }
        statusItem = NSStatusBar.system.statusItem(withLength: NSStatusItem.variableLength)
        statusItem.autosaveName = "ai.sirsi.pantheon.eye"
        statusItem.isVisible = true
        if let button = statusItem.button {
            button.image = Self.makeSirsiStatusMark()
            button.imagePosition = .imageOnly  // becomes .imageLeading when a waste figure rides beside it
            button.action = #selector(togglePopover(_:))
            button.target = self
        }

        buildMainWindow()

        engine.onTitle = { [weak self] label in
            guard let self = self, let button = self.statusItem.button else { return }
            // The Sirsi mark is always the icon; a clear cleanup action (≥1 GB)
            // rides beside it. Never surface a bare, ambiguous byte count.
            button.title = label.isEmpty ? "" : " \(label)"
            button.imagePosition = label.isEmpty ? .imageOnly : .imageLeading
            button.toolTip = "Pantheon: \(self.engine.titleStatus) system health"
        }
        engine.refresh()
        // Tint the Eye to REAL health immediately — a health glyph that only colors
        // after you click it is half-useful. diagnose() sets healthStatus → onTitle.
        Task { @MainActor in await engine.diagnose() }

        // Owner-gated toasts (board schema 1.1.0): open `to: user` router items
        // must reach the owner as a notification, click → action screen. Only
        // meaningful from the signed .app bundle — UNUserNotificationCenter
        // throws in an unbundled dev binary.
        if Bundle.main.bundleIdentifier != nil {
            let nc = UNUserNotificationCenter.current()
            nc.delegate = self
            nc.requestAuthorization(options: [.alert, .sound]) { _, _ in }
        }
        Task { @MainActor in await self.checkOwnerGated() }

        // Periodic refresh so the Eye tracks reality at a glance: cheap waste re-read
        // + a health diagnose (≥60s — never a tight tick; A27 forbids flooding).
        // Selector delivery stays on the main run loop. Avoid capturing weak
        // AppDelegate state from Timer's concurrently-executing closure: Swift
        // 6 correctly rejects that on current hosted macOS toolchains.
        refreshTimer = Timer.scheduledTimer(
            timeInterval: 90,
            target: self,
            selector: #selector(handleRefreshTimer(_:)),
            userInfo: nil,
            repeats: true
        )

        // A Pantheon launch opens its actual application workspace. The status
        // item remains available after the window is closed, but it is never
        // the sole route into the product.
        window.makeKeyAndOrderFront(nil)
        NSApp.activate(ignoringOtherApps: true)
    }

    @objc private func handleRefreshTimer(_ timer: Timer) {
        engine.refresh()
        Task { @MainActor [weak self] in
            guard let self else { return }
            await engine.diagnose()
            await checkOwnerGated()
        }
    }

    // checkOwnerGated re-reads the board and toasts genuinely-new owner-gated
    // items. Board-file-only on the timer path: the CLI fallback would spawn a
    // process every 90s when the conduit is down — the file IS the cheap signal.
    private func checkOwnerGated() async {
        let boardPath = (("~/.sirsi/router-board.json") as NSString).expandingTildeInPath
        guard FileManager.default.fileExists(atPath: boardPath) else { return }
        await engine.loadRouterBoard()
        guard Bundle.main.bundleIdentifier != nil else { return }
        for item in engine.claimNewOwnerGated() {
            let content = UNMutableNotificationContent()
            content.title = String(item.title.prefix(64))
            content.body = item.why ?? "An item needs your decision."
            content.userInfo = ["id": item.id]
            // Keep the original best-effort notification semantics while using
            // the concurrency-safe API required by current Swift toolchains.
            try? await UNUserNotificationCenter.current().add(
                UNNotificationRequest(identifier: item.id, content: content, trigger: nil))
        }
    }

    // buildMainWindow constructs Pantheon's primary workspace. It deliberately
    // uses ordinary macOS window semantics: users can switch to it from the
    // Dock, resize it for dense operational work, and close it without losing
    // the menu-bar companion.
    private func buildMainWindow() {
        let hosting = NSHostingView(rootView: PantheonDesktopView(engine: engine))
        // CRITICAL for resize: NSHostingView otherwise pins Auto Layout
        // constraints to its content's intrinsic (fitting) size, which locks the
        // window at a fixed size no matter the .resizable mask. Clearing
        // sizingOptions and letting it fill via the autoresizing mask lets the
        // user resize freely (the 2026-07-09 'not sizable' report).
        if #available(macOS 13.0, *) { hosting.sizingOptions = [] }
        hosting.translatesAutoresizingMaskIntoConstraints = true
        hosting.autoresizingMask = [.width, .height]

        let w = NSWindow(
            contentRect: NSRect(x: 0, y: 0, width: 1180, height: 780),
            styleMask: [.titled, .closable, .resizable],
            backing: .buffered, defer: false)
        w.title = "Sirsi Pantheon"
        w.titlebarAppearsTransparent = false
        w.appearance = NSAppearance(named: .darkAqua)
        w.backgroundColor = NSColor(red: 0.025, green: 0.047, blue: 0.036, alpha: 1)
        w.isMovableByWindowBackground = false
        w.level = .normal
        w.hidesOnDeactivate = false
        w.collectionBehavior = [.moveToActiveSpace, .fullScreenAuxiliary]
        w.toolbarStyle = .unifiedCompact
        let container = NSView(frame: NSRect(x: 0, y: 0, width: 1180, height: 780))
        hosting.frame = container.bounds
        container.addSubview(hosting)
        w.contentView = container
        w.minSize = NSSize(width: 900, height: 620)
        w.setFrameAutosaveName("SirsiPantheonWorkspace")
        w.isReleasedWhenClosed = false
        window = w
    }

    @objc private func togglePopover(_ sender: Any?) {
        guard window != nil else { return }
        engine.refresh()
        window.makeKeyAndOrderFront(sender)
        NSApp.activate(ignoringOtherApps: true)
    }

    // openOwnerItem deep-links: show the panel and let RootView push the
    // action screen for this item id (engine.pendingOwnerItemID is observed).
    func openOwnerItem(id: String) {
        engine.pendingOwnerItemID = id
        if window?.isVisible != true { togglePopover(nil) }
    }

    func applicationShouldHandleReopen(_ sender: NSApplication, hasVisibleWindows flag: Bool) -> Bool {
        if !flag { togglePopover(nil) }
        return true
    }
}

// Toast click → the item's action screen. willPresent keeps banners visible
// even while the app is "active" (a menubar agent is technically always-ish
// active, which would otherwise swallow every toast).
extension AppDelegate: UNUserNotificationCenterDelegate {
    nonisolated func userNotificationCenter(_ center: UNUserNotificationCenter,
                                            didReceive response: UNNotificationResponse,
                                            withCompletionHandler completionHandler: @escaping () -> Void) {
        let id = response.notification.request.content.userInfo["id"] as? String
        // The delegate completion is nonisolated. Acknowledge it before
        // hopping to the main actor so Swift 6 does not capture a non-Sendable
        // callback across actor isolation.
        completionHandler()
        Task { @MainActor [weak self] in
            guard let self, let id else { return }
            self.openOwnerItem(id: id)
        }
    }

    nonisolated func userNotificationCenter(_ center: UNUserNotificationCenter,
                                            willPresent notification: UNNotification,
                                            withCompletionHandler completionHandler: @escaping (UNNotificationPresentationOptions) -> Void) {
        completionHandler([.banner, .sound])
    }
}
