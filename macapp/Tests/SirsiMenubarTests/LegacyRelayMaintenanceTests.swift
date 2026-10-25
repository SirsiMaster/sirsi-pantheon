import XCTest
@testable import SirsiMenubar

@MainActor final class LegacyRelayMaintenanceTests: XCTestCase {
    private let scope = URL(fileURLWithPath: "/owned/maintenance")
    private let artifact = URL(fileURLWithPath: "/owned/retained/sirsi")
    private let digest = String(repeating: "a", count: 64)
    private func plan() -> [String: Any] {
        ["profile": "root-relay-legacy-maintenance-v1",
         "original": ["label": "ai.sirsi.router.relay", "target": "system/ai.sirsi.router.relay", "pid": 123, "started": "old-generation"],
         "legacy_payload": artifact.path, "sha256": digest]
    }
    private func transaction(_ phase: String = "prepared") -> [String: Any] {
        ["id": String(repeating: "b",count:32), "revision": 1, "plan": plan(), "phase": phase,
         "preserved_envelopes": 3, "old_outcomes": "unknown_not_certified_drained", "next_action": "Read exact retained transaction"]
    }
    private func result(_ value: [String: Any], exit: Int32 = 0) throws -> CommandExecution {
        CommandExecution(stdout: try JSONSerialization.data(withJSONObject: value), exitCode: exit, childState: .exitConfirmed)
    }

    func testReviewedConfirmationIsOnceAndNavigationCannotRetarget() async throws {
        let fixture = LegacyRelayFixture()
        let planned = try fixture.preview(), retained = try fixture.mutation("confirm")
        var calls = [[String]]()
        let model = LegacyRelayMaintenanceModel(directory: scope, capture: { args, input, _, authorize in
            XCTAssertNil(input); XCTAssertTrue(authorize()); calls.append(args)
            return args[2] == "preview" ? planned : retained
        }, storage: fixture.storage)
        model.select(artifact)
        await model.review()
        let approval = try XCTUnwrap(model.confirmation())
        let task = try XCTUnwrap(model.confirm(approval))
        XCTAssertNil(model.confirm(approval))
        await task.value
        XCTAssertEqual(model.transaction?.phase,"bootstrap_requested")
        XCTAssertNotNil(model.error)
        XCTAssertEqual(calls[1],["launchd","legacy-maintenance","confirm","--legacy-payload",artifact.path,"--expected-plan-sha256",digest,"--confirm-maintenance"] + (try fixture.ticketArgs("confirm")))
        model.changeDirectory(URL(fileURLWithPath:"/other"))
        XCTAssertNil(model.resume(try XCTUnwrap(model.transaction)))
        XCTAssertEqual(calls.count,2)
    }

    func testCancelledObservationDoesNotPublishPlan() async throws {
        let fixture = LegacyRelayFixture()
        let planned = try fixture.preview()
        let model = LegacyRelayMaintenanceModel(directory: scope, capture: { _, _, _, _ in
            withUnsafeCurrentTask { $0?.cancel() }; return planned
        }, storage: fixture.storage)
        model.select(artifact)
        let task = Task { @MainActor in await model.review() }
        await task.value
        XCTAssertNil(model.preview); XCTAssertFalse(model.busy); XCTAssertFalse(model.canConfirm)
    }

    func testStatusResumeAndAbortUseExactTransactionAndRetainErrorDebt() async throws {
        let fixture = LegacyRelayFixture()
        let retained = try fixture.status()
        var calls = [[String]]()
        let model = LegacyRelayMaintenanceModel(directory: scope, capture: { args, _, _, authorize in
            XCTAssertTrue(authorize()); calls.append(args)
            if args[2] == "status" { return retained }
            if args[2] == "prepare-operation" { return try! fixture.preview(args[4]) }
            return try! fixture.mutation(args[2])
        }, storage: fixture.storage)
        await model.readStatus()
        let expected = try XCTUnwrap(model.transaction)
        XCTAssertNil(model.abortBeforeRetirement(expected), "Recovery requires a distinct reviewed ticket")
        await model.reviewAction("abort", expected: expected)
        let abort = try XCTUnwrap(model.abortBeforeRetirement(expected)); await abort.value
        XCTAssertEqual(calls[2],["launchd","legacy-maintenance","abort","--transaction",expected.id,"--expected-plan-sha256",digest,"--confirm-maintenance"] + (try fixture.ticketArgs("abort")))
        XCTAssertNotNil(model.error)
        XCTAssertNil(model.resume(expected)) // An old consent cannot select the updated transaction.
        let updated = try XCTUnwrap(model.transaction)
        await model.reviewAction("recover", expected: updated)
        let resume = try XCTUnwrap(model.resume(updated)); await resume.value
        XCTAssertEqual(calls.last?[2],"recover")
    }

