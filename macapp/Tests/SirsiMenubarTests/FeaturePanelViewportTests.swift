import AppKit
import SwiftUI
import XCTest
@testable import SirsiMenubar

final class FeaturePanelViewportTests: XCTestCase {
    private let panelSize = NSSize(width: 560, height: 460)

    @MainActor
    func testFullDiskAccessGuideNativeScrollGeometry() throws {
        let nav = Nav()
        let guide = FDAGuideView(returnToTrash: true).environmentObject(nav)
        let window = try host(guide)
        defer { window.close() }
        settle(window)

        let root = try XCTUnwrap(window.contentView)
        let scroll = try XCTUnwrap(nativeScrollView(in: root), "FDA guide must expose its real native ScrollView")
        let document = try XCTUnwrap(scroll.documentView)
        XCTAssertGreaterThan(document.bounds.height, scroll.contentView.bounds.height)

        try scrollToDocumentEnd(scroll, window: window)
    }

    @MainActor
    func testFullDiskAccessFinalReturnControlAccessibilityFrameIsVisible() throws {
        let oracle = independentAccessibilityOracle()
        guard oracle.available else {
            throw XCTSkip("Independent native SwiftUI Text/Button AX oracle unavailable; FDA final-control visibility UNVERIFIED. \(oracle.diagnostics)")
        }
        print("FDA_AX_ORACLE_CONTROL_PASS \(oracle.diagnostics)")

        let nav = Nav()
        let window = try host(FDAGuideView(returnToTrash: true).environmentObject(nav))
        defer { window.close() }
        settle(window)

        let root = try XCTUnwrap(window.contentView)
        let scroll = try XCTUnwrap(nativeScrollView(in: root), "FDA guide must expose its real native ScrollView")
        try scrollToDocumentEnd(scroll, window: window)

        let observed = try waitForAccessibilityTree(window) { snapshot in
            snapshot.strings.contains { $0.contains("Access is not confirmed until that read succeeds") } &&
                snapshot.buttons.contains { $0.labels.contains("Back to Trash") }
        }
        print("FDA_NATIVE_AX_AFTER_SCROLL \(observed.diagnostics)")
        XCTAssertTrue(observed.strings.contains { $0.contains("Access is not confirmed until that read succeeds") },
                      "Final return guidance should be present in the native AX tree after scrolling. \(observed.diagnostics)")
        let backButton = try XCTUnwrap(
            observed.buttons.first { $0.labels.contains("Back to Trash") },
            "Expected a native AX button titled or described as Back to Trash after scrolling. \(observed.diagnostics)"
        )
        let buttonFrame = try XCTUnwrap(
            accessibilityFrame(of: backButton.element),
            "The Back to Trash AX button must expose native position and size. \(observed.diagnostics)"
        )
        let clipScreenFrame = window.convertToScreen(scroll.contentView.convert(scroll.contentView.bounds, to: nil))
        XCTAssertGreaterThan(buttonFrame.width, 0, "Back to Trash must have positive AX width")
        XCTAssertGreaterThan(buttonFrame.height, 0, "Back to Trash must have positive AX height")
        XCTAssertTrue(clipScreenFrame.contains(buttonFrame),
                      "The complete Back to Trash AX frame must be within the visible scroll clip. " +
                        "button=\(NSStringFromRect(buttonFrame)) clip=\(NSStringFromRect(clipScreenFrame))")
    }

    @MainActor
    func testMaatRecoveryScrollIsBoundedAndSelectedSectionKeepsViewport() throws {
        var readerCalls = 0
        let recovery = MaatPreparedPreflightRecovery(reader: { _ in
            readerCalls += 1
            return .init(data: Data(), exitStatus: 1, timedOut: false, launchError: "inert viewport fixture")
        })
        let statusData = try JSONSerialization.data(withJSONObject: [
            "state": "incomplete",
            "journal_readiness": "unavailable",
            "receipt_recovery_error": String(repeating: "synthetic retained receipt detail ", count: 80)
        ])
        recovery.accept(try JSONDecoder().decode(MaatFailureMemoryStatus.self, from: statusData))

        let selectedContent = SelectedContentViewportProbe()
            .frame(maxWidth: .infinity, maxHeight: .infinity)
        let frame = MaatWorkspaceRecoveryFrame(
            recovery: recovery,
            content: AnyView(selectedContent)
        ).environmentObject(Nav())
        let window = try host(frame)
        defer { window.close() }
        settle(window)

        let root = try XCTUnwrap(window.contentView)
        let scroll = try XCTUnwrap(nativeScrollView(in: root), "Ma’at recovery must expose its bounded native ScrollView")
        let document = try XCTUnwrap(scroll.documentView)
        let viewportHeight = scroll.contentView.bounds.height
        let probe = try XCTUnwrap(
            descendants(of: root).first { $0.identifier?.rawValue == "selected-content-viewport" },
            "Expected the selected section's native viewport probe"
        )

        XCTAssertLessThanOrEqual(scroll.frame.height, 221, "The recovery scroll region must stay bounded")
        XCTAssertGreaterThan(document.bounds.height, viewportHeight, "Long retained recovery detail must overflow its viewport")
        XCTAssertGreaterThan(probe.frame.width, 500)
        XCTAssertGreaterThan(probe.frame.height, 100, "Selected content must retain a usable viewport below recovery")
        XCTAssertEqual(readerCalls, 0, "Rendering and scrolling must not call the injected recovery reader")

        let selectedHeight = probe.frame.height
        try scrollToDocumentEnd(scroll, window: window)
        XCTAssertEqual(probe.frame.height, selectedHeight, accuracy: 1,
                       "Scrolling recovery must not consume or resize the selected section")
    }

