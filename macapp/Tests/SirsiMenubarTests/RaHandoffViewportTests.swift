import AppKit
import SwiftUI
import XCTest
@testable import SirsiMenubar

final class RaHandoffViewportTests: XCTestCase {
    @MainActor
    func testMountedHandoffSheetGeometryStaysBoundedAcrossSizesAndTypeScales() throws {
        _ = NSApplication.shared

        for width in [CGFloat(420), 560] {
            for typeScale in [CGFloat(1), 1.5] {
                let content = handoffContent(typeScale: typeScale)
                let sizingHost = NSHostingView(rootView: content)
                let fitting = sizingHost.fittingSize
                XCTAssertTrue(fitting.width.isFinite && fitting.height.isFinite,
                              "The mounted production sheet must produce finite fitting dimensions")
                XCTAssertGreaterThanOrEqual(fitting.width, 420)
                XCTAssertLessThanOrEqual(fitting.width, 720,
                                         "The production sheet must cap its ideal width despite 43 long recipients")
                XCTAssertLessThanOrEqual(fitting.height, 680,
                                         "The production sheet must cap its ideal height")

                let window = try mountedPanel(content, width: width, height: 420)
                defer { window.close() }
                settle(window)
                XCTAssertTrue(window.isVisible, "The native handoff panel must remain visible for geometry inspection")
                XCTAssertFalse(window.isKeyWindow, "The viewport fixture must not activate its nonactivating panel")

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
                let recipientFrame = root.convert(recipientScroll.bounds, from: recipientScroll)
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
            }
        }
    }

