import XCTest
@testable import SirsiMenubar

// Public metadata only. No UserDefaults, engine, filesystem, authorization or
// service calls: each model owns this same in-memory receipt across remounts.
@MainActor final class LegacyRelayFixture {
    var saved: Data?
    var failWrite = false
    var storage: LegacyRelayOperationStorage {
        LegacyRelayOperationStorage(read: { self.saved }, write: {
            if self.failWrite { throw NSError(domain: "FixtureReceiptWrite", code: 1) }
            self.saved = $0
        })
    }
    let scope = URL(fileURLWithPath: "/owned/maintenance")
    let artifact = URL(fileURLWithPath: "/owned/retained/sirsi")
    let digest = String(repeating: "a", count: 64)
    func plan(_ suffix: String = "old-generation") -> [String: Any] {
        ["profile": "root-relay-legacy-maintenance-v1",
         "original": ["label": "ai.sirsi.router.relay", "target": "system/ai.sirsi.router.relay", "pid": 123, "started": suffix],
         "legacy_payload": artifact.path, "sha256": digest]
    }
    func transaction(_ phase: String = "prepared", id: String = String(repeating: "b", count: 32)) -> [String: Any] {
        ["id": id, "revision": 1, "plan": plan(), "phase": phase,
         "preserved_envelopes": 3, "old_outcomes": "unknown_not_certified_drained", "next_action": "Read exact retained transaction"]
    }
    func ticket(_ operation: String = "confirm", generation: Int = 1,
                id: String = String(repeating: "d", count: 32)) -> [String: Any] {
        ["schema": 1, "generation": generation, "operation_id": id, "nonce": String(repeating: "b", count: 32),
         "operation": operation, "plan": plan(), "transaction": String(repeating: "b", count: 32),
         "restore_sha256": operation == "restore-confirm" ? String(repeating: "c", count: 64) : "", "restore_revision": operation == "restore-confirm" ? 1 : 0]
    }
    func bytes(_ value: [String: Any]) throws -> Data { try JSONSerialization.data(withJSONObject: value, options: [.sortedKeys]) }
    func ticketJSON(_ operation: String = "confirm", generation: Int = 1,
                    id: String = String(repeating: "d", count: 32)) throws -> String {
        String(decoding: try bytes(ticket(operation, generation: generation, id: id)), as: UTF8.self)
    }
    func result(_ value: [String: Any], exit: Int32 = 0) throws -> CommandExecution {
        CommandExecution(stdout: try bytes(value), exitCode: exit, childState: .exitConfirmed)
    }
    func preview(_ operation: String = "confirm", generation: Int = 1) throws -> CommandExecution {
        try result(["plan": plan(), "ticket": ticket(operation, generation: generation),
                    "ticket_json": ticketJSON(operation, generation: generation)])
    }
    func status(_ phase: String = "prepared", id: String = String(repeating: "b", count: 32)) throws -> CommandExecution {
        try result(["status": transaction(phase, id: id), "admission": ["state": "terminal"], "ticket_json": ticketJSON()])
    }
    func mutation(_ operation: String, phase: String = "bootstrap_requested", safe: Bool = false, exit: Int32 = 1) throws -> CommandExecution {
        try result(["ticket_json": ticketJSON(operation), "status": transaction(phase), "terminal": true, "safe_for_new_preview": safe], exit: exit)
    }
    func ticketArgs(_ operation: String) throws -> [String] { ["--app-ticket", Data(try ticketJSON(operation).utf8).base64EncodedString()] }
}
