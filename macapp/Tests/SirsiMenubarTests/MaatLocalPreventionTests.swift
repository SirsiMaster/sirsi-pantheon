import XCTest
import Darwin
@testable import SirsiMenubar

final class MaatLocalPreventionTests: XCTestCase {
    private let digest = String(repeating: "a", count: 64)
    private func entry() -> MaatCase {
        MaatCase(id: "local-prevention", time: "2026-10-25T00:00:00Z", kind: "failure memory preflight",
                 category: "memory", status: "open", priority: .normal, requester: "maat", resource: "pantheon",
                 affected: "local-macos", determination: "reject", assessed: "install-managed-liveness-watch",
                 why: "retained local prevention", evidence: "fixture:prevention")
    }
    private func receipt(effect: String = "retire-local-prevention") throws -> [String: Any] {
        var object: [String: Any] = [
            "schema": "sirsi.maat.local-prevention-disposition-receipt.v1", "plan_sha256": digest,
            "incident_key": digest, "occurrence_key": digest, "occurrence_body_sha256": digest,
            "effect": effect, "conclusion": "Owner accepts this known unhealthy occurrence",
            "owner_review_evidence": "diagnostic-finding:sha256=" + digest,
            "owner_acceptance_evidence": "diagnostic-acceptance:sha256=" + digest,
            "projection_evidence": "maat-local-prevention-disposition:" + digest,
            "recorded_at_utc": "2026-10-25T00:00:01Z",
            "health": "historical-known-unhealthy; current-health-unobserved", "action_authorized": false
        ]
        if effect == "retire-local-prevention" { object["event_key"] = digest }
        return object
    }
    private func decode(_ object: [String: Any]) throws -> MaatLocalDispositionReceipt {
        try JSONDecoder().decode(MaatLocalDispositionReceipt.self, from: JSONSerialization.data(withJSONObject: object))
    }

    func testClosedDispositionReceiptSeparatesBothEffectsFromHealthAndFutureAuthority() throws {
        for effect in ["accept-only", "retire-local-prevention"] {
            let row = try decode(receipt(effect: effect))
            XCTAssertTrue(row.matches(sha: digest, key: digest))
            XCTAssertEqual(row.health, "historical-known-unhealthy; current-health-unobserved")
            XCTAssertFalse(row.action_authorized)
            XCTAssertEqual(row.event_key != nil, effect == "retire-local-prevention")
        }
    }

    func testContradictoryForeignOrIncompleteDispositionReceiptsRefuse() throws {
        for mode in ["authority", "health", "effect", "incident", "head", "projection", "schema", "missing-event", "extra-event"] {
            var object = try receipt(effect: mode == "extra-event" ? "accept-only" : "retire-local-prevention")
            switch mode {
            case "authority": object["action_authorized"] = true
            case "health": object["health"] = "healthy"
            case "effect": object["effect"] = "approved"
            case "incident": object["incident_key"] = String(repeating: "b", count: 64)
            case "head": object["occurrence_body_sha256"] = "unknown"
            case "projection": object["projection_evidence"] = "caller:accepted"
            case "schema": object["schema"] = "future"
            case "missing-event": object.removeValue(forKey: "event_key")
            case "extra-event": object["event_key"] = digest
            default: XCTFail("unknown fixture")
            }
            XCTAssertFalse(try decode(object).matches(sha: digest, key: digest), mode)
        }
    }

    @MainActor
    func testCancelledHeldLaunchRefusesAtActualAuthorizationBoundaryAndRemainsRetryable() async {
        let row = entry()
        var held: MaatLocalPreventionLaunch?
        var release = false
        var admitted = 0
        let model = MaatLocalPreventionModel(capture: { request in
            held = request
            // Bounded inert stand-in for the existing pre-Process.run hold seam.
            for _ in 0..<1000 where !release { await Task.yield() }
            if request.authorize() { admitted += 1 }
            return CommandExecution(cancelled: !request.authorize(), childState: .notStarted)
        }, historyRun: { _, _ in
            XCTFail("screen command must not read history after refused launch")
            return SirsiEngine.JSONCommandRead(data: Data(), exitStatus: 1, timedOut: false, launchError: nil)
        })
        model.recordPreventionScreen(row)
        for _ in 0..<100 where held == nil { await Task.yield() }
        XCTAssertEqual(held?.arguments, ["maat", "failure-memory", "preflight", "--component", "pantheon", "--profile", "local-macos", "--operation", "install-managed-liveness-watch", "--confirm", "--json"])
        model.cancel(row, "")
        XCTAssertEqual(held?.authorize(), false)
        release = true
        await model.waitForCurrent(row, "")
        XCTAssertEqual(admitted, 0)
        XCTAssertFalse(model.state(row, "").busy)
        XCTAssertNil(model.state(row, "").nextCase)
        XCTAssertNotNil(model.state(row, "").message)
        // The engine-owned model survives view reconstruction; cancellation
        // does not clear prior authority into a fresh implicit confirmation.
        let retained = model
        XCTAssertEqual(retained.state(row, "").generation, model.state(row, "").generation)
    }