    @MainActor
    func testMountedHandoffCancelAndSendHaveVisibleFixedAXFrames() throws {
        try requireIndependentSwiftUIAXOracle()

        for width in [CGFloat(420), 560] {
            for typeScale in [CGFloat(1), 1.5] {
                let window = try mountedPanel(handoffContent(typeScale: typeScale), width: width, height: 420)
                defer { window.close() }
                settle(window)
                XCTAssertTrue(window.isVisible, "The production panel must remain visible for AX inspection")
                XCTAssertFalse(window.isKeyWindow, "The production fixture must remain nonactivating")

                let root = try XCTUnwrap(window.contentView)
                let scrollViews = descendants(of: root).compactMap { $0 as? NSScrollView }
                let formScroll = try XCTUnwrap(
                    scrollViews.first { scroll in
                        guard let document = scroll.documentView else { return false }
                        return document.bounds.height > scroll.contentView.bounds.height + 1
                    },
                    "Expected the mounted handoff form's vertically overflowing native scroll view"
                )
                let snapshot = try readAccessibilityButtons(in: window)
                print("RA_HANDOFF_PRODUCTION_AX width=\(width) typeScale=\(typeScale) \(snapshot.diagnostics)")
                let cancelMatches = snapshot.buttons.filter { $0.label == "Cancel" }
                let sendMatches = snapshot.buttons.filter { $0.label == "Send to Ra fabric" }
                XCTAssertEqual(cancelMatches.count, 1,
                               "Cancel must have one unambiguous native accessibility button. \(snapshot.diagnostics)")
                XCTAssertEqual(sendMatches.count, 1,
                               "Send must have one unambiguous native accessibility button. \(snapshot.diagnostics)")
                let cancel = try XCTUnwrap(cancelMatches.first,
                                           "Expected Cancel in the mounted native accessibility tree. \(snapshot.diagnostics)")
                let send = try XCTUnwrap(sendMatches.first,
                                         "Expected Send in the mounted native accessibility tree. \(snapshot.diagnostics)")

                let formFrame = root.convert(formScroll.bounds, from: formScroll)
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
    private func handoffContent(typeScale: CGFloat) -> some View {
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
        return RaHandoffView(engine: engine, isPresented: .constant(true))
            .environment(\.snapshotMode, false)
            .environment(\.sirsiTypeScale, typeScale)
    }

    @MainActor
    private func mountedPanel<Content: View>(_ content: Content, width: CGFloat, height: CGFloat) throws -> NSPanel {
        _ = NSApplication.shared
        guard let screen = NSScreen.main else {
            throw XCTSkip("A native screen is required to host the handoff viewport")
        }
        let size = NSSize(width: width, height: height)
        let visible = screen.visibleFrame
        let origin = NSPoint(
            x: visible.minX + max(0, (visible.width - size.width) / 2),
            y: visible.minY + max(0, (visible.height - size.height) / 2)
        )
        let hosting = NSHostingView(rootView: content.frame(width: size.width, height: size.height))
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
        return window
    }

    @MainActor
    private func requireIndependentSwiftUIAXOracle() throws {
        var diagnostics = "Independent SwiftUI Text/Button AX oracle not yet read"
        do {
            var presses = 0
            let oracle = VStack {
                Text("Native SwiftUI AX text control")
                Button("Native SwiftUI AX button control") { presses += 1 }
            }
            let window = try mountedPanel(oracle, width: 420, height: 120)
            defer { window.close() }
            settle(window)
            guard window.isVisible, !window.isKeyWindow else {
                throw MountedAXFailure.oracleWindowUnavailable
            }

            let snapshot = try readAccessibilityButtons(in: window)
            diagnostics = snapshot.diagnostics
            guard snapshot.strings.contains("Native SwiftUI AX text control"),
                  let button = snapshot.buttons.first(where: { $0.label == "Native SwiftUI AX button control" }) else {
                throw MountedAXFailure.oracleControlsUnavailable
            }
            guard let pressable = button.element as? NSAccessibilityProtocol else {
                throw MountedAXFailure.oraclePressUnavailable
            }
            guard pressable.accessibilityPerformPress() else {
                throw MountedAXFailure.oraclePressUnavailable
            }
            settle(window)
            guard presses == 1 else {
                throw MountedAXFailure.oraclePressNotObserved(presses)
            }
            print("RA_HANDOFF_SWIFTUI_AX_ORACLE_PASS \(diagnostics) presses=\(presses)")
        } catch {
            let reason = "UNVERIFIED: independent SwiftUI Text/Button AX oracle unavailable: \(error). \(diagnostics)"
            print("RA_HANDOFF_SWIFTUI_AX_ORACLE_UNAVAILABLE \(reason)")
            throw XCTSkip(reason)
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
    private func readAccessibilityButtons(in window: NSWindow) throws -> MountedAXSnapshot {
        var pending: [Any] = [window]
        var visited: Set<ObjectIdentifier> = []
        var strings: Set<String> = []
        var buttons: [MountedAXButton] = []
        var diagnostics: [String] = []
        while let next = pending.popLast() {
            guard let element = next as? NSObject else { throw MountedAXFailure.nonObjectChild }
            guard visited.insert(ObjectIdentifier(element)).inserted else { continue }
            guard visited.count <= 10_000 else { throw MountedAXFailure.treeTooLarge }
            guard let accessibility = element as? NSAccessibilityProtocol else {
                throw MountedAXFailure.nonAccessibilityElement
            }

            let role = accessibility.accessibilityRole()?.rawValue
            let title = accessibility.accessibilityTitle()
            let label = accessibility.accessibilityLabel()
            let value = accessibility.accessibilityValue() as? String
            let children = accessibility.accessibilityChildren() ?? []
            strings.formUnion([title, label, value].compactMap { $0 })
            if role == NSAccessibility.Role.button.rawValue,
               let name = [title, label].compactMap({ $0 }).first(where: { !$0.isEmpty }) {
                buttons.append(MountedAXButton(label: name, frame: accessibilityFrame(of: element), element: element))
            }
            if diagnostics.count < 80 {
                diagnostics.append("\(type(of: element)) role=\(role ?? "nil") children=\(children.count) title=\((title ?? "nil").prefix(100)) label=\((label ?? "nil").prefix(100)) value=\((value ?? "nil").prefix(100))")
            }
            pending.append(contentsOf: children)
        }
        return MountedAXSnapshot(
            strings: strings,
            buttons: buttons,
            diagnostics: "visible=\(window.isVisible) key=\(window.isKeyWindow) nodes=\(visited.count)\n" + diagnostics.joined(separator: "\n")
        )
    }

    @MainActor
    private func accessibilityFrame(of element: NSObject) -> NSRect? {
        // AppKit accessibility frames use screen coordinates.
        (element as? NSAccessibilityProtocol)?.accessibilityFrame()
    }

    private struct MountedAXButton {
        let label: String
        let frame: NSRect?
        let element: NSObject
    }

    private struct MountedAXSnapshot {
        let strings: Set<String>
        let buttons: [MountedAXButton]
        let diagnostics: String
    }

    private enum MountedAXFailure: Error {
        case nonObjectChild
        case nonAccessibilityElement
        case treeTooLarge
        case oracleWindowUnavailable
        case oracleControlsUnavailable
        case oraclePressUnavailable
        case oraclePressNotObserved(Int)
    }
}
