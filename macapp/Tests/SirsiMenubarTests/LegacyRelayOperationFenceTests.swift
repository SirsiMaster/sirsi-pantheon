import XCTest
@testable import SirsiMenubar

@MainActor final class LegacyRelayOperationFenceTests: XCTestCase {
    private func unknown() -> CommandExecution {
        CommandExecution(cancelled: true, captureError: "Reply lost; outcome unknown", childState: .exitUnknown)
    }

    func testOldAndEmptyStatusCannotClearOutstandingOperationAcrossRemount() async throws {
        for empty in [false, true] {
            let f = LegacyRelayFixture()
            let preview = try f.preview(generation: 2)
            var calls = 0
            let model = LegacyRelayMaintenanceModel(directory: f.scope, capture: { args, _, _, _ in
                calls += 1
                return args[2] == "preview" ? preview : self.unknown()
            }, storage: f.storage)
            model.select(f.artifact)
            await model.review()
            let approved = try XCTUnwrap(model.confirmation())
            try await XCTUnwrap(model.confirm(approved)).value
            let retained = try XCTUnwrap(model.outstanding)
            let oldReceipt = try XCTUnwrap(retained.receipt)
            XCTAssertEqual(calls, 2)
            let status = try f.result(["status": empty ? [:] : f.transaction("completed", id: String(repeating: "e", count: 32)),
                                       "admission": ["state": "terminal"], "ticket_json": f.ticketJSON()])
            // Empty journal metadata is encoded like the Go zero-value struct.
            let emptyStatus = try f.result(["status": ["id": "", "revision": 0, "plan": f.plan(), "phase": "",
                "preserved_envelopes": 0, "old_outcomes": "", "next_action": ""],
                "admission": ["state": "reserved"], "ticket_json": f.ticketJSON()])
            let remounted = LegacyRelayMaintenanceModel(directory: f.scope, capture: { _, _, _, _ in
                empty ? emptyStatus : status
            }, storage: f.storage)
            XCTAssertTrue(remounted.outcomeUnknown)
            await remounted.readStatus()
            XCTAssertEqual(remounted.outstanding, retained)
            XCTAssertEqual(remounted.outstanding?.receipt, oldReceipt)
            XCTAssertTrue(remounted.outcomeUnknown)
            XCTAssertNil(remounted.confirmation())
            XCTAssertEqual(remounted.observedTransaction?.id, empty ? nil : String(repeating: "e", count: 32))
        }
    }

    func testOnlyMatchingExplicitFenceReopensPreviewAndRetainsOriginalReceipt() async throws {
        let f = LegacyRelayFixture()
        var mismatched = true
        let model = LegacyRelayMaintenanceModel(directory: f.scope, capture: { args, _, _, _ in
            if args[2] == "preview" { return try! f.preview() }
            if args[2] == "resolve-operation" {
                return try! f.result(["ticket": f.ticket(id: String(repeating: mismatched ? "e" : "d", count: 32)),
                    "disposition": "future-execution-fenced", "status": f.transaction("aborted"),
                    "safe_for_new_preview": true, "past_outcome": "never-admitted"])
            }
            return self.unknown()
        }, storage: f.storage)
        model.select(f.artifact); await model.review()
        try await XCTUnwrap(model.confirm(try XCTUnwrap(model.confirmation()))).value
        let retained = try XCTUnwrap(model.outstanding)
        try await XCTUnwrap(model.resolveOutstanding(retained)).value
        XCTAssertTrue(model.outcomeUnknown)
        XCTAssertEqual(model.outstanding, retained)
        mismatched = false
        try await XCTUnwrap(model.resolveOutstanding(retained)).value
        XCTAssertFalse(model.outcomeUnknown)
        XCTAssertFalse(model.lifecycleDebt)
        XCTAssertNil(model.outstanding)
        XCTAssertEqual(model.lastResolved, retained)
        XCTAssertNotNil(model.lastResolved?.receipt?.issue)
        await model.review()
        XCTAssertNotNil(model.confirmation(), "Only verified explicit fencing permits fresh consent")
    }

    func testFailedFenceReceiptPersistenceKeepsBindingAndBlocksConsent() async throws {
        let f = LegacyRelayFixture()
        let model = LegacyRelayMaintenanceModel(directory: f.scope, capture: { args, _, _, _ in
            if args[2] == "preview" { return try! f.preview() }
            if args[2] == "resolve-operation" {
                return try! f.result(["ticket": f.ticket(), "disposition": "future-execution-fenced",
                    "status": f.transaction("aborted"), "safe_for_new_preview": true, "past_outcome": "unknown"])
            }
            return self.unknown()
        }, storage: f.storage)
        model.select(f.artifact); await model.review()
        try await XCTUnwrap(model.confirm(try XCTUnwrap(model.confirmation()))).value
        let retained = try XCTUnwrap(model.outstanding)
        f.failWrite = true
        try await XCTUnwrap(model.resolveOutstanding(retained)).value
        XCTAssertEqual(model.outstanding, retained)
        XCTAssertTrue(model.outcomeUnknown)
        XCTAssertNotNil(model.error)
        await model.review()
        XCTAssertNil(model.confirmation())
    }

    func testStatusRemainsResponsiveDuringDialogAndCancelRetainsUnknownBinding() async throws {
        let f = LegacyRelayFixture()
        let entered = expectation(description: "inert dialog pending")
        var calls = 0
        let model = LegacyRelayMaintenanceModel(directory: f.scope, capture: { args, _, _, _ in
            calls += 1
            if args[2] == "preview" { return try! f.preview() }
            entered.fulfill()
            do { try await Task.sleep(nanoseconds: 30_000_000_000) } catch {}
            return self.unknown()
        }, storage: f.storage)
        model.select(f.artifact); await model.review()
        let task = try XCTUnwrap(model.confirm(try XCTUnwrap(model.confirmation())))
        await fulfillment(of: [entered], timeout: 2)
        await model.readStatus()
        XCTAssertEqual(calls, 2, "Status must not launch a second root dialog")
        XCTAssertTrue(model.busy)
        XCTAssertNotNil(model.statusNotice)
        model.cancelInvocation()
        await task.value
        XCTAssertFalse(model.busy)
        XCTAssertTrue(model.outcomeUnknown)
        XCTAssertNotNil(model.outstanding?.receipt)
    }
}
