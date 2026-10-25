import XCTest
@testable import SirsiMenubar

final class MaatRetainedPreflightTests: XCTestCase {
    private let receiptSHA = String(repeating: "a", count: 64)
    private let actionSHA = String(repeating: "b", count: 64)
    private let registrySHA = String(repeating: "c", count: 64)

    private func entry() -> MaatCase {
        MaatCase(id: "memory-case", time: "2026-10-24T00:00:00Z", kind: "failure memory preflight",
                 category: "memory", status: "open", priority: .normal, requester: "maat", resource: "pantheon",
                 affected: "local-macos", determination: "pass", assessed: "install-managed-liveness-watch",
                 why: "historical", evidence: "maat-failure-memory:receipt-sha256=\(receiptSHA);action-sha256=\(actionSHA);registry-sha256=\(registrySHA)")
    }
    private func response(relation: String = "same") throws -> Data {
        try JSONSerialization.data(withJSONObject: [
            "schema": "sirsi.maat.failure-memory-receipt-read.v1", "receipt_sha256": receiptSHA,
            "receipt": ["schema": "sirsi.maat.failure-memory.v1", "action": ["component": "pantheon", "profile": "local-macos", "operation": "install-managed-liveness-watch"],
                        "action_manifest_sha256": actionSHA, "registry_snapshot_sha256": registrySHA, "registry_snapshot_version": 2,
                        "evaluated_guards": NSNull(), "measured_checks": NSNull(), "decision": "pass", "incident_keys": NSNull(),
                        "recovery_actions": NSNull(), "recovery_reference": "", "evaluated_at_utc": "2026-10-24T00:00:00Z"],
            "store_binding": ["state": "initialized", "store_uuid": "b3c515a2-4773-4653-86d1-8a5734c2ee45"],
            "read_at_utc": "2026-10-25T00:00:00Z", "freshness": "historical", "registry_relation": relation, "action_authorized": false
        ])
    }
    private func read(_ data: Data, exit: Int32? = 0, timeout: Bool = false) -> SirsiEngine.JSONCommandRead {
        SirsiEngine.JSONCommandRead(data: data, exitStatus: exit, timedOut: timeout, launchError: nil)
    }

    func testCapturedReceiptUsesFixedCanonicalReaderAndNoStoreOperand() throws {
        let identity = try XCTUnwrap(MaatCapturedPreflightIdentity(evidence: entry().evidence))
        XCTAssertEqual(identity.arguments, ["maat", "failure-memory", "receipt", "--sha256", receiptSHA, "--json"])
        for evidence in [entry().evidence + ";extra=x", entry().evidence.uppercased(), "../receipt", "$(touch /tmp/no)"] {
            XCTAssertNil(MaatCapturedPreflightIdentity(evidence: evidence))
        }
    }

    func testHistoricalRecoveryBindsOnlyCapturedDigestAndExplicitConsent() throws {
        let identity = try XCTUnwrap(MaatCapturedPreflightIdentity(evidence: entry().evidence))
        XCTAssertEqual(identity.recoveryArguments, ["maat", "failure-memory", "receipt", "--sha256", receiptSHA, "--json", "--recover", "--confirm"])
        XCTAssertFalse(identity.arguments.contains("--recover"))
        XCTAssertFalse(identity.recoveryArguments.contains("--store"))
        XCTAssertFalse(identity.recoveryArguments.contains { $0.hasPrefix("/") })
    }

    func testFailedHistoricalRecoveryPreservesOriginalDetailAndNeverAuthorizes() throws {
        let original = try response()
        var state = MaatRetainedPreflightReadState()
        state.finish(read(original), entry: entry(), cancelled: false)
        state.finish(read(Data("incomplete retained target".utf8), exit: 1), entry: entry(), cancelled: false)
        XCTAssertEqual(state.canonicalCLIResponse, original)
        XCTAssertEqual(state.receipt?.action_authorized, false)
        XCTAssertNotNil(state.message)
        state.finish(read(try response(relation: "changed")), entry: entry(), cancelled: false)
        XCTAssertEqual(state.receipt?.receipt.evaluated_at_utc, "2026-10-24T00:00:00Z")
        XCTAssertEqual(state.receipt?.freshness, "historical")
        XCTAssertEqual(state.receipt?.action_authorized, false)
    }

    func testHistoricalSameAndChangedReceiptsNeverAuthorizeAction() throws {
        for relation in ["same", "changed", "unverifiable"] {
            let bytes = try response(relation: relation)
            var state = MaatRetainedPreflightReadState()
            state.finish(read(bytes), entry: entry(), cancelled: false)
            XCTAssertNil(state.message)
            XCTAssertEqual(state.canonicalCLIResponse, bytes)
            XCTAssertEqual(state.receipt?.registry_relation, relation)
            XCTAssertEqual(state.receipt?.receipt.decision, "pass")
            XCTAssertEqual(state.receipt?.action_authorized, false)
            XCTAssertEqual(state.receipt?.receipt.evaluated_at_utc, "2026-10-24T00:00:00Z")
        }
    }

