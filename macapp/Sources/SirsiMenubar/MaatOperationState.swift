import Combine
import Foundation

/// Both browse surfaces publish one state value. Only the current request may
/// commit after suspension; failures retain the latest committed page here.
@MainActor
final class MaatHistoryRefreshStore: ObservableObject {
    @Published private(set) var state: MaatHistoryRefreshState

    typealias Reader = @Sendable ([String]) async -> SirsiEngine.JSONCommandRead
    private let reader: Reader
    init(projection: MaatCasebookProjection? = nil,
         read: @escaping Reader = { await SirsiEngine.runJSONRead(args: $0) }) {
        state = MaatHistoryRefreshState(projection: projection)
        self.reader = read
    }

    // A verified write starts a new snapshot of the committed query; its old
    // cursor is not replayed. load retains the existing generation/cancel fence.
    func refresh(scope: String) async {
        await load(scope: scope, query: state.refreshQuery, run: reader)
    }

    func load(scope: String, query: String? = nil, cursor: String? = nil,
              run: @escaping @Sendable ([String]) async -> SirsiEngine.JSONCommandRead) async {
        guard !Task.isCancelled, let request = state.begin(scope: scope, query: query, cursor: cursor) else { return }
        let read = await run(request.arguments)
        guard state.isCurrent(request) else { return }
        state.complete(read, for: request, cancelled: Task.isCancelled)
    }
}

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
    case historyIncomplete
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

/// Retained only from this reader's successful subprocess response. It is
/// read progress, never permission to dispatch a mutation.
struct MaatExactHistoryContinuation: Sendable {
    let evidence: String
    let queryID: String
    let snapshot: String
    let cursor: String
}

struct MaatExactHistoryRead: Sendable {
    let read: SirsiEngine.JSONCommandRead
    let continuation: MaatExactHistoryContinuation?

    var data: Data { read.data }
    var exitStatus: Int32? { read.exitStatus }
    var timedOut: Bool { read.timedOut }
    var launchError: String? { read.launchError }
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

