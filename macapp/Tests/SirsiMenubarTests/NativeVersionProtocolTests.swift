import Foundation
import XCTest
@testable import SirsiMenubar

final class NativeVersionProtocolTests: XCTestCase {
    private let executable = "/fixture/Contents/MacOS/sirsi-menubar"

    func testResponseUsesOnlyTheNativeBundlesStampedIdentity() throws {
        let data = try NativeVersionProtocol.response(bundleInfo: stampedBundle(), executablePath: executable)
        let object = try XCTUnwrap(JSONSerialization.jsonObject(with: data) as? [String: String])

        XCTAssertEqual(object["binary"], "sirsi-menubar")
        XCTAssertEqual(object["version"], "v0.24.154")
        XCTAssertEqual(object["path"], executable)
        XCTAssertEqual(Set(object.keys), ["binary", "version", "path"])
        XCTAssertNil(object["commit"], "The native bundle has no stamped Go commit")
        XCTAssertNil(object["dirty"], "The native bundle has no Go working-tree state")
    }

    func testMissingForeignMalformedOrMismatchedNativeStampRefuses() {
        let cases: [(String, [String: Any], String)] = [
            ("missing bundle id", removing("CFBundleIdentifier"), executable),
            ("foreign bundle", replacing("CFBundleIdentifier", with: "example.foreign"), executable),
            ("wrong executable", replacing("CFBundleExecutable", with: "sirsi"), executable),
            ("missing short version", removing("CFBundleShortVersionString"), executable),
            ("malformed short version", replacing("CFBundleShortVersionString", with: "154"), executable),
            ("missing build", removing("CFBundleVersion"), executable),
            ("mismatched build", replacing("CFBundleVersion", with: "0.24.153"), executable),
            ("relative executable path", stampedBundle(), "Contents/MacOS/sirsi-menubar"),
            ("foreign executable leaf", stampedBundle(), "/fixture/Contents/MacOS/sirsi"),
            ("missing executable path", stampedBundle(), "")
        ]

        for (name, info, path) in cases {
            XCTAssertThrowsError(try NativeVersionProtocol.response(bundleInfo: info, executablePath: path), name)
        }
    }

    private func stampedBundle() -> [String: Any] {
        [
            "CFBundleIdentifier": "ai.sirsi.pantheon",
            "CFBundleExecutable": "sirsi-menubar",
            "CFBundleShortVersionString": "0.24.154",
            "CFBundleVersion": "0.24.154"
        ]
    }

    private func removing(_ key: String) -> [String: Any] {
        var info = stampedBundle()
        info.removeValue(forKey: key)
        return info
    }

    private func replacing(_ key: String, with value: String) -> [String: Any] {
        var info = stampedBundle()
        info[key] = value
        return info
    }
}
