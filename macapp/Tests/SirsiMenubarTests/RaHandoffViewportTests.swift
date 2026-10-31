import AppKit
import SwiftUI
import XCTest
@testable import SirsiMenubar

final class RaHandoffViewportTests: XCTestCase {
    @MainActor
    func testMountedHandoffSheetBoundsLongRecipientListAndKeepsActionsOutsideScroll() throws {
        _ = NSApplication.shared

        for width in [CGFloat(420), 560] {
            for typeScale in [CGFloat(1), 1.5] {
                guard let screen = NSScreen.main else {
                    throw XCTSkip("A native screen is required to host the handoff viewport")
                }
                let engine = SirsiEngine()
                engine.threadRoster = (0..<43).map { index in
                    AgentHeartbeat(
                        agent: String(format: "fixture-recipient-%02d-%@", index, String(repeating: "long-agent-", count: 4)),
                        live: 1,
                        idle: 0,
                        staleN: 0,
                        freshestIdle: 0,
                        surfaces: ["codex"]
                    )
                }
                let content = RaHandoffView(engine: engine, isPresented: .constant(true))
                    .environment(\.snapshotMode, false)
                    .environment(\.sirsiTypeScale, typeScale)
                let sizingHost = NSHostingView(rootView: content)
                let fitting = sizingHost.fittingSize
                XCTAssertTrue(fitting.width.isFinite && fitting.height.isFinite,
                              "The mounted production sheet must produce finite fitting dimensions")
                XCTAssertGreaterThanOrEqual(fitting.width, 420)
                XCTAssertLessThanOrEqual(fitting.width, 720,
                                         "The production sheet must cap its ideal width despite 43 long recipients")
                XCTAssertLessThanOrEqual(fitting.height, 680,
                                         "The production sheet must cap its ideal height")

                let size = NSSize(width: width, height: 420)
                let visible = screen.visibleFrame
                let origin = NSPoint(
                    x: visible.minX + max(0, (visible.width - size.width) / 2),
                    y: visible.minY + max(0, (visible.height - size.height) / 2)
                )
                let hosting = NSHostingView(rootView: content
                    .frame(width: size.width, height: size.height))
                hosting.sizingOptions = []
                hosting.autoresizingMask = [.width, .height]
                hosting.frame = NSRect(origin: .zero, size: size)

                let window = NSPanel(
                    contentRect: NSRect(origin: origin, size: size),
                    styleMask: [.titled, .closable, .resizable, .nonactivatingPanel],
                    backing: .buffered,
                    defer: false
                )
                window.title = "Ra handoff viewport fixture"
                window.isFloatingPanel = false
                window.hidesOnDeactivate = false
                window.becomesKeyOnlyIfNeeded = true
                window.isReleasedWhenClosed = false
                window.contentView = hosting
                window.orderFront(nil)
                defer { window.close() }
                settle(window)
                XCTAssertTrue(window.isVisible, "The native handoff panel must remain visible for AX inspection")
                XCTAssertFalse(window.isKeyWindow, "The viewport fixture must not activate its nonactivating panel")

                // This independent AppKit control distinguishes an unavailable
                // panel AX tree from missing accessibility on the SwiftUI view.
                let nativeOracle = NSButton(title: "Viewport native AX oracle", target: nil, action: nil)
                nativeOracle.frame = NSRect(x: 8, y: 8, width: 150, height: 24)
                hosting.addSubview(nativeOracle)
                settle(window)

                let root = try XCTUnwrap(window.contentView)
                let views = descendants(of: root)
                let scrollViews = views.compactMap { $0 as? NSScrollView }
                let formScroll = try XCTUnwrap(
                    scrollViews.first { scroll in
                        guard let document = scroll.documentView else { return false }
                        return document.bounds.height > scroll.contentView.bounds.height + 1
                    },
                    "Expected an actual native NSScrollView whose form document overflows vertically"
                )
                // Existing probes set NSView.identifier directly; do not assume a SwiftUI
                // accessibility identifier is mirrored there. Find native scrolls by geometry.
                let recipientScroll = try XCTUnwrap(
                    descendants(of: formScroll).compactMap { $0 as? NSScrollView }.first { scroll in
                        guard let document = scroll.documentView else { return false }
                        return document.bounds.width > scroll.contentView.bounds.width + 1
                    },
                    "Expected an actual nested NSScrollView whose recipient document overflows horizontally"
                )
                let snapshot = try readAccessibilityButtons(in: window)
                print("RA_HANDOFF_AX_TREE width=\(width) typeScale=\(typeScale) \(snapshot.diagnostics)")
                let buttons = snapshot.buttons
                let oracleMatches = buttons.filter { $0.label == "Viewport native AX oracle" }
                XCTAssertEqual(oracleMatches.count, 1,
                               "The same visible panel must expose the independent native AX oracle. \(snapshot.diagnostics)")
                let cancelMatches = buttons.filter { $0.label == "Cancel" }
                let sendMatches = buttons.filter { $0.label == "Send to Ra fabric" }
                XCTAssertEqual(cancelMatches.count, 1,
                               "Cancel must have one unambiguous native accessibility button. \(snapshot.diagnostics)")
                XCTAssertEqual(sendMatches.count, 1,
                               "Send must have one unambiguous native accessibility button. \(snapshot.diagnostics)")
                let cancel = try XCTUnwrap(cancelMatches.first,
                                           "Expected Cancel in the mounted native accessibility tree. \(snapshot.diagnostics)")
                let send = try XCTUnwrap(sendMatches.first,
                                         "Expected Send in the mounted native accessibility tree. \(snapshot.diagnostics)")

                let recipientFrame = root.convert(recipientScroll.bounds, from: recipientScroll)
                let formFrame = root.convert(formScroll.bounds, from: formScroll)
                XCTAssertLessThanOrEqual(recipientFrame.width, width,
                                         "Recipient content must scroll inside the bounded sheet")
                XCTAssertGreaterThan(
                    try XCTUnwrap(recipientScroll.documentView).bounds.width,
                    recipientScroll.contentView.bounds.width,
                    "The 43-recipient fixture must overflow the horizontal list viewport"
                )
                XCTAssertGreaterThan(
                    try XCTUnwrap(formScroll.documentView).bounds.height,
                    formScroll.contentView.bounds.height,
                    "The form must remain vertically scrollable at minimum sheet height"
                )
                let contentScreenFrame = window.convertToScreen(root.convert(root.bounds, to: nil))
                let formScreenFrame = window.convertToScreen(root.convert(formFrame, to: nil))
                let cancelFrame = try XCTUnwrap(cancel.frame, "Cancel must expose its native accessibility frame")
                let sendFrame = try XCTUnwrap(send.frame, "Send must expose its native accessibility frame")
                XCTAssertTrue(contentScreenFrame.contains(cancelFrame), "Cancel must be visible inside the sheet")
                XCTAssertTrue(contentScreenFrame.contains(sendFrame), "Send must be visible inside the sheet")
                XCTAssertFalse(formScreenFrame.intersects(cancelFrame), "Cancel must remain fixed outside the form scroll region")
                XCTAssertFalse(formScreenFrame.intersects(sendFrame), "Send must remain fixed outside the form scroll region")
            }
        }
    }

