import XCTest
@testable import SirsiMenubar

@MainActor
final class RelayObservationTests: XCTestCase {
    private func board(age: TimeInterval = 0, count: Int? = 0, access: String = "service-private",
                       source: String = "relay-producer", eligible: Bool = false,
                       unreadable: Bool = true, spool: String = "/owned/relay", inode: UInt64 = 10,
                       coverageComplete: Bool? = nil, healthError: String? = nil) throws -> RouterBoard {
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        let stamp = formatter.string(from: Date().addingTimeInterval(-age))
        var row: [String: Any] = ["agent": "codex-pantheon", "unreadable": unreadable,
            "observer_access": access, "retry_state": "observed", "retry_source": source,
            "retry_observed_at": stamp, "retry_started_at": stamp,
            "repair_eligible": eligible,
            "scope": ["canonical_spool": spool, "root_device": 9, "root_inode": inode]]
        if let count { row["retry_count"] = count }
        if eligible { row["repair_spool"] = spool }
        var object: [String: Any] = ["outbox": [row]]
        if let coverageComplete {
            object["outbox_coverage"] = ["complete": coverageComplete, "source": source, "started_at": stamp, "observed_at": stamp]
        }
        if let healthError { object["outbox_health_error"] = healthError }
        return try JSONDecoder().decode(RouterBoard.self,
            from: JSONSerialization.data(withJSONObject: object))
    }

    func testServicePrivateZeroIsObservedButMissingIsUnknown() throws {
        let engine = SirsiEngine()
        engine.routerBoard = try board()
        XCTAssertTrue(engine.routerOutboxBlockers.isEmpty)
        XCTAssertEqual(engine.routerServiceOutboxes.count, 1)
        XCTAssertTrue(engine.routerServiceOutboxes[0].observationText(at: Date()).contains("Service-managed private outbox · 0 retries observed"))
        XCTAssertTrue(engine.routerRetryOutbox.isEmpty)
        engine.routerBoard = try board(count: nil)
        XCTAssertEqual(engine.routerOutboxBlockers.count, 1)
        XCTAssertTrue(engine.routerServiceOutboxes.isEmpty)
        XCTAssertTrue(engine.routerBoard!.outbox![0].observationText(at: Date()).contains("unavailable"))
    }

    func testFreshBoardDoesNotFreshenStaleOrFutureProducer() throws {
        let engine = SirsiEngine()
        for age in [16.0, -3.0] {
            engine.routerBoard = try board(age: age, count: 4)
            engine.routerReadAt = Date()
            XCTAssertTrue(engine.routerObservationIsCurrent(at: Date()))
            XCTAssertEqual(engine.routerOutboxBlockers.count, 1)
            XCTAssertTrue(engine.routerServiceOutboxes.isEmpty)
        }
    }

    func testPartialProducerKeepsEligibleLaneVisibleWithoutMakingMissingZero() throws {
        let good = try board(count: 2).outbox![0]
        let bad = try board(count: nil).outbox![0]
        XCTAssertTrue(good.serviceObservationIsCurrent(at: Date()))
        XCTAssertFalse(bad.serviceObservationIsCurrent(at: Date()))
        // Go supplies aggregate completeness; the row projection never sums a
        // subset into a fabric total. Each lane retains its own eligibility.
        XCTAssertEqual(good.retryCount, 2)
        XCTAssertNil(bad.retryCount)
        let engine = SirsiEngine()
        engine.routerBoard = try JSONDecoder().decode(RouterBoard.self,
            from: JSONSerialization.data(withJSONObject: ["outbox": [
                ["agent": "eligible", "unreadable": true, "observer_access": "service-private",
                 "retry_source": "relay-producer", "retry_state": "observed", "retry_count": 2,
                 "retry_observed_at": good.retryObservedAt!],
                ["agent": "unknown", "unreadable": true, "observer_access": "service-private",
                 "retry_source": "relay-producer", "retry_state": "unavailable",
                 "retry_observed_at": good.retryObservedAt!]
            ], "outbox_observation": ["complete": false]]))
        XCTAssertEqual(engine.routerServiceOutboxes.map(\.agent), ["eligible"])
        XCTAssertEqual(engine.routerOutboxBlockers.map(\.agent), ["unknown"])
        XCTAssertTrue(engine.routerHasBlockers)
    }