    @MainActor
    func testUnverifiableInspectionNeverCreatesPreviewOrRepairAuthority() async {
        let row = entry()
        var arguments: [[String]] = []
        let model = MaatLocalPreventionModel(capture: { request in
            arguments.append(request.arguments)
            XCTAssertTrue(request.authorize())
            return CommandExecution(stdout: Data("{}".utf8), exitCode: 1, childState: .exitConfirmed)
        }, historyRun: { _, _ in
            XCTFail("unverifiable inspection cannot become a new reviewed action")
            return SirsiEngine.JSONCommandRead(data: Data(), exitStatus: 1, timedOut: false, launchError: nil)
        })
        model.inspect(row, digest)
        await model.waitForCurrent(row, digest)
        XCTAssertEqual(arguments.count, 2)
        XCTAssertEqual(arguments[0], ["maat", "failure-memory", "failure", "--incident-key", digest, "--json"])
        XCTAssertFalse(arguments.joined().contains("install"))
        XCTAssertNil(model.state(row, digest).view)
        XCTAssertNil(model.state(row, digest).preview)
        XCTAssertNil(model.state(row, digest).receipt)
        XCTAssertNotNil(model.state(row, digest).message)
    }
    private func boundEntry() -> MaatCase {
        let reference = MaatPreflightReceiptReference(receipt_sha256: digest, action_sha256: digest, registry_sha256: digest,
            registry_snapshot_version: 2, action: MaatLocalPreventionModel.scope,
            store_binding: MaatPreflightStoreBinding(state: "initialized", store_uuid: "b3c515a2-4773-4653-86d1-8a5734c2ee45"))
        return MaatCase(id: "local-prevention", time: "2026-10-25T00:00:00Z", kind: "failure memory preflight",
            category: "memory", status: "open", priority: .normal, requester: "maat", resource: "pantheon",
            affected: "local-macos", determination: "reject", assessed: "install-managed-liveness-watch",
            why: "retained local prevention", evidence: "maat-failure-memory:receipt-sha256=\(digest);action-sha256=\(digest);registry-sha256=\(digest)",
            failureMemoryReceipt: reference, failureMemoryIncidentKeys: [digest])
    }
    private func failureObject() -> [String: Any] {
        let uuid = "b3c515a2-4773-4653-86d1-8a5734c2ee45"
        let scope = ["component": "pantheon", "profile": "local-macos", "operation": "install-managed-liveness-watch"]
        let completion: [String: Any] = ["schema": "sirsi.maat.liveness-completion.v1", "store_uuid": uuid,
            "attempt_id": uuid, "attempt_sha256": digest, "action_manifest_sha256": digest, "preflight_receipt_sha256": digest,
            "producer": "pantheon.local-managed-watch/v1", "mode": "direct-install", "invocation": "returned-success", "observation": "known-unloaded",
            "host": "inert-fixture", "scope": scope, "owner_uid": geteuid(), "managed_label": "ai.sirsi.liveness-watch",
            "target_plist_path": "/owned/inert/watch.plist", "expected_plist_sha256": digest, "observation_sha256": digest, "outcome_sha256": digest,
            "recorded_at_utc": "2026-10-25T00:00:00Z"]
        let signature = ["namespace": "pantheon", "version": "local-managed-watch-failure/v1", "failure_class": "managed-watch-postcondition",
            "operation": "install-managed-liveness-watch", "component": "pantheon", "invariant": "managed-watch-present-and-loaded-after-installer-success",
            "error_code": "watch-unloaded-after-success"]
        return ["schema": "sirsi.maat.local-managed-watch-failure-read.v1", "store_uuid": uuid,
            "incident": ["schema": "sirsi.maat.failure-memory.v1", "key": digest, "evidence_sha256": digest,
                "guard_id": "maat.active-incident", "guard_version": "v1", "scope": scope, "signature": signature],
            "head": ["key": digest, "body_sha256": digest, "status": "active", "evidence_sha256": digest, "recorded_at_utc": "2026-10-25T00:00:00Z"],
            "events": [], "completion": completion,
            "historical_outcome": ["schema": "sirsi.maat.failure-memory.v1", "attempt_id": uuid, "recorded_at_utc": "2026-10-25T00:00:00Z", "determination": "incomplete"],
            "preflight_receipt_sha256": digest,
            "originating_preflight": ["schema": "sirsi.maat.failure-memory.v1", "action": scope, "action_manifest_sha256": digest,
                "registry_snapshot_sha256": digest, "registry_snapshot_version": 2, "decision": "pass", "recovery_reference": "", "evaluated_at_utc": "2026-10-25T00:00:00Z"],
            "current_fact_reference": "unavailable:historical-read-does-not-observe-current-health", "prevention": "active",
            "health": "historical-known-unhealthy; current-health-unobserved", "dispositions": [], "action_authorized": false]
    }

