import XCTest
@testable import SirsiMenubar

final class SNEControlReadModelTests: XCTestCase {

    func testBundledSirsiResolverPrefersExecutableInsideBundle() throws {
        let root = URL(fileURLWithPath: NSTemporaryDirectory())
            .appendingPathComponent(UUID().uuidString, isDirectory: true)
        let executable = root
            .appendingPathComponent("Pantheon.app", isDirectory: true)
            .appendingPathComponent("Contents", isDirectory: true)
            .appendingPathComponent("MacOS", isDirectory: true)
            .appendingPathComponent("sirsi", isDirectory: false)
        try FileManager.default.createDirectory(at: executable.deletingLastPathComponent(), withIntermediateDirectories: true)
        try Data("fixture".utf8).write(to: executable)
        try FileManager.default.setAttributes([.posixPermissions: 0o755], ofItemAtPath: executable.path)
        defer { try? FileManager.default.removeItem(at: root) }

        XCTAssertEqual(SirsiEngine.bundledSirsiBinary(bundleURL: root.appendingPathComponent("Pantheon.app")), executable.path)
    }

    func testBundledControlEngineResolverRequiresEmbeddedHelper() throws {
        let root = URL(fileURLWithPath: NSTemporaryDirectory())
            .appendingPathComponent(UUID().uuidString, isDirectory: true)
        let app = root.appendingPathComponent("Pantheon.app", isDirectory: true)
        let executable = app
            .appendingPathComponent("Contents/Library/Helpers", isDirectory: true)
            .appendingPathComponent("pantheon-engine", isDirectory: false)
        try FileManager.default.createDirectory(at: executable.deletingLastPathComponent(), withIntermediateDirectories: true)
        XCTAssertNil(SNELocalControlBridge.bundledEngine(bundleURL: app))
        try Data("fixture".utf8).write(to: executable)
        try FileManager.default.setAttributes([.posixPermissions: 0o755], ofItemAtPath: executable.path)
        defer { try? FileManager.default.removeItem(at: root) }

        XCTAssertEqual(SNELocalControlBridge.bundledEngine(bundleURL: app), executable.path)
    }
    func testSnapshotFixtureRetainsExactActiveIdentity() {
        let fixture = SNEReadViewState.snapshotFixture

        XCTAssertTrue(fixture.configured)
        XCTAssertTrue(fixture.ready)
        XCTAssertEqual(fixture.serviceState, "ready")
        XCTAssertEqual(fixture.activeModel, fixture.lifecycle.modelID)
        XCTAssertEqual(fixture.lifecycle.runtimeID, "sne-v2-api4096")
        XCTAssertEqual(fixture.lifecycle.runtimeSHA256?.count, 64)
        XCTAssertEqual(fixture.lifecycle.modelManifestSHA256?.count, 64)
        XCTAssertTrue(fixture.lifecycleToolsReady)
    }

    func testReadModelDecodesLifecycleAndCatalogTruthfully() throws {
        let payload = """
        {
          "configured": true,
          "ready": false,
          "service_state": "awaiting-session-for-gpu-reacquisition",
          "active_model": "gemma-test",
          "device_family": "Apple Silicon",
          "unified_memory_bytes": 34359738368,
          "catalog": [{
            "catalog_entry": "gemma-test-entry",
            "model_id": "gemma-test",
            "runtime_id": "runtime-test",
            "parameter_class": "test",
            "execution_mode": "plain",
            "weight_format": "affine",
            "weight_bits": 8,
            "memory_bytes": 1,
            "support_status": "awaiting-session",
            "installed": true,
            "active": true,
            "action_label": "Retry",
            "action_enabled": true,
            "removal_enabled": false,
            "license_acceptance_required": false
          }],
          "runtime_catalog": {
            "state": "verified",
            "rollback_available": true,
            "update_feed_configured": false
          },
          "lifecycle": {
            "model_id": "gemma-test",
            "runtime_id": "runtime-test",
            "state": "awaiting-session-for-gpu-reacquisition",
            "error_code": "metal_session_locked"
          },
          "lifecycle_tools_ready": false,
          "lifecycle_tools_status": "Session required"
        }
        """

        let state = try JSONDecoder().decode(SNEReadViewState.self, from: Data(payload.utf8))

        XCTAssertFalse(state.ready)
        XCTAssertEqual(state.serviceState, "awaiting-session-for-gpu-reacquisition")
        XCTAssertEqual(state.lifecycle.errorCode, "metal_session_locked")
        XCTAssertEqual(state.catalog.single?.actionLabel, "Retry")
        XCTAssertFalse(state.lifecycleToolsReady)
        XCTAssertEqual(state.lifecycleToolsStatus, "Session required")
    }

    func testUnavailableCapabilityDoesNotInstructAServiceRestart() {
        let message = SNEControlError.capabilityUnavailable.localizedDescription

        XCTAssertTrue(message.contains("No SNE state changed"))
        XCTAssertTrue(message.contains("owner action"))
        XCTAssertFalse(message.localizedCaseInsensitiveContains("restart"))
    }
}

private extension Collection {
    var single: Element? { count == 1 ? first : nil }
}