    func testUnsupportedRepairNeverRunsEvenWithForgedEligibleFlag() async throws {
        let engine = SirsiEngine()
        engine.routerBoard = try board(eligible: true)
        engine.routerReadAt = Date()
        var commands = 0, reads = 0, records = 0
        let target = RelayOutboxRepairApproval(agent: "codex-pantheon", scope: try XCTUnwrap(engine.routerBoard?.outbox?.first?.scope))
        XCTAssertNil(engine.relayOutboxRepairApproval(for: engine.routerBoard!.outbox![0]))
        let result = await engine.repairRelayOutbox(approval: target, execute: { _ in
            commands += 1
            return CommandExecution(exitCode: 0, childState: .exitConfirmed)
        }, refresh: { reads += 1; return true }, record: { _ in records += 1 })
        XCTAssertEqual(commands, 0)
        XCTAssertEqual(reads, 0)
        XCTAssertEqual(records, 0)
        XCTAssertTrue(result.contains("Repair unavailable"))
    }

    func testRepairUsesTypedCommandThenFreshReadbackAndSeparatesFailure() async throws {
        for refreshSucceeds in [false, true] {
            let engine = SirsiEngine()
            engine.routerBoard = try board(access: "unavailable", source: "", eligible: true)
            engine.routerReadAt = Date()
            let target = try XCTUnwrap(engine.relayOutboxRepairApproval(for: engine.routerBoard!.outbox![0]))
            var steps: [String] = []
            let result = await engine.repairRelayOutbox(approval: target, execute: { args in
                XCTAssertEqual(args, ["router", "relay", "repair-outbox", "--agent", "codex-pantheon", "--spool", "/owned/relay",
                                      "--expected-root-device", "9", "--expected-root-inode", "10"])
                steps.append("command")
                return CommandExecution(exitCode: 0, childState: .exitConfirmed)
            }, refresh: {
                steps.append("readback")
                if refreshSucceeds {
                    // +0.01 is inside the accepted 2s clock-skew bound and
                    // avoids millisecond formatter truncation before the
                    // method's captured operation start in an instant fake.
                    engine.routerBoard = try! self.board(age: -0.01, access: "observed", source: "direct-metadata", unreadable: false)
                }
                return refreshSucceeds
            }, record: { _ in steps.append("record") })
            XCTAssertEqual(steps, ["command", "readback", "record"])
            XCTAssertTrue(result.contains("Repair command completed"))
            XCTAssertEqual(result.contains("fresh direct outbox metadata is readable"), refreshSucceeds)
            XCTAssertEqual(result.contains("Verification unavailable"), !refreshSucceeds)
            XCTAssertFalse(engine.busy)
        }
    }

    func testNonzeroRepairCannotTurnGreenAfterSuccessfulReadback() async throws {
        let engine = SirsiEngine()
        engine.routerBoard = try board(access: "unavailable", source: "", eligible: true)
        engine.routerReadAt = Date()
        let target = try XCTUnwrap(engine.relayOutboxRepairApproval(for: engine.routerBoard!.outbox![0]))
        var reads = 0
        let result = await engine.repairRelayOutbox(approval: target, execute: { _ in
            CommandExecution(stderr: Data("owned fixture failure".utf8), exitCode: 4, childState: .exitConfirmed)
        }, refresh: { reads += 1; return true }, record: { _ in })
        XCTAssertEqual(reads, 1)
        XCTAssertTrue(result.contains("did not complete successfully"))
        XCTAssertFalse(result.contains("fresh direct outbox metadata is readable"))
    }

    func testRefreshReportsProducerAgeAndRefresherFailureNotCollectionAge() async throws {
        let engine = SirsiEngine()
        engine.routerBoard = try board(age: 16)
        let old = engine.routerBoard?.outbox?.first?.retryObservedAt
        engine.routerReadAt = Date()
        let stale = await engine.refreshRelayOutboxObservation(refresh: { true })
        XCTAssertTrue(stale.contains("current relay retry evidence is unavailable"))
        XCTAssertEqual(engine.routerBoard?.outbox?.first?.retryObservedAt, old)
        let failed = await engine.refreshRelayOutboxObservation(refresh: { false })
        XCTAssertTrue(failed.contains("refresh failed"))
        engine.routerBoard = try board(count: 0)
        let fresh = await engine.refreshRelayOutboxObservation(refresh: { true })
        XCTAssertTrue(fresh.contains("Fresh relay producer observations verified"))
    }

