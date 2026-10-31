import Combine
import Foundation

private final class MaatHistoryBurstCancellation: @unchecked Sendable {
    private let lock = NSLock()
    private var cancelled = false
    private var finished = false

    @discardableResult
    func cancel() -> Bool {
        lock.lock()
        defer { lock.unlock() }
        guard !finished else { return false }
        cancelled = true
        return true
    }

    func finish() -> Bool {
        lock.lock()
        defer { lock.unlock() }
        finished = true
        return cancelled
    }

    var isCancelled: Bool {
        lock.lock()
        defer { lock.unlock() }
        return cancelled
    }
}

/// Both browse surfaces publish one state value. Only the current request may
/// commit after suspension; failures retain the latest committed page here.
@MainActor
final class MaatHistoryRefreshStore: ObservableObject {
    @Published private(set) var state: MaatHistoryRefreshState

    typealias Reader = @Sendable ([String]) async -> SirsiEngine.JSONCommandRead
    typealias MonotonicNow = @Sendable () -> TimeInterval
    typealias DeadlineSleep = @Sendable (TimeInterval) async throws -> Void
    typealias DeadlineHandled = @Sendable () async -> Void
    typealias CancellationDelivery = @Sendable (@escaping @MainActor @Sendable () -> Void) -> Void
    private let reader: Reader
    private let maximumBurstReads: Int
    private let maximumBurstDuration: TimeInterval
    private let monotonicNow: MonotonicNow
    private let deadlineSleep: DeadlineSleep
    private let deadlineHandled: DeadlineHandled
    private let cancellationDelivery: CancellationDelivery
    private var activeBurstID: UUID?
    private var activeCancellation: MaatHistoryBurstCancellation?
    private var readTask: Task<Void, Never>?
    private var deadlineTask: Task<Void, Never>?

    init(projection: MaatCasebookProjection? = nil,
         maximumBurstReads: Int = 128,
         maximumBurstDuration: TimeInterval = 8,
         monotonicNow: @escaping MonotonicNow = { ProcessInfo.processInfo.systemUptime },
         deadlineSleep: @escaping DeadlineSleep = { seconds in
             let nanoseconds = UInt64(max(0, seconds) * 1_000_000_000)
             try await Task.sleep(nanoseconds: nanoseconds)
         },
         deadlineHandled: @escaping DeadlineHandled = {},
         cancellationDelivery: @escaping CancellationDelivery = { callback in
             _ = Task { @MainActor in callback() }
         },
         read: @escaping Reader = { await SirsiEngine.runJSONRead(args: $0) }) {
        state = MaatHistoryRefreshState(projection: projection)
        self.reader = read
        self.maximumBurstReads = min(128, max(1, maximumBurstReads))
        self.maximumBurstDuration = min(8, max(0.001, maximumBurstDuration))
        self.monotonicNow = monotonicNow
        self.deadlineSleep = deadlineSleep
        self.deadlineHandled = deadlineHandled
        self.cancellationDelivery = cancellationDelivery
    }

    // A verified write starts a new snapshot of the committed query; its old
    // cursor is not replayed. load retains the existing generation/cancel fence.
    func refresh(scope: String) async {
        await load(scope: scope, query: state.refreshQuery, run: reader)
    }

    func load(scope: String, query: String? = nil, cursor: String? = nil,
              run: @escaping @Sendable ([String]) async -> SirsiEngine.JSONCommandRead) async {
        guard !Task.isCancelled else { return }
        cancelActiveBurst()
        guard let request = state.begin(scope: scope, query: query, cursor: cursor) else { return }

        let burstID = UUID()
        let cancellation = MaatHistoryBurstCancellation()
        let started = monotonicNow()
        activeBurstID = burstID
        activeCancellation = cancellation
        let task = Task { [weak self] in
            guard let self else { return }
            await self.runBurst(id: burstID, first: request, started: started,
                                cancellation: cancellation, run: run)
        }
        readTask = task
        let sleepUntilDeadline = deadlineSleep
        let burstDuration = maximumBurstDuration
        let deliverCancellation = cancellationDelivery
        deadlineTask = Task { [weak self] in
            do {
                try await sleepUntilDeadline(burstDuration)
            } catch {
                return
            }
            guard let self else { return }
            await self.deadlineReached(for: burstID)
            await self.deadlineHandled()
        }
        await withTaskCancellationHandler {
            await task.value
        } onCancel: { [weak self] in
            if cancellation.cancel() {
                deliverCancellation { [weak self] in self?.cancelBurst(id: burstID) }
            }
        }
        finishBurst(id: burstID)
    }

