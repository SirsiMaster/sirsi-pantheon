import SwiftUI
import XCTest
@testable import SirsiMenubar

@MainActor final class MaatDiagnosticRecordTests: XCTestCase {
    private let store = "b3c515a2-4773-4653-86d1-8a5734c2ee45"
    private let id = String(repeating: "a", count: 64)
    private func reply(_ value: Any, exit: Int32 = 0) throws -> SirsiEngine.JSONCommandRead {
        .init(data: try JSONSerialization.data(withJSONObject: value), exitStatus: exit, timedOut: false, launchError: nil)
    }
    // Wire fields are the existing Go MaatVerdict/DiagnosticRecordIntent DTOs;
    // this is decoder coverage, not an assertion of Go execution.
    private var verdict: [String: Any] {
        ["feather_weight": 1, "gate": "changes", "confidence": 1,
         "subject": ["kind":"host", "ref":"diagnostic", "head_sha":id],
         "floor": ["passed":true, "checks":[]],
         "model": ["provider":"maat-local:deterministic", "version":"v1", "local":true, "latency_ms":0], "findings":[]]
    }
    private var ready: [String: Any] { ["state":"ready", "store_uuid":store] }
    private var empty: [String: Any] { ["schema":"sirsi.maat.diagnostic-record.v1", "pending":NSNull()] }
    private func intent(_ selectedID: String? = nil) -> [String: Any] {
        let selectedID = selectedID ?? id
        return ["schema":"sirsi.maat.diagnostic-record.v1", "id":selectedID,
                "manifest":["schema":"sirsi.maat.diagnostic-record.v1", "uid":501, "store_uuid":store,
                            "observation":["schema":"sirsi.maat.local-diagnostic-observation.v1", "source_sha256":id,
                                           "host_sha256":id, "observed_at_utc":"2026-10-25T00:00:00Z", "score":80, "statuses":[]]],
                "receipt":["receipt_sha256":id, "action_sha256":selectedID, "registry_sha256":id,
                           "registry_snapshot_version":2,
                           "action":["component":"pantheon", "profile":"local-macos", "operation":"record-system-one-diagnostic"],
                           "store_binding":["state":"initialized", "store_uuid":store]]]
    }
    func testActualRootExposesDiagnosticsWithoutCaseOrSuccessfulCasebook() async throws {
        var calls:[[String]]=[]
        let absent=try reply(["state":"uninitialized"])
        let model = MaatDiagnosticRecordModel(uid:501) { args in calls.append(args); return absent }
        let engine = SirsiEngine(maatCaseOperations: MaatCaseOperationStore(diagnosticRecord:model))
        let root = MaatWorkspaceView(engine:engine)
        let frame = try XCTUnwrap(root.body as? MaatWorkspaceRecoveryFrame)
        let entry = try XCTUnwrap(frame.localDiagnostics)
        XCTAssertTrue(entry.model === model)
        XCTAssertEqual(entry.inspectTitle,"Inspect record readiness")
        XCTAssertEqual(entry.observeTitle,"Observe locally")
        XCTAssertNil(model.approval(.initialize))
        XCTAssertNil(model.approval(.record))
        XCTAssertTrue(calls.isEmpty,"Mount must not launch CLI")
        await entry.inspect()
        XCTAssertEqual(calls,[MaatDiagnosticRecordModel.inspectArguments])
        XCTAssertNotNil(model.approval(.initialize),"Actual root callback must reach first-use review without a case")
    }
    func testObservationAvailableWithoutMemoryAndNeverCalledRecorded() async throws {
        let response = try reply(verdict)
        var calls:[[String]]=[]
        let model = MaatDiagnosticRecordModel(uid:501) { args in calls.append(args); return response }
        await model.observe()
        XCTAssertNotNil(model.observation); XCTAssertNil(model.recorded)
        XCTAssertEqual(calls,[["maat","triage","--json"]])
        XCTAssertTrue(model.message?.contains("not recorded") == true)
        XCTAssertNil(model.approval(.record))
    }
    func testFirstUseRequiresFreshInspectionAndConsumedConfirmation() async throws {
        let absent = try reply(["state":"uninitialized"])
        let initialized = try reply(["status":"initialized", "store_uuid":store,"imported_incidents":false])
        let status = try reply(ready)
        var calls:[[String]]=[]
        let model = MaatDiagnosticRecordModel(uid:501) { args in
            calls.append(args)
            if args.contains("--initialize-failure-memory") { return initialized }
            return calls.count < 4 ? absent : status
        }
        await model.inspect()
        let approval = try XCTUnwrap(model.approval(.initialize))
        let first = await model.confirm(approval); XCTAssertTrue(first)
        let repeated = await model.confirm(approval); XCTAssertFalse(repeated)
        XCTAssertEqual(calls.filter{$0.contains("--initialize-failure-memory")}.count,1)
        XCTAssertFalse(calls.contains{$0.contains("triage")})
        XCTAssertEqual(model.memory?.storeUUID,store)
        XCTAssertNil(model.recorded)
    }
    func testChangedFirstUseStateRefusesInitialization() async throws {
        let absent = try reply(["state":"uninitialized"]), status = try reply(ready)
        var calls:[[String]]=[]
        let model = MaatDiagnosticRecordModel(uid:501) { args in calls.append(args); return calls.count == 1 ? absent : status }
        await model.inspect(); let approval = try XCTUnwrap(model.approval(.initialize))
        let result = await model.confirm(approval); XCTAssertFalse(result)
        XCTAssertFalse(calls.contains{$0.contains("--initialize-failure-memory")})
    }
    func testVerifiedRecordPreservesExistingDecisionWireAndOnceConsent() async throws {
        let status = try reply(ready), noDebt = try reply(empty)
        var success = verdict
        success["record_state"]="recorded_verified";success["record_id"]=id
        success["snapshot_evidence"]="sha256:"+id;success["decision_evidence"]="sha256:"+id
        let response = try reply(success)
        var calls:[[String]]=[]
        let model = MaatDiagnosticRecordModel(uid:501) { args in
            calls.append(args)
            if args == MaatDiagnosticRecordModel.pendingArguments { return noDebt }
            if args == MaatDiagnosticRecordModel.inspectArguments { return status }
            return response
        }
        await model.inspect();let approval=try XCTUnwrap(model.approval(.record))
        let first=await model.confirm(approval);XCTAssertTrue(first)
        let again=await model.confirm(approval);XCTAssertFalse(again)
        XCTAssertEqual(calls.filter{$0 == ["maat","triage","--confirm","--json"]}.count,1)
        XCTAssertEqual(model.recorded?.verdict.gate,"changes")
        XCTAssertFalse(model.deliveryUnknown)
    }
    func testRefusedRecordKeepsObservationAndReconcilesOnlyExactCapturedIntent() async throws {
        let status=try reply(ready),noDebt=try reply(empty)
        let refusal=try reply(["schema":"sirsi.maat.diagnostic-record.v1","record_state":"delivery_unverified","record_id":id,"observation":verdict],exit:1)
        let debt=try reply(["schema":"sirsi.maat.diagnostic-record.v1","pending":[intent()]])
        let reconciled=try reply(["schema":"sirsi.maat.diagnostic-record.v1","id":id,"state":"recorded_verified","intent_sha256":id,"decision_evidence":"sha256:"+id,"verdict_sha256":id])
        var calls:[[String]]=[];var hasDebt=false
        let model=MaatDiagnosticRecordModel(uid:501){args in
            calls.append(args)
            if args == MaatDiagnosticRecordModel.inspectArguments{return status}
            if args == MaatDiagnosticRecordModel.pendingArguments{return hasDebt ? debt : noDebt}
            if args.contains("--reconcile-record"){return reconciled}
            hasDebt=true;return refusal
        }
        await model.inspect();let record=try XCTUnwrap(model.approval(.record))
        let failed=await model.confirm(record);XCTAssertFalse(failed)
        XCTAssertNotNil(model.observation);XCTAssertNil(model.recorded);XCTAssertTrue(model.deliveryUnknown)
        await model.inspect();let selected=try XCTUnwrap(model.approval(.reconcile(id)))
        let resolved=await model.confirm(selected);XCTAssertTrue(resolved)
        let repeatResult=await model.confirm(selected);XCTAssertFalse(repeatResult)
        XCTAssertFalse(model.deliveryUnknown)
        XCTAssertEqual(calls.filter{$0.contains("--reconcile-record")},[["maat","triage","--reconcile-record",id,"--confirm","--json"]])
        XCTAssertEqual(calls.filter{$0 == ["maat","triage","--confirm","--json"]}.count,1)
    }
    func testLaterRefusalOrUnknownReplyCannotReusePriorRecordedLabel() async throws {
        for lostReply in [false, true] {
            let status = try reply(ready), noDebt = try reply(empty)
            var prior = verdict
            prior["gate"] = "pass"; prior["record_state"] = "recorded_verified"; prior["record_id"] = id
            prior["snapshot_evidence"] = "sha256:" + id; prior["decision_evidence"] = "sha256:" + id
            let success = try reply(prior)
            let refusal = try reply(["schema":"sirsi.maat.diagnostic-record.v1", "record_state":"delivery_unverified",
                                     "record_id":id, "observation":verdict], exit:1)
            var submissions = 0
            let model = MaatDiagnosticRecordModel(uid:501) { args in
                if args == MaatDiagnosticRecordModel.inspectArguments { return status }
                if args == MaatDiagnosticRecordModel.pendingArguments { return noDebt }
                submissions += 1
                if submissions == 1 { return success }
                return lostReply ? .init(data:Data(), exitStatus:nil, timedOut:true, launchError:nil) : refusal
            }
            await model.inspect()
            let first = try XCTUnwrap(model.approval(.record))
            let recorded = await model.confirm(first); XCTAssertTrue(recorded)
            XCTAssertEqual(model.recorded?.verdict.gate, "pass")
            await model.inspect()
            let next = try XCTUnwrap(model.approval(.record))
            let failed = await model.confirm(next); XCTAssertFalse(failed)
            XCTAssertEqual(submissions, 2)
            XCTAssertNil(model.recorded, "Prior success must not label the later invocation as recorded")
            XCTAssertTrue(model.deliveryUnknown)
            XCTAssertEqual(model.observation?.gate, lostReply ? "pass" : "changes")
            XCTAssertNil(model.approval(.record))
        }
    }
    func testUnknownReplyThenEmptyInspectionCannotAuthorizeResend() async throws {
        let status=try reply(ready),noDebt=try reply(empty)
        let model=MaatDiagnosticRecordModel(uid:501){args in
            if args == MaatDiagnosticRecordModel.inspectArguments{return status}
            if args == MaatDiagnosticRecordModel.pendingArguments{return noDebt}
            return .init(data:Data(),exitStatus:nil,timedOut:true,launchError:nil)
        }
        await model.inspect();let approval=try XCTUnwrap(model.approval(.record))
        let result=await model.confirm(approval);XCTAssertFalse(result)
        await model.inspect()
        XCTAssertTrue(model.deliveryUnknown);XCTAssertNil(model.approval(.record))
        XCTAssertFalse(model.busy)
    }
    func testChangedOrForeignIntentRefusesReconciliationWithoutAppend() async throws {
        let status=try reply(ready)
        for foreign in [false,true] {
            var row=intent();if foreign {var manifest=row["manifest"] as! [String:Any];manifest["store_uuid"]="other";row["manifest"]=manifest}
            let debt=try reply(["schema":"sirsi.maat.diagnostic-record.v1","pending":[row]])
            let changed=try reply(empty)
            var reads=0;var effects=0
            let model=MaatDiagnosticRecordModel(uid:501){args in
                if args == MaatDiagnosticRecordModel.inspectArguments{return status}
                if args == MaatDiagnosticRecordModel.pendingArguments{reads+=1;return reads==1 ? debt : changed}
                effects+=1;return changed
            }
            await model.inspect()
            if foreign {XCTAssertNil(model.approval(.reconcile(id)))}
            else {let approval=try XCTUnwrap(model.approval(.reconcile(id)));let result=await model.confirm(approval);XCTAssertFalse(result)}
            XCTAssertEqual(effects,0)
        }
    }
    func testMalformedHistoryCannotOfferRecordOrInitialization() async throws {
        let status=try reply(ready),malformed=try reply(["schema":"sirsi.maat.diagnostic-record.v1","pending":[["id":id]]])
        let model=MaatDiagnosticRecordModel(uid:501){args in
            args == MaatDiagnosticRecordModel.inspectArguments ? status : malformed
        }
        await model.inspect();XCTAssertNil(model.approval(.record))
        XCTAssertNil(model.approval(.initialize))
        XCTAssertNil(model.recorded)
    }
    func testCanceledReaderCannotPublishRecordOrLeaveBusyAndFreshInspectionWorks() async throws {
        let status=try reply(ready),noDebt=try reply(empty)
        var started=false
        let model=MaatDiagnosticRecordModel(uid:501){args in
            if args == MaatDiagnosticRecordModel.inspectArguments{return status}
            if args == MaatDiagnosticRecordModel.pendingArguments{return noDebt}
            started=true
            do {try await Task.sleep(nanoseconds:1_000_000_000)} catch {}
            return .init(data:Data(),exitStatus:1,timedOut:false,launchError:nil)
        }
        await model.inspect();let approval=try XCTUnwrap(model.approval(.record))
        let invocation=Task{@MainActor in await model.confirm(approval)}
        for _ in 0..<100 where !started {await Task.yield()}
        XCTAssertTrue(started);model.cancel()
        let result=await invocation.value;XCTAssertFalse(result)
        XCTAssertFalse(model.busy);XCTAssertTrue(model.deliveryUnknown);XCTAssertNil(model.recorded)
        await model.inspect();XCTAssertEqual(model.memory?.state,"ready")
        XCTAssertNil(model.approval(.record),"Fresh inspection must not waive canceled unknown delivery")
    }
}
