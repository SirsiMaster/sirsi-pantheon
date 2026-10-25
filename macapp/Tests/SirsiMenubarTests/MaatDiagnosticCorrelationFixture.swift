import Foundation
import XCTest
@testable import SirsiMenubar

// Inert canonical-wire fixtures, not a CLI/store implementation. Every request
// goes through the actual production model reader seam with its actual argv.
@MainActor final class MaatDiagnosticCorrelationFixture {
    let store = "b3c515a2-4773-4653-86d1-8a5734c2ee45"
    let recordID = String(repeating: "a", count: 64)
    var calls: [[String]] = []
    var initialized = true
    var state = "prepared"
    var requestID: String?
    var recordReply: SirsiEngine.JSONCommandRead?
    var readLost = false
    var prepareLost = false
    var onRecord: (@MainActor () async -> Void)?
    var override: (@MainActor ([String]) async -> SirsiEngine.JSONCommandRead?)?
    static func reply(_ object: Any, exit: Int32 = 0) -> SirsiEngine.JSONCommandRead {
        do { return .init(data: try JSONSerialization.data(withJSONObject: object, options: [.sortedKeys]), exitStatus: exit, timedOut: false, launchError: nil) }
        catch { XCTFail("invalid inert JSON fixture: \(error)"); return .init(data: Data(), exitStatus: 1, timedOut: false, launchError: nil) }
    }
    static var lost: SirsiEngine.JSONCommandRead { .init(data: Data(), exitStatus: nil, timedOut: true, launchError: nil) }
    var ready: [String: Any] { initialized ? ["state":"ready", "store_uuid":store] : ["state":"uninitialized"] }
    var verdict: [String: Any] {
        ["feather_weight":1, "gate":"changes", "confidence":1,
         "subject":["kind":"host", "ref":"diagnostic", "head_sha":recordID, "repo":"fixture-mac"],
         "floor":["passed":true,"checks":[]], "model":["provider":"maat-local:deterministic","version":"v1","local":true,"latency_ms":0], "findings":[]]
    }
    var success: [String: Any] {
        var value = verdict
        value["record_state"] = "recorded_verified"; value["record_id"] = recordID
        value["snapshot_evidence"] = "diagnostic:sha256=" + recordID
        value["decision_evidence"] = "maat-system-one:sha256=" + recordID
        return value
    }
    func intent(id: String? = nil) -> [String: Any] {
        let id = id ?? recordID
        return ["schema":MaatDiagnosticWire.recordSchema, "id":id,
                "manifest":["schema":MaatDiagnosticWire.recordSchema, "uid":501,"store_uuid":store,
                            "observation":["schema":"sirsi.maat.local-diagnostic-observation.v2", "source_sha256":id,"host_sha256":id,"observed_at_utc":"2026-10-25T00:00:00Z"]],
                "receipt":["receipt_sha256":id,"action_sha256":id,"registry_sha256":id,"registry_snapshot_version":2,
                           "action":["component":"pantheon","profile":"local-macos","operation":"record-system-one-diagnostic"],
                           "store_binding":["state":"initialized","store_uuid":store]]]
    }
    func result(state: String = "recorded_verified", id: String? = nil) -> [String: Any] {
        let id = id ?? recordID
        return ["schema":MaatDiagnosticWire.recordSchema,"id":id,"state":state,"intent_sha256":id,
                "decision_evidence":state == "recorded_verified" ? "maat-system-one:sha256=" + id : "",
                "verdict_sha256":state == "recorded_verified" ? id : ""]
    }
    func request(_ id: String, state: String = "prepared") -> [String: Any] {
        var row: [String: Any] = ["request":["schema":MaatDiagnosticWire.requestSchema,"id":id,"uid":501,"store_uuid":store],"state":state]
        if ["pending","recorded_verified","not_submitted","not_recorded"].contains(state) { row["intent"] = intent() }
        if ["recorded_verified","not_submitted","not_recorded"].contains(state) { row["result"] = result(state:state) }
        return row
    }
    func page(requests: [[String: Any]] = [], records: [[String: Any]] = [], cursor: String? = nil,
              after: Bool = false, snapshot: String? = nil) -> [String: Any] {
        var history: [String: Any] = ["schema":"sirsi.maat.diagnostic-discovery.v1", "query_id":recordID,
            "snapshot":snapshot ?? recordID,"coverage":cursor == nil ? "complete" : "partial", "after_cursor":after,
            "returned":requests.count + records.count,"count_scope":"page"]
        if let cursor { history["next_cursor"] = cursor }
        return ["schema":MaatDiagnosticWire.requestSchema,"requests":requests,"records":records,"history":history]
    }
    func read(_ args: [String]) async -> SirsiEngine.JSONCommandRead {
        calls.append(args)
        if let value = await override?(args) { return value }
        if args == MaatDiagnosticRecordModel.inspectArguments { return Self.reply(ready) }
        if args.contains("--initialize-failure-memory") {
            initialized = true; return Self.reply(["status":"initialized","store_uuid":store,"imported_incidents":false])
        }
        if args.contains("--inspect-record-requests") { return Self.reply(page(requests:requestID.map { [request($0,state:state)] } ?? [])) }
        for op in ["prepare", "record", "read", "resolve"] {
            let flag = "--" + op + "-record-request"
            guard let index = args.firstIndex(of:flag), args.indices.contains(index+1) else { continue }
            let id = args[index+1]
            switch op {
            case "prepare": requestID = id; state = "prepared"; return prepareLost ? Self.lost : Self.reply(request(id))
            case "record":
                await onRecord?()
                let reply = recordReply ?? Self.reply(success)
                state = reply.exitStatus == 0 ? "recorded_verified" : "pending"
                return reply
            case "read": return readLost ? Self.lost : Self.reply(request(id,state:state))
            default:
                state = ["prepared","interrupted"].contains(state) ? "fenced_not_recorded" : "recorded_verified"
                return Self.reply(request(id,state:state))
            }
        }
        return Self.reply(verdict)
    }
    func model() -> MaatDiagnosticRecordModel { MaatDiagnosticRecordModel(uid:501, localHost:{ "fixture-mac" }, reader:{ await self.read($0) }) }
}