    static func readCompleteExactHistory(
        evidence: String,
        continuing prior: MaatExactHistoryContinuation? = nil,
        maximumReads: Int = 16,
        run: @escaping @Sendable ([String], Int) async -> SirsiEngine.JSONCommandRead
    ) async -> MaatExactHistoryRead {
        func refusal(_ message: String) -> MaatExactHistoryRead {
            MaatExactHistoryRead(read: SirsiEngine.JSONCommandRead(data: Data(), exitStatus: nil,
                timedOut: false, launchError: message), continuation: nil)
        }
        guard prior.map({ $0.evidence == evidence }) ?? true else {
            return refusal("The selected evidence changed. Start a fresh case review.")
        }
        let deadline = ProcessInfo.processInfo.systemUptime + 60
        var cursor = prior?.cursor
        var expectedQueryID = prior?.queryID
        var expectedSnapshot = prior?.snapshot
        var last: MaatExactHistoryRead?
        for _ in 0..<min(16, max(1, maximumReads)) {
            if Task.isCancelled {
                return refusal("History preflight was cancelled before any action ran.")
            }
            let remaining = deadline - ProcessInfo.processInfo.systemUptime
            guard remaining > 0 else { break }
            let timeout = min(12, max(1, Int(remaining.rounded(.up))))
            let read = await run(maatExactCasebookReadArguments(evidence: evidence, cursor: cursor), timeout)
            if Task.isCancelled {
                return refusal("History preflight was cancelled before any action ran.")
            }
            guard read.exitStatus == 0, !read.timedOut, read.launchError == nil,
                  let projection = decodeProjection(from: read.data),
                  let history = projection.history,
                  history.scope == "all", !history.queryID.isEmpty, !history.snapshot.isEmpty else {
                return MaatExactHistoryRead(read: read, continuation: nil)
            }
            guard expectedQueryID.map({ $0 == history.queryID }) ?? true,
                  expectedSnapshot.map({ $0 == history.snapshot }) ?? true else {
                return refusal("The exact history snapshot changed during continuation. Start a fresh case review.")
            }
            expectedQueryID = history.queryID
            expectedSnapshot = history.snapshot
            if history.coverage == "complete" {
                return MaatExactHistoryRead(read: read, continuation: nil)
            }
            guard history.coverage == "partial",
                  let next = history.nextCursor, !next.isEmpty,
                  history.countScope == "scanned-so-far" else {
                return refusal("The exact history continuation changed or was unavailable. Start a fresh case review.")
            }
            guard next != cursor else {
                return refusal("The exact history read made no progress. Start a fresh case review.")
            }
            cursor = next
            last = MaatExactHistoryRead(read: read, continuation: MaatExactHistoryContinuation(
                evidence: evidence, queryID: history.queryID, snapshot: history.snapshot, cursor: next))
        }
        if Task.isCancelled {
            return refusal("History preflight was cancelled before any action ran.")
        }
        return last ?? refusal("Exact history remains incomplete. Start a fresh case review.")
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
        guard !original.evidence.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else {
            return .rejected("This case has no exact evidence identity. Refresh and review the case before trying again.")
        }
        guard let history = projection.history,
              history.scope == "all",
              history.countScope == "query",
              !history.queryID.isEmpty,
              !history.snapshot.isEmpty,
              history.coverage == "complete",
              history.hasMoreKnown,
              !history.hasMore,
              history.candidateMatchesComplete,
              history.candidateMatches == 1,
              history.evidenceMatchesComplete == true,
              history.evidenceMatches == 1,
              history.totalComplete,
              history.nextCursor == nil,
              history.returned == projection.cases.count,
              history.total == projection.cases.count,
              history.pageSize >= history.returned else {
            return .rejected("The exact case history is incomplete or changed. Refresh and review the case before trying again.")
        }
        guard projection.journalIntegrity.invalidCount == 0 else {
            return .rejected("The decision journal reports invalid records. Review its integrity before taking this action.")
        }
        let evidenceMatches = projection.cases.filter { $0.evidence == original.evidence }
        guard evidenceMatches.count == 1 else {
            return .rejected("This evidence identity is missing or ambiguous in the complete casebook. Review the retained history before taking this action.")
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
    let failureMemoryReceipts: MaatPreflightReceiptReadStore
    let preparedPreflightRecovery: MaatPreparedPreflightRecovery
    let localPreventions: MaatLocalPreventionModel
    let diagnosticRecord: MaatDiagnosticRecordModel
    @Published private(set) var records: [MaatOperationKey: MaatOperationRecord] = [:]
    private var historyReads: [MaatOperationKey: (generation: UUID, continuation: MaatExactHistoryContinuation)] = [:]

    init(preparedPreflightRecovery: MaatPreparedPreflightRecovery? = nil,
         localPreventions: MaatLocalPreventionModel? = nil,
         failureMemoryReceipts: MaatPreflightReceiptReadStore? = nil,
         diagnosticRecord: MaatDiagnosticRecordModel? = nil) {
        self.preparedPreflightRecovery = preparedPreflightRecovery ?? MaatPreparedPreflightRecovery()
        self.localPreventions = localPreventions ?? MaatLocalPreventionModel()
        self.failureMemoryReceipts = failureMemoryReceipts ?? MaatPreflightReceiptReadStore()
        self.diagnosticRecord = diagnosticRecord ?? MaatDiagnosticRecordModel()
    }

    func record(for key: MaatOperationKey) -> MaatOperationRecord? {
        records[key]
    }

    /// Synchronous on MainActor: the first confirmed tap claims this key
    /// before any suspension, so a second tap cannot launch a duplicate.
    @discardableResult
    func begin(_ key: MaatOperationKey) -> Bool {
        if let existing = records[key] {
            if existing.phase == .historyIncomplete {
                var resumed = existing
                resumed.phase = .preflighting
                records[key] = resumed
                return true
            }
            guard existing.phase == .retryReady else { return false }
            historyReads[key] = nil
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
        historyReads[key] = nil
        let prior = records[key]
        records[key] = MaatOperationRecord(phase: .needsReview, receipt: prior?.receipt, guidance: guidance)
    }

    func complete(_ key: MaatOperationKey, receipt: MaatOperationReceipt) {
        historyReads[key] = nil
        records[key] = MaatOperationRecord(phase: .completed, receipt: receipt, guidance: nil)
    }

    /// A completed or rejected action is replayable only after a separate,
    /// successful fresh read has verified the same open case and clean journal.
    func authorizeReviewedRetry(_ key: MaatOperationKey, reviewedGeneration: UUID) {
        guard let prior = records[key], prior.generation == reviewedGeneration,
              !prior.phase.isInFlight else { return }
        historyReads[key] = nil
        records[key] = MaatOperationRecord(
            phase: .retryReady,
            receipt: prior.receipt,
            guidance: "Fresh casebook review succeeded. Confirm again to retry this same case action."
        )
    }

    func rejectRetryReview(_ key: MaatOperationKey, reviewedGeneration: UUID, guidance: String) {
        guard let prior = records[key], prior.generation == reviewedGeneration,
              !prior.phase.isInFlight else { return }
        historyReads[key] = nil
        records[key] = MaatOperationRecord(phase: .needsReview, receipt: prior.receipt, guidance: guidance)
    }

    func historyContinuation(for key: MaatOperationKey, generation: UUID) -> MaatExactHistoryContinuation? {
        guard let saved = historyReads[key], saved.generation == generation else { return nil }
        return saved.continuation
    }

    func hasHistoryContinuation(for key: MaatOperationKey) -> Bool {
        guard let record = records[key] else { return false }
        return historyContinuation(for: key, generation: record.generation) != nil
    }

    func pauseHistory(_ key: MaatOperationKey, generation: UUID,
                      continuation: MaatExactHistoryContinuation, retryReview: Bool = false) {
        guard var record = records[key], record.generation == generation,
              continuation.evidence == key.evidence else { return }
        historyReads[key] = (generation, continuation)
        record.phase = retryReview ? .needsReview : .historyIncomplete
        record.guidance = "The exact history read is incomplete. Continue this review or start fresh; no new action has run."
        records[key] = record
    }

    func restartHistory(_ key: MaatOperationKey) {
        guard let prior = records[key], !prior.phase.isInFlight else { return }
        historyReads[key] = nil
        records[key] = MaatOperationRecord(phase: prior.phase, receipt: prior.receipt,
            guidance: "History progress was discarded. Start a fresh review before taking an action.")
    }
}
