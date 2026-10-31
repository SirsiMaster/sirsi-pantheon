import XCTest
import CryptoKit
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

    func testTypedPrivateHostBindingMatchesOnlyExactLocalHashAndKeepsLegacyStrict() throws {
        var object = screenObject()
        var subject = try XCTUnwrap(object["subject"] as? [String:Any])
        let digest = SHA256.hash(data:Data(host.utf8)).map{String(format:"%02x",$0)}.joined()
        subject["repo"] = "local-host-" + digest
        subject["host_binding"] = "sirsi.maat.local-host-sha256.v1"
        object["subject"] = subject
        let current = try verdict(object)
        XCTAssertTrue(maatHasLocalDiagnosticCustody(current,finding:current.findings[0],localHost:host))
        XCTAssertFalse(maatHasLocalDiagnosticCustody(current,finding:current.findings[0],localHost:"foreign"))
        guard case .current = maatCurrentDiagnosticResolution(verdict:current,finding:current.findings[0],localHost:host,read:try read()) else {return XCTFail("private new projection lost fresh-check custody")}
        for marker in ["", "unknown", "sirsi.maat.local-host-sha256.v2"] {
            subject["host_binding"] = marker; object["subject"] = subject
            let invalid = try verdict(object)
            XCTAssertFalse(maatHasLocalDiagnosticCustody(invalid,finding:invalid.findings[0],localHost:host))
        }
        subject["host_binding"] = "sirsi.maat.local-host-sha256.v1";subject["repo"] = "local-host-" + digest + "x";object["subject"] = subject
        let malformed = try verdict(object)
        XCTAssertFalse(maatHasLocalDiagnosticCustody(malformed,finding:malformed.findings[0],localHost:host))
        let legacy = try verdict()
        XCTAssertTrue(maatHasLocalDiagnosticCustody(legacy,finding:legacy.findings[0],localHost:host))
    }

    @MainActor func testActualGoRecordedPrivateProjectionRoundTripsNativeCustodyAndFreshScreen() async throws {
        guard let path = ProcessInfo.processInfo.environment["PANTHEON_TEST_DIAGNOSTIC_REQUEST_INTEROP"] else {
            throw XCTSkip("Main supplies the actual Go request/preparation/record/read/discovery artifact")
        }
        let bytes = try Data(contentsOf:URL(fileURLWithPath:path)); XCTAssertLessThanOrEqual(bytes.count,8*1024*1024)
        let object = try XCTUnwrap(JSONSerialization.jsonObject(with:bytes) as? [String:Any])
        let store = try XCTUnwrap(object["store_uuid"] as? String), key = try XCTUnwrap(object["request_id"] as? String)
        let uuid = try XCTUnwrap(UUID(uuidString:key)); var submissions = 0
        let empty = MaatDiagnosticCorrelationFixture.reply(try XCTUnwrap(object["empty_discovery"]))
        let prepared = MaatDiagnosticCorrelationFixture.reply(try XCTUnwrap(object["prepared"]))
        let recorded = MaatDiagnosticCorrelationFixture.reply(try XCTUnwrap(object["recorded"]))
        let lookup = MaatDiagnosticCorrelationFixture.reply(try XCTUnwrap(object["lookup"]))
        let correlation = MaatDiagnosticRecordModel(localHost:{ "fixture-mac" },newRequestID:{ uuid }) { args in
            if args == MaatDiagnosticRecordModel.inspectArguments { return MaatDiagnosticCorrelationFixture.reply(["state":"ready","store_uuid":store]) }
            if args == MaatDiagnosticRecordModel.pendingArguments { return empty }
            if args == MaatDiagnosticRecordModel.requestArguments("prepare",id:key) { return prepared }
            if args == MaatDiagnosticRecordModel.requestArguments("record",id:key,confirm:true) { submissions += 1; return recorded }
            XCTAssertEqual(args,MaatDiagnosticRecordModel.requestArguments("read",id:key))
            return lookup
        }
        let model = MaatDiagnosticResolutionModel()
        await model.recordFreshScreen(localHost:"fixture-mac",using:correlation)
        XCTAssertEqual(submissions,1); XCTAssertNotNil(model.screenResult)
        XCTAssertEqual(correlation.requestCompletion?.request.id,key); XCTAssertEqual(correlation.requestCompletion?.request.store_uuid,store)
        XCTAssertTrue(maatDiagnosticHostMatches(try XCTUnwrap(model.screenResult).verdict.subject,localHost:"fixture-mac"))
        await model.recordFreshScreen(localHost:"fixture-mac",using:correlation); XCTAssertEqual(submissions,1)
        // Preserve the independently qualified producer host/check/privacy
        // assertions while using the accepted correlated request flow.
        guard let healthPath = ProcessInfo.processInfo.environment["PANTHEON_TEST_DIAGNOSTIC_WIRE_OUTPUT"] else {
            throw XCTSkip("Main also supplies the independently qualified Go health-wire fixture for inherited dedicated/unknown-check custody assertions")
        }
        let healthBytes = try Data(contentsOf:URL(fileURLWithPath:healthPath))
        XCTAssertLessThanOrEqual(healthBytes.count,8*1024*1024)
        let result = try JSONDecoder().decode(MaatSystemOneCommandResult.self,from:healthBytes)
        let localHost = "fixture-mac"
        XCTAssertEqual(result.verdict.subject.hostBinding,"sirsi.maat.local-host-sha256.v1")
        XCTAssertNotEqual(result.verdict.subject.repo,localHost)
        let foreignCorrelation = MaatDiagnosticRecordModel(localHost:{ "foreign-mac" },newRequestID:{ uuid }) { args in
            if args == MaatDiagnosticRecordModel.inspectArguments { return MaatDiagnosticCorrelationFixture.reply(["state":"ready","store_uuid":store]) }
            if args == MaatDiagnosticRecordModel.pendingArguments { return empty }
            if args == MaatDiagnosticRecordModel.requestArguments("prepare",id:key) { return prepared }
            if args == MaatDiagnosticRecordModel.requestArguments("record",id:key,confirm:true) { return recorded }
            XCTAssertEqual(args,MaatDiagnosticRecordModel.requestArguments("read",id:key))
            return lookup
        }
        let foreign = MaatDiagnosticResolutionModel()
        await foreign.recordFreshScreen(localHost:"foreign-mac",using:foreignCorrelation)
        XCTAssertNil(foreign.screenResult)
        for check in ["Memory Death Spiral","/Users/PRIVATE_UNKNOWN_CHECK --token SECRET_CANARY"] {
            let suffix = SHA256.hash(data:Data(check.utf8)).prefix(6).map{String(format:"%02x",$0)}.joined()
            let finding = try XCTUnwrap(result.verdict.findings.first{$0.id == "host-health-" + suffix})
            XCTAssertTrue(maatHasLocalDiagnosticCustody(result.verdict,finding:finding,localHost:localHost))
            let fresh = try read(findings:[["check":check,"severity":2,"message":"fresh bounded observation"]])
            guard case .current = maatCurrentDiagnosticResolution(verdict:result.verdict,finding:finding,localHost:localHost,read:fresh) else {return XCTFail("redacted check lost actual fresh identity")}
            if case .current = maatCurrentDiagnosticResolution(verdict:result.verdict,finding:finding,localHost:localHost,
                read:try read(findings:[["check":check+"changed","severity":2,"message":"changed"]])) {XCTFail("changed check matched")}
        }
        let dedicated = try XCTUnwrap(result.verdict.findings.first{maatUsesDedicatedDiagnosticCustody($0)})
        XCTAssertTrue(maatHasLocalDiagnosticCustody(result.verdict,finding:dedicated,localHost:localHost))
        if case .current = maatCurrentDiagnosticResolution(verdict:result.verdict,finding:dedicated,localHost:localHost,
            read:try read(findings:[["check":"binary-drift","severity":2,"message":"current"]])) {XCTFail("generic resolver bypassed dedicated custody")}
        let text = String(decoding:healthBytes,as:UTF8.self)
        XCTAssertFalse(text.contains("PRIVATE_UNKNOWN_CHECK"));XCTAssertFalse(text.contains("SECRET_CANARY"))

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
        let model = MaatDiagnosticResolutionModel(), fixture = MaatDiagnosticCorrelationFixture()
        let correlation = fixture.model()
        for _ in 0..<2 { await model.recordFreshScreen(localHost:host,using:correlation) }
        XCTAssertEqual(fixture.calls.filter{$0.contains("--record-request")}.count,1)
        let key = try XCTUnwrap(fixture.requestID)
        XCTAssertEqual(fixture.calls.filter{$0.contains("--record-request")},[MaatDiagnosticRecordModel.requestArguments("record",id:key,confirm:true)])
        XCTAssertTrue(model.screenAttempted); XCTAssertNotNil(model.screenResult)
        XCTAssertNotNil(correlation.requestCompletion)
        XCTAssertTrue(model.message.contains("does not prove")); XCTAssertTrue(model.message.contains("original case")); XCTAssertNil(model.finding)
    }
    @MainActor func testFailedOrUnknownRescreenRetainsReceiptWithoutAutomaticReplay() async {
        for reply in [MaatDiagnosticCorrelationFixture.lost,
                      MaatDiagnosticCorrelationFixture.reply(["error":"inert refusal"],exit:1),
                      MaatDiagnosticCorrelationFixture.reply([:])] {
            let fixture = MaatDiagnosticCorrelationFixture(), model = MaatDiagnosticResolutionModel()
            fixture.recordReply = reply; fixture.readLost = true
            let correlation = fixture.model()
            for _ in 0..<2 { await model.recordFreshScreen(localHost:host,using:correlation) }
            XCTAssertEqual(fixture.calls.filter{$0.contains("--record-request")}.count,1)
            XCTAssertNil(model.screenResult); XCTAssertNotNil(correlation.activeRequestID); XCTAssertTrue(correlation.deliveryUnknown)
            XCTAssertTrue(model.message.contains("Inspect Casebook")); XCTAssertTrue(model.message.contains("will not replay"))
        }
    }
    @MainActor func testContradictoryOrForeignFreshScreenNeverClaimsRecordedObservation() async throws {
        let template = MaatDiagnosticCorrelationFixture()
        var variants: [[String: Any]] = []
        for (key,value) in [("snapshot_evidence","diagnostic:sha256=other"),("decision_evidence",""),("contract","imported"),("gate","unknown")] {
            var changed = template.success; changed[key] = value; variants.append(changed)
        }
        var foreign = template.success, subject = template.success["subject"] as! [String:Any]
        subject["repo"] = "other-mac"; foreign["subject"] = subject; variants.append(foreign)
        for object in variants {
            let fixture = MaatDiagnosticCorrelationFixture(), model = MaatDiagnosticResolutionModel()
            fixture.recordReply = MaatDiagnosticCorrelationFixture.reply(object)
            let correlation = fixture.model()
            await model.recordFreshScreen(localHost:host,using:correlation)
            XCTAssertNil(model.screenResult); XCTAssertTrue(model.message.contains("not confirmed"))
            XCTAssertEqual(fixture.calls.filter{$0.contains("--record-request")}.count,1)
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
        let fixture = MaatDiagnosticCorrelationFixture()
        fixture.recordReply = MaatDiagnosticCorrelationFixture.lost; fixture.readLost = true
        let correlation = fixture.model(), v = try verdict(), nav = Nav()
        let engine = SirsiEngine(maatCaseOperations:MaatCaseOperationStore(diagnosticRecord:correlation))
        var bridge: MaatDiagnosticResolutionView? = MaatDiagnosticResolutionView(engine:engine,origin:originCase(v),finding:v.findings[0])
        weak var owned: MaatDiagnosticResolutionModel?
        owned = bridge?.model
        let identity = ObjectIdentifier(try XCTUnwrap(owned))
        nav.push(try XCTUnwrap(bridge)); bridge = nil
        let bridgeID = nav.stack.last?.id
        fixture.onRecord = {
            XCTAssertTrue(owned?.busy == true); XCTAssertNotNil(correlation.activeRequestID)
            nav.push(MaatCasebookView(engine:engine)); nav.pop()
            XCTAssertEqual(nav.stack.last?.id,bridgeID)
            await owned?.recordFreshScreen(localHost:self.host,using:correlation)
        }
        await owned?.recordFreshScreen(localHost:host,using:correlation)
        XCTAssertTrue(correlation.deliveryUnknown); let key = try XCTUnwrap(correlation.activeRequestID)
        XCTAssertEqual(ObjectIdentifier(try XCTUnwrap(owned)),identity)
        nav.push(MaatCasebookView(engine:engine)); nav.pop()
        await owned?.recordFreshScreen(localHost:host,using:correlation)
        XCTAssertEqual(fixture.calls.filter{$0.contains("--record-request")}.count,1)
        XCTAssertEqual(correlation.activeRequestID,key); XCTAssertTrue(owned?.screenAttempted == true)
        XCTAssertTrue(owned?.message.contains("will not replay") == true)
        nav.pop(); XCTAssertTrue(nav.atRoot)
    }
}
