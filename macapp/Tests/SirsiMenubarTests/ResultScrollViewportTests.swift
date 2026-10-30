import AppKit
import SwiftUI
import XCTest
@testable import SirsiMenubar

final class ResultScrollViewportTests: XCTestCase {
    @MainActor
    func testPreloadedResultScrollsItsNativeDocumentAtEnlargedTypeScale() throws {
        let result = try syntheticResult()
        let engine = SirsiEngine()
        let nav = Nav()
        var authorizationChecks = 0
        let resultView = ResultView(
            engine: engine,
            title: "Result scroll regression",
            args: ["scan"],
            preloaded: result,
            authorizeExecution: {
                authorizationChecks += 1
                return false
            }
        )
        .environmentObject(nav)
        .environment(\.snapshotMode, false)
        .environment(\.sirsiTypeScale, 1.5)
        .frame(width: 560, height: 460)

        let hostingView = NSHostingView(rootView: resultView)
        hostingView.frame = NSRect(x: 0, y: 0, width: 560, height: 460)
        let window = NSWindow(
            contentRect: NSRect(x: 0, y: 0, width: 560, height: 460),
            styleMask: [],
            backing: .buffered,
            defer: false
        )
        window.isReleasedWhenClosed = false
        window.contentView = hostingView
        defer { window.close() }

        window.contentView?.layoutSubtreeIfNeeded()
        window.displayIfNeeded()
        hostingView.layoutSubtreeIfNeeded()

        let scrollViews = descendants(of: hostingView).compactMap { $0 as? NSScrollView }
        let scrollView = try XCTUnwrap(
            scrollViews.first { candidate in
                guard let document = candidate.documentView else { return false }
                return document.bounds.height > candidate.contentView.bounds.height
            },
            "Expected ResultView's live SwiftUI ScrollView to expose an overflowing native document"
        )
        let document = try XCTUnwrap(scrollView.documentView)
        let clipView = scrollView.contentView
        let viewportHeight = clipView.bounds.height
        let documentHeight = document.bounds.height

        XCTAssertGreaterThan(documentHeight, viewportHeight)

        let startY = clipView.bounds.origin.y
        let endY = document.isFlipped
            ? documentHeight - viewportHeight
            : 0
        clipView.scroll(to: NSPoint(x: clipView.bounds.origin.x, y: endY))
        scrollView.reflectScrolledClipView(clipView)
        hostingView.layoutSubtreeIfNeeded()

        let finalY = clipView.bounds.origin.y
        XCTAssertGreaterThan(abs(finalY - startY), 1, "Scrolling to the document end must move the native clip view")
        XCTAssertEqual(authorizationChecks, 0, "Rendering and scrolling must not authorize or invoke a result action")
    }

    private func syntheticResult() throws -> CommandResult {
        let evidence: [[String: String]] = (1...30).map { index in
            [
                "label": "Evidence item \(index)",
                "value": "Retained diagnostic detail for item \(index), with enough text to wrap at enlarged type size."
            ]
        }
        let object: [String: Any] = [
            "command": "sirsi scan --json",
            "summary": "Synthetic result with enough evidence to require a scrollable document.",
            "status": "ok",
            "evidence": evidence,
            "next_actions": [
                [
                    "label": "Review the final action",
                    "command": "sirsi status",
                    "description": "A passive final action included to exercise the complete result layout."
                ]
            ]
        ]
        let data = try JSONSerialization.data(withJSONObject: object)
        return try JSONDecoder().decode(CommandResult.self, from: data)
    }

    @MainActor
    private func descendants(of view: NSView) -> [NSView] {
        [view] + view.subviews.flatMap { descendants(of: $0) }
    }
}
