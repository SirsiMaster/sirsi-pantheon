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
    private actor RootHistoryProbe {
        var calls: [[String]] = []
        var holdNext = false
        var held: CheckedContinuation<SirsiEngine.JSONCommandRead, Never>?
        var heldScope = "system-one"
        func hold() { holdNext = true }
        func isHeld() -> Bool { held != nil }
        func arguments() -> [[String]] { calls }
        func read(_ args: [String]) async -> SirsiEngine.JSONCommandRead {
            calls.append(args)
            let scope = args.firstIndex(of: "--scope").map { args[$0 + 1] } ?? "invalid"
            if holdNext {
                holdNext = false; heldScope = scope
                return await withCheckedContinuation { held = $0 }
            }
            return Self.page(scope: scope, snapshot: "read-\(calls.count)")
        }
        func release() {
            holdNext = false
            held?.resume(returning: Self.page(scope: heldScope, snapshot: "late-old-refresh"))
            held = nil
        }
        nonisolated static func page(scope: String, snapshot: String) -> SirsiEngine.JSONCommandRead {
            let raw = """
            {"cases":[],"summary":{"total":0,"open":0,"urgent":0,"high":0,"resolved":0},
             "journal_integrity":{"invalid_count":0,"issues":[]},
             "history":{"scope":"\(scope)","query_id":"fixture-query","snapshot":"\(snapshot)",
             "returned":0,"candidate_matches":0,"count_scope":"query","candidate_matches_complete":true,
             "page_size":50,"coverage":"complete","has_more":false,"has_more_known":true,"total_complete":true,"total":0}}
            """
            return .init(data: Data(raw.utf8), exitStatus: 0, timedOut: false, launchError: nil)
        }
    }

    private func successfulRecordReader() throws -> MaatDiagnosticRecordModel.Reader {
        let status = try reply(ready), noDebt = try reply(empty)
        var result = verdict
        result["record_state"] = "recorded_verified"; result["record_id"] = id
        result["snapshot_evidence"] = "diagnostic:sha256:" + id
        result["decision_evidence"] = "maat-system-one:sha256=" + id
        let success = try reply(result)
        return { args in
            if args == MaatDiagnosticRecordModel.inspectArguments { return status }
            if args == MaatDiagnosticRecordModel.pendingArguments { return noDebt }
            XCTAssertEqual(args, ["maat", "triage", "--confirm", "--json"])
            return success
        }
    }

    func testActualRootConfirmedRefreshesBothQueryOwnedHistoriesAndRetainsResultOnRemount() async throws {
        let systemProbe = RootHistoryProbe(), decisionProbe = RootHistoryProbe()
        let system = MaatHistoryRefreshStore(read: { await systemProbe.read($0) })
        let decisions = MaatHistoryRefreshStore(read: { await decisionProbe.read($0) })
        await system.load(scope: "system-one", query: "system-filter") { await systemProbe.read($0) }
        await decisions.load(scope: "all", query: "decision-filter") { await decisionProbe.read($0) }
        let model = MaatDiagnosticRecordModel(uid: 501, reader: try successfulRecordReader())
        let engine = SirsiEngine(maatCaseOperations: MaatCaseOperationStore(diagnosticRecord: model))
        let root = MaatWorkspaceView(engine: engine, systemOneHistory: system, decisionHistory: decisions)
        let frame = try XCTUnwrap(root.body as? MaatWorkspaceRecoveryFrame)
        let control = try XCTUnwrap(frame.localDiagnostics)
        await control.inspect()
        let approval = try XCTUnwrap(model.approval(.record))
        await control.confirmed(approval)
        let systemCalls = await systemProbe.arguments(), decisionCalls = await decisionProbe.arguments()
        XCTAssertEqual(systemCalls.count, 2); XCTAssertEqual(decisionCalls.count, 2)
        XCTAssertEqual(systemCalls.last, maatCasebookHistoryArguments(scope: "system-one", query: "system-filter"))
        XCTAssertEqual(decisionCalls.last, maatCasebookHistoryArguments(scope: "all", query: "decision-filter"))
        XCTAssertEqual(system.state.query, "system-filter"); XCTAssertEqual(decisions.state.query, "decision-filter")
        XCTAssertEqual(model.completed, .recorded(id: id, store: store,
            snapshot: "diagnostic:sha256:" + id, decision: "maat-system-one:sha256=" + id))
        let remounted = MaatWorkspaceView(engine: engine, systemOneHistory: system, decisionHistory: decisions)
        let again = try XCTUnwrap((remounted.body as? MaatWorkspaceRecoveryFrame)?.localDiagnostics)
        XCTAssertTrue(again.model === model)
        XCTAssertEqual(again.completedDetails, control.completedDetails)
        XCTAssertTrue(again.completedDetails.contains("Record: " + id))
        await again.confirmed(approval)
        let after = await systemProbe.arguments()
        XCTAssertEqual(after.count, 2, "Consumed consent must not refresh or record again")
    }

    func testActualConfirmedReconciliationRetainsExactTypedResultAfterInspectionAndRemount() async throws {
        for state in ["recorded_verified", "not_submitted"] {
            let status = try reply(ready), noDebt = try reply(empty)
            let debt = try reply(["schema":"sirsi.maat.diagnostic-record.v1", "pending":[intent()]])
            let decision = state == "recorded_verified" ? "maat-system-one:sha256=" + id : ""
            let verdictSHA = state == "recorded_verified" ? id : ""
            let result = try reply(["schema":"sirsi.maat.diagnostic-record.v1", "id":id, "state":state,
                                    "intent_sha256":id, "decision_evidence":decision, "verdict_sha256":verdictSHA])
            var done = false, effects = 0, refreshes = 0
            let model = MaatDiagnosticRecordModel(uid:501) { args in
                if args == MaatDiagnosticRecordModel.inspectArguments { return status }
                if args == MaatDiagnosticRecordModel.pendingArguments { return done ? noDebt : debt }
                XCTAssertEqual(args, ["maat","triage","--reconcile-record",self.id,"--confirm","--json"])
                effects += 1; done = true; return result
            }
            let view = MaatDiagnosticRecordView(model:model, refresh:{ refreshes += 1 })
            await view.inspect()
            let approval = try XCTUnwrap(model.approval(.reconcile(id)))
            await view.confirmed(approval)
            XCTAssertTrue(model.pending.isEmpty)
            XCTAssertEqual(model.completed, .reconciled(id:id, store:store, state:state, intent:id, decision:decision, verdict:verdictSHA))
            XCTAssertNil(model.recorded, "Reconciliation must not synthesize a new observed/recorded verdict")
            XCTAssertTrue(view.completedDetails.contains("State: " + state))
            XCTAssertTrue(view.completedDetails.contains("Intent SHA256: " + id))
            let remounted = MaatDiagnosticRecordView(model:model, refresh:{ refreshes += 1 })
            await remounted.inspect()
            XCTAssertEqual(remounted.completedDetails, view.completedDetails)
            await remounted.confirmed(approval)
            XCTAssertEqual(effects, 1); XCTAssertEqual(refreshes, 1)
        }
    }

    func testActualConfirmedInitializationRetainsVerifiedReadbackIndependentlyOfReadiness() async throws {
        let absent = try reply(["state":"uninitialized"]), status = try reply(ready), noDebt = try reply(empty)
        let initialized = try reply(["status":"initialized","store_uuid":store,"imported_incidents":false])
        var exists = false, effects = 0
        let model = MaatDiagnosticRecordModel(uid:501) { args in
            if args == MaatDiagnosticRecordModel.inspectArguments { return exists ? status : absent }
            if args == MaatDiagnosticRecordModel.pendingArguments { return noDebt }
            XCTAssertEqual(args, maatCasebookFailureMemoryArguments(.initialize))
            effects += 1; exists = true; return initialized
        }
        let view = MaatDiagnosticRecordView(model:model, refresh:{})
        await view.inspect(); let approval = try XCTUnwrap(model.approval(.initialize))
        await view.confirmed(approval)
        XCTAssertEqual(model.completed, .initialized(store:store, readbackState:"ready", importedIncidents:false))
        XCTAssertTrue(view.completedDetails.contains("Store: " + store))
        XCTAssertTrue(view.completedDetails.contains("Readback: ready"))
        XCTAssertNil(model.recorded); XCTAssertNil(model.observation)
        await view.inspect()
        XCTAssertEqual(model.completed, .initialized(store:store, readbackState:"ready", importedIncidents:false))
        await view.confirmed(approval); XCTAssertEqual(effects,1)
    }

    func testRootRefreshLateReplyCannotRollBackNewHistoryFilter() async throws {
        let probe = RootHistoryProbe()
        let system = MaatHistoryRefreshStore(read: { await probe.read($0) })
        await system.load(scope:"system-one", query:"old-filter") { await probe.read($0) }
        let decisionsProbe = RootHistoryProbe()
        let decisions = MaatHistoryRefreshStore(read:{ await decisionsProbe.read($0) })
        let model = MaatDiagnosticRecordModel(uid:501, reader:try successfulRecordReader())
        let engine = SirsiEngine(maatCaseOperations:MaatCaseOperationStore(diagnosticRecord:model))
        let root = MaatWorkspaceView(engine:engine,systemOneHistory:system,decisionHistory:decisions)
        let control = try XCTUnwrap((root.body as? MaatWorkspaceRecoveryFrame)?.localDiagnostics)
        await control.inspect(); let approval = try XCTUnwrap(model.approval(.record))
        await probe.hold()
        let task = Task { @MainActor in await control.confirmed(approval) }
        for _ in 0..<1000 { if await probe.isHeld() { break }; await Task.yield() }
        let held = await probe.isHeld(); XCTAssertTrue(held)
        await system.load(scope:"system-one",query:"new-filter") { await probe.read($0) }
        await probe.release(); await task.value
        XCTAssertEqual(system.state.query,"new-filter")
        XCTAssertNotEqual(system.state.projection?.history?.snapshot,"late-old-refresh")
        XCTAssertNil(system.state.error); XCTAssertFalse(system.state.loading)
    }

    func testWriteRefreshUsesPendingSelectedQueryAndStartsFreshSnapshot() async throws {
        let probe = RootHistoryProbe()
        let history = MaatHistoryRefreshStore(read:{await probe.read($0)})
        await history.load(scope:"all",query:"committed") {await probe.read($0)}
        await probe.hold()
        let selected = Task { @MainActor in
            await history.load(scope:"all",query:"new-selection") {await probe.read($0)}
        }
        for _ in 0..<1000 {if await probe.isHeld(){break};await Task.yield()}
        let held = await probe.isHeld();XCTAssertTrue(held)
        await history.refresh(scope:"all")
        await probe.release();await selected.value
        XCTAssertEqual(history.state.query,"new-selection")
        let calls = await probe.arguments()
        XCTAssertEqual(calls.last,maatCasebookHistoryArguments(scope:"all",query:"new-selection"))
        XCTAssertFalse(calls.last?.contains("--cursor") ?? true)
        XCTAssertNil(history.state.error)
    }

    func testCancelledRootRefreshKeepsCompletedResultAndPriorProjection() async throws {
        let probe = RootHistoryProbe(), decisionsProbe = RootHistoryProbe()
        let system = MaatHistoryRefreshStore(read:{await probe.read($0)})
        await system.load(scope:"system-one",query:"retained") {await probe.read($0)}
        let prior = system.state.projection?.history?.snapshot
        let decisions = MaatHistoryRefreshStore(read:{await decisionsProbe.read($0)})
        let model = MaatDiagnosticRecordModel(uid:501,reader:try successfulRecordReader())
        let engine = SirsiEngine(maatCaseOperations:MaatCaseOperationStore(diagnosticRecord:model))
        let root = MaatWorkspaceView(engine:engine,systemOneHistory:system,decisionHistory:decisions)
        let control = try XCTUnwrap((root.body as? MaatWorkspaceRecoveryFrame)?.localDiagnostics)
        await control.inspect();let approval = try XCTUnwrap(model.approval(.record))
        await probe.hold()
        let task = Task { @MainActor in await control.confirmed(approval) }
        for _ in 0..<1000 {if await probe.isHeld(){break};await Task.yield()}
        let held = await probe.isHeld();XCTAssertTrue(held)
        task.cancel();await probe.release();await task.value
        XCTAssertEqual(system.state.projection?.history?.snapshot,prior)
        XCTAssertFalse(system.state.loading)
        XCTAssertTrue(system.state.error?.contains("cancelled") == true)
        XCTAssertNotNil(model.completed)
        XCTAssertTrue(control.completedDetails.contains("Record: " + id))
        let unseen = await decisionsProbe.arguments();XCTAssertTrue(unseen.isEmpty)
    }

    func testOversizedCompletionEvidenceRefusesWithoutClearingUnknownDelivery() async throws {
        let status = try reply(ready), noDebt = try reply(empty)
        var value = verdict
        value["record_state"] = "recorded_verified";value["record_id"] = id
        value["snapshot_evidence"] = "diagnostic:sha256:" + id
        value["decision_evidence"] = String(repeating:"x",count:257)
        let oversized = try reply(value)
        let model = MaatDiagnosticRecordModel(uid:501) {args in
            if args == MaatDiagnosticRecordModel.inspectArguments{return status}
            if args == MaatDiagnosticRecordModel.pendingArguments{return noDebt}
            return oversized
        }
        await model.inspect();let approval = try XCTUnwrap(model.approval(.record))
        let result = await model.confirm(approval)
        XCTAssertFalse(result);XCTAssertNil(model.completed);XCTAssertNil(model.recorded)
        XCTAssertTrue(model.deliveryUnknown);XCTAssertNil(model.approval(.record))
    }

}
