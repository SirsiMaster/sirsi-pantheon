import XCTest
@testable import SirsiMenubar

@MainActor final class MaatAppUpdateTests: XCTestCase {
    private func maintenanceFixture() -> LegacyRelayMaintenanceModel {
        let fixture = LegacyRelayFixture()
        return LegacyRelayMaintenanceModel(directory: fixture.scope, storage: fixture.storage)
    }

    private func expectNoFurtherInvocation(_ unexpected: Task<Void, Never>?,
                                           file: StaticString = #filePath, line: UInt = #line) async {
        guard let unexpected else { return }
        // Adversarial controls may expose a replay bug. Always cancel/join the
        // returned owned task before failing, rather than leaking fixture work.
        unexpected.cancel(); await unexpected.value
        XCTFail("Consumed consent returned another invocation task", file: file, line: line)
    }

    @MainActor private final class HeldRead {
        let entered: XCTestExpectation
        private var continuation: CheckedContinuation<Void, Never>?
        private var released = false
        init(_ entered: XCTestExpectation) { self.entered = entered }
        func read() async -> SirsiEngine.JSONCommandRead {
            await wait()
            return MaatDiagnosticCorrelationFixture.lost
        }
        func wait() async {
            entered.fulfill()
            await withCheckedContinuation { (value: CheckedContinuation<Void, Never>) in
                if self.released { value.resume() }
                else { self.continuation = value }
            }
        }
        func release() {
            released = true; let pending = continuation; continuation = nil
            pending?.resume()
        }
        var pending: Bool { continuation != nil }
    }

    func testActualUpdateControlInitializesThenRechecksAndConfirmsCanonicalOnce() async throws {
        let fixture = MaatDiagnosticCorrelationFixture(); fixture.initialized = false
        let memory = fixture.model(), recovery = MaatPreparedPreflightRecovery()
        var submitted = [[String]]()
        let bridge = LegacyRelayInstallerCapture(qualify: { "/owned/inert/sirsi" }, invoke: { path, args, _, authorize in
            XCTAssertEqual(path, "/owned/inert/sirsi"); XCTAssertTrue(authorize())
            submitted.append(args)
            return CommandExecution(exitCode: 0, childState: .exitConfirmed)
        })
        let legacyFixture = LegacyRelayFixture()
        let maintenance = LegacyRelayMaintenanceModel(directory: legacyFixture.scope, storage: legacyFixture.storage)
        let model = MaatAppUpdateModel(memory: memory, recovery: recovery, maintenance: maintenance, capture: { args, authorize in
            await bridge.capture(args: args, authorize: authorize)
        })
        let panel = LegacyRelayMaintenancePanel(directory: legacyFixture.scope, appUpdate: model)
        let view = panel.managedUpdateControl
        XCTAssertTrue(view.model === model)
        XCTAssertTrue(view.memoryControl.model === memory)
        XCTAssertTrue(view.recoveryControl.model === recovery)
        let owner = MaatCaseOperationStore(preparedPreflightRecovery: recovery, diagnosticRecord: memory, legacyMaintenance: maintenance)
        XCTAssertTrue(owner.appUpdate.maintenance === owner.legacyMaintenance)
        XCTAssertTrue(panel.managedUpdateControl.model.maintenance === owner.legacyMaintenance)
        XCTAssertTrue(owner.appUpdate.memory === owner.diagnosticRecord)
        XCTAssertTrue(owner.appUpdate.recovery === owner.preparedPreflightRecovery)
        await view.reviewUpdate()?.value
        XCTAssertNil(model.approval); XCTAssertTrue(submitted.isEmpty)
        let initialize = try XCTUnwrap(memory.approval(.initialize))
        await view.memoryControl.confirmed(initialize)
        XCTAssertEqual(fixture.calls.filter { $0 == maatCasebookFailureMemoryArguments(.initialize) }.count, 1)
        XCTAssertTrue(view.memoryControl.completedDetails.contains("Initialization verified"))
        XCTAssertTrue(submitted.isEmpty, "Initialization alone cannot start Installer")
        await view.reviewSavedAction()?.value
        let approved = try XCTUnwrap(model.approval)
        XCTAssertEqual(approved.storeUUID, fixture.store)
        let task = try XCTUnwrap(view.confirm(approved))
        await expectNoFurtherInvocation(view.confirm(approved)); await task.value
        XCTAssertEqual(submitted, [["update", "--app"]])
        await expectNoFurtherInvocation(view.confirm(approved))
        XCTAssertNil(model.attemptReport, "Command exit0 is not an installed-success receipt")
        let unrecognized = await bridge.capture(args: ["update", "--app", "; touch anything"], authorize: { true })
        XCTAssertEqual(unrecognized.childState, .notStarted)
        XCTAssertEqual(submitted.count, 1)
    }

