import XCTest
@testable import SirsiMenubar

@MainActor final class MaatAppUpdateTests: XCTestCase {
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
            entered.fulfill()
            await withCheckedContinuation { (value: CheckedContinuation<Void, Never>) in
                if self.released { value.resume() }
                else { self.continuation = value }
            }
            return MaatDiagnosticCorrelationFixture.lost
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
        let model = MaatAppUpdateModel(memory: memory, recovery: recovery, capture: { args, authorize in
            await bridge.capture(args: args, authorize: authorize)
        })
        let legacyFixture = LegacyRelayFixture()
        let panel = LegacyRelayMaintenancePanel(directory: legacyFixture.scope,
            model: LegacyRelayMaintenanceModel(directory: legacyFixture.scope, storage: legacyFixture.storage), appUpdate: model)
        let view = panel.managedUpdateControl
        XCTAssertTrue(view.model === model)
        XCTAssertTrue(view.memoryControl.model === memory)
        XCTAssertTrue(view.recoveryControl.model === recovery)
        let owner = MaatCaseOperationStore(preparedPreflightRecovery: recovery, diagnosticRecord: memory)
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
            let model = MaatAppUpdateModel(memory: memory, recovery: recovery, capture: { _, _ in
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
        let model = MaatAppUpdateModel(memory: fixture.model(), recovery: MaatPreparedPreflightRecovery(), capture: { _, _ in
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
            let model = MaatAppUpdateModel(memory: fixture.model(), recovery: MaatPreparedPreflightRecovery(), capture: { _, _ in
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
        let model = MaatAppUpdateModel(memory: memory, recovery: MaatPreparedPreflightRecovery(), capture: { args, authorize in
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
        let unknownModel = MaatAppUpdateModel(memory: fixture.model(), recovery: MaatPreparedPreflightRecovery(), capture: { _, _ in
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
}
