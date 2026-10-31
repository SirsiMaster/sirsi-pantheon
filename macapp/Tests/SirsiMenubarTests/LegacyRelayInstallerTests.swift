import XCTest
@testable import SirsiMenubar

@MainActor final class LegacyRelayInstallerTests: XCTestCase {
    private func selection(_ f: LegacyRelayFixture, digest: String? = nil) throws -> CommandExecution {
        try f.result(["schema": 1, "release": "0.24.200",
            "package": "/private/var/tmp/sirsi-maintenance-installer-fixture/SirsiPantheon-0.24.200-arm64.pkg",
            "sha256": digest ?? f.digest, "device": 1, "inode": 2, "bytes": 20,
            "implementation": String(repeating: "d", count: 64)])
    }

    func testNormalInstallerUsesCapturedSelectionOnceAndRechecksInstalledPair() async throws {
        let f = LegacyRelayFixture()
        let selected = try selection(f)
        var opens = [URL](), argv = [[String]](), statuses = 0
        let model = LegacyRelayMaintenanceModel(directory: f.scope, capture: { args, _, _, _ in
            statuses += 1
            XCTAssertEqual(args, ["launchd", "legacy-maintenance", "status"])
            return try! f.status("completed")
        }, installerCapture: { args, _, _, authorize in
            XCTAssertTrue(authorize()); argv.append(args)
            return selected
        }, openInstaller: { opens.append($0); return true }, storage: f.storage)
        await model.prepareInstaller()
        let approved = try XCTUnwrap(model.installer)
        let task = try XCTUnwrap(model.install(approved))
        XCTAssertNil(model.install(approved))
        await task.value
        XCTAssertEqual(opens.map(\.path), [approved.package])
        XCTAssertEqual(argv, [["launchd", "maintenance-installer"], ["launchd", "maintenance-installer", "--verify-selection",
            selected.stdoutData.map { String(format: "%02x", $0) }.joined()]])
        XCTAssertEqual(statuses, 0, "Opening Installer is not proof of installation or service action")
        await model.recheckInstalledPair()
        XCTAssertEqual(statuses, 1)
        XCTAssertEqual(model.transaction?.phase, "completed")
    }

    func testTypedRollbackSelectionUsesOnlyExactCurrentCLICommand() async throws {
        let selection = try XCTUnwrap(AppRollbackSelection(release: "0.24.137"))
        XCTAssertNil(AppRollbackSelection(release: "latest"))
        XCTAssertNotNil(AppRollbackSelection(release: "0.24.136"))
        XCTAssertNil(AppRollbackSelection(release: "0.24.114"))
        XCTAssertNil(AppRollbackSelection(release: "0.24.137/../0.24.200"))
        var argv = [[String]]()
        let memoryFixture = MaatDiagnosticCorrelationFixture()
        let maintenanceFixture = LegacyRelayFixture()
        let maintenance = LegacyRelayMaintenanceModel(directory: maintenanceFixture.scope, storage: maintenanceFixture.storage)
        let model = MaatAppUpdateModel(memory: memoryFixture.model(), recovery: MaatPreparedPreflightRecovery(), maintenance: maintenance, capture: { args, authorize in
            XCTAssertTrue(authorize()); argv.append(args)
            return CommandExecution(stdout: Data("Ma'at rollback attempt retained".utf8), exitCode: 0, childState: .exitConfirmed)
        })
        await model.review(.rollback(selection))?.value
        let approved = try XCTUnwrap(model.approval)
        await model.confirm(approved)?.value
        XCTAssertEqual(argv, [["update", "--rollback-version", "0.24.137", "--confirm-rollback"]])
        XCTAssertTrue(model.commandReceipt?.stdout.contains("rollback attempt retained") == true)
    }

    func testRetainedMaatAttemptButtonUsesOnlyDataOnlyRecheckAndShowsReceipt() async throws {
        let retained = Data("{\"schema\":\"sirsi.maat.app-update-reconciliation.v1\",\"state\":\"resolved\",\"attempt_id\":\"a5340000-0000-4000-8000-000000000001\",\"target_release\":\"0.24.137\",\"current_release\":\"0.24.138\",\"observation\":{\"release\":\"0.24.137\"}}".utf8)
        var argv = [[String]]()
        let memoryFixture = MaatDiagnosticCorrelationFixture()
        let maintenanceFixture = LegacyRelayFixture()
        let maintenance = LegacyRelayMaintenanceModel(directory: maintenanceFixture.scope, storage: maintenanceFixture.storage)
        let model = MaatAppUpdateModel(memory: memoryFixture.model(), recovery: MaatPreparedPreflightRecovery(), maintenance: maintenance, capture: { args, authorize in
            XCTAssertTrue(authorize()); argv.append(args)
            return CommandExecution(stdout: retained, exitCode: 0, childState: .exitConfirmed)
        })
        await model.inspectAttempt()?.value
        XCTAssertEqual(argv, [["update", "--reconcile-app-attempt"]])
        XCTAssertEqual(model.attemptReport?.attemptID, "a5340000-0000-4000-8000-000000000001")
        XCTAssertEqual(model.attemptReport?.state, "resolved")
        XCTAssertTrue(model.message?.contains("0.24.137") == true)
        XCTAssertNotNil(model.readbackReceipt)
        XCTAssertNil(model.commandReceipt, "Data-only readback cannot synthesize an Installer/update invocation")
    }

    func testChangedOrUnverifiedPackageNeverOpensInstaller() async throws {
        for failure in ["changed", "trust-refused", "canceled"] {
            let f = LegacyRelayFixture()
            var calls = 0, opens = 0
            let original = try selection(f), changed = try selection(f, digest: String(repeating: "b", count: 64))
            let model = LegacyRelayMaintenanceModel(directory: f.scope, installerCapture: { _, _, _, _ in
                calls += 1
                if calls == 1 { return original }
                switch failure {
                case "changed": return changed
                case "canceled": return CommandExecution(cancelled: true, childState: .notStarted)
                default: return CommandExecution(stderr: Data("Package signature/protocol refused".utf8), exitCode: 1, childState: .exitConfirmed)
                }
            }, openInstaller: { _ in opens += 1; return true }, storage: f.storage)
            await model.prepareInstaller()
            let approved = try XCTUnwrap(model.installer)
            let task = try XCTUnwrap(model.install(approved)); await task.value
            XCTAssertEqual(opens, 0)
            XCTAssertNotNil(model.error)
            XCTAssertNil(model.install(approved), "A failed captured selection cannot be replayed")
        }
    }

    func testUnsupportedInstalledCapabilityStopsBeforeAuthorization() async {
        var invocations = 0
        let bridge = LegacyRelayPrivilegedBridge(qualify: { String(repeating: "a", count: 64) },
            identity: { nil }, compatibility: {
                let old = Data("{\"schema\":1,\"protocol\":\"older\",\"release\":\"0.24.136\",\"implementation\":\"old\",\"native\":true}".utf8)
                let capability = try JSONDecoder().decode(LegacyRelayPrivilegedBridge.Capability.self, from: old)
                XCTAssertFalse(capability.supported)
                throw NSError(domain: "MaintenanceHelperProtocolUnavailable", code: 1)
            }) { _, _ in invocations += 1; return CommandExecution(childState: .exitUnknown) }
        let result = await bridge.capture(args: ["launchd", "legacy-maintenance", "status"], stdin: nil, authorize: { true })
        XCTAssertEqual(invocations, 0)
        XCTAssertEqual(result.childState, .notStarted)
        XCTAssertNotNil(result.launchError)
    }
}