    @MainActor
    private func host<Content: View>(_ content: Content) throws -> NSWindow {
        _ = NSApplication.shared
        guard let screen = NSScreen.main else {
            throw XCTSkip("A native screen is required to host the AppKit scroll viewport")
        }
        let visible = screen.visibleFrame
        let origin = NSPoint(
            x: visible.minX + max(0, (visible.width - panelSize.width) / 2),
            y: visible.minY + max(0, (visible.height - panelSize.height) / 2)
        )
        let hostingView = NSHostingView(rootView: content
            .environment(\.snapshotMode, false)
            .environment(\.sirsiTypeScale, 1.5)
            .frame(width: panelSize.width, height: panelSize.height))
        hostingView.sizingOptions = []
        hostingView.autoresizingMask = [.width, .height]
        hostingView.frame = NSRect(origin: .zero, size: panelSize)

        let window = NSPanel(contentRect: NSRect(origin: origin, size: panelSize),
                             styleMask: [.titled, .closable, .resizable, .nonactivatingPanel],
                             backing: .buffered, defer: false)
        window.title = "Pantheon feature viewport fixture"
        window.isFloatingPanel = false
        window.hidesOnDeactivate = false
        window.becomesKeyOnlyIfNeeded = true
        window.isReleasedWhenClosed = false
        window.contentView = hostingView
        window.orderFront(nil)
        settle(window)
        guard window.isVisible, !window.isKeyWindow else {
            window.close()
            throw AXFailure.hostStateUnavailable
        }
        return window
    }

    @MainActor
    private func settle(_ window: NSWindow) {
        window.contentView?.layoutSubtreeIfNeeded()
        window.displayIfNeeded()
        RunLoop.main.run(until: Date().addingTimeInterval(0.03))
        window.contentView?.layoutSubtreeIfNeeded()
    }

    @MainActor
    private func nativeScrollView(in view: NSView) -> NSScrollView? {
        if let scroll = view as? NSScrollView { return scroll }
        for child in view.subviews {
            if let scroll = nativeScrollView(in: child) { return scroll }
        }
        return nil
    }

    @MainActor
    private func descendants(of view: NSView) -> [NSView] {
        [view] + view.subviews.flatMap { descendants(of: $0) }
    }

    @MainActor
    private func scrollToDocumentEnd(_ scroll: NSScrollView, window: NSWindow,
                                    file: StaticString = #filePath, line: UInt = #line) throws {
        let document = try XCTUnwrap(scroll.documentView, file: file, line: line)
        let clip = scroll.contentView
        let startY = clip.bounds.origin.y
        let endY = document.isFlipped
            ? document.bounds.maxY - clip.bounds.height
            : document.bounds.minY
        XCTAssertGreaterThan(abs(endY - startY), 1, file: file, line: line)
        clip.scroll(to: NSPoint(x: clip.bounds.origin.x, y: endY))
        scroll.reflectScrolledClipView(clip)
        settle(window)

        let visible = clip.documentVisibleRect
        if document.isFlipped {
            XCTAssertGreaterThanOrEqual(visible.maxY, document.bounds.maxY - 1, file: file, line: line)
        } else {
            XCTAssertLessThanOrEqual(visible.minY, document.bounds.minY + 1, file: file, line: line)
        }
        XCTAssertGreaterThan(abs(clip.bounds.origin.y - startY), 1,
                             "The native clip view should advance to the document end", file: file, line: line)
    }

    private struct AXButton {
        let element: NSObject
        let labels: [String]
    }

    private struct AXSnapshot {
        var strings: Set<String> = []
        var buttons: [AXButton] = []
        var diagnostics = ""
    }

    private struct AXOracleResult {
        let available: Bool
        let diagnostics: String
    }