    func testFailureDecoderBindsInitializedStoreActorClosedProducerAndHistoricalState() throws {
        let row = boundEntry()
        let good = failureObject()
        let read = try JSONDecoder().decode(MaatLocalFailureRead.self, from: JSONSerialization.data(withJSONObject: good))
        XCTAssertTrue(read.matches(entry: row, key: digest))
        for mode in ["store", "producer", "owner", "unknown", "reobserve", "head", "authority", "history"] {
            var bad = good
            var completion = try XCTUnwrap(bad["completion"] as? [String: Any])
            switch mode {
            case "store": bad["store_uuid"] = "5ad6fd92-b555-4f34-b592-e5e18a96d144"
            case "producer": completion["producer"] = "caller:verified"
            case "owner": completion["owner_uid"] = UInt32.max
            case "unknown": completion["observation"] = "unverifiable"
            case "reobserve": completion["mode"] = "reobserve-only"
            case "head": bad["prevention"] = "accepted-not-repaired"
            case "authority": bad["action_authorized"] = true
            case "history": bad["health"] = "healthy"
            default: XCTFail("unknown fixture")
            }
            bad["completion"] = completion
            let value = try JSONDecoder().decode(MaatLocalFailureRead.self, from: JSONSerialization.data(withJSONObject: bad))
            XCTAssertFalse(value.matches(entry: row, key: digest), mode)
        }
    }

    @MainActor
    func testActualInspectionModelRetainsVerifiedFullHistoryAfterLaterReadFailure() async throws {
        let row = boundEntry()
        let bytes = try JSONSerialization.data(withJSONObject: failureObject())
        var available = true
        let model = MaatLocalPreventionModel(capture: { request in
            XCTAssertTrue(request.authorize())
            if request.arguments.contains("pending") {
                return CommandExecution(stdout: Data("{}".utf8), exitCode: 1, childState: .exitConfirmed)
            }
            return CommandExecution(stdout: available ? bytes : Data("broken".utf8), exitCode: available ? 0 : 1, childState: .exitConfirmed)
        }, historyRun: { _, _ in
            XCTFail("read-only historical inspection must not launch repair or rescreen")
            return SirsiEngine.JSONCommandRead(data: Data(), exitStatus: 1, timedOut: false, launchError: nil)
        })
        model.inspect(row, digest)
        await model.waitForCurrent(row, digest)
        XCTAssertEqual(model.state(row, digest).view?.head.key, digest)
        XCTAssertEqual(model.state(row, digest).view?.originating_preflight.decision, "pass")
        available = false
        model.inspect(row, digest)
        await model.waitForCurrent(row, digest)
        XCTAssertEqual(model.state(row, digest).view?.head.key, digest)
        XCTAssertEqual(model.state(row, digest).view?.health, "historical-known-unhealthy; current-health-unobserved")
        XCTAssertNotNil(model.state(row, digest).message)
        XCTAssertNil(model.state(row, digest).preview)
    }

    private func exactHistory() throws -> Data {
        let row = boundEntry()
        let scope = ["component": "pantheon", "profile": "local-macos", "operation": "install-managed-liveness-watch"]
        let body: [String: Any] = ["id": row.id, "time": row.time, "kind": row.kind, "category": row.category,
            "status": row.status, "priority": "normal", "requester": row.requester, "resource": row.resource,
            "affected": row.affected, "determination": row.determination, "assessed": row.assessed, "why": row.why, "evidence": row.evidence,
            "failure_memory_incident_keys": [digest], "failure_memory_receipt": ["receipt_sha256": digest, "action_sha256": digest,
                "registry_sha256": digest, "registry_snapshot_version": 2, "action": scope,
                "store_binding": ["state": "initialized", "store_uuid": "b3c515a2-4773-4653-86d1-8a5734c2ee45"]]]
        return try JSONSerialization.data(withJSONObject: ["cases": [body], "graph": ["nodes": [], "edges": []],
            "summary": ["total": 1, "open": 1, "resolved": 0, "urgent": 0, "high": 0],
            "journal_integrity": ["invalid_count": 0, "issues": []],
            "history": ["scope": "all", "query_id": "query:local-owner", "snapshot": "sha256:local-owner",
                "returned": 1, "candidate_matches": 1, "count_scope": "query", "evidence_matches": 1,
                "evidence_matches_complete": true, "candidate_matches_complete": true, "page_size": 50,
                "coverage": "complete", "has_more": false, "has_more_known": true, "total_complete": true, "total": 1]])
    }