    func cancel() {
        guard let activeBurstID else { return }
        cancelBurst(id: activeBurstID)
    }

    private func runBurst(
        id: UUID,
        first: MaatHistoryRefreshRequest,
        started: TimeInterval,
        cancellation: MaatHistoryBurstCancellation,
        run: @escaping Reader
    ) async {
        var request = first
        var seenCursors = Set<String>()
        if let cursor = request.cursor { seenCursors.insert(cursor) }
        var reads = 0

        while activeBurstID == id, state.isCurrent(request), !Task.isCancelled, !cancellation.isCancelled {
            if monotonicNow() - started >= maximumBurstDuration {
                pauseBurst(id: id, failure: .timedOut)
                return
            }
            let read = await run(request.arguments)
            reads += 1
            guard activeBurstID == id, state.isCurrent(request), !Task.isCancelled,
                  !cancellation.isCancelled else { return }
            if monotonicNow() - started >= maximumBurstDuration {
                pauseBurst(id: id, failure: .timedOut)
                return
            }

            guard state.complete(read, for: request, cancelled: false, keepLoading: true,
                                 seenCursors: seenCursors) else { return }
            guard state.failure == nil,
                  let history = state.projection?.history,
                  history.scope == request.scope,
                  history.coverage == "partial" else {
                finishBurst(id: id)
                return
            }
            guard let next = history.nextCursor, !next.isEmpty, seenCursors.insert(next).inserted else {
                state.rejectContinuationForRestart()
                finishBurst(id: id)
                return
            }
            guard reads < maximumBurstReads else {
                pauseBurst(id: id, failure: nil)
                return
            }
            if monotonicNow() - started >= maximumBurstDuration {
                pauseBurst(id: id, failure: .timedOut)
                return
            }
            guard let continuation = state.begin(scope: request.scope, cursor: next) else {
                finishBurst(id: id)
                return
            }
            request = continuation
        }
        finishBurst(id: id)
    }

    private func pauseBurst(id: UUID, failure: MaatHistoryRefreshFailure?) {
        guard activeBurstID == id else { return }
        _ = activeCancellation?.cancel()
        state.pauseContinuation(failure: failure)
        releaseBurst(id: id, cancelRead: true)
    }

    private func deadlineReached(for id: UUID) {
        pauseBurst(id: id, failure: .timedOut)
    }

    private func cancelBurst(id: UUID) {
        pauseBurst(id: id, failure: .cancelled)
    }

    private func cancelActiveBurst() {
        guard let activeBurstID else { return }
        _ = activeCancellation?.cancel()
        releaseBurst(id: activeBurstID, cancelRead: true)
    }

    private func finishBurst(id: UUID, cancelRead: Bool = false) {
        guard activeBurstID == id else { return }
        if activeCancellation?.finish() == true {
            state.pauseContinuation(failure: .cancelled)
        }
        releaseBurst(id: id, cancelRead: cancelRead)
    }

