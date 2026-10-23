import Combine
import Foundation

/// The stable identity of one evidence-bound Ma'at action. A changed case or
/// action produces a different key, so a stale detail screen cannot reuse a
/// receipt belonging to newer evidence.
struct MaatOperationKey: Hashable {
    let caseID: String
    let evidence: String
    let status: String
    let kind: String
    let resource: String
    let why: String
    let determination: String
    let actionKind: String
    let actionID: String
    let actionEvidence: String

    init(case entry: MaatCase) {
        caseID = entry.id
        evidence = entry.evidence
        status = entry.status
        kind = entry.kind
        resource = entry.resource
        why = entry.why
        determination = entry.determination
        actionKind = entry.nextAction?.kind ?? ""
        actionID = entry.nextAction?.actionID ?? ""
        actionEvidence = entry.nextAction?.evidence ?? ""
    }

    func matches(_ entry: MaatCase) -> Bool {
        self == MaatOperationKey(case: entry)
    }
}

struct MaatOperationReceipt: Equatable {
    let summary: String
    let succeeded: Bool
}

enum MaatOperationPhase: Equatable {
    case preflighting
    case running
    case needsReview
    case completed
    case retryReady

    var isInFlight: Bool {
        self == .preflighting || self == .running
    }
}

struct MaatOperationRecord: Equatable {
    let generation = UUID()
    var phase: MaatOperationPhase
    var receipt: MaatOperationReceipt?
    var guidance: String?
}

enum MaatCasePreflightDecision: Equatable {
    case ready
    case rejected(String)
}

/// Pure preflight rules shared by the view and deterministic unit tests.
enum MaatOperationPreflight {
    static func decodeProjection(from data: Data) -> MaatCasebookProjection? {
        guard let raw = String(data: data, encoding: .utf8),
              let start = raw.firstIndex(of: "{"),
              let json = String(raw[start...]).data(using: .utf8),
              let root = try? JSONSerialization.jsonObject(with: json) as? [String: Any],
              let integrity = root["journal_integrity"] as? [String: Any],
              let invalidCount = integrity["invalid_count"] as? Int,
              invalidCount >= 0 else { return nil }
        return try? JSONDecoder().decode(MaatCasebookProjection.self, from: json)
    }

    static func evaluate(
        original: MaatCase,
        exitStatus: Int32?,
        timedOut: Bool,
        launchError: String?,
        projection: MaatCasebookProjection?
    ) -> MaatCasePreflightDecision {
        if let launchError {
            return .rejected("The casebook could not be read: \(launchError)")
        }
        if timedOut {
            return .rejected("The casebook read timed out. Refresh and review the case before trying again.")
        }
        guard exitStatus == 0 else {
            return .rejected("The casebook read did not succeed. Refresh and review the case before trying again.")
        }
        guard let projection else {
            return .rejected("The casebook response could not be decoded. Refresh and review the case before trying again.")
        }
        guard projection.journalIntegrity.invalidCount == 0 else {
            return .rejected("The decision journal reports invalid records. Review its integrity before taking this action.")
        }
        let matches = projection.cases.filter { $0.id == original.id }
        guard matches.count == 1 else {
            let message = matches.isEmpty
                ? "This case is no longer in the casebook. Refresh the casebook and review the current record."
                : "The casebook contains multiple records with this case ID. Resolve the ambiguity before taking this action."
            return .rejected(message)
        }
        let latest = matches[0]
        guard latest.status.lowercased() != "resolved" else {
            return .rejected("This case is already resolved. Refresh the casebook and review its latest evidence.")
        }
        guard MaatOperationKey(case: latest) == MaatOperationKey(case: original) else {
            return .rejected("The case evidence or next action changed. Refresh the casebook and review the updated record.")
        }
        return .ready
    }
}

/// Engine-owned storage keeps an invocation and its receipt alive when the
/// workspace navigation tree replaces its detail view.
@MainActor
final class MaatCaseOperationStore: ObservableObject {
    @Published private(set) var records: [MaatOperationKey: MaatOperationRecord] = [:]

    func record(for key: MaatOperationKey) -> MaatOperationRecord? {
        records[key]
    }

    /// Synchronous on MainActor: the first confirmed tap claims this key
    /// before any suspension, so a second tap cannot launch a duplicate.
    @discardableResult
    func begin(_ key: MaatOperationKey) -> Bool {
        if let existing = records[key] {
            guard existing.phase == .retryReady else { return false }
            records[key] = MaatOperationRecord(phase: .preflighting, receipt: existing.receipt, guidance: nil)
            return true
        }
        records[key] = MaatOperationRecord(phase: .preflighting, receipt: nil, guidance: nil)
        return true
    }

    func markRunning(_ key: MaatOperationKey) {
        guard var record = records[key], record.phase == .preflighting else { return }
        record.phase = .running
        record.guidance = nil
        records[key] = record
    }

    func rejectPreflight(_ key: MaatOperationKey, guidance: String) {
        let prior = records[key]
        records[key] = MaatOperationRecord(phase: .needsReview, receipt: prior?.receipt, guidance: guidance)
    }

    func complete(_ key: MaatOperationKey, receipt: MaatOperationReceipt) {
        records[key] = MaatOperationRecord(phase: .completed, receipt: receipt, guidance: nil)
    }

    /// A completed or rejected action is replayable only after a separate,
    /// successful fresh read has verified the same open case and clean journal.
    func authorizeReviewedRetry(_ key: MaatOperationKey, reviewedGeneration: UUID) {
        guard let prior = records[key], prior.generation == reviewedGeneration,
              !prior.phase.isInFlight else { return }
        records[key] = MaatOperationRecord(
            phase: .retryReady,
            receipt: prior.receipt,
            guidance: "Fresh casebook review succeeded. Confirm again to retry this same case action."
        )
    }

    func rejectRetryReview(_ key: MaatOperationKey, reviewedGeneration: UUID, guidance: String) {
        guard let prior = records[key], prior.generation == reviewedGeneration,
              !prior.phase.isInFlight else { return }
        records[key] = MaatOperationRecord(phase: .needsReview, receipt: prior.receipt, guidance: guidance)
    }
}
