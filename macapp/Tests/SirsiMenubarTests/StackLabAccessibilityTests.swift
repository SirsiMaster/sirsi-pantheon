import AppKit
import SwiftUI
import XCTest
@testable import SirsiMenubar

final class StackLabAccessibilityTests: XCTestCase {
    @MainActor func testNativeHostingTextAndButtonPositiveControl() throws {
        try requireIndependentAXOracle()
    }

    @MainActor func testMaatRecipeExposesTextComponentsAndPressableHandoffs() throws {
        try requireIndependentAXOracle()
        let entry = try recipe("maat-system-one-recipe-v1.json")
        // Use the full source recipe, not a reduced, hand-built card. This is
        // the long document that disappeared from the installed app's AX tree.
        XCTAssertGreaterThan(entry.components.count, 20)
        let observed = try inspect(entry, buttons: ["Open Apollo recipe", "Open Ma'at System One"])
        for text in [entry.id, entry.purpose, entry.sourcePath, entry.wing, entry.product,
                     try XCTUnwrap(entry.components.first).id, try XCTUnwrap(entry.components.last).id] {
            XCTAssertTrue(observed.contains(text), "Missing native AX text: \(text)")
        }
    }

    @MainActor func testReleaseRecipeRetainsPressablePreflightHandoff() throws {
        try requireIndependentAXOracle()
        let entry = try recipe("pantheon-release-artifact-recipe-v1.json")
        let observed = try inspect(entry, buttons: ["Open Ma'at release preflight"])
        XCTAssertTrue(observed.contains(entry.id))
        XCTAssertTrue(observed.contains(entry.sourcePath))
    }