    private func releaseBurst(id: UUID, cancelRead: Bool) {
        guard activeBurstID == id else { return }
        _ = activeCancellation?.finish()
        activeBurstID = nil
        activeCancellation = nil
        deadlineTask?.cancel()
        deadlineTask = nil
        if cancelRead { readTask?.cancel() }
        readTask = nil
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

enum MaatOperationIntent: Equatable {
    case standard
    case binaryDriftUpdater
    case inactiveDisposition
}

enum MaatConfirmedResolutionSelection: Equatable {
    case standard
    case binaryDriftUpdater
    case inactiveDisposition

    init(inactiveDisposition: Bool, actionKind: String, actionID: String) {
        guard actionKind == "maat_repair", actionID == "binary-drift" else {
            self = .standard
            return
        }
        self = inactiveDisposition ? .inactiveDisposition : .binaryDriftUpdater
    }

    var operationIntent: MaatOperationIntent {
        switch self {
        case .standard: return .standard
        case .binaryDriftUpdater: return .binaryDriftUpdater
        case .inactiveDisposition: return .inactiveDisposition
        }
    }
}

struct MaatOperationProgress: Equatable {
    let title: String
    let detail: String
}

struct MaatOperationRecord: Equatable {
    let generation: UUID
    var phase: MaatOperationPhase
    var receipt: MaatOperationReceipt?
    var guidance: String?
    var intent: MaatOperationIntent

    init(phase: MaatOperationPhase, receipt: MaatOperationReceipt?, guidance: String?,
         intent: MaatOperationIntent = .standard) {
        generation = UUID()
        self.phase = phase
        self.receipt = receipt
        self.guidance = guidance
        self.intent = intent
    }

    var progress: MaatOperationProgress? {
        guard phase.isInFlight else { return nil }
        switch intent {
        case .standard:
            return nil
        case .binaryDriftUpdater:
            return MaatOperationProgress(title: "Applying the reviewed updater…",
                detail: "This operation stays locked until the owned updater exits and Ma’at records a fresh verification or an explicit failure. The updater is bounded to two minutes before verification.")
        case .inactiveDisposition:
            if phase == .preflighting {
                return MaatOperationProgress(title: "Confirming the exact case…",
                    detail: "Ma’at is reading retained case history before the fresh binary scan. No updater is run.")
            }
            return MaatOperationProgress(title: "Rechecking the binary finding…",
                detail: "Ma’at is checking current sibling binaries and recording the exact observation. No updater is run.")
        }
    }
}

enum MaatCasePreflightDecision: Equatable {
    case ready
    case invalidIntegrity(MaatJournalIntegrity)
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
    // A completed review may retain only the cursor that opened its terminal
    // page. A later explicit confirmation must fetch that page again; this is
    // never a cached projection or permission to dispatch.
    let retryReadCursor: MaatExactHistoryContinuation?

    init(read: SirsiEngine.JSONCommandRead,
         continuation: MaatExactHistoryContinuation?,
         retryReadCursor: MaatExactHistoryContinuation? = nil) {
        self.read = read
        self.continuation = continuation
        self.retryReadCursor = retryReadCursor
    }

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
                let retryReadCursor = cursor.map {
                    MaatExactHistoryContinuation(evidence: evidence, queryID: history.queryID,
                        snapshot: history.snapshot, cursor: $0)
                }
                return MaatExactHistoryRead(read: read, continuation: nil,
                    retryReadCursor: retryReadCursor)
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
        guard projection.journalIntegrity.invalidCount == 0 else {
            return .invalidIntegrity(projection.journalIntegrity)
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
    private let suppliedLegacyMaintenance: LegacyRelayMaintenanceModel?
    lazy var legacyMaintenance = suppliedLegacyMaintenance ?? LegacyRelayMaintenanceModel(directory: URL(fileURLWithPath: NSHomeDirectory()))
    lazy var appUpdate = MaatAppUpdateModel(memory: diagnosticRecord, recovery: preparedPreflightRecovery, maintenance: legacyMaintenance)
    @Published private(set) var records: [MaatOperationKey: MaatOperationRecord] = [:]
    private var historyReads: [MaatOperationKey: (generation: UUID, continuation: MaatExactHistoryContinuation)] = [:]

    init(preparedPreflightRecovery: MaatPreparedPreflightRecovery? = nil,
         localPreventions: MaatLocalPreventionModel? = nil,
         failureMemoryReceipts: MaatPreflightReceiptReadStore? = nil,
         diagnosticRecord: MaatDiagnosticRecordModel? = nil,
         legacyMaintenance: LegacyRelayMaintenanceModel? = nil) {
        let recoveryOwner = preparedPreflightRecovery ?? MaatPreparedPreflightRecovery()
        let diagnosticOwner = diagnosticRecord ?? MaatDiagnosticRecordModel()
        self.preparedPreflightRecovery = recoveryOwner
        self.localPreventions = localPreventions ?? MaatLocalPreventionModel()
        self.failureMemoryReceipts = failureMemoryReceipts ?? MaatPreflightReceiptReadStore()
        self.diagnosticRecord = diagnosticOwner
        self.suppliedLegacyMaintenance = legacyMaintenance
    }

    func record(for key: MaatOperationKey) -> MaatOperationRecord? {
        records[key]
    }

    /// Synchronous on MainActor: the first confirmed tap claims this key
    /// before any suspension, so a second tap cannot launch a duplicate.
    @discardableResult
    func begin(_ key: MaatOperationKey, intent: MaatOperationIntent = .standard) -> Bool {
        if let existing = records[key] {
            if existing.phase == .historyIncomplete {
                var resumed = existing
                if resumed.intent != intent { historyReads[key] = nil }
                resumed.phase = .preflighting
                resumed.intent = intent
                records[key] = resumed
                return true
            }
            guard existing.phase == .retryReady else { return false }
            let reviewedContinuation = existing.intent == .inactiveDisposition && intent == .inactiveDisposition
                ? historyContinuation(for: key, generation: existing.generation)
                : nil
            let next = MaatOperationRecord(phase: .preflighting, receipt: existing.receipt,
                guidance: nil, intent: intent)
            records[key] = next
            if let reviewedContinuation, reviewedContinuation.evidence == key.evidence {
                historyReads[key] = (next.generation, reviewedContinuation)
            } else {
                historyReads[key] = nil
            }
            return true
        }
        records[key] = MaatOperationRecord(phase: .preflighting, receipt: nil, guidance: nil, intent: intent)
        return true
    }

    func markRunning(_ key: MaatOperationKey) {
        guard var record = records[key], record.phase == .preflighting else { return }
        historyReads[key] = nil
        record.phase = .running
        record.guidance = nil
        records[key] = record
    }

    func rejectPreflight(_ key: MaatOperationKey, guidance: String) {
        historyReads[key] = nil
        let prior = records[key]
        records[key] = MaatOperationRecord(phase: .needsReview, receipt: prior?.receipt, guidance: guidance, intent: prior?.intent ?? .standard)
    }

    func complete(_ key: MaatOperationKey, receipt: MaatOperationReceipt) {
        historyReads[key] = nil
        records[key] = MaatOperationRecord(phase: .completed, receipt: receipt, guidance: nil, intent: records[key]?.intent ?? .standard)
    }

    /// A completed or rejected action is replayable only after a separate,
    /// successful fresh read has verified the same open case and clean journal.
    @discardableResult
    func authorizeReviewedRetry(_ key: MaatOperationKey, reviewedGeneration: UUID,
                                continuation: MaatExactHistoryContinuation? = nil) -> Bool {
        guard let prior = records[key], prior.generation == reviewedGeneration,
              !prior.phase.isInFlight,
              continuation.map({ prior.intent == .inactiveDisposition && isValidRetryContinuation($0, for: key) }) ?? true else { return false }
        historyReads[key] = nil
        let retry = MaatOperationRecord(
            phase: .retryReady,
            receipt: prior.receipt,
            guidance: "Fresh casebook review succeeded. Confirm again to retry this same case action.",
            intent: prior.intent
        )
        records[key] = retry
        if let continuation {
            // This cursor is only a terminal-page read handle for the typed
            // inactive-disposition command. That command independently checks
            // the current strict journal and host before recording, then
            // rechecks under the mutation lock. It is never a general action
            // freshness proof or a cached projection.
            historyReads[key] = (retry.generation, continuation)
        }
        return true
    }

    func rejectRetryReview(_ key: MaatOperationKey, reviewedGeneration: UUID, guidance: String) {
        guard let prior = records[key], prior.generation == reviewedGeneration,
              !prior.phase.isInFlight else { return }
        historyReads[key] = nil
        records[key] = MaatOperationRecord(phase: .needsReview, receipt: prior.receipt, guidance: guidance, intent: prior.intent)
    }

    func historyContinuation(for key: MaatOperationKey, generation: UUID) -> MaatExactHistoryContinuation? {
        guard let saved = historyReads[key], saved.generation == generation,
              isValidRetryContinuation(saved.continuation, for: key) else { return nil }
        return saved.continuation
    }

    func hasHistoryContinuation(for key: MaatOperationKey) -> Bool {
        guard let record = records[key],
              record.phase == .historyIncomplete || record.phase == .needsReview else { return false }
        return historyContinuation(for: key, generation: record.generation) != nil
    }

    func pauseHistory(_ key: MaatOperationKey, generation: UUID,
                      continuation: MaatExactHistoryContinuation, retryReview: Bool = false) {
        guard var record = records[key], record.generation == generation,
              isValidRetryContinuation(continuation, for: key) else { return }
        historyReads[key] = (generation, continuation)
        record.phase = retryReview ? .needsReview : .historyIncomplete
        record.guidance = "The exact history read is incomplete. Continue this review or start fresh; no new action has run."
        records[key] = record
    }

    func restartHistory(_ key: MaatOperationKey) {
        guard let prior = records[key], !prior.phase.isInFlight else { return }
        historyReads[key] = nil
        records[key] = MaatOperationRecord(phase: prior.phase, receipt: prior.receipt,
            guidance: "History progress was discarded. Start a fresh review before taking an action.", intent: prior.intent)
    }

    private func isValidRetryContinuation(_ continuation: MaatExactHistoryContinuation,
                                          for key: MaatOperationKey) -> Bool {
        continuation.evidence == key.evidence && !continuation.queryID.isEmpty &&
            !continuation.snapshot.isEmpty && !continuation.cursor.isEmpty
    }
}