    @MainActor
    private func settle(_ window: NSWindow) {
        window.contentView?.layoutSubtreeIfNeeded()
        window.displayIfNeeded()
        RunLoop.main.run(until: Date().addingTimeInterval(0.03))
        window.contentView?.layoutSubtreeIfNeeded()
    }

    @MainActor
    private func descendants(of view: NSView) -> [NSView] {
        [view] + view.subviews.flatMap { descendants(of: $0) }
    }

    @MainActor
    @MainActor
    private func readAccessibilityButtons(in window: NSWindow) throws -> MountedAXSnapshot {
        var pending: [Any] = [window]
        var visited: Set<ObjectIdentifier> = []
        var buttons: [MountedAXButton] = []
        var diagnostics: [String] = []
        while let next = pending.popLast() {
            guard let element = next as? NSObject else { throw MountedAXFailure.nonObjectChild }
            guard visited.insert(ObjectIdentifier(element)).inserted else { continue }
            guard visited.count <= 10_000 else { throw MountedAXFailure.treeTooLarge }

            let role = element.accessibilityAttributeValue(.role) as? String
            let title = element.accessibilityAttributeValue(.title) as? String
            let label = element.accessibilityAttributeValue(.description) as? String
            let value = element.accessibilityAttributeValue(.value)
            let children = element.accessibilityAttributeValue(.children) as? [Any] ?? []
            if role == NSAccessibility.Role.button.rawValue,
               let name = [title, label].compactMap({ $0 }).first(where: { !$0.isEmpty }) {
                buttons.append(MountedAXButton(label: name, frame: accessibilityFrame(of: element)))
            }
            if diagnostics.count < 80 {
                diagnostics.append("\(type(of: element)) role=\(role ?? "nil") children=\(children.count) title=\((title ?? "nil").prefix(100)) label=\((label ?? "nil").prefix(100)) value=\(String(describing: value).prefix(100))")
            }
            pending.append(contentsOf: children)
        }
        return MountedAXSnapshot(
            buttons: buttons,
            diagnostics: "visible=\(window.isVisible) key=\(window.isKeyWindow) nodes=\(visited.count)\n" + diagnostics.joined(separator: "\n")
        )
    }

    private func accessibilityFrame(of element: NSObject) -> NSRect? {
        guard let position = element.accessibilityAttributeValue(.position) as? NSValue,
              let size = element.accessibilityAttributeValue(.size) as? NSValue else {
            return nil
        }
        // AppKit reports AX positions in screen coordinates.
        return NSRect(origin: position.pointValue, size: size.sizeValue)
    }

    private struct MountedAXButton {
        let label: String
        let frame: NSRect?
    }

    private struct MountedAXSnapshot {
        let buttons: [MountedAXButton]
        let diagnostics: String
    }

    private enum MountedAXFailure: Error {
        case nonObjectChild
        case treeTooLarge
    }
}
