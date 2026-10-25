import SwiftUI
import XCTest
import CryptoKit
@testable import SirsiMenubar

@MainActor final class MaatDiagnosticCorrelationTests: XCTestCase {
    private let key = "690c358a-d905-4546-871d-de40a76cc0ac"
    func testRequestArgumentsUseLiteralCanonicalGoFlagsForAllFourOperations() {
        XCTAssertEqual(MaatDiagnosticRecordModel.requestArguments("prepare", id:key),
                       ["maat", "triage", "--prepare-record-request", key, "--json"])
        XCTAssertEqual(MaatDiagnosticRecordModel.requestArguments("record", id:key, confirm:true),
                       ["maat", "triage", "--record-request", key, "--confirm", "--json"])
        XCTAssertEqual(MaatDiagnosticRecordModel.requestArguments("read", id:key),
                       ["maat", "triage", "--read-record-request", key, "--json"])
        XCTAssertEqual(MaatDiagnosticRecordModel.requestArguments("resolve", id:key, confirm:true),
                       ["maat", "triage", "--resolve-record-request", key, "--confirm", "--json"])
    }
    func testDiscoveryPasses129HistoricalRowsBeforeRequestAndNeverTreatsFirstEmptyRequestsAsEmptyHistory() async throws {
        let fixture = MaatDiagnosticCorrelationFixture(); var page = 0
        fixture.override = { args in
            guard args.contains("--inspect-record-requests") else { return nil }
            defer { page += 1 }
            let begin = page * 50, end = min(begin + 50,129)
            let records: [[String:Any]] = begin < end ? (begin..<end).map {
                ["intent":fixture.intent(id:String(format:"%064x",$0)),"state":"pending"]
            } : []
            let last = page == 2
            let rows = last ? [fixture.request(self.key)] : []
            return MaatDiagnosticCorrelationFixture.reply(fixture.page(requests:rows,records:records,cursor:last ? nil : "page-\(page+1)",after:page > 0))
        }
        let model = fixture.model(); await model.inspect()
        XCTAssertTrue(model.discoveryComplete); XCTAssertEqual(model.records.count,129); XCTAssertEqual(model.requests.map(\.id),[key])
        XCTAssertNil(model.approval(.record)); XCTAssertNotNil(model.approval(.resolveRequest(key)))
        XCTAssertEqual(fixture.calls.filter{$0.contains("--request-cursor")}.count,2)
        XCTAssertFalse(fixture.calls.contains{$0.contains("--record-request")})
    }
    func testFiniteDiscoveryContinuesSameSnapshotWithoutGlobalHistoryCap() async throws {
        let fixture = MaatDiagnosticCorrelationFixture(); var count = 0
        fixture.override = { args in
            guard args.contains("--inspect-record-requests") else { return nil }
            count += 1
            return MaatDiagnosticCorrelationFixture.reply(fixture.page(records:[["intent":fixture.intent(id:String(format:"%064x",count)),"state":"pending"]],cursor:count == 10 ? nil : "cursor-\(count)",after:count > 1))
        }
        let model = fixture.model(); await model.inspect()
        XCTAssertEqual(count,8); XCTAssertFalse(model.discoveryComplete); XCTAssertNotNil(model.discoveryCursor); XCTAssertNil(model.approval(.record))
        await model.continueDiscovery()
        XCTAssertEqual(count,10); XCTAssertTrue(model.discoveryComplete); XCTAssertEqual(model.records.count,10)
        XCTAssertNil(model.discoveryCursor); XCTAssertEqual(model.pending.count,10)
    }
    func testDiscoverySnapshotDriftRetainsPartialAndRequiresExplicitRestart() async throws {
        let fixture = MaatDiagnosticCorrelationFixture(); var count = 0
        fixture.override = { args in
            guard args.contains("--inspect-record-requests") else { return nil }
            count += 1
            let snapshot = count == 2 ? String(repeating:"b",count:64) : fixture.recordID
            return MaatDiagnosticCorrelationFixture.reply(fixture.page(requests:[fixture.request(count == 2 ? "73276ac2-1fb7-4ebd-abfc-ccfb6e9cf057" : self.key)],cursor:count == 1 ? "next" : nil,after:count == 2,snapshot:snapshot))
        }
        let model = fixture.model(); await model.inspect()
        XCTAssertTrue(model.discoveryRestartRequired); XCTAssertFalse(model.discoveryComplete); XCTAssertEqual(model.requests.count,1)
        XCTAssertNil(model.approval(.record)); await model.continueDiscovery(); XCTAssertEqual(count,2)
        await model.inspect(); XCTAssertTrue(model.discoveryComplete); XCTAssertFalse(model.discoveryRestartRequired)
        XCTAssertFalse(fixture.calls.last?.contains("--request-cursor") ?? true)
    }
    func testDuplicateRowsAndRepeatedCursorsRefuseCompletion() async throws {
        for duplicate in [true,false] {
            let fixture = MaatDiagnosticCorrelationFixture(); var count = 0
            fixture.override = { args in
                guard args.contains("--inspect-record-requests") else { return nil }
                count += 1
                let id = duplicate ? fixture.recordID : String(format:"%064x",count)
                return MaatDiagnosticCorrelationFixture.reply(fixture.page(records:[["intent":fixture.intent(id:id),"state":"pending"]],cursor:"repeat",after:count > 1))
            }
            let model = fixture.model(); await model.inspect()
            XCTAssertTrue(model.discoveryRestartRequired); XCTAssertFalse(model.discoveryComplete); XCTAssertEqual(model.records.count,1)
            XCTAssertNil(model.approval(.record))
        }
    }
    func testLostPreparationRetainsUUIDBeforeReaderAndResolveFencesWithoutInvoke() async throws {
        let fixture = MaatDiagnosticCorrelationFixture(); fixture.prepareLost = true
        let model = fixture.model()
        fixture.override = { args in
            if let index = args.firstIndex(of:"--prepare-record-request") {
                XCTAssertEqual(model.activeRequestID,args[index+1]); XCTAssertTrue(model.deliveryUnknown)
            }; return nil
        }
        await model.inspect(); let approval = try XCTUnwrap(model.approval(.record))
        let failed = await model.confirm(approval); XCTAssertFalse(failed)
        let retained = try XCTUnwrap(model.activeRequestID)
        XCTAssertTrue(MaatDiagnosticWire.uuid(retained)); XCTAssertFalse(fixture.calls.contains{$0.contains("--record-request")})
        await model.readRequest(retained)
        XCTAssertEqual(model.activeRequestID,retained,"Prepared is not a completed result and cannot authorize resend")
        let resolve = try XCTUnwrap(model.approval(.resolveRequest(retained)))
        let resolved = await model.confirm(resolve); XCTAssertTrue(resolved)
        XCTAssertEqual(model.requestCompletion?.state,"fenced_not_recorded"); XCTAssertFalse(model.deliveryUnknown)
        XCTAssertEqual(model.requestCompletion?.request.id,retained)
        XCTAssertFalse(fixture.calls.contains{$0.contains("--record-request")})
        await model.inspect(); let fresh = try XCTUnwrap(model.approval(.record))
        fixture.prepareLost = false
        let success = await model.confirm(fresh); XCTAssertTrue(success)
        XCTAssertNotEqual(fixture.requestID,retained)
    }
    func testResolveFirstDiscoveredDebtCannotBeReplacedByFreshRecordConsent() async throws {
        let fixture = MaatDiagnosticCorrelationFixture(); fixture.requestID = key; fixture.state = "interrupted"
        let model = fixture.model(); await model.inspect()
        XCTAssertNil(model.approval(.record))
        let resolve = try XCTUnwrap(model.approval(.resolveRequest(key)))
        let resolved = await model.confirm(resolve); XCTAssertTrue(resolved)
        XCTAssertEqual(model.requestCompletion?.request.id,key)
        XCTAssertEqual(fixture.calls.filter{$0.contains("--resolve-record-request")},[MaatDiagnosticRecordModel.requestArguments("resolve",id:key,confirm:true)])
        XCTAssertFalse(fixture.calls.contains{$0.contains("--record-request")})
    }
    func testExactTerminalReadRetainsTypedReceiptWithoutReplayingLostOutput() async throws {
        let fixture = MaatDiagnosticCorrelationFixture(); fixture.readLost = true
        let model = fixture.model(); await model.inspect(); let approval = try XCTUnwrap(model.approval(.record))
        let failed = await model.confirm(approval); XCTAssertFalse(failed)
        let retained = try XCTUnwrap(model.activeRequestID)
        fixture.readLost = false; await model.readRequest(retained)
        XCTAssertFalse(model.deliveryUnknown); XCTAssertNil(model.activeRequestID); XCTAssertEqual(model.requestCompletion?.request.id,retained)
        XCTAssertEqual(model.requestCompletion?.recordID,fixture.recordID)
        XCTAssertNil(model.recorded,"Exact metadata read cannot manufacture the lost observation payload")
        let completed = model.requestCompletion
        fixture.override = { args in args.contains("--inspect-record-requests") ? MaatDiagnosticCorrelationFixture.reply(["error":"snapshot_changed"],exit:1) : nil }
        await model.inspect(); XCTAssertEqual(model.requestCompletion,completed)
        let engine = SirsiEngine(maatCaseOperations:MaatCaseOperationStore(diagnosticRecord:model))
        let root = MaatWorkspaceView(engine:engine)
        XCTAssertTrue((root.body as? MaatWorkspaceRecoveryFrame)?.localDiagnostics?.completedDetails.contains("Request: " + retained) == true)
        XCTAssertEqual(fixture.calls.filter{$0.contains("--record-request")}.count,1)
    }
    func testForeignTupleContradictoryStateAndResultRelationAreRefused() throws {
        let fixture = MaatDiagnosticCorrelationFixture(), valid = fixture.request(key,state:"recorded_verified")
        var variants: [[String:Any]] = []
        for (field,value) in [("uid",502 as Any),("store_uuid","other" as Any),("id","not-a-uuid" as Any)] {
            var row = valid, identity = valid["request"] as! [String:Any]; identity[field] = value; row["request"] = identity; variants.append(row)
        }
        var noResult = valid; noResult.removeValue(forKey:"result"); variants.append(noResult)
        var noIntent = valid; noIntent.removeValue(forKey:"intent"); variants.append(noIntent)
        var changed = valid, result = valid["result"] as! [String:Any]; result["id"] = String(repeating:"b",count:64); changed["result"] = result; variants.append(changed)
        var wrongState = valid; wrongState["state"] = "prepared"; variants.append(wrongState)
        for row in variants {
            let read = try JSONDecoder().decode(MaatDiagnosticWire.Request.self,from:JSONSerialization.data(withJSONObject:row))
            XCTAssertFalse(read.valid(uid:501,store:fixture.store))
        }
    }
    func testUnknownReadOfUnrelatedRequestNeverClearsOriginalUncertainty() async throws {
        let fixture = MaatDiagnosticCorrelationFixture(); fixture.prepareLost = true
        let model = fixture.model(); await model.inspect(); let approval = try XCTUnwrap(model.approval(.record))
        let failed = await model.confirm(approval); XCTAssertFalse(failed)
        let original = model.activeRequestID
        fixture.state = "recorded_verified"; await model.readRequest(key)
        XCTAssertEqual(model.activeRequestID,original); XCTAssertTrue(model.deliveryUnknown); XCTAssertNil(model.approval(.record))
    }
    func testLargeCompletedDiscoveryCanConfirmWithoutRestartingWholeChainForever() async throws {
        let fixture = MaatDiagnosticCorrelationFixture()
        fixture.override = { args in
            guard args.contains("--inspect-record-requests") else { return nil }
            let n = args.firstIndex(of:"--request-cursor").flatMap { Int(args[$0+1]) } ?? 0
            let id = String(format:"%064x",n)
            return MaatDiagnosticCorrelationFixture.reply(fixture.page(records:[["intent":fixture.intent(id:id),"state":"recorded_verified","result":fixture.result(id:id)]],cursor:n == 8 ? nil : String(n+1),after:n > 0))
        }
        let model = fixture.model(); await model.inspect(); XCTAssertNil(model.approval(.record))
        await model.continueDiscovery(); XCTAssertTrue(model.discoveryComplete)
        let approval = try XCTUnwrap(model.approval(.record)); let result = await model.confirm(approval)
        XCTAssertTrue(result); XCTAssertEqual(fixture.calls.filter{$0.contains("--record-request")}.count,1)
        XCTAssertEqual(fixture.calls.filter{$0.contains("--inspect-record-requests")}.count,10,"Nine initial pages plus one whole-namespace snapshot check; no perpetual first-batch restart")
    }
    func testChangedKnownResolutionTupleRefusesBeforeEffect() async throws {
        let fixture = MaatDiagnosticCorrelationFixture(); fixture.requestID = key; fixture.state = "prepared"
        let model = fixture.model(); await model.inspect(); let approval = try XCTUnwrap(model.approval(.resolveRequest(key)))
        fixture.state = "interrupted"
        let resolved = await model.confirm(approval)
        XCTAssertFalse(resolved); XCTAssertFalse(fixture.calls.contains{$0.contains("--resolve-record-request")})
        XCTAssertFalse(fixture.calls.contains{$0.contains("--record-request")})
    }
    func testChangedStoreCannotMoveRetainedUnknownRequestToAnotherStore() async throws {
        let fixture = MaatDiagnosticCorrelationFixture(); fixture.prepareLost = true
        let model = fixture.model(); await model.inspect(); let record = try XCTUnwrap(model.approval(.record))
        let failed = await model.confirm(record); XCTAssertFalse(failed)
        let original = try XCTUnwrap(model.activeRequestID)
        fixture.override = { args in args == MaatDiagnosticRecordModel.inspectArguments ? MaatDiagnosticCorrelationFixture.reply(["state":"ready","store_uuid":"8d1948ca-8e97-4a9e-abcd-75790be3e4fa"]) : nil }
        await model.inspect(); XCTAssertNil(model.approval(.resolveRequest(original)))
        let read = await model.readRequest(original); XCTAssertFalse(read)
        XCTAssertEqual(model.activeRequestID,original); XCTAssertTrue(model.deliveryUnknown)
    }
    func testCancelledContinuationRetainsLastCompletedReceiptAndPartialRows() async throws {
        let fixture = MaatDiagnosticCorrelationFixture()
        let actual = fixture.model(); await actual.inspect(); let approval = try XCTUnwrap(actual.approval(.record))
        let success = await actual.confirm(approval); XCTAssertTrue(success)
        let completed = actual.requestCompletion
        var count = 0, held = false
        fixture.override = { args in
            guard args.contains("--inspect-record-requests") else { return nil }
            count += 1
            if count == 9 { held = true; do {try await Task.sleep(nanoseconds:1_000_000_000)} catch {} }
            return MaatDiagnosticCorrelationFixture.reply(fixture.page(records:[["intent":fixture.intent(id:String(format:"%064x",count)),"state":"pending"]],cursor:"page-\(count)",after:count > 1))
        }
        await actual.inspect(); XCTAssertEqual(actual.records.count,8)
        let continuation = Task { @MainActor in await actual.continueDiscovery() }
        for _ in 0..<1000 where !held {await Task.yield()}
        XCTAssertTrue(held); actual.cancel(); await continuation.value
        XCTAssertFalse(actual.busy); XCTAssertTrue(actual.discoveryRestartRequired); XCTAssertFalse(actual.discoveryComplete)
        XCTAssertEqual(actual.records.count,8); XCTAssertEqual(actual.requestCompletion,completed)
        XCTAssertNil(actual.approval(.record))
        XCTAssertEqual(fixture.calls.filter{$0.contains("--record-request")}.count,1)
    }
    func testRecordedResultRequiresExactDecisionVerdictDigestBindingBeforeClearingUnknown() async throws {
        let fixture = MaatDiagnosticCorrelationFixture()
        func decoded(_ row: [String:Any]) throws -> MaatDiagnosticWire.Request {
            try JSONDecoder().decode(MaatDiagnosticWire.Request.self,from:JSONSerialization.data(withJSONObject:row))
        }
        let matching = try decoded(fixture.request(key,state:"recorded_verified"))
        let intent = try XCTUnwrap(matching.intent)
        XCTAssertTrue(try XCTUnwrap(matching.result).valid(intent:intent))
        XCTAssertTrue(matching.valid(uid:501,store:fixture.store))
        // Existing readiness and pending/non-recorded contracts are controls,
        // not rewritten fixtures or new ways to infer completed delivery.
        for state in ["prepared","pending","not_submitted","not_recorded"] {
            let control = try decoded(fixture.request(key,state:state))
            XCTAssertTrue(control.valid(uid:501,store:fixture.store),state)
            if state == "not_submitted" || state == "not_recorded" {
                var contradictory = fixture.request(key,state:state)
                var result = try XCTUnwrap(contradictory["result"] as? [String:Any])
                result["decision_evidence"] = "maat-system-one:sha256=" + fixture.recordID
                contradictory["result"] = result
                XCTAssertFalse(try decoded(contradictory).valid(uid:501,store:fixture.store),state)
            }
        }
        for evidence in ["maat-system-one:sha256=" + String(repeating:"b",count:64),
                         "other-decision:sha256=" + fixture.recordID] {
            var contradictory = fixture.request(key,state:"recorded_verified")
            var result = try XCTUnwrap(contradictory["result"] as? [String:Any])
            result["decision_evidence"] = evidence; contradictory["result"] = result
            let value = try decoded(contradictory)
            XCTAssertFalse(try XCTUnwrap(value.result).valid(intent:intent),"Contradictory Decision/verdict digest accepted")
            XCTAssertFalse(value.valid(uid:501,store:fixture.store))
            let owned = MaatDiagnosticCorrelationFixture(); owned.prepareLost = true
            let model = owned.model(); await model.inspect()
            XCTAssertEqual(model.memory?.state,"ready")
            let approval = try XCTUnwrap(model.approval(.record))
            let prepared = await model.confirm(approval); XCTAssertFalse(prepared)
            let retained = try XCTUnwrap(model.activeRequestID)
            owned.override = { args in
                guard args.contains("--read-record-request") else {return nil}
                var row = owned.request(retained,state:"recorded_verified")
                var result = row["result"] as! [String:Any]
                result["decision_evidence"] = evidence; row["result"] = result
                return MaatDiagnosticCorrelationFixture.reply(row)
            }
            let verified = await model.readRequest(retained)
            XCTAssertFalse(verified); XCTAssertTrue(model.deliveryUnknown)
            XCTAssertEqual(model.activeRequestID,retained); XCTAssertNil(model.requestCompletion)
            XCTAssertFalse(owned.calls.contains{$0.contains("--record-request")})
        }
    }