    @MainActor
    func testConfirmedModelUsesExactHistoryPlanConsentReceiptAndPreservesNoRepairMeaning() async throws {
        let row = boundEntry(), key = digest
        let history = try exactHistory()
        let evidence = row.evidence
        let conclusion = "Owner accepts this known unhealthy occurrence"
        let plan: [String: Any] = ["schema": "sirsi.maat.local-prevention-disposition-plan.v1",
            "store_uuid": "b3c515a2-4773-4653-86d1-8a5734c2ee45", "incident_key": key,
            "occurrence_key": key, "occurrence_body_sha256": key,
            "scope": ["component": "pantheon", "profile": "local-macos", "operation": "install-managed-liveness-watch"],
            "failure_evidence_sha256": key, "preflight_receipt_sha256": key,
            "owner_review_evidence": "diagnostic-finding:sha256=" + key, "effect": "retire-local-prevention",
            "conclusion": conclusion, "current_fact_reference": "unavailable:historical-read-does-not-observe-current-health",
            "reviewed_at_utc": "2026-10-25T00:00:00Z"]
        // Capture immutable fixture bytes before any asynchronous closure.
        let preview = try JSONSerialization.data(withJSONObject: ["schema": "sirsi.maat.local-prevention-disposition-preview.v1",
            "plan": plan, "plan_sha256": key, "can_commit": true, "constraints": [], "confirmation": "Accept — not repaired"])
        let ownerReceipt = try JSONSerialization.data(withJSONObject: receipt())
        let active = try JSONSerialization.data(withJSONObject: failureObject())
        var historical = failureObject()
        historical["head"] = ["key": key, "body_sha256": key, "status": "retired", "evidence_sha256": key, "recorded_at_utc": "2026-10-25T00:00:01Z"]
        historical["prevention"] = "accepted-not-repaired"
        let retired = try JSONSerialization.data(withJSONObject: historical)
        let pending = try JSONSerialization.data(withJSONObject: ["schema": "sirsi.maat.local-prevention-recovery-inspection.v1",
            "store_uuid": "b3c515a2-4773-4653-86d1-8a5734c2ee45", "incident_key": key, "registry_state": "verified", "offers": [], "action_authorized": false])
        var commands: [[String]] = [], committed = false
        let model = MaatLocalPreventionModel(capture: { request in
            XCTAssertTrue(request.authorize())
            commands.append(request.arguments)
            let args = request.arguments
            let bytes: Data
            if args.contains("preview") { bytes = preview }
            else if args.contains("commit") { committed = true; bytes = ownerReceipt }
            else if args.contains("pending") { bytes = pending }
            else { bytes = committed ? retired : active }
            return CommandExecution(stdout: bytes, exitCode: 0, childState: .exitConfirmed)
        }, historyRun: { args, _ in
            XCTAssertTrue(args.contains(evidence))
            return SirsiEngine.JSONCommandRead(data: history, exitStatus: 0, timedOut: false, launchError: nil)
        })
        model.inspect(row, key)
        await model.waitForCurrent(row, key)
        model.preview(row, key, effect: .lift, conclusion: conclusion)
        await model.waitForCurrent(row, key)
        XCTAssertEqual(model.state(row, key).preview?.plan_sha256, key)
        model.commit(row, key, expectedSHA: String(repeating: "f", count: 64))
        XCTAssertFalse(committed)
        model.commit(row, key, expectedSHA: key)
        await model.waitForCurrent(row, key)
        XCTAssertTrue(committed)
        XCTAssertEqual(commands.filter { $0.contains("commit") }, [["maat", "failure-memory", "disposition", "commit", "--plan-sha256", key, "--confirm", "--json"]])
        XCTAssertEqual(model.state(row, key).receipt?.health, "historical-known-unhealthy; current-health-unobserved")
        XCTAssertEqual(model.state(row, key).view?.prevention, "accepted-not-repaired")
        XCTAssertNil(model.state(row, key).pendingPlanSHA)
        let count = commands.count
        model.commit(row, key, expectedSHA: key)
        await model.waitForCurrent(row, key)
        XCTAssertEqual(commands.count, count)
        XCTAssertFalse(commands.joined().contains("install"))
    }

}
