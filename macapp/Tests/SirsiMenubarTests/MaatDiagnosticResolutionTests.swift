import XCTest
@testable import SirsiMenubar

final class MaatDiagnosticResolutionTests: XCTestCase {
    private let head = String(repeating: "a", count: 64)
    private let host = "fixture-mac"

    private func screenObject() -> [String: Any] {
        ["schema_version": 1, "feather_weight": 60, "confidence": 1, "gate": "changes",
         "subject": ["kind": "host", "repo": host, "ref": "diagnostic", "head_sha": head],
         "floor": ["passed": true, "checks": []],
         "model": ["provider": "maat-local:deterministic", "version": "v1", "local": true, "latency_ms": 0],
         "findings": [["id": "host-health-11c3ecf24b29", "severity": "major", "category": "host-health",
                       "claim": "Historical disk condition", "evidence": "diagnostic:sha256=\(head):11c3ecf24b29",
                       "fix_hint": "sh -c 'must never execute'; sirsi self-update"]]]
    }

    private func verdict(_ object: [String: Any]? = nil) throws -> MaatSystemOneVerdict {
        try JSONDecoder().decode(MaatSystemOneVerdict.self, from: JSONSerialization.data(withJSONObject: object ?? screenObject()))
    }

    private func read(findings: [[String: Any]]? = nil, status: String = "red", exit: Int32? = 0,
                      timeout: Bool = false, error: String? = nil) throws -> SirsiEngine.JSONCommandRead {
        let findings = findings ?? [["check": "Disk Space", "severity": 3, "message": "Fresh disk condition",
                                     "detail": "Current bounded operands", "fix": "sirsi clean --include-caution", "fixKind": "instant", "resolution": "repair"]]
        return .init(data: try JSONSerialization.data(withJSONObject: ["status": status, "findings": findings]),
                     exitStatus: exit, timedOut: timeout, launchError: error)
    }

    func testDecodedCanonicalIdentityUsesFreshOperandsAndNeverHintText() throws {
        let v = try verdict()
        guard case .current(let current) = maatCurrentDiagnosticResolution(verdict: v, finding: v.findings[0], localHost: host, read: try read()) else {
            return XCTFail("Canonical local finding must reach the existing resolver")
        }
        XCTAssertEqual(current.check, "Disk Space")
        XCTAssertEqual(current.message, "Fresh disk condition")
        XCTAssertEqual(current.detail, "Current bounded operands")
        XCTAssertEqual(current.fix, "sirsi clean --include-caution")
        XCTAssertFalse(current.fix?.contains("self-update") == true)
        XCTAssertEqual(v.findings[0].claim, "Historical disk condition")
    }

    func testForeignImportedMalformedAndDuplicateCustodyRefuses() throws {
        let baseline = screenObject()
        var variants: [[String: Any]] = []
        for (key, value) in [("repo", "other-mac"), ("kind", "repository"), ("ref", "source"), ("head_sha", "not-a-sha")] {
            var object = baseline
            var subject = try XCTUnwrap(object["subject"] as? [String: Any])
            subject[key] = value; object["subject"] = subject; variants.append(object)
        }
        var imported = baseline; imported["contract"] = "maat.adr070.source-screen"; variants.append(imported)
        var v2 = baseline; v2["schema_version"] = 2; variants.append(v2)
        var duplicate = baseline
        let findings = try XCTUnwrap(baseline["findings"] as? [[String: Any]])
        duplicate["findings"] = findings + findings; variants.append(duplicate)
        var wrongEvidence = baseline
        var wrongFindings = findings; wrongFindings[0]["evidence"] = "diagnostic:sha256=other:11c3ecf24b29"
        wrongEvidence["findings"] = wrongFindings; variants.append(wrongEvidence)
        for object in variants {
            let v = try verdict(object)
            XCTAssertFalse(maatHasLocalDiagnosticCustody(v, finding: v.findings[0], localHost: host))
            if case .current = maatCurrentDiagnosticResolution(verdict: v, finding: v.findings[0], localHost: host, read: try read()) {
                XCTFail("Mismatched custody must never reach the resolver")
            }
        }
    }

