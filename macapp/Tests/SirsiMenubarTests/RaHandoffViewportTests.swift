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
                XCTAssertGreaterThanOrEqual(fitting.width, 420)
                XCTAssertLessThanOrEqual(fitting.width, 720,
                                         "The production sheet must cap its ideal width despite 43 long recipients")
                XCTAssertLessThanOrEqual(fitting.height, 680,
                                         "The production sheet must cap its ideal height")

                let size = NSSize(width: width, height: 420)
                let hosting = NSHostingView(rootView: content
                    .frame(width: size.width, height: size.height))
                hosting.sizingOptions = []
                hosting.autoresizingMask = [.width, .height]
                hosting.frame = NSRect(origin: .zero, size: size)

                let window = NSPanel(
                    contentRect: NSRect(origin: .zero, size: size),
                    styleMask: [.titled, .closable, .resizable, .nonactivatingPanel],
                    backing: .buffered,
                    defer: false
                )
                window.title = "Ra handoff viewport fixture"
                window.isReleasedWhenClosed = false
                window.contentView = hosting
                window.orderFront(nil)
                defer { window.close() }
                settle(window)

                let root = try XCTUnwrap(window.contentView)
                let views = descendants(of: root)
                let recipientScroll = try XCTUnwrap(
                    views.first { $0.identifier?.rawValue == "ra-handoff-recipient-scroll" } as? NSScrollView
                )
                let formScroll = try XCTUnwrap(
                    views.first { $0.identifier?.rawValue == "ra-handoff-form-scroll" } as? NSScrollView
                )
                let cancel = try XCTUnwrap(views.first { $0.identifier?.rawValue == "ra-handoff-cancel" })
                let send = try XCTUnwrap(views.first { $0.identifier?.rawValue == "ra-handoff-send" })

                XCTAssertLessThanOrEqual(recipientScroll.frame.width, width,
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
                XCTAssertFalse(isDescendant(cancel, of: formScroll), "Cancel must remain fixed above the form scroll region")
                XCTAssertFalse(isDescendant(send, of: formScroll), "Send must remain fixed below the form scroll region")
                XCTAssertTrue(root.bounds.contains(cancel.convert(cancel.bounds, to: root)))
                XCTAssertTrue(root.bounds.contains(send.convert(send.bounds, to: root)))
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
    private func isDescendant(_ view: NSView, of ancestor: NSView) -> Bool {
        var current = view.superview
        while let candidate = current {
            if candidate === ancestor { return true }
            current = candidate.superview
        }
        return false
    }
}
