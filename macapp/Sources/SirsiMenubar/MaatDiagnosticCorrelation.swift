import Foundation

// Closed projections of the canonical Go request/record DTOs. A request UUID
// correlates an invocation; it never substitutes for a record digest or receipt.
enum MaatDiagnosticWire {
    static let requestSchema = "sirsi.maat.diagnostic-request.v1"
    static let recordSchema = "sirsi.maat.diagnostic-record.v1"
    static func uuid(_ value: String) -> Bool { UUID(uuidString: value)?.uuidString.lowercased() == value }
    static func digest(_ value: String) -> Bool { MaatCapturedPreflightIdentity.digest(value) }
    struct Identity: Decodable, Equatable {
        let schema: String; let id: String; let uid: UInt32; let store_uuid: String
        func valid(uid: UInt32, store: String) -> Bool {
            schema == requestSchema && uuid(id) && self.uid == uid && store_uuid == store && uuid(store)
        }
    }
    struct Intent: Decodable {
        struct Manifest: Decodable {
            struct Observation: Decodable {
                let schema: String; let source_sha256: String; let host_sha256: String; let observed_at_utc: String
            }
            let schema: String; let uid: UInt32; let store_uuid: String; let observation: Observation
        }
        let schema: String; let id: String; let manifest: Manifest; let receipt: MaatPreflightReceiptReference
        func valid(uid: UInt32, store: String) -> Bool {
            schema == recordSchema && digest(id) && manifest.schema == recordSchema && manifest.uid == uid &&
            manifest.store_uuid == store && uuid(store) &&
            ["sirsi.maat.local-diagnostic-observation.v1", "sirsi.maat.local-diagnostic-observation.v2"].contains(manifest.observation.schema) &&
            digest(manifest.observation.source_sha256) && digest(manifest.observation.host_sha256) &&
            !manifest.observation.observed_at_utc.isEmpty && manifest.observation.observed_at_utc.utf8.count <= 64 &&
            receipt.store_binding.valid && receipt.store_binding.state == "initialized" && receipt.store_binding.store_uuid == store &&
            receipt.registry_snapshot_version == 2 && receipt.action == MaatPreflightScope(component: "pantheon", profile: "local-macos", operation: "record-system-one-diagnostic") &&
            receipt.action_sha256 == id && digest(receipt.receipt_sha256) && digest(receipt.registry_sha256)
        }
    }
    struct Result: Decodable, Equatable {
        let schema: String; let id: String; let state: String
        let intent_sha256: String; let decision_evidence: String; let verdict_sha256: String
        func valid(intent: Intent) -> Bool {
            guard schema == recordSchema, id == intent.id, digest(intent_sha256) else { return false }
            switch state {
            case "recorded_verified":
                return digest(verdict_sha256) &&
                    decision_evidence == "maat-system-one:sha256=" + verdict_sha256
            case "not_submitted", "not_recorded": return decision_evidence.isEmpty && verdict_sha256.isEmpty
            default: return false
            }
        }
    }
    struct Request: Decodable, Identifiable {
        let request: Identity; let state: String; let intent: Intent?; let result: Result?
        var id: String { request.id }
        var terminal: Bool { ["recorded_verified", "not_submitted", "not_recorded", "fenced_not_recorded"].contains(state) }
        func valid(uid: UInt32, store: String) -> Bool {
            guard request.valid(uid: uid, store: store), ["prepared", "interrupted", "pending", "recorded_verified", "not_submitted", "not_recorded", "fenced_not_recorded"].contains(state) else { return false }
            if let intent, !intent.valid(uid: uid, store: store) { return false }
            if let result { return intent.map { result.valid(intent: $0) && result.state == state } ?? false }
            if ["recorded_verified", "not_submitted", "not_recorded"].contains(state) { return false }
            if state == "pending" { return intent != nil }
            return intent == nil
        }
    }
    struct Record: Decodable, Identifiable {
        let intent: Intent; let state: String; let result: Result?
        var id: String { intent.id }
        func valid(uid: UInt32, store: String) -> Bool {
            guard intent.valid(uid: uid, store: store) else { return false }
            if let result { return result.valid(intent: intent) && result.state == state }
            return state == "pending"
        }
    }
    struct Page: Decodable {
        struct History: Decodable {
            let schema: String; let query_id: String; let snapshot: String; let coverage: String
            let next_cursor: String?; let after_cursor: Bool; let returned: Int; let count_scope: String
        }
        let schema: String; let requests: [Request]; let records: [Record]; let history: History
        func valid(uid: UInt32, store: String, after: Bool) -> Bool {
            let n = requests.count + records.count
            guard schema == requestSchema, history.schema == "sirsi.maat.diagnostic-discovery.v1",
                  digest(history.query_id), digest(history.snapshot), history.after_cursor == after,
                  history.count_scope == "page", history.returned == n, n <= 50,
                  requests.allSatisfy({ $0.valid(uid: uid, store: store) }), records.allSatisfy({ $0.valid(uid: uid, store: store) }) else { return false }
            switch history.coverage {
            case "complete": return history.next_cursor == nil
            case "partial": return n > 0 && !(history.next_cursor?.isEmpty ?? true) && (history.next_cursor?.utf8.count ?? 0) <= 2048
            default: return false
            }
        }
    }
    // Kept independently from discovery/readiness. No output, prompt or raw CLI
    // error is retained in this typed completed-operation receipt.
    struct Completion: Equatable {
        let request: Identity; let state: String; let recordID: String?
        let intentSHA256: String?; let decision: String?; let verdictSHA256: String?
        init(_ value: Request) {
            request = value.request; state = value.state; recordID = value.intent?.id
            intentSHA256 = value.result?.intent_sha256; decision = value.result?.decision_evidence
            verdictSHA256 = value.result?.verdict_sha256
        }
        var details: [String] {
            ["Request: \(request.id)", "Actor UID: \(request.uid)", "Store: \(request.store_uuid)", "State: \(state)",
             "Record: \(recordID ?? "none")", "Intent SHA256: \(intentSHA256 ?? "none")", "Decision: \(decision ?? "none")", "Verdict SHA256: \(verdictSHA256 ?? "none")"]
        }
    }
}