    @MainActor
    private func independentAccessibilityOracle() -> AXOracleResult {
        var diagnostics = "Independent public Text/Button host not yet available"
        do {
            var callbackCount = 0
            let window = try host(VStack {
                Text("Native SwiftUI AX text control")
                Button("Native SwiftUI AX button control") { callbackCount += 1 }
            })
            defer { window.close() }
            let observed = try waitForAccessibilityTree(window) { snapshot in
                snapshot.strings.contains("Native SwiftUI AX text control") &&
                    snapshot.buttons.contains { $0.labels.contains("Native SwiftUI AX button control") }
            }
            diagnostics = observed.diagnostics
            guard observed.strings.contains("Native SwiftUI AX text control"),
                  let button = observed.buttons.first(where: { $0.labels.contains("Native SwiftUI AX button control") }) else {
                return AXOracleResult(available: false, diagnostics: diagnostics + "\nSynthetic Text/Button missing")
            }
            guard let accessibility = button.element as? NSAccessibilityProtocol,
                  accessibility.accessibilityPerformPress() else {
                return AXOracleResult(available: false, diagnostics: diagnostics + "\nSynthetic button has no press action")
            }
            diagnostics += "\nsynthetic button press accepted"
            for _ in 0..<40 where callbackCount == 0 { settle(window) }
            guard callbackCount == 1 else {
                return AXOracleResult(available: false,
                                      diagnostics: diagnostics + "\nsynthetic inert callback count=\(callbackCount), expected 1")
            }
            diagnostics += "\nsynthetic inert callback count=1"
            return AXOracleResult(available: true, diagnostics: diagnostics)
        } catch {
            diagnostics += "\nIndependent AX oracle error: \(error)"
            return AXOracleResult(available: false, diagnostics: diagnostics)
        }
    }

    @MainActor
    private func waitForAccessibilityTree(_ window: NSWindow, matching matches: (AXSnapshot) -> Bool) throws -> AXSnapshot {
        var snapshot = try accessibilityTree(window)
        for _ in 0..<40 {
            if matches(snapshot) { break }
            settle(window)
            snapshot = try accessibilityTree(window)
        }
        return snapshot
    }

    @MainActor
    private func accessibilityTree(_ window: NSWindow) throws -> AXSnapshot {
        var pending: [Any] = [window]
        var visited: Set<ObjectIdentifier> = []
        var snapshot = AXSnapshot()
        var diagnostics: [String] = []
        while let next = pending.popLast() {
            guard let element = next as? NSObject else {
                throw AXFailure.nonObjectChild
            }
            guard visited.insert(ObjectIdentifier(element)).inserted else { continue }
            guard visited.count <= 10_000 else { throw AXFailure.treeTooLarge }

            guard let accessibility = element as? NSAccessibilityProtocol else {
                throw AXFailure.nonAccessibilityElement
            }
            let role = accessibility.accessibilityRole()?.rawValue
            let title = accessibility.accessibilityTitle()
            let description = accessibility.accessibilityLabel()
            let value = accessibility.accessibilityValue() as? String
            let children = accessibility.accessibilityChildren() ?? []
            for text in [title, description, value].compactMap({ $0 }) {
                snapshot.strings.insert(text)
            }
            let labels: [String] = [title, description].compactMap { value in
                guard let value, !value.isEmpty else { return nil }
                return value
            }
            if role == NSAccessibility.Role.button.rawValue, !labels.isEmpty {
                snapshot.buttons.append(AXButton(element: element, labels: labels))
            }
            if diagnostics.count < 64 {
                diagnostics.append("\(type(of: element)) role=\(role ?? "nil") formal=\(element is any NSAccessibilityProtocol) children=\(children.count) title=\((title ?? "nil").prefix(100)) description=\((description ?? "nil").prefix(100)) value=\((value ?? "nil").prefix(100))")
            }
            pending.append(contentsOf: children)
        }
        snapshot.diagnostics = "visible=\(window.isVisible) key=\(window.isKeyWindow) nodes=\(visited.count)\n" +
            diagnostics.joined(separator: "\n")
        return snapshot
    }

    @MainActor
    private func accessibilityFrame(of element: NSObject) -> NSRect? {
        (element as? NSAccessibilityProtocol)?.accessibilityFrame()
    }

    private enum AXFailure: Error {
        case nonObjectChild
        case nonAccessibilityElement
        case treeTooLarge
        case hostStateUnavailable
    }
}

@MainActor
private struct SelectedContentViewportProbe: NSViewRepresentable {
    func makeNSView(context: Context) -> NSView {
        let view = NSView()
        view.identifier = NSUserInterfaceItemIdentifier("selected-content-viewport")
        return view
    }

    func updateNSView(_ view: NSView, context: Context) {}
}
