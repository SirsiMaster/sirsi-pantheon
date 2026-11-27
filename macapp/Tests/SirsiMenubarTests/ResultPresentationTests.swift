import AppKit
import SwiftUI
import XCTest
@testable import SirsiMenubar

final class ResultPresentationTests: XCTestCase {
    @MainActor
    func testStructuredCommandErrorDetailsPreserveAndWrapLongFailure() throws {
        let failure = "Cache purge failed: macOS denied administrator authorization while removing the selected cache entries. No cache data was removed. Recheck the administrator account and disk permissions, then review the finding before retrying."
        let payload = """
        {"summary":"Cache purge could not be completed.","status":"error","evidence":[],"next_actions":[],"errors":["\(failure)"]}
        """
        let result = try JSONDecoder().decode(CommandResult.self, from: Data(payload.utf8))
        XCTAssertEqual(result.errors, [failure])

        let short = try renderErrors(["Purge failed."])
        let long = try renderErrors(result.errors)

        XCTAssertGreaterThan(long.size.height, short.size.height)
        XCTAssertLessThanOrEqual(long.size.width, 320)
    }

    @MainActor
    func testLoadingPlaceholderHasUsableResultPanelMinimumSize() throws {
        // Keep this a passive placeholder: no engine, command, or .task is attached.
        let placeholder = VStack(alignment: .leading, spacing: 12) {
            ProgressView()
            Text("Preparing results…")
                .sirsiFont(.callout)
                .foregroundStyle(.secondary)
        }
        .padding(20)
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
        .background(PantheonTheme.canvas)
        .modifier(ResultPanelLayout())
        .environment(\.colorScheme, .dark)

        let hostingView = NSHostingView(rootView: placeholder)
        let fittingSize = hostingView.fittingSize

        XCTAssertGreaterThanOrEqual(fittingSize.width, 560)
        XCTAssertGreaterThanOrEqual(fittingSize.height, 460)

        if let directory = ProcessInfo.processInfo.environment["PANTHEON_RESULT_LAYOUT_CAPTURE_DIR"] {
            try capture(hostingView, size: fittingSize, directory: directory)
        }
    }

    @MainActor
    private func renderErrors(_ errors: [String]) throws -> NSImage {
        let content = ResultView.ResultErrorDetails(errors: errors)
            .frame(width: 320)
            .fixedSize(horizontal: false, vertical: true)
            .environment(\.colorScheme, .dark)
            .background(PantheonTheme.canvas)
        let renderer = ImageRenderer(content: content)
        renderer.scale = 2
        return try XCTUnwrap(renderer.nsImage)
    }

    @MainActor
    private func capture<Content: View>(_ hostingView: NSHostingView<Content>, size: CGSize, directory: String) throws {
        let root = URL(fileURLWithPath: directory, isDirectory: true)
        try FileManager.default.createDirectory(at: root, withIntermediateDirectories: true)
        let bounds = NSRect(origin: .zero, size: size)
        hostingView.setFrameSize(size)
        hostingView.layoutSubtreeIfNeeded()
        let bitmap = try XCTUnwrap(hostingView.bitmapImageRepForCachingDisplay(in: bounds))
        hostingView.cacheDisplay(in: bounds, to: bitmap)
        let data = try XCTUnwrap(bitmap.representation(using: .png, properties: [:]))
        try data.write(to: root.appendingPathComponent("result-panel-loading-minimum.png"))
    }
}