    func testCorruptMemoryOffersExistingRecoveryWithoutInitializationOrInstaller() async throws {
        for state in ["unverifiable", "corrupt", "missing_existing"] {
            let fixture = MaatDiagnosticCorrelationFixture()
            fixture.override = { args in
                args == MaatDiagnosticRecordModel.inspectArguments
                    ? MaatDiagnosticCorrelationFixture.reply(["state": state, "detail": "preserve existing custody"]) : nil
            }
            var recoveryReads = [[String]](), commands = 0
            let recovery = MaatPreparedPreflightRecovery(reader: { args in
                recoveryReads.append(args)
                return MaatDiagnosticCorrelationFixture.reply(["state":"unverifiable", "detail":"retained custody cannot be adopted"])
            })
            let memory = fixture.model()
            let model = MaatAppUpdateModel(memory: memory, recovery: recovery, maintenance: maintenanceFixture(), capture: { _, _ in
                commands += 1; return CommandExecution(exitCode: 1, childState: .exitConfirmed)
            })
            let view = MaatAppUpdateControl(model: model)
            await view.reviewUpdate()?.value
            XCTAssertNil(model.approval); XCTAssertNil(memory.approval(.initialize))
            await view.recoveryControl.inspect()
            XCTAssertEqual(recoveryReads, [MaatDiagnosticRecordModel.inspectArguments])
            XCTAssertEqual(commands, 0)
            XCTAssertFalse(fixture.calls.contains { $0.contains("--initialize-failure-memory") })
            XCTAssertEqual(model.pendingAction, .update)
            XCTAssertNotNil(model.message)
        }
    }

    func testCancelJoinsHeldReviewAndQueuedConfirmationNeverInvokesUpdate() async throws {
        let fixture = MaatDiagnosticCorrelationFixture()
        let entered = expectation(description: "owned memory read entered")
        let held = HeldRead(entered); defer { held.release() }
        fixture.override = { args in args == MaatDiagnosticRecordModel.inspectArguments ? await held.read() : nil }
        var commands = 0
        let model = MaatAppUpdateModel(memory: fixture.model(), recovery: MaatPreparedPreflightRecovery(), maintenance: maintenanceFixture(), capture: { _, _ in
            commands += 1; return CommandExecution(exitCode: 0, childState: .exitConfirmed)
        })
        let view = MaatAppUpdateControl(model: model)
        let review = try XCTUnwrap(view.reviewUpdate())
        await fulfillment(of: [entered], timeout: 2)
        view.cancel(); held.release(); await review.value
        XCTAssertFalse(held.pending); XCTAssertFalse(model.busy); XCTAssertFalse(model.memory.busy); XCTAssertNil(model.approval)
        XCTAssertEqual(commands, 0)
        fixture.override = nil
        await view.reviewSavedAction()?.value
        let approved = try XCTUnwrap(model.approval)
        let confirm = try XCTUnwrap(view.confirm(approved))
        view.cancel(); await confirm.value
        XCTAssertNil(model.approval); XCTAssertFalse(model.busy); XCTAssertFalse(model.memory.busy); XCTAssertEqual(commands, 0)
        XCTAssertFalse(fixture.calls.contains { $0.contains("--initialize-failure-memory") })
    }