    func testActualGoRequestPageAndResultInterop() async throws {
        guard let path = ProcessInfo.processInfo.environment["PANTHEON_TEST_DIAGNOSTIC_REQUEST_INTEROP"] else {
            throw XCTSkip("Main must supply actual Go-produced canonical request/preparation/read/discovery bytes")
        }
        let data = try Data(contentsOf:URL(fileURLWithPath:path))
        XCTAssertLessThanOrEqual(data.count,8*1024*1024)
        let object = try XCTUnwrap(JSONSerialization.jsonObject(with:data) as? [String:Any])
        let store = try XCTUnwrap(object["store_uuid"] as? String), key = try XCTUnwrap(object["request_id"] as? String)
        let acceptedArgv = try XCTUnwrap(object["native_argv"] as? [String:[String]])
        XCTAssertEqual(Set(acceptedArgv.keys),Set(["prepare","record","read","resolve"]))
        for (operation, confirmed) in [("prepare",false),("record",true),("read",false),("resolve",true)] {
            XCTAssertEqual(MaatDiagnosticRecordModel.requestArguments(operation,id:key,confirm:confirmed),acceptedArgv[operation],
                           "Native argv must equal the argv accepted by the actual Go command fixture")
        }
        let uid = UInt32(getuid())
        for name in ["prepared","lookup","resolved"] {
            let bytes = try JSONSerialization.data(withJSONObject:try XCTUnwrap(object[name]))
            let read = try JSONDecoder().decode(MaatDiagnosticWire.Request.self,from:bytes)
            XCTAssertTrue(read.valid(uid:uid,store:store)); XCTAssertEqual(read.id,key)
            XCTAssertEqual(read.state,name == "prepared" ? "prepared" : "recorded_verified")
        }
        let page = try JSONDecoder().decode(MaatDiagnosticWire.Page.self,from:JSONSerialization.data(withJSONObject:try XCTUnwrap(object["discovery"])))
        XCTAssertTrue(page.valid(uid:uid,store:store,after:false)); XCTAssertEqual(page.requests.map(\.id),[key])
        XCTAssertEqual(page.history.coverage,"complete")
        let reply = MaatDiagnosticCorrelationFixture.reply(try XCTUnwrap(object["recorded"]))
        let value = try JSONDecoder().decode(MaatSystemOneCommandResult.self,from:reply.data)
        XCTAssertTrue(maatDiagnosticHostMatches(value.verdict.subject,localHost:"fixture-mac"))
        XCTAssertFalse(maatDiagnosticHostMatches(value.verdict.subject,localHost:"foreign-mac"))
        for check in ["Memory Death Spiral","/Users/PRIVATE_UNKNOWN_CHECK --token SECRET_CANARY"] {
            let suffix = SHA256.hash(data:Data(check.utf8)).prefix(6).map {String(format:"%02x",$0)}.joined()
            let finding = try XCTUnwrap(value.verdict.findings.first {$0.id == "host-health-" + suffix})
            let report = MaatDiagnosticCorrelationFixture.reply(["status":"red","findings":[["check":check,"severity":2,"message":"fresh inert warning"]]])
            guard case .current = maatCurrentDiagnosticResolution(verdict:value.verdict,finding:finding,localHost:"fixture-mac",read:report) else {
                return XCTFail("Actual Go-coded unknown check lost its original fresh-match identity")
            }
            let changed = MaatDiagnosticCorrelationFixture.reply(["status":"red","findings":[["check":check+"changed","severity":2,"message":"changed inert warning"]]])
            if case .current = maatCurrentDiagnosticResolution(verdict:value.verdict,finding:finding,localHost:"fixture-mac",read:changed) {
                XCTFail("Changed check matched original Go-coded identity")
            }
        }
    }
}
