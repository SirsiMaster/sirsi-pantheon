import SwiftUI
import XCTest
@testable import SirsiMenubar

@MainActor final class MaatWorkspaceRecoveryEntryTests: XCTestCase {
    // No MaatCase is constructed: this is the actual ordinary root body,
    // its always-present entry and callbacks used by its production buttons.
    // External normal-window proof separately checks visible rendering/presses.
    private func frame(engine: SirsiEngine, casebook: MaatCasebookProjection?) throws -> MaatWorkspaceRecoveryFrame {
        let root = MaatWorkspaceView(engine: engine, preloadedCasebook: casebook)
        let productionBody: Any = root.body
        let frame = try XCTUnwrap(productionBody as? MaatWorkspaceRecoveryFrame,
                                 "ordinary Ma'at root omitted its case-independent recovery frame")
        XCTAssertTrue(frame.recovery === engine.maatCaseOperations.preparedPreflightRecovery)
        XCTAssertEqual(frame.localMemory.inspectTitle, "Inspect local memory")
        XCTAssertTrue(frame.localMemory.model === frame.recovery)
        return frame
    }

    private func empty(_ invalid: Bool = false) throws -> MaatCasebookProjection {
        let bytes = Data("""
        {"cases":[],"summary":{"total":0,"open":0,"urgent":0,"high":0,"resolved":0},
        "journal_integrity":{"invalid_count":\(invalid ? 1 : 0),"issues":[]}}
        """.utf8)
        return try JSONDecoder().decode(MaatCasebookProjection.self, from: bytes)
    }

    private func wire() throws -> (prepared: Data, historical: Data, clean: Data) {
        let uuid = "b3c515a2-4773-4653-86d1-8a5734c2ee45"
        let sha = String(repeating: "a", count: 64)
        let scope = ["component":"pantheon", "profile":"local-macos", "operation":"install-managed-liveness-watch"]
        let binding = ["state":"initialized", "store_uuid":uuid]
        let reference: [String:Any] = ["receipt_sha256":sha,"action_sha256":String(repeating:"b",count:64),"registry_sha256":String(repeating:"c",count:64),"registry_snapshot_version":2,"action":scope,"store_binding":binding]
        let target: [String:Any] = ["schema":"sirsi.maat.prepared-preflight-receipt.v1","reference":reference,"store_device":1,"store_inode":2,"intent_key":String(repeating:"d",count:64),"byte_length":512]
        let receipt: [String:Any] = ["schema":"sirsi.maat.failure-memory.v1","action":scope,"action_manifest_sha256":String(repeating:"b",count:64),"registry_snapshot_sha256":String(repeating:"c",count:64),"registry_snapshot_version":2,"evaluated_guards":NSNull(),"measured_checks":NSNull(),"decision":"pass","incident_keys":NSNull(),"recovery_actions":NSNull(),"recovery_reference":"","evaluated_at_utc":"2026-10-24T00:00:00Z"]
        let read = try JSONSerialization.data(withJSONObject: ["schema":"sirsi.maat.failure-memory-receipt-read.v1","receipt_sha256":sha,"receipt":receipt,"store_binding":binding,"read_at_utc":"2026-10-25T00:00:00Z","freshness":"historical","registry_relation":"unverifiable","action_authorized":false])
        let targetJSON = try JSONSerialization.data(withJSONObject: target)
        let prepared: [String:Any] = ["schema":"sirsi.maat.prepared-preflight-receipt.v1","target":target,"target_json":String(decoding:targetJSON,as:UTF8.self),"read_json":String(decoding:read,as:UTF8.self)]
        return (try JSONSerialization.data(withJSONObject: ["state":"incomplete","journal_readiness":"ready","store_uuid":uuid,"prepared_preflight":prepared]),
                read, try JSONSerialization.data(withJSONObject: ["state":"ready","journal_readiness":"ready","store_uuid":uuid]))
    }