    func testChangedStoreOrPreparedDebtConsumesConsentBeforeAnyUpdate() async throws {
        for state in ["other-store", "unverifiable", "incomplete", "projection", "receipt-debt"] {
            let fixture = MaatDiagnosticCorrelationFixture()
            var commands = 0
            let model = MaatAppUpdateModel(memory: fixture.model(), recovery: MaatPreparedPreflightRecovery(), maintenance: maintenanceFixture(), capture: { _, _ in
                commands += 1; return CommandExecution(exitCode: 0, childState: .exitConfirmed)
            })
            let view = MaatAppUpdateControl(model: model)
            await view.reviewUpdate()?.value
            let approved = try XCTUnwrap(model.approval)
            fixture.override = { args in
                guard args == MaatDiagnosticRecordModel.inspectArguments else { return nil }
                var status: [String: Any] = ["state":"ready", "store_uuid":fixture.store]
                switch state {
                case "other-store": status["store_uuid"] = "b3c515a2-4773-4653-86d1-8a5734c2ee46"
                case "unverifiable": status["state"] = "unverifiable"
                case "incomplete": status["incomplete_attempt"] = true
                case "projection": status["projection_debt"] = true
                default: status["receipt_recovery_error"] = "prepared receipt is not verified"
                }
                return MaatDiagnosticCorrelationFixture.reply(status)
            }
            await view.confirm(approved)?.value
            XCTAssertNil(model.approval); await expectNoFurtherInvocation(view.confirm(approved))
            XCTAssertEqual(commands, 0)
            XCTAssertTrue(model.message?.contains("Memory changed") == true)
        }
    }

    func testFailedOwnedRollbackRetainsReceiptAndRequiresFreshMemoryRecoveryReview() async throws {
        let fixture = MaatDiagnosticCorrelationFixture()
        var commands = [[String]]()
        var readExit: Int32 = 0
        let memory = fixture.model()
        let model = MaatAppUpdateModel(memory: memory, recovery: MaatPreparedPreflightRecovery(), maintenance: maintenanceFixture(), capture: { args, authorize in
            XCTAssertTrue(authorize()); commands.append(args)
            if args == ["update", "--reconcile-app-attempt"] {
                return CommandExecution(stdout: Data("{\"schema\":\"sirsi.maat.app-update-reconciliation.v1\",\"state\":\"no_pending_attempt\"}".utf8), exitCode: readExit, childState: .exitConfirmed)
            }
            if commands.count == 1 {
                fixture.initialized = false
                return CommandExecution(stderr: Data("requires initialized failure-memory store".utf8), exitCode: 1, childState: .exitConfirmed)
            }
            return CommandExecution(stdout: Data("canonical retry returned".utf8), exitCode: 0, childState: .exitConfirmed)
        })
        let view = MaatAppUpdateControl(model: model)
        let selection = try XCTUnwrap(AppRollbackSelection(release: "0.24.137"))
        await view.reviewRollback(selection)?.value
        let first = try XCTUnwrap(model.approval)
        await view.confirm(first)?.value
        XCTAssertEqual(model.commandReceipt?.exitCode, 1); XCTAssertNil(model.attemptReport)
        XCTAssertEqual(commands, [selection.commandArguments])
        await view.reviewSavedAction()?.value
        XCTAssertNil(model.approval)
        let initialize = try XCTUnwrap(memory.approval(.initialize))
        await view.memoryControl.confirmed(initialize)
        XCTAssertEqual(model.commandReceipt?.exitCode, 1, "Recovery cannot discard the failed update receipt")
        XCTAssertEqual(commands.count, 1)
        await view.reviewSavedAction()?.value
        let retry = try XCTUnwrap(model.approval)
        XCTAssertEqual(retry.action, .rollback(selection))
        let task = try XCTUnwrap(view.confirm(retry)); await expectNoFurtherInvocation(view.confirm(retry)); await task.value
        XCTAssertEqual(commands, [selection.commandArguments, selection.commandArguments])
        await model.inspectAttempt()?.value
        XCTAssertEqual(commands.last, ["update", "--reconcile-app-attempt"])
        XCTAssertEqual(model.attemptReport?.state, "no_pending_attempt")
        XCTAssertTrue(model.message?.contains("does not establish") == true)
        readExit = 1
        await model.inspectAttempt()?.value
        XCTAssertNil(model.attemptReport, "A contradictory nonzero read cannot retain a current verified no-pending report")
        XCTAssertTrue(model.message?.contains("unverified") == true)
        XCTAssertEqual(commands.filter { $0 == selection.commandArguments }.count, 2)
        var unknownCalls = 0
        let unknownModel = MaatAppUpdateModel(memory: fixture.model(), recovery: MaatPreparedPreflightRecovery(), maintenance: maintenanceFixture(), capture: { _, _ in
            unknownCalls += 1
            return CommandExecution(timedOut: true, childState: .exitUnknown)
        })
        let unknownView = MaatAppUpdateControl(model: unknownModel)
        await unknownView.reviewUpdate()?.value
        let unknownApproval = try XCTUnwrap(unknownModel.approval)
        await unknownView.confirm(unknownApproval)?.value
        XCTAssertEqual(unknownCalls, 1)
        XCTAssertTrue(unknownModel.commandReceipt?.timedOut == true)
        XCTAssertNil(unknownModel.attemptReport)
        await expectNoFurtherInvocation(unknownView.confirm(unknownApproval))
        XCTAssertEqual(fixture.calls.filter { $0 == maatCasebookFailureMemoryArguments(.initialize) }.count, 1)
    }