    func testMalformedMismatchTimeoutAndCancellationPreserveOriginalReceipt() throws {
        let valid = try response()
        var initial = MaatRetainedPreflightReadState()
        initial.finish(read(valid), entry: entry(), cancelled: false)
        for mode in ["receipt", "action", "registry", "decision", "authority", "schema", "missing-checks", "timeout", "cancel", "nonzero", "truncated"] {
            var state = initial
            var object = try XCTUnwrap(JSONSerialization.jsonObject(with: valid) as? [String: Any])
            var receipt = try XCTUnwrap(object["receipt"] as? [String: Any])
            switch mode {
            case "receipt": object["receipt_sha256"] = String(repeating: "d", count: 64)
            case "action": receipt["action_manifest_sha256"] = String(repeating: "d", count: 64)
            case "registry": receipt["registry_snapshot_sha256"] = String(repeating: "d", count: 64)
            case "decision": receipt["decision"] = "reject"
            case "authority": object["action_authorized"] = true
            case "schema": object["schema"] = "unknown"
            case "missing-checks": receipt.removeValue(forKey: "measured_checks")
            default: break
            }
            object["receipt"] = receipt
            let data: Data
            if mode == "truncated" { data = Data("{".utf8) } else { data = try JSONSerialization.data(withJSONObject: object) }
            state.finish(read(data, exit: mode == "nonzero" ? 1 : 0, timeout: mode == "timeout"), entry: entry(), cancelled: mode == "cancel")
            XCTAssertNotNil(state.message, mode)
            XCTAssertEqual(state.canonicalCLIResponse, valid, mode)
            XCTAssertEqual(state.receipt?.receipt_sha256, receiptSHA, mode)
            XCTAssertEqual(entry().status, "open", mode)
        }
    }

    func testMissingLegacyReceiptStaysUnavailableWithoutReplacingPriorDetail() throws {
        let valid = try response()
        var state = MaatRetainedPreflightReadState()
        state.finish(read(valid), entry: entry(), cancelled: false)
        let missing = try JSONSerialization.data(withJSONObject: ["schema": "sirsi.maat.failure-memory-receipt-read.v1", "receipt_sha256": receiptSHA, "error_code": "historical-receipt-unavailable"])
        state.finish(read(missing, exit: 1), entry: entry(), cancelled: false)
        XCTAssertTrue(state.message?.contains("Historical receipt bytes unavailable") == true)
        XCTAssertEqual(state.canonicalCLIResponse, valid)
        XCTAssertEqual(state.receipt?.receipt_sha256, receiptSHA)
    }

    func testCapturedStoreBindingAndScopeMustMatchHistoricalResponse() throws {
        let data = try response()
        let identity = try XCTUnwrap(MaatCapturedPreflightIdentity(evidence: entry().evidence))
        let decoded = try XCTUnwrap(MaatRetainedPreflightRead.decode(data))
        var row: [String: Any] = ["id": "bound", "kind": "failure memory preflight", "resource": "pantheon", "affected": "local-macos", "assessed": "install-managed-liveness-watch", "determination": "pass", "evidence": entry().evidence,
             "failure_memory_receipt": ["receipt_sha256": receiptSHA, "action_sha256": actionSHA, "registry_sha256": registrySHA, "registry_snapshot_version": 2, "action": ["component": "pantheon", "profile": "local-macos", "operation": "install-managed-liveness-watch"], "store_binding": ["state": "initialized", "store_uuid": "b3c515a2-4773-4653-86d1-8a5734c2ee45"]]]
        let matching = try JSONDecoder().decode(MaatCase.self, from: JSONSerialization.data(withJSONObject: row))
        XCTAssertTrue(identity.matches(decoded, entry: matching))
        var reference = try XCTUnwrap(row["failure_memory_receipt"] as? [String: Any])
        reference["store_binding"] = ["state": "initialized", "store_uuid": "c4d626b3-5884-4764-97e2-9b684d3dff56"]
        row["failure_memory_receipt"] = reference
        let foreign = try JSONDecoder().decode(MaatCase.self, from: JSONSerialization.data(withJSONObject: row))
        XCTAssertFalse(identity.matches(decoded, entry: foreign))
    }

    @MainActor func testReceiptModelCustodyBelongsToExistingEngineOperationLifetime() {
        let operations = MaatCaseOperationStore()
        let firstMount = operations.failureMemoryReceipts
        let pushedAndReturnedMount = operations.failureMemoryReceipts
        XCTAssertTrue(firstMount === pushedAndReturnedMount)
    }
}