    func testActualWorkspaceRootEntryInspectsEmptyAndUnavailableCasebook() async throws {
        for projection in [try empty(), try empty(true), nil] {
            var calls: [[String]] = []
            let model = MaatPreparedPreflightRecovery { args in
                calls.append(args)
                return .init(data: Data(), exitStatus: 1, timedOut: false, launchError: nil)
            }
            let engine = SirsiEngine(maatCaseOperations: MaatCaseOperationStore(preparedPreflightRecovery: model))
            let entry = try frame(engine: engine, casebook: projection).localMemory
            XCTAssertTrue(projection?.cases.isEmpty ?? true)
            XCTAssertTrue(calls.isEmpty, "mount must not submit or mutate")
            XCTAssertNil(model.current)
            XCTAssertNil(model.message, "Inspect must exist before any model state")
            await entry.inspect()
            XCTAssertEqual(calls, [["maat","repair","liveness-watch","--inspect-failure-memory","--json"]])
            XCTAssertNotNil(model.message)
            XCTAssertNil(model.confirmation())
            XCTAssertFalse(model.busy)
        }
    }

    func testActualRootFirstUseReviewUsesSameDiagnosticModelAndFreshReadOnlyInspection()async throws{
        let uninitialized=Data(#"{"state":"uninitialized","journal_readiness":"ready","incomplete_attempt":false,"projection_debt":false}"#.utf8)
        var diagnosticCalls:[[String]]=[]
        let diagnostic=MaatDiagnosticRecordModel(uid:501,localHost:{"fixture-mac"},reader:{args in
            diagnosticCalls.append(args)
            return .init(data:uninitialized,exitStatus:0,timedOut:false,launchError:nil)
        })
        var recoveryCalls:[[String]]=[]
        let recovery=MaatPreparedPreflightRecovery{args in
            recoveryCalls.append(args)
            return .init(data:uninitialized,exitStatus:0,timedOut:false,launchError:nil)
        }
        let operations=MaatCaseOperationStore(preparedPreflightRecovery:recovery,diagnosticRecord:diagnostic)
        let engine=SirsiEngine(maatCaseOperations:operations)
        let root=try frame(engine:engine,casebook:nil)
        XCTAssertTrue(root.localDiagnostics?.model === diagnostic)
        XCTAssertTrue(diagnosticCalls.isEmpty)
        XCTAssertTrue(recoveryCalls.isEmpty,"mount performs no inspection or mutation")
        await root.localMemory.inspect()
        XCTAssertTrue(recovery.canReviewFirstUseInitialization)
        XCTAssertTrue(root.localMemory.model.summary.contains("not created"))
        await root.reviewFirstUseInitialization()
        XCTAssertTrue(root.localDiagnostics?.model === diagnostic)
        XCTAssertEqual(diagnosticCalls,[MaatDiagnosticRecordModel.inspectArguments])
        XCTAssertEqual(recoveryCalls,[MaatDiagnosticRecordModel.inspectArguments])
        XCTAssertNotNil(diagnostic.approval(.initialize),"fresh inspect exposes existing consent review only")
        XCTAssertFalse(diagnosticCalls.contains(maatCasebookFailureMemoryArguments(.initialize)))
        XCTAssertNil(root.localMemory.model.confirmation())
    }

    func testStrictJournalIssueRoutesToExistingIntegrityCardWithoutCasebookLoad()async throws{
        let status=Data(#"{"state":"uninitialized","journal_readiness":"invalid","journal_integrity":{"invalid_count":1,"count_is_lower_bound":true,"issues":[{"line":5001,"digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","reason":"required field is missing"}]}}"#.utf8)
        var diagnosticCalls:[[String]]=[]
        let diagnostic=MaatDiagnosticRecordModel(uid:501,localHost:{"fixture-mac"},reader:{args in
            diagnosticCalls.append(args)
            return .init(data:status,exitStatus:0,timedOut:false,launchError:nil)
        })
        let recovery=MaatPreparedPreflightRecovery{_ in .init(data:status,exitStatus:0,timedOut:false,launchError:nil)}
        let operations=MaatCaseOperationStore(preparedPreflightRecovery:recovery,diagnosticRecord:diagnostic)
        let engine=SirsiEngine(maatCaseOperations:operations)
        let root=try frame(engine:engine,casebook:nil)
        await root.localMemory.inspect()
        let card=try XCTUnwrap(root.localMemory.journalIntegrityCard)
        XCTAssertTrue(card.integrity.countIsLowerBound)
        XCTAssertEqual(card.integrity.invalidCount,1)
        XCTAssertEqual(card.integrity.issues.first?.line,5001)
        XCTAssertFalse(recovery.canReviewFirstUseInitialization)
        await root.reviewFirstUseInitialization()
        XCTAssertTrue(diagnosticCalls.isEmpty,"invalid strict journal routes to preservation review, not initialization consent")
    }

    func testActualRootConfirmationCallbackRecoversOnceThenRefreshesSameInspector() async throws {
        let wire = try wire()
        var calls: [[String]] = []
        let model = MaatPreparedPreflightRecovery { args in
            calls.append(args)
            return .init(data: calls.count == 1 ? wire.prepared : (calls.count == 2 ? wire.historical : wire.clean),
                         exitStatus: 0, timedOut: false, launchError: nil)
        }
        let engine = SirsiEngine(maatCaseOperations: MaatCaseOperationStore(preparedPreflightRecovery: model))
        let entry = try frame(engine: engine, casebook: empty()).localMemory
        await entry.inspect()
        let prepared = try XCTUnwrap(model.current)
        let approval = try XCTUnwrap(entry.recoveryView.model.confirmation())
        XCTAssertEqual(calls.count, 1, "review is not confirmation")
        await entry.recoveryView.recoverConfirmed(approval)
        XCTAssertEqual(calls, [["maat","repair","liveness-watch","--inspect-failure-memory","--json"],
                               prepared.recoveryArguments,
                               ["maat","repair","liveness-watch","--inspect-failure-memory","--json"]])
        XCTAssertEqual(model.recovered?.action_authorized, false)
        XCTAssertTrue(model.message?.contains("fresh normal assessment") == true)
        XCTAssertNil(model.confirmation())
        await entry.recoveryView.recoverConfirmed(approval)
        XCTAssertEqual(calls.count, 3, "consumed approval replayed or refreshed without recovery")
        let remount = try frame(engine: engine, casebook: nil)
        XCTAssertTrue(remount.localMemory.model === model)
        XCTAssertNotNil(remount.localMemory.model.retained?.read)
    }

    func testRootFailedRecoveryKeepsReceiptWithoutRefreshOrReplay() async throws {
        let wire = try wire()
        var calls: [[String]] = []
        let model = MaatPreparedPreflightRecovery { args in
            calls.append(args)
            return .init(data: calls.count == 1 ? wire.prepared : Data(), exitStatus: calls.count == 1 ? 0 : 1,
                         timedOut: false, launchError: nil)
        }
        let engine = SirsiEngine(maatCaseOperations: MaatCaseOperationStore(preparedPreflightRecovery: model))
        let entry = try frame(engine: engine, casebook: nil).localMemory
        await entry.inspect()
        let prepared = try XCTUnwrap(model.retained)
        let approval = try XCTUnwrap(model.confirmation())
        await entry.recoveryView.recoverConfirmed(approval)
        XCTAssertEqual(calls.count, 2, "failed recovery must not refresh into apparent success")
        XCTAssertEqual(model.retained, prepared)
        XCTAssertNil(model.recovered)
        XCTAssertNil(model.confirmation())
        XCTAssertFalse(model.busy)
        XCTAssertEqual(entry.inspectTitle, "Inspect local memory")
    }

    func testRootCancellationJoinsReadWithoutRefreshingOrLosingInspect() async throws {
        let wire = try wire()
        let entered = expectation(description: "root confirmed recovery reader entered")
        var calls = 0
        var joined = false
        let model = MaatPreparedPreflightRecovery { _ in
            calls += 1
            if calls == 1 {
                return .init(data: wire.prepared, exitStatus: 0, timedOut: false, launchError: nil)
            }
            entered.fulfill()
            do { try await Task.sleep(nanoseconds: 2_000_000_000) } catch {}
            joined = true
            return .init(data: wire.historical, exitStatus: 0, timedOut: false, launchError: nil)
        }
        let engine = SirsiEngine(maatCaseOperations: MaatCaseOperationStore(preparedPreflightRecovery: model))
        let entry = try frame(engine: engine, casebook: nil).localMemory
        await entry.inspect()
        let retained = try XCTUnwrap(model.retained)
        let approval = try XCTUnwrap(model.confirmation())
        let operation = Task { @MainActor in await entry.recoveryView.recoverConfirmed(approval) }
        await fulfillment(of: [entered], timeout: 1)
        model.cancelRecovery()
        await operation.value
        XCTAssertTrue(joined)
        XCTAssertEqual(calls, 2)
        XCTAssertFalse(model.busy)
        XCTAssertNil(model.recovered)
        XCTAssertNil(model.confirmation())
        XCTAssertEqual(model.retained, retained)
        XCTAssertEqual(try frame(engine: engine, casebook: nil).localMemory.inspectTitle, "Inspect local memory")
    }
}