    private func installerSelection(_ fixture: LegacyRelayFixture) throws -> CommandExecution {
        try fixture.result(["schema": 1, "release": "0.24.200",
            "package": "/private/var/tmp/sirsi-maintenance-installer-fixture/SirsiPantheon-0.24.200-arm64.pkg",
            "sha256": fixture.digest, "device": 1, "inode": 2, "bytes": 20,
            "implementation": String(repeating: "d", count: 64)])
    }

    func testManagedCaptureBlocksLegacyInstallerAndRootUntilCancellationJoins() async throws {
        let fixture = LegacyRelayFixture(), memory = MaatDiagnosticCorrelationFixture()
        let selected = try installerSelection(fixture), preview = try fixture.preview()
        var rootCalls = [[String]](), installerCalls = [[String]](), opens = 0, updates = 0
        let legacy = LegacyRelayMaintenanceModel(directory: fixture.scope, capture: { args, _, _, authorize in
            XCTAssertTrue(authorize()); rootCalls.append(args)
            return preview
        }, installerCapture: { args, _, _, authorize in
            XCTAssertTrue(authorize()); installerCalls.append(args); return selected
        }, openInstaller: { _ in opens += 1; return true }, storage: fixture.storage)
        legacy.select(fixture.artifact); await legacy.review()
        let rootApproval = try XCTUnwrap(legacy.confirmation())
        await legacy.prepareInstaller()
        let installerApproval = try XCTUnwrap(legacy.installer)
        let entered = expectation(description: "managed capture owns shared custody")
        let held = HeldRead(entered); defer { held.release() }
        let model = MaatAppUpdateModel(memory: memory.model(), recovery: MaatPreparedPreflightRecovery(), maintenance: legacy, capture: { args, authorize in
            XCTAssertEqual(args, ["update", "--app"]); XCTAssertTrue(authorize()); updates += 1
            await held.wait()
            XCTAssertFalse(authorize(), "Cancel must invalidate the actual capture authorizer")
            return CommandExecution(cancelled: true, childState: .exitConfirmed)
        })
        await model.review(.update)?.value
        let approved = try XCTUnwrap(model.approval)
        let task = try XCTUnwrap(model.confirm(approved))
        await fulfillment(of: [entered], timeout: 2)
        XCTAssertTrue(legacy.busy); XCTAssertTrue(legacy.managedAppRequestActive)
        // Direct model entrypoints, not only disabled SwiftUI buttons.
        let unwantedInstaller = legacy.install(installerApproval)
        let unwantedRoot = legacy.confirm(rootApproval)
        await legacy.prepareInstaller()
        let refreshedInstaller = legacy.installer.flatMap { legacy.install($0) }
        await legacy.review()
        let refreshedRoot = legacy.confirmation().flatMap { legacy.confirm($0) }
        legacy.cancelInvocation()
        XCTAssertTrue(model.busy); XCTAssertTrue(legacy.busy, "Keep custody until owned capture joins")
        let unwantedManaged = model.confirm(approved)
        held.release(); await task.value
        await expectNoFurtherInvocation(unwantedManaged)
        await expectNoFurtherInvocation(unwantedInstaller)
        await expectNoFurtherInvocation(unwantedRoot)
        await expectNoFurtherInvocation(refreshedInstaller)
        await expectNoFurtherInvocation(refreshedRoot)
        XCTAssertEqual(rootCalls.count, 1); XCTAssertEqual(installerCalls.count, 1)
        XCTAssertEqual(opens, 0); XCTAssertEqual(updates, 1)
        XCTAssertFalse(held.pending); XCTAssertFalse(model.busy); XCTAssertFalse(legacy.busy)
        XCTAssertFalse(legacy.managedAppRequestActive); XCTAssertNotNil(model.commandReceipt)
        await expectNoFurtherInvocation(legacy.install(installerApproval))
        await expectNoFurtherInvocation(legacy.confirm(rootApproval))
        await expectNoFurtherInvocation(model.confirm(approved))
        await model.reviewSavedAction()?.value
        let fresh = try XCTUnwrap(model.approval)
        XCTAssertNotEqual(fresh.maintenanceEpoch, approved.maintenanceEpoch)
        XCTAssertEqual(updates, 1, "Fresh review alone cannot replay the update")
    }

