import SwiftUI
import XCTest
@testable import SirsiMenubar

@MainActor final class MaatDiagnosticRecordTests: XCTestCase {
    private let store = "b3c515a2-4773-4653-86d1-8a5734c2ee45"
    private let id = String(repeating: "a", count: 64)
    private func reply(_ value: Any, exit: Int32 = 0) throws -> SirsiEngine.JSONCommandRead { MaatDiagnosticCorrelationFixture.reply(value,exit:exit) }
    private var ready: [String: Any] { MaatDiagnosticCorrelationFixture().ready }
    private var empty: [String: Any] { MaatDiagnosticCorrelationFixture().page() }
    private func intent() -> [String: Any] { MaatDiagnosticCorrelationFixture().intent() }
    func testActualRootExposesDiagnosticsWithoutCaseOrSuccessfulCasebook() async throws {
        let fixture = MaatDiagnosticCorrelationFixture(); fixture.initialized = false
        let model = fixture.model()
        let actual = SirsiEngine(maatCaseOperations:MaatCaseOperationStore(diagnosticRecord:model))
        let root = MaatWorkspaceView(engine:actual)
        let entry = try XCTUnwrap((root.body as? MaatWorkspaceRecoveryFrame)?.localDiagnostics)
        XCTAssertTrue(entry.model === model); XCTAssertTrue(fixture.calls.isEmpty)
        await entry.inspect()
        XCTAssertEqual(fixture.calls,[MaatDiagnosticRecordModel.inspectArguments]); XCTAssertNotNil(model.approval(.initialize))
        XCTAssertNil(model.approval(.record)); XCTAssertEqual(entry.observeTitle,"Observe locally"); XCTAssertEqual(entry.inspectTitle,"Inspect record readiness")
    }
    func testObservationAvailableWithoutMemoryAndNeverCalledRecorded() async throws {
        let fixture = MaatDiagnosticCorrelationFixture()
        let model = fixture.model()
        await model.observe(); XCTAssertNotNil(model.observation); XCTAssertNil(model.recorded)
        XCTAssertEqual(fixture.calls,[["maat","triage","--json"]]); XCTAssertNil(model.approval(.record))
        XCTAssertTrue(model.message?.contains("not recorded") == true)
    }
    func testFirstUseRequiresFreshInspectionAndConsumedConfirmation() async throws {
        let fixture = MaatDiagnosticCorrelationFixture(); fixture.initialized = false
        let model = fixture.model(); await model.inspect()
        let approval = try XCTUnwrap(model.approval(.initialize))
        let first = await model.confirm(approval), repeated = await model.confirm(approval)
        XCTAssertTrue(first); XCTAssertFalse(repeated)
        XCTAssertEqual(fixture.calls.filter{$0.contains("--initialize-failure-memory")}.count,1)
        XCTAssertFalse(fixture.calls.contains{$0.contains("triage")}); XCTAssertEqual(model.memory?.storeUUID,store); XCTAssertNil(model.recorded)
    }
    func testChangedFirstUseStateRefusesInitialization() async throws {
        let fixture = MaatDiagnosticCorrelationFixture(); fixture.initialized = false
        let model = fixture.model(); await model.inspect(); let approval = try XCTUnwrap(model.approval(.initialize))
        fixture.initialized = true; let result = await model.confirm(approval)
        XCTAssertFalse(result); XCTAssertFalse(fixture.calls.contains{$0.contains("--initialize-failure-memory")})
    }
    func testFirstUseApprovalRejectsUnknownAndConflictingDebt()async throws{
        for fields:[String:Any] in [
            ["state":"unknown"],
            ["state":"uninitialized","journal_readiness":"invalid"],
            ["state":"uninitialized","journal_readiness":"unavailable"],
            ["state":"uninitialized","journal_readiness":"unknown"],
            ["state":"uninitialized"],
            ["state":"uninitialized","journal_readiness":"ready","incomplete_attempt":true],
            ["state":"uninitialized","journal_readiness":"ready","projection_debt":true],
            ["state":"uninitialized","journal_readiness":"ready","receipt_recovery_error":"prepared-inspection-unavailable"]
        ] {
            let read=try reply(fields)
            let model=MaatDiagnosticRecordModel(uid:501){_ in read}
            await model.inspect()
            XCTAssertNil(model.approval(.initialize),"\(fields) must not authorize first-use initialization")
        }
    }
    func testTypedPreCreationJournalRefusalRefreshesIntegrityInspectorWithoutRetry()async throws{
        let absent=try reply(["state":"uninitialized","journal_readiness":"ready"])
        let refusal=try reply(["status":"refused","stage":"before_store_creation","journal_readiness":"invalid",
            "journal_integrity":["invalid_count":1,"count_is_lower_bound":true,"issues":[["line":5001,"digest":"sha256:"+String(repeating:"a",count:64),"reason":"requester is required"]]]],exit:1)
        var calls:[[String]]=[]
        var integrityRefreshes=0
        let model=MaatDiagnosticRecordModel(uid:501){args in
            calls.append(args)
            if args==MaatDiagnosticRecordModel.inspectArguments { return absent }
            return refusal
        }
        let view=MaatDiagnosticRecordView(model:model,refresh:{},initializationRefresh:{integrityRefreshes+=1})
        await view.inspect()
        let approval=try XCTUnwrap(model.approval(.initialize))
        await view.confirmed(approval)
        XCTAssertEqual(calls,[MaatDiagnosticRecordModel.inspectArguments,MaatDiagnosticRecordModel.inspectArguments,
                              maatCasebookFailureMemoryArguments(.initialize)])
        XCTAssertEqual(model.journalIntegrityRefusal?.invalidCount,1)
        XCTAssertTrue(model.journalIntegrityRefusal?.countIsLowerBound == true)
        XCTAssertEqual(integrityRefreshes,1,"typed refusal routes to fresh top-level integrity inspection")
        XCTAssertNil(model.approval(.initialize),"typed refusal requires fresh readiness inspection before any new consent")
        XCTAssertNil(model.completed)
        XCTAssertTrue(model.message?.contains("before store creation") == true)
    }
    func testVerifiedRecordPreservesExistingDecisionWireAndOnceConsent() async throws {
        let fixture = MaatDiagnosticCorrelationFixture()
        let model = fixture.model()
        let actual = model
        await actual.inspect(); let approval = try XCTUnwrap(actual.approval(.record))
        let first = await actual.confirm(approval), repeated = await actual.confirm(approval)
        XCTAssertTrue(first); XCTAssertFalse(repeated)
        XCTAssertEqual(fixture.calls.filter{$0.contains("--record-request")}.count,1)
        let key = try XCTUnwrap(fixture.requestID)
        XCTAssertEqual(fixture.calls.filter{$0.contains("--prepare-record-request")},[MaatDiagnosticRecordModel.requestArguments("prepare",id:key)])
        XCTAssertEqual(fixture.calls.filter{$0.contains("--read-record-request")},[MaatDiagnosticRecordModel.requestArguments("read",id:key)])
        XCTAssertEqual(actual.recorded?.verdict.gate,"changes"); XCTAssertFalse(actual.deliveryUnknown)
        XCTAssertEqual(actual.requestCompletion?.recordID,id)
    }
    func testRefusedRecordKeepsObservationAndReconcilesOnlyExactCapturedIntent() async throws {
        let fixture = MaatDiagnosticCorrelationFixture()
        fixture.recordReply = try reply(["schema":MaatDiagnosticWire.recordSchema,"record_state":"delivery_unverified","record_id":id,"observation":fixture.verdict],exit:1)
        let model = fixture.model(); await model.inspect(); let approval = try XCTUnwrap(model.approval(.record))
        let failed = await model.confirm(approval); XCTAssertFalse(failed)
        XCTAssertNotNil(model.observation); XCTAssertNil(model.recorded); XCTAssertTrue(model.deliveryUnknown)
        let key = try XCTUnwrap(model.activeRequestID)
        await model.inspect(); let resolve = try XCTUnwrap(model.approval(.resolveRequest(key)))
        let resolved = await model.confirm(resolve), repeated = await model.confirm(resolve)
        XCTAssertTrue(resolved); XCTAssertFalse(repeated); XCTAssertFalse(model.deliveryUnknown)
        XCTAssertEqual(fixture.calls.filter{$0.contains("--record-request")}.count,1)
        XCTAssertEqual(fixture.calls.filter{$0.contains("--resolve-record-request")},[MaatDiagnosticRecordModel.requestArguments("resolve",id:key,confirm:true)])
    }
    func testLaterRefusalOrUnknownReplyCannotReusePriorRecordedLabel() async throws {
        for lost in [false,true] {
            let fixture = MaatDiagnosticCorrelationFixture()
            var priorReply = fixture.success; priorReply["gate"] = "pass"
            fixture.recordReply = try reply(priorReply)
            let actual = fixture.model(); await actual.inspect()
            let first = try XCTUnwrap(actual.approval(.record)); let success = await actual.confirm(first); XCTAssertTrue(success)
            let prior = actual.completed
            fixture.recordReply = lost ? MaatDiagnosticCorrelationFixture.lost : try reply(["schema":MaatDiagnosticWire.recordSchema,"record_state":"delivery_unverified","record_id":id,"observation":fixture.verdict],exit:1)
            fixture.readLost = lost; await actual.inspect()
            let next = try XCTUnwrap(actual.approval(.record)); let result = await actual.confirm(next)
            XCTAssertFalse(result); XCTAssertNil(actual.recorded); XCTAssertTrue(actual.deliveryUnknown)
            XCTAssertEqual(actual.completed,prior); XCTAssertNil(actual.approval(.record))
            XCTAssertEqual(actual.observation?.gate,lost ? "pass" : "changes")
            XCTAssertEqual(fixture.calls.filter{$0.contains("--record-request")}.count,2)
        }
    }
    func testUnknownReplyThenEmptyInspectionCannotAuthorizeResend() async throws {
        let fixture = MaatDiagnosticCorrelationFixture(); fixture.readLost = true; fixture.recordReply = MaatDiagnosticCorrelationFixture.lost
        let model = fixture.model(); await model.inspect(); let approval = try XCTUnwrap(model.approval(.record))
        let result = await model.confirm(approval); XCTAssertFalse(result)
        let retained = model.activeRequestID
        fixture.override = { args in args.contains("--inspect-record-requests") ? MaatDiagnosticCorrelationFixture.reply(fixture.page()) : nil }
        await model.inspect(); XCTAssertEqual(model.activeRequestID,retained); XCTAssertTrue(model.deliveryUnknown)
        XCTAssertNil(model.approval(.record)); XCTAssertFalse(model.busy)
    }
    func testChangedOrForeignIntentRefusesReconciliationWithoutAppend() async throws {
        for foreign in [false,true] {
            let fixture = MaatDiagnosticCorrelationFixture(); fixture.prepareLost = true
            let model = fixture.model(); await model.inspect(); let approval = try XCTUnwrap(model.approval(.record))
            let result = await model.confirm(approval); XCTAssertFalse(result)
            let key = try XCTUnwrap(model.activeRequestID), resolve = try XCTUnwrap(model.approval(.resolveRequest(try XCTUnwrap(model.activeRequestID))))
            fixture.override = { args in
                if args.contains("--resolve-record-request") {
                    var value = fixture.request(key,state:"recorded_verified")
                    var identity = value["request"] as! [String:Any]; identity[foreign ? "store_uuid" : "id"] = foreign ? "other" : "b9a444f3-a7b0-4dcf-af6f-c62d42321ea4"; value["request"] = identity
                    return MaatDiagnosticCorrelationFixture.reply(value)
                }; return nil
            }
            let resolved = await model.confirm(resolve); XCTAssertFalse(resolved)
            XCTAssertTrue(model.deliveryUnknown); XCTAssertNil(model.requestCompletion)
            XCTAssertFalse(fixture.calls.contains{$0.contains("--record-request")})
        }
    }
    func testMalformedHistoryCannotOfferRecordOrInitialization() async throws {
        let fixture = MaatDiagnosticCorrelationFixture()
        fixture.override = { args in args.contains("--inspect-record-requests") ? MaatDiagnosticCorrelationFixture.reply(["schema":MaatDiagnosticWire.requestSchema,"requests":[]]) : nil }
        let model = fixture.model(); await model.inspect(); XCTAssertNil(model.approval(.record)); XCTAssertNil(model.approval(.initialize))
        XCTAssertTrue(model.discoveryRestartRequired); XCTAssertNil(model.recorded)
    }
    func testCanceledReaderCannotPublishRecordOrLeaveBusyAndFreshInspectionWorks() async throws {
        let fixture = MaatDiagnosticCorrelationFixture(); var entered = false
        fixture.onRecord = { entered = true; do { try await Task.sleep(nanoseconds:1_000_000_000) } catch {} }
        let model = fixture.model(); await model.inspect(); let approval = try XCTUnwrap(model.approval(.record))
        let task = Task { @MainActor in await model.confirm(approval) }
        for _ in 0..<1000 where !entered { await Task.yield() }
        XCTAssertTrue(entered); model.cancel(); let result = await task.value
        XCTAssertFalse(result); XCTAssertTrue(model.deliveryUnknown); XCTAssertNotNil(model.activeRequestID); XCTAssertFalse(model.busy)
        await model.inspect(); XCTAssertNil(model.approval(.record)); XCTAssertNil(model.recorded)
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
        let fixture = MaatDiagnosticCorrelationFixture()
        return { await fixture.read($0) }
    }