    func testMissingClearedOrAmbiguousFreshFindingNeverOffersRepair() throws {
        let v = try verdict()
        let warning: [String: Any] = ["check": "Disk Space", "severity": 2, "message": "current"]
        let rows: [[[String: Any]]] = [[], [["check": "Disk Space", "severity": 1, "message": "clear"]], [warning, warning],
                                      [["check": "RAM Pressure", "severity": 3, "message": "unrelated"]]]
        for findings in rows {
            if case .current = maatCurrentDiagnosticResolution(verdict: v, finding: v.findings[0], localHost: host, read: try read(findings: findings)) {
                XCTFail("No unique current warning")
            }
        }
    }

    func testIncompleteMalformedAndUnrecognizedDiagnosticNeverOffersRepair() throws {
        let v = try verdict()
        let reads = [try read(exit: 1), try read(exit: nil), try read(timeout: true), try read(error: "failed"), try read(status: "future"),
                     SirsiEngine.JSONCommandRead(data: Data("{".utf8), exitStatus: 0, timedOut: false, launchError: nil)]
        for result in reads {
            if case .current = maatCurrentDiagnosticResolution(verdict: v, finding: v.findings[0], localHost: host, read: result) {
                XCTFail("Incomplete observation must remain unverified")
            }
        }
    }

    func testDedicatedMutationCustodyIsNotBypassedByGenericResolver() throws {
        for (check, suffix) in [("binary-drift", "bece80d49c3b"), ("liveness-watch", "e6e1cd3b64c4"), ("launchd Disabled Override", "590a0ef5b9a4")] {
            var object = screenObject()
            object["findings"] = [["id": "host-health-\(suffix)", "category": "host-health", "severity": "major",
                                   "evidence": "diagnostic:sha256=\(head):\(suffix)"]]
            let v = try verdict(object)
            XCTAssertTrue(maatUsesDedicatedDiagnosticCustody(v.findings[0]))
            if case .current = maatCurrentDiagnosticResolution(verdict: v, finding: v.findings[0], localHost: host,
                read: try read(findings: [["check": check, "severity": 3, "message": "current", "fix": "sirsi self-update"]])) {
                XCTFail("Dedicated preview/journal/fence must retain mutation custody")
            }
        }
    }

