import Foundation

enum NativeVersionProtocol {
    enum IdentityError: Error {
        case unavailable
    }

    private static let releasePattern = #"^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?$"#

    // Report only identity stamped on this native bundle. Go-only commit,
    // date, dirty-tree, and schema fields are intentionally absent.
    static func response(bundleInfo: [String: Any], executablePath: String) throws -> Data {
        guard bundleInfo["CFBundleIdentifier"] as? String == "ai.sirsi.pantheon",
              bundleInfo["CFBundleExecutable"] as? String == "sirsi-menubar",
              let version = bundleInfo["CFBundleShortVersionString"] as? String,
              let build = bundleInfo["CFBundleVersion"] as? String,
              version == build,
              version.range(of: releasePattern, options: .regularExpression) != nil,
              executablePath.hasPrefix("/"),
              URL(fileURLWithPath: executablePath).lastPathComponent == "sirsi-menubar" else {
            throw IdentityError.unavailable
        }

        let path = URL(fileURLWithPath: executablePath).standardizedFileURL.path
        let envelope: [String: String] = [
            "binary": "sirsi-menubar",
            "version": "v\(version)",
            "path": path
        ]
        return try JSONSerialization.data(withJSONObject: envelope, options: [.sortedKeys, .withoutEscapingSlashes])
    }
}