    func testLegacyInstallerOrRootCaptureRejectsManagedConsentEvenAfterCleanup() async throws {
        for operation in ["installer", "root"] {
            let fixture = LegacyRelayFixture(), memory = MaatDiagnosticCorrelationFixture()
            let selected = try installerSelection(fixture), preview = try fixture.preview()
            let terminal = try fixture.mutation("confirm", phase: "completed", safe: true, exit: 0)
            let entered = expectation(description: "legacy " + operation + " owns shared custody")
            let held = HeldRead(entered); defer { held.release() }
            var opens = 0, rootEffects = 0, managedEffects = 0
            let legacy = LegacyRelayMaintenanceModel(directory: fixture.scope, capture: { args, _, _, authorize in
                XCTAssertTrue(authorize())
                if args[2] == "preview" { return preview }
                XCTAssertEqual(args[2], "confirm"); rootEffects += 1
                await held.wait(); XCTAssertTrue(authorize()); return terminal
            }, installerCapture: { args, _, _, authorize in
                XCTAssertTrue(authorize())
                if args.contains("--verify-selection") { await held.wait(); XCTAssertTrue(authorize()) }
                return selected
            }, openInstaller: { _ in opens += 1; return true }, storage: fixture.storage)
            let model = MaatAppUpdateModel(memory: memory.model(), recovery: MaatPreparedPreflightRecovery(), maintenance: legacy, capture: { args, authorize in
                XCTAssertEqual(args, ["update", "--app"]); XCTAssertTrue(authorize()); managedEffects += 1
                return CommandExecution(exitCode: 0, childState: .exitConfirmed)
            })
            await model.review(.update)?.value
            let stale = try XCTUnwrap(model.approval)
            let task: Task<Void, Never>
            if operation == "installer" {
                await legacy.prepareInstaller()
                task = try XCTUnwrap(legacy.install(try XCTUnwrap(legacy.installer)))
            } else {
                legacy.select(fixture.artifact); await legacy.review()
                task = try XCTUnwrap(legacy.confirm(try XCTUnwrap(legacy.confirmation())))
            }
            await fulfillment(of: [entered], timeout: 2)
            let unexpected = model.confirm(stale)
            let unexpectedReview = model.review(.update)
            XCTAssertEqual(managedEffects, 0)
            held.release(); await task.value
            await expectNoFurtherInvocation(unexpected)
            await expectNoFurtherInvocation(unexpectedReview)
            XCTAssertFalse(held.pending); XCTAssertFalse(legacy.busy)
            XCTAssertEqual(opens, operation == "installer" ? 1 : 0)
            XCTAssertEqual(rootEffects, operation == "root" ? 1 : 0)
            await expectNoFurtherInvocation(model.confirm(stale))
            XCTAssertEqual(managedEffects, 0, "Cleanup cannot revive previously reviewed consent")
            await model.reviewSavedAction()?.value
            let fresh = try XCTUnwrap(model.approval)
            XCTAssertNotEqual(fresh.maintenanceEpoch, stale.maintenanceEpoch)
            let owned = try XCTUnwrap(model.confirm(fresh)); await owned.value
            XCTAssertEqual(managedEffects, 1)
            await expectNoFurtherInvocation(model.confirm(fresh))
            // Also cover a complete legacy busy cycle between review and
            // confirmation, without first consuming the managed approval.
            await model.review(.update)?.value
            let queued = try XCTUnwrap(model.approval)
            await legacy.prepareInstaller()
            XCTAssertFalse(legacy.busy)
            await expectNoFurtherInvocation(model.confirm(queued))
            XCTAssertEqual(managedEffects, 1)
            XCTAssertNil(model.approval)
        }
    }

}