    @MainActor func testOpeningResolverRechecksSemanticEvidenceAndFreshness() async throws {
        let v = try verdict(), model = MaatDiagnosticResolutionModel()
        let fresh = try read()
        await model.refresh(verdict: v, finding: v.findings[0], localHost: host) { fresh }
        let current = try XCTUnwrap(model.finding)
        XCTAssertTrue(model.canOpen(snapshot: .init(current: true, finding: current)))
        XCTAssertFalse(model.canOpen(snapshot: .init(current: false, finding: current)))
        XCTAssertFalse(model.canOpen(snapshot: .init(current: true, finding: nil)))
        let changed = try JSONDecoder().decode(DiagFinding.self, from: Data(#"{"check":"Disk Space","severity":3,"message":"changed","fix":"sirsi clean --confirm"}"#.utf8))
        XCTAssertNil(model.beginReview(snapshot: .init(current: true, finding: changed)))
        XCTAssertNil(model.finding)
        XCTAssertTrue(model.message.contains("Read it again"))
    }

    @MainActor func testLateOldReadCannotOverwriteNewFailure() async throws {
        let v = try verdict(), model = MaatDiagnosticResolutionModel()
        let older = try read(), newer = try read(error: "new failure")
        await model.refresh(verdict: v, finding: v.findings[0], localHost: host) {
            await model.refresh(verdict: v, finding: v.findings[0], localHost: self.host) { newer }
            return older
        }
        XCTAssertNil(model.finding)
        XCTAssertFalse(model.busy)
        XCTAssertTrue(model.message.contains("unavailable or incomplete"))
    }

    @MainActor func testCancellationInvalidatesLateReadAndPreventsNotStartedRead() async throws {
        let v = try verdict(), model = MaatDiagnosticResolutionModel()
        let fresh = try read()
        await model.refresh(verdict: v, finding: v.findings[0], localHost: host) {
            model.cancelReview()
            return fresh
        }
        XCTAssertNil(model.finding)
        XCTAssertFalse(model.busy)
        XCTAssertTrue(model.message.contains("cancelled"))
        var reads = 0
        let task = Task { @MainActor in
            await model.refresh(verdict: v, finding: v.findings[0], localHost: self.host) { reads += 1; return fresh }
        }
        task.cancel()
        await task.value
        XCTAssertEqual(reads, 0)
        XCTAssertNil(model.finding)
    }

    @MainActor func testConfirmedRescreenUsesExactCanonicalArgumentsOnceAndDoesNotClaimRepair() async throws {
        let model = MaatDiagnosticResolutionModel()
        var response = screenObject()
        response["decision_evidence"] = "maat-system-one:sha256=" + String(repeating: "b", count: 64)
        response["snapshot_evidence"] = "diagnostic:sha256=" + head
        let receipt = CommandExecution(stdout: try JSONSerialization.data(withJSONObject: response), exitCode: 0, childState: .exitConfirmed)
        var calls = 0
        for _ in 0..<2 {
            await model.recordFreshScreen(localHost: host) { args in
                calls += 1
                XCTAssertEqual(args, ["maat", "triage", "--confirm", "--json"])
                return receipt
            }
        }
        XCTAssertEqual(calls, 1)
        XCTAssertTrue(model.screenAttempted)
        XCTAssertNotNil(model.screenResult)
        XCTAssertEqual(model.screenReceipt, receipt)
        XCTAssertTrue(model.message.contains("does not prove"))
        XCTAssertTrue(model.message.contains("original case"))
        XCTAssertNil(model.finding)
    }

    @MainActor func testFailedOrUnknownRescreenRetainsReceiptWithoutAutomaticReplay() async {
        for receipt in [CommandExecution(stdout: Data("failed write".utf8), exitCode: 1, childState: .exitConfirmed),
                        CommandExecution(stdout: Data("partial".utf8), timedOut: true, childState: .exitUnknown),
                        CommandExecution(stdout: Data("{}".utf8), exitCode: 0, childState: .exitConfirmed)] {
            let model = MaatDiagnosticResolutionModel()
            var calls = 0
            for _ in 0..<2 { await model.recordFreshScreen { _ in calls += 1; return receipt } }
            XCTAssertEqual(calls, 1)
            XCTAssertNil(model.screenResult)
            XCTAssertEqual(model.screenReceipt, receipt)
            XCTAssertTrue(model.message.contains("Inspect Casebook"))
            XCTAssertTrue(model.message.contains("will not replay"))
        }
    }

    @MainActor func testContradictoryOrForeignFreshScreenNeverClaimsRecordedObservation() async throws {
        var baseline = screenObject()
        baseline["decision_evidence"] = "maat-system-one:sha256=" + String(repeating: "b", count: 64)
        baseline["snapshot_evidence"] = "diagnostic:sha256=" + head
        var variants: [[String: Any]] = []
        for (key, value) in [("snapshot_evidence", "diagnostic:sha256=other"), ("decision_evidence", ""),
                             ("contract", "imported"), ("gate", "unknown")] {
            var changed = baseline; changed[key] = value; variants.append(changed)
        }
        var foreign = baseline
        var subject = try XCTUnwrap(foreign["subject"] as? [String: Any])
        subject["repo"] = "other-mac"; foreign["subject"] = subject; variants.append(foreign)
        for object in variants {
            let model = MaatDiagnosticResolutionModel()
            let receipt = CommandExecution(stdout: try JSONSerialization.data(withJSONObject: object), exitCode: 0, childState: .exitConfirmed)
            await model.recordFreshScreen(localHost: host) { _ in receipt }
            XCTAssertNil(model.screenResult)
            XCTAssertEqual(model.screenReceipt, receipt)
            XCTAssertTrue(model.message.contains("not confirmed"))
        }
    }

    private func originCase(_ v: MaatSystemOneVerdict) -> MaatCase {
        MaatCase(id: "retained-origin", time: "2026-10-25T00:00:00Z", kind: "system one screen",
                 category: "system-one", status: "open", priority: .high, requester: "fixture",
                 resource: host, affected: head, determination: "changes", assessed: "host diagnostic",
                 why: "retained observation", evidence: "maat-system-one:sha256=" + String(repeating: "b", count: 64),
                 systemOne: v)
    }

    @MainActor func testActualNavResolverAndCasebookBackRetainOriginAndFreshReadState() async throws {
        let v = try verdict(), engine = SirsiEngine(), nav = Nav()
        // Default production initializer owns the workflow before Nav type
        // erasure; this test does not inject a separately retained model.
        let bridge = MaatDiagnosticResolutionView(engine: engine, origin: originCase(v), finding: v.findings[0])
        nav.push(MaatCasebookView(engine: engine))
        let parentID = try XCTUnwrap(nav.stack.last?.id)
        nav.push(bridge)
        let bridgeID = try XCTUnwrap(nav.stack.last?.id)
        let evidence = bridge.origin.evidence
        let modelID = ObjectIdentifier(bridge.model)
        let fresh = try read()
        await bridge.model.refresh(verdict: v, finding: v.findings[0], localHost: host) { fresh }
        let current = try XCTUnwrap(bridge.model.beginReview(snapshot: .init(current: true, finding: bridge.model.finding)))
        nav.push(FindingView(engine: engine, finding: current))
        XCTAssertEqual(nav.stack.count, 3)
        nav.pop()
        XCTAssertEqual(nav.stack.last?.id, bridgeID)
        XCTAssertEqual(ObjectIdentifier(bridge.model), modelID)
        XCTAssertEqual(bridge.origin.evidence, evidence)
        XCTAssertEqual(bridge.model.finding?.message, "Fresh disk condition")
        // Cancellation invalidates current read authority, never the origin.
        bridge.model.cancelReview()
        nav.push(MaatCasebookView(engine: engine))
        nav.pop()
        XCTAssertEqual(nav.stack.last?.id, bridgeID)
        XCTAssertEqual(nav.stack.first?.id, parentID)
        XCTAssertNil(bridge.model.finding)
        XCTAssertNil(bridge.model.beginReview(snapshot: .init(current: true, finding: current)))
        await bridge.model.refresh(verdict: v, finding: v.findings[0], localHost: host) { fresh }
        XCTAssertTrue(bridge.model.canOpen(snapshot: .init(current: true, finding: bridge.model.finding)))
        XCTAssertEqual(bridge.origin.evidence, evidence)
        XCTAssertEqual(bridge.origin.status, "open")
    }

    @MainActor func testActualNavOwnsPendingAndUnknownWriteAfterDestinationValueIsReleased() async throws {
        let v = try verdict(), engine = SirsiEngine(), nav = Nav()
        var bridge: MaatDiagnosticResolutionView? = MaatDiagnosticResolutionView(
            engine: engine, origin: originCase(v), finding: v.findings[0])
        weak var ownedModel = bridge?.model
        let modelID = ObjectIdentifier(try XCTUnwrap(ownedModel))
        nav.push(try XCTUnwrap(bridge))
        let bridgeID = try XCTUnwrap(nav.stack.last?.id)
        bridge = nil // Only the actual Nav destination value owns the model.
        XCTAssertNotNil(ownedModel)
        let unknown = CommandExecution(stdout: Data("retained partial write output".utf8),
                                       timedOut: true, childState: .exitUnknown)
        var captures = 0
        await ownedModel?.recordFreshScreen(localHost: host) { args in
            captures += 1
            XCTAssertEqual(args, ["maat", "triage", "--confirm", "--json"])
            XCTAssertEqual(ownedModel?.busy, true)
            XCTAssertEqual(ownedModel?.screenAttempted, true)
            nav.push(MaatCasebookView(engine: engine))
            nav.pop()
            XCTAssertEqual(nav.stack.last?.id, bridgeID)
            XCTAssertEqual(ownedModel.map { ObjectIdentifier($0) }, modelID)
            // Pending writes cannot renew their latch after a navigation turn.
            await ownedModel?.recordFreshScreen(localHost: self.host) { _ in captures += 1; return unknown }
            return unknown
        }
        XCTAssertEqual(captures, 1)
        XCTAssertNotNil(ownedModel)
        XCTAssertEqual(ownedModel?.screenReceipt, unknown)
        XCTAssertEqual(ownedModel?.screenAttempted, true)
        XCTAssertEqual(ownedModel?.busy, false)
        nav.push(MaatCasebookView(engine: engine))
        nav.pop()
        XCTAssertEqual(nav.stack.last?.id, bridgeID)
        XCTAssertEqual(ObjectIdentifier(try XCTUnwrap(ownedModel)), modelID)
        await ownedModel?.recordFreshScreen(localHost: host) { _ in captures += 1; return unknown }
        XCTAssertEqual(captures, 1, "Back after an unknown write cannot create new journal intent")
        XCTAssertEqual(ownedModel?.screenReceipt, unknown)
        XCTAssertTrue(ownedModel?.message.contains("will not replay") == true)
        nav.pop()
        XCTAssertTrue(nav.atRoot)
    }
}