    func testPreparedTransactionIsRetainedAndActionable() async throws {
        let fixture = LegacyRelayFixture()
        let planned = try fixture.preview()
        let prepared = try fixture.result([
            "ticket_json": fixture.ticketJSON(),
            "status": fixture.transaction("prepared"),
            "terminal": true,
            "safe_for_new_preview": false
        ], exit: 1)
        let model = LegacyRelayMaintenanceModel(directory: scope, capture: { args, _, _, authorize in
            XCTAssertTrue(authorize())
            return args[2] == "preview" ? planned : prepared
        }, storage: fixture.storage)
        model.select(artifact)
        await model.review()
        let approval = try XCTUnwrap(model.confirmation())
        let task = try XCTUnwrap(model.confirm(approval))
        await task.value

        XCTAssertEqual(model.transaction?.phase, "prepared")
        XCTAssertTrue(model.lifecycleDebt)
        XCTAssertFalse(model.outcomeUnknown)
        XCTAssertNil(model.outstanding)
        XCTAssertTrue(model.statusNotice?.contains("not been published") == true)
        XCTAssertTrue(model.statusNotice?.contains("Resume activation") == true)
        XCTAssertNotNil(model.error, "The nonzero command result remains visible alongside the retained transaction")
    }

    func testPreparedTransactionWithoutTerminalExactBindingRemainsUnknown() async throws {
        let fixture = LegacyRelayFixture()
        let planned = try fixture.preview()
        let uncommitted = try fixture.result([
            "ticket_json": fixture.ticketJSON(),
            "status": fixture.transaction("prepared"),
            "terminal": false,
            "safe_for_new_preview": false
        ], exit: 1)
        let model = LegacyRelayMaintenanceModel(directory: scope, capture: { args, _, _, authorize in
            XCTAssertTrue(authorize())
            return args[2] == "preview" ? planned : uncommitted
        }, storage: fixture.storage)
        model.select(artifact)
        await model.review()
        let approval = try XCTUnwrap(model.confirmation())
        let task = try XCTUnwrap(model.confirm(approval))
        await task.value

        XCTAssertTrue(model.outcomeUnknown)
        XCTAssertNotNil(model.outstanding)
        XCTAssertNil(model.transaction)
    }

    func testRestoreForwardsCapturedPlanOnceWithoutReencoding() async throws {
        let fixture = LegacyRelayFixture()
        let retained = try fixture.status()
        let restoreDigest = String(repeating:"c",count:64)
        let restore = try result(["transaction": String(repeating:"b",count:32),"revision":1,"maintenance_sha256":digest,
                                  "artifact": artifact.path,"sha256":restoreDigest,"unknown_preserved_field":"exact captured bytes"])
        var captured: String?
        var operations = [String]()
        let model = LegacyRelayMaintenanceModel(directory: scope, capture: { args, input, _, authorize in
            XCTAssertTrue(authorize()); operations.append(args[2])
            if args[2] == "restore-preview" { return restore }
            if args[2] == "prepare-operation" { XCTAssertEqual(input, String(decoding: restore.stdoutData, as: UTF8.self)); return try! fixture.preview("restore-confirm") }
            if args[2] == "restore-confirm" { captured=input; XCTAssertEqual(args,["launchd","legacy-maintenance","restore-confirm","--expected-restore-sha256",restoreDigest,"--confirm-maintenance"] + (try! fixture.ticketArgs("restore-confirm"))); return try! fixture.mutation("restore-confirm") }
            return retained
        }, storage: fixture.storage)
        await model.readStatus()
        await model.reviewRestore(artifact:artifact,expected:try XCTUnwrap(model.transaction))
        let approval = try XCTUnwrap(model.restoreApproval)
        let task = try XCTUnwrap(model.restore(approval))
        XCTAssertNil(model.restore(approval)); await task.value
        XCTAssertEqual(captured,String(decoding:restore.stdoutData,as:UTF8.self))
        XCTAssertEqual(operations,["status","restore-preview","prepare-operation","restore-confirm"])
    }
}