    func testExpiredDirectPositiveRemainsVisibleAsUnknownInFreshBoard() throws {
        let engine = SirsiEngine()
        engine.routerBoard = try board(age: 16, count: 3, access: "observed", source: "direct-metadata", unreadable: false)
        engine.routerReadAt = Date()
        XCTAssertTrue(engine.routerObservationIsCurrent(at: Date()))
        XCTAssertTrue(engine.routerRetryOutbox.isEmpty)
        XCTAssertEqual(engine.routerOutboxBlockers.map(\.agent), ["codex-pantheon"])
        XCTAssertTrue(engine.routerOutboxBlockers[0].observationText(at: Date()).contains("3 retries (retained, not current)"))
        XCTAssertTrue(engine.routerHasBlockers)
        XCTAssertNotEqual(engine.routerSummary, "healthy")
    }

    func testIncompleteCoverageIndependentOfEligibleRowsAndOptionalPublisher() throws {
        let engine = SirsiEngine()
        for coverage in [false, true] {
            engine.routerBoard = try board(count: 2, access: "observed", source: "direct-metadata", unreadable: false,
                                          coverageComplete: coverage, healthError: "relay observation: publisher_missing")
            engine.routerReadAt = Date()
            XCTAssertEqual(engine.routerRetryOutbox.count, 1, "valid sibling must remain visible")
            XCTAssertTrue(engine.routerOutboxBlockers.isEmpty, "do not invalidate the eligible row")
            XCTAssertEqual(engine.routerOutboxCoverageUnknown, !coverage)
            XCTAssertEqual(engine.routerHasBlockers, !coverage)
            XCTAssertEqual(engine.routerSummary == "healthy", coverage)
        }
        // Legacy errors without explicit coverage cannot be cleared by a
        // nonempty row list either.
        engine.routerBoard = try board(count: 2, access: "observed", source: "direct-metadata", unreadable: false,
                                      healthError: "relay observer: limit")
        XCTAssertTrue(engine.routerHasBlockers)
        XCTAssertNotEqual(engine.routerSummary, "healthy")
    }

    func testRepairConsentRejectsChangedSpoolOrRootBeforeLaunch() async throws {
        for replacement in [("/owned/relay-b", UInt64(11)), ("/owned/relay-a", UInt64(99))] {
            let engine = SirsiEngine()
            engine.routerBoard = try board(access: "unavailable", source: "", eligible: true, spool: "/owned/relay-a")
            engine.routerReadAt = Date()
            let approval = try XCTUnwrap(engine.relayOutboxRepairApproval(for: engine.routerBoard!.outbox![0]))
            engine.routerBoard = try board(access: "unavailable", source: "", eligible: true,
                                          spool: replacement.0, inode: replacement.1)
            var launches = 0, reads = 0
            let result = await engine.repairRelayOutbox(approval: approval, execute: { _ in
                launches += 1
                return CommandExecution(exitCode: 0, childState: .exitConfirmed)
            }, refresh: { reads += 1; return true }, record: { _ in XCTFail("refused offer must not record an operation") })
            XCTAssertEqual(launches, 0)
            XCTAssertEqual(reads, 0)
            XCTAssertTrue(result.contains("approved agent/root scope changed"))
        }
    }

    func testOtherSpoolReadbackCannotVerifyApprovedRepair() async throws {
        let engine = SirsiEngine()
        engine.routerBoard = try board(access: "unavailable", source: "", eligible: true, spool: "/owned/relay-a")
        engine.routerReadAt = Date()
        let approval = try XCTUnwrap(engine.relayOutboxRepairApproval(for: engine.routerBoard!.outbox![0]))
        var launches = 0
        let result = await engine.repairRelayOutbox(approval: approval, execute: { args in
            launches += 1
            XCTAssertEqual(args, ["router", "relay", "repair-outbox", "--agent", "codex-pantheon", "--spool", "/owned/relay-a",
                                  "--expected-root-device", "9", "--expected-root-inode", "10"])
            return CommandExecution(exitCode: 0, childState: .exitConfirmed)
        }, refresh: {
            engine.routerBoard = try! self.board(age: -0.01, access: "observed", source: "direct-metadata",
                                                unreadable: false, spool: "/owned/relay-b", inode: 11)
            return true
        }, record: { _ in })
        XCTAssertEqual(launches, 1)
        XCTAssertTrue(result.contains("Verification unavailable"))
        XCTAssertFalse(result.contains("fresh direct outbox metadata is readable"))
    }
}
