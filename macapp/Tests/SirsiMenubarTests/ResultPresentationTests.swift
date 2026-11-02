import AppKit
import SwiftUI
import XCTest
@testable import SirsiMenubar

final class ResultPresentationTests: XCTestCase {
    @MainActor
    func testActionStylesGrowWithTypeScale() throws {
        for prominent in [false, true] {
            let regular = try renderAction(width: 520, scale: 1.0, prominent: prominent)
            let enlarged = try renderAction(width: 520, scale: 1.5, prominent: prominent)

            XCTAssertGreaterThanOrEqual(regular.size.height, 52)
            XCTAssertGreaterThanOrEqual(enlarged.size.height, 52)
            XCTAssertGreaterThan(enlarged.size.height, regular.size.height)
            XCTAssertLessThanOrEqual(regular.size.width, 520)
            XCTAssertLessThanOrEqual(enlarged.size.width, 520)
        }
    }

    @MainActor
    func testLongActionCopyWrapsAtNarrowWidthAndEnlargedTypeScale() throws {
        for prominent in [false, true] {
            let short = try renderAction(width: 300, scale: 1.5, prominent: prominent)
            let long = try renderAction(width: 300, scale: 1.5, prominent: prominent, long: true)

            XCTAssertGreaterThan(long.size.height, short.size.height)
            XCTAssertLessThanOrEqual(long.size.width, 300)
        }
    }

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

    // Render only the inert production label and style. The empty action cannot
    // launch a command or perform a repair.
    @MainActor
    private func renderAction(
        width: CGFloat,
        scale: CGFloat,
        prominent: Bool,
        long: Bool = false
    ) throws -> NSImage {
        let label = ResultActionLabel(
            title: long ? "Restore managed services and verify their recovery" : "Relieve selected cache",
            detail: long
                ? "Review the affected services, apply the selected recovery, and confirm that each service returns to its expected state."
                : "Show the reason and expected result before applying this change.",
            prominent: prominent,
            systemImage: "info.circle"
        )
        let content = Button {} label: {
            label
        }
        .buttonStyle(ResultActionButtonStyle(prominent: prominent))
        .environment(\.sirsiTypeScale, scale)
        .environment(\.colorScheme, .dark)
        .frame(width: width)
        .fixedSize(horizontal: false, vertical: true)
        .background(PantheonTheme.canvas)

        let renderer = ImageRenderer(content: content)
        renderer.scale = 2
        let image = try XCTUnwrap(renderer.nsImage)
        if let directory = ProcessInfo.processInfo.environment["PANTHEON_RESULT_LAYOUT_CAPTURE_DIR"] {
            let root = URL(fileURLWithPath: directory, isDirectory: true)
            try FileManager.default.createDirectory(at: root, withIntermediateDirectories: true)
            let bitmap = try XCTUnwrap(image.tiffRepresentation.flatMap(NSBitmapImageRep.init(data:)))
            let data = try XCTUnwrap(bitmap.representation(using: .png, properties: [:]))
            let variant = prominent ? "prominent" : "secondary"
            let copy = long ? "long" : "short"
            try data.write(to: root.appendingPathComponent("result-action-\(variant)-\(Int(width))-\(Int(scale * 100))-\(copy).png"))
        }
        return image
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