    @MainActor func testShortWingDetailStillExposesText() throws {
        try requireIndependentAXOracle()
        let data = Data(#"{"id":"stacklab.wing.fixture","kind":"wing","source_path":"contracts/stacklab/fixture-wing.json","purpose":"Local wing contract fixture","next_action":"Review the local source"}"#.utf8)
        let entry = try JSONDecoder().decode(StackLabCatalogEntry.self, from: data)
        let observed = try inspect(entry, buttons: [])
        for text in [entry.id, entry.purpose, entry.sourcePath, entry.nextAction] {
            XCTAssertTrue(observed.contains(text), "Missing native AX text: \(text)")
        }
    }

    @MainActor func testComponentCardExposesSourceTestsAndUpgradeInstructions() throws {
        try requireIndependentAXOracle()
        let entry = try recipe("maat-system-one-recipe-v1.json")
        let component = try XCTUnwrap(entry.components.first)
        let window = try host(MaybeScroll { StackLabComponentCard(component: component) })
        defer { window.close() }
        let observed = try inspect(window, nav: nil, buttons: [])
        for text in [component.id, "Source", "Tests", "Inputs", "Outputs", "Upgrade recipe"]
            + component.source + component.tests + component.inputs + component.outputs + component.writes + component.upgradeRecipe {
            XCTAssertTrue(observed.contains(text), "Missing native AX component field: \(text)")
        }
    }

    private func recipe(_ filename: String) throws -> StackLabCatalogEntry {
        let root = URL(fileURLWithPath: #filePath).deletingLastPathComponent()
            .deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent()
        let path = "contracts/stacklab/\(filename)"
        let data = try Data(contentsOf: root.appendingPathComponent(path))
        var object = try XCTUnwrap(JSONSerialization.jsonObject(with: data) as? [String: Any])
        // These two fields belong to the canonical catalog envelope; the rest
        // is the actual versioned recipe, decoded by the production model.
        object["kind"] = "recipe"
        object["source_path"] = path
        return try JSONDecoder().decode(StackLabCatalogEntry.self, from: JSONSerialization.data(withJSONObject: object))
    }

    // No product content, recipe result, navigation or cached availability may
    // decide this precondition. Run the same public Text/Button host afresh
    // before EACH dependent test; a prior control pass is not a suite-wide grant.
    @MainActor private func requireIndependentAXOracle() throws {
        var diagnostics = "Independent public Text/Button host not yet available"
        do {
            var presses = 0
            let window = try host(VStack {
                Text("Native SwiftUI AX text control")
                Button("Native SwiftUI AX button control") { presses += 1 }
            })
            defer { window.close() }
            let observed = try waitForTree(window) { snapshot in
                snapshot.strings.contains("Native SwiftUI AX text control") &&
                    snapshot.buttons.contains { $0.label == "Native SwiftUI AX button control" }
            }
            diagnostics = observed.diagnostics
            guard observed.strings.contains("Native SwiftUI AX text control"),
                  let button = observed.buttons.first(where: { $0.label == "Native SwiftUI AX button control" }) else {
                throw AXFailure.controlUnreadable
            }
            diagnostics += "\ncontrol actions=\(button.element.accessibilityActionNames())"
            try press(button, in: window)
            for _ in 0..<40 where presses == 0 { settle(window) }
            guard presses == 1 else {
                diagnostics += "\nactual control button callbacks=\(presses), expected1"
                throw AXFailure.controlPressNotObserved
            }
            print("NATIVE_AX_CONTROL readable_and_pressed_once \(diagnostics)")
        } catch {
            // An unavailable independent oracle is not evidence about Stack Lab.
            // Product assertions run outside this catch and must fail normally.
            let reason = "In-process AX oracle unavailable: \(error). \(diagnostics). " +
                "No product verdict; separately bound external public AX proof remains required."
            print("NATIVE_AX_ORACLE_UNAVAILABLE \(reason)")
            throw XCTSkip(reason)
        }
    }

    @MainActor private func inspect(_ entry: StackLabCatalogEntry, buttons: Set<String>) throws -> Set<String> {
        let nav = Nav()
        let window = try host(StackLabContractDetail(engine: SirsiEngine(), entry: entry).environmentObject(nav))
        defer { window.close() }
        return try inspect(window, nav: nav, buttons: buttons)
    }

    @MainActor private func inspect(_ window: NSWindow, nav: Nav?, buttons: Set<String>) throws -> Set<String> {
        var strings: Set<String> = []
        var pressed: Set<String> = []
        let scroll = try XCTUnwrap(scrollView(in: try XCTUnwrap(window.contentView)), "Expected the live native scroll view")
        // Walk the real scroll document through a finite number of viewports.
        // Lazy sections are allowed to be absent until scrolled into view.
        for page in 0..<128 {
            let observed = try tree(window)
            if page == 0 { print("NATIVE_AX_INITIAL \(observed.diagnostics)") }
            strings.formUnion(observed.strings)
            for button in observed.buttons {
                let label = button.label
                guard buttons.contains(label), !pressed.contains(label) else { continue }
                let nav = try XCTUnwrap(nav)
                let before = nav.stack.count
                try press(button, in: window)
                XCTAssertEqual(nav.stack.count, before + 1, "Handoff must use the real typed NavLink")
                nav.pop()
                pressed.insert(label)
            }
            let document = try XCTUnwrap(scroll.documentView)
            let clip = scroll.contentView
            let next = min(clip.bounds.minY + max(100, clip.bounds.height * 0.75),
                           max(0, document.bounds.height - clip.bounds.height))
            if next <= clip.bounds.minY { break }
            clip.scroll(to: NSPoint(x: 0, y: next))
            scroll.reflectScrolledClipView(clip)
            settle(window)
            XCTAssertLessThan(page, 127, "Recipe exceeded the bounded native traversal")
        }
        XCTAssertEqual(pressed, buttons, "Every expected native action must be exposed and pressable")
        return strings
    }

    @MainActor private func host<V: View>(_ content: V) throws -> NSWindow {
        _ = NSApplication.shared
        guard let screen = NSScreen.main else { throw AXFailure.noGUISession }
        let visible = screen.visibleFrame.insetBy(dx: 16, dy: 16)
        let hosting = NSHostingView(rootView: content.environment(\.snapshotMode, false))
        hosting.sizingOptions = []
        hosting.autoresizingMask = [.width, .height]
        let window = NSPanel(contentRect: NSRect(x: visible.minX, y: visible.minY,
                                                width: min(1180, visible.width), height: min(780, visible.height)),
                             styleMask: [.titled, .closable, .resizable, .nonactivatingPanel], backing: .buffered, defer: false)
        window.title = "Pantheon owned native AX fixture"
        window.isFloatingPanel = false
        window.hidesOnDeactivate = false
        window.becomesKeyOnlyIfNeeded = true
        window.isReleasedWhenClosed = false
        window.contentView = hosting
        // Owned fixture only: no activation, events, external AX permissions,
        // engine tasks, real workspace, or interaction with the running app.
        // Keep the fixture on-screen: an offscreen host is not an independent
        // control for the accessibility behavior observed in a visible app.
        window.orderFront(nil)
        settle(window)
        guard window.isVisible, !window.isKeyWindow else {
            window.close()
            throw AXFailure.hostStateUnavailable
        }
        return window
    }

    @MainActor private func settle(_ window: NSWindow) {
        window.contentView?.layoutSubtreeIfNeeded()
        RunLoop.main.run(until: Date().addingTimeInterval(0.03))
    }

    @MainActor private func scrollView(in view: NSView) -> NSScrollView? {
        if let scroll = view as? NSScrollView { return scroll }
        for child in view.subviews {
            if let scroll = scrollView(in: child) { return scroll }
        }
        return nil
    }

    private struct AXButton {
        let element: NSObject
        let label: String
    }

    private struct AXSnapshot {
        var strings: Set<String> = []
        var buttons: [AXButton] = []
        var diagnostics: String = ""
    }

    @MainActor private func waitForTree(_ window: NSWindow, matching matches: (AXSnapshot) -> Bool) throws -> AXSnapshot {
        var snapshot = try tree(window)
        // Wait for the independent control, not indefinitely for missing recipe
        // content. Retain the final tree on failure instead of hiding it.
        for _ in 0..<40 {
            if matches(snapshot) { break }
            settle(window)
            snapshot = try tree(window)
        }
        return snapshot
    }

    @MainActor private func press(_ button: AXButton, in window: NSWindow) throws {
        guard button.element.accessibilityActionNames().contains(.press) else {
            throw AXFailure.pressUnavailable
        }
        button.element.accessibilityPerformAction(.press)
        settle(window)
    }

    @MainActor private func tree(_ window: NSWindow) throws -> AXSnapshot {
        var pending: [Any] = [window]
        var visited: Set<ObjectIdentifier> = []
        var snapshot = AXSnapshot()
        var diagnostics: [String] = []
        while let next = pending.popLast() {
            guard let element = next as? NSObject else { throw AXFailure.nonObjectChild }
            guard visited.insert(ObjectIdentifier(element)).inserted else { continue }
            guard visited.count <= 10_000 else { throw AXFailure.treeTooLarge }
            // AppKit's public informal client bridge supports virtual AX
            // objects without requiring formal conformance to the *complete*
            // NSAccessibility protocol. The old cast could silently discard
            // such objects. These are AX requests only, never NSView subtree reads
            // or reflection into the SwiftUI view model.
            let role = element.accessibilityAttributeValue(.role) as? String
            let label = element.accessibilityAttributeValue(.description) as? String
            let value = element.accessibilityAttributeValue(.value) as? String
            let title = element.accessibilityAttributeValue(.title) as? String
            let children = element.accessibilityAttributeValue(.children) as? [Any] ?? []
            for text in [label, value, title].compactMap({ $0 }) { snapshot.strings.insert(text) }
            if role == NSAccessibility.Role.button.rawValue, let name = title ?? label {
                snapshot.buttons.append(AXButton(element: element, label: name))
            }
            if diagnostics.count < 64 {
                diagnostics.append("\(type(of: element)) role=\(role ?? "nil") formal=\(element is any NSAccessibilityProtocol) children=\(children.count) text=\((title ?? label ?? value ?? "nil").prefix(160))")
            }
            pending.append(contentsOf: children)
        }
        snapshot.diagnostics = "visible=\(window.isVisible) key=\(window.isKeyWindow) nodes=\(visited.count)\n" + diagnostics.joined(separator: "\n")
        return snapshot
    }

    private enum AXFailure: Error {
        case treeTooLarge, nonObjectChild, pressUnavailable
        case noGUISession, hostStateUnavailable, controlUnreadable, controlPressNotObserved
    }
}