    func testActualRootConfirmedRefreshesBothQueryOwnedHistoriesAndRetainsResultOnRemount() async throws {
        let systemProbe = RootHistoryProbe(), decisionProbe = RootHistoryProbe()
        let system = MaatHistoryRefreshStore(read: { await systemProbe.read($0) })
        let decisions = MaatHistoryRefreshStore(read: { await decisionProbe.read($0) })
        await system.load(scope: "system-one", query: "system-filter") { await systemProbe.read($0) }
        await decisions.load(scope: "all", query: "decision-filter") { await decisionProbe.read($0) }
        let model = MaatDiagnosticRecordModel(uid: 501, localHost:{ "fixture-mac" }, reader: try successfulRecordReader())
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
            snapshot: "diagnostic:sha256=" + id, decision: "maat-system-one:sha256=" + id))
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
            let debt = try reply(MaatDiagnosticCorrelationFixture().page(records:[["intent":intent(),"state":"pending"]]))
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
            XCTAssertEqual(effects, 1); XCTAssertEqual(refreshes, state == "recorded_verified" ? 1 : 0)
        }
    }

    func testActualConfirmedInitializationRetainsVerifiedReadbackIndependentlyOfReadiness() async throws {
        var readyStatus=ready; readyStatus["journal_readiness"]="ready"
        let absent = try reply(["state":"uninitialized","journal_readiness":"ready"]), status = try reply(readyStatus), noDebt = try reply(empty)
        let initialized = try reply(["status":"initialized","store_uuid":store,"imported_incidents":false])
        var exists = false, effects = 0
        let model = MaatDiagnosticRecordModel(uid:501) { args in
            if args == MaatDiagnosticRecordModel.inspectArguments { return exists ? status : absent }
            if args == MaatDiagnosticRecordModel.pendingArguments { return noDebt }
            XCTAssertEqual(args, maatCasebookFailureMemoryArguments(.initialize))
            effects += 1; exists = true; return initialized
        }
        var initializedRefreshes=0
        let view = MaatDiagnosticRecordView(model:model, refresh:{},initializationRefresh:{initializedRefreshes+=1})
        await view.inspect(); let approval = try XCTUnwrap(model.approval(.initialize))
        await view.confirmed(approval)
        XCTAssertEqual(model.completed, .initialized(store:store, readbackState:"ready", importedIncidents:false))
        XCTAssertEqual(initializedRefreshes,1,"verified initialization refreshes the top memory inspector")
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
        let model = MaatDiagnosticRecordModel(uid:501, localHost:{ "fixture-mac" }, reader:try successfulRecordReader())
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
        let model = MaatDiagnosticRecordModel(uid:501,localHost:{ "fixture-mac" },reader:try successfulRecordReader())
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

    func testDedicatedFreshScreenRefreshesActualRootQueryOwnersAndKeepsEngineModel() async throws {
        let fixture = MaatDiagnosticCorrelationFixture(), model = MaatDiagnosticResolutionModel()
        let correlation = fixture.model(), systemProbe = RootHistoryProbe(), decisionProbe = RootHistoryProbe()
        let system = MaatHistoryRefreshStore(read:{await systemProbe.read($0)})
        let decisions = MaatHistoryRefreshStore(read:{await decisionProbe.read($0)})
        await system.load(scope:"system-one",query:"owned-filter") {await systemProbe.read($0)}
        await decisions.load(scope:"all",query:"decision-filter") {await decisionProbe.read($0)}
        let engine = SirsiEngine(maatCaseOperations:MaatCaseOperationStore(diagnosticRecord:correlation))
        let root = MaatWorkspaceView(engine:engine,systemOneHistory:system,decisionHistory:decisions)
        let frame = try XCTUnwrap(root.body as? MaatWorkspaceRecoveryFrame)
        XCTAssertTrue(frame.localDiagnostics?.model === correlation); XCTAssertTrue(fixture.calls.isEmpty)
        await model.recordFreshScreen(localHost:"fixture-mac",using:correlation)
        let systemCalls = await systemProbe.arguments(), decisionCalls = await decisionProbe.arguments()
        XCTAssertEqual(systemCalls.count,2); XCTAssertEqual(decisionCalls.count,2)
        XCTAssertEqual(systemCalls.last,maatCasebookHistoryArguments(scope:"system-one",query:"owned-filter"))
        XCTAssertEqual(decisionCalls.last,maatCasebookHistoryArguments(scope:"all",query:"decision-filter"))
        XCTAssertNotNil(correlation.requestCompletion); XCTAssertNotNil(model.screenResult)
        await model.recordFreshScreen(localHost:"fixture-mac",using:correlation)
        let finalCalls = await systemProbe.arguments(); XCTAssertEqual(finalCalls.count,2)
    }

    func testOversizedCompletionEvidenceRefusesWithoutClearingUnknownDelivery() async throws {
        let fixture = MaatDiagnosticCorrelationFixture()
        var bad = fixture.request("f9312875-79fa-49b9-a4a0-db3c5562f9af",state:"recorded_verified")
        fixture.override = { args in
            guard args.contains("--read-record-request"), let key = fixture.requestID else { return nil }
            bad = fixture.request(key,state:"recorded_verified")
            var result = bad["result"] as! [String:Any]; result["decision_evidence"] = String(repeating:"x",count:257); bad["result"] = result
            return MaatDiagnosticCorrelationFixture.reply(bad)
        }
        let model = fixture.model(); await model.inspect(); let approval = try XCTUnwrap(model.approval(.record))
        let result = await model.confirm(approval); XCTAssertFalse(result)
        XCTAssertNil(model.completed); XCTAssertNil(model.recorded); XCTAssertTrue(model.deliveryUnknown); XCTAssertNil(model.approval(.record))
    }
}
