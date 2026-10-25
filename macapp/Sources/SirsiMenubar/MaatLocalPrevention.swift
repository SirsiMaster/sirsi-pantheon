import Foundation
import SwiftUI
import Darwin

enum MaatLocalPreventionEffect: String, CaseIterable {
    case acceptOnly = "accept-only"
    case lift = "retire-local-prevention"
    var title: String { self == .acceptOnly ? "Accept conclusion only — keep prevention" : "Accept — not repaired; lift this prevention" }
}
struct MaatLocalHead: Decodable {
    let key, body_sha256, status, evidence_sha256, recorded_at_utc: String
}
struct MaatLocalSignature: Decodable {
    let namespace, version, failure_class, operation, component, invariant, error_code: String
}
struct MaatLocalIncident: Decodable {
    let schema, key, evidence_sha256, guard_id, guard_version: String
    let scope: MaatPreflightScope
    let signature: MaatLocalSignature
}
struct MaatLocalCompletion: Decodable {
    let schema, store_uuid, attempt_id, attempt_sha256, action_manifest_sha256, preflight_receipt_sha256: String
    let producer, mode, invocation, observation, host: String
    let scope: MaatPreflightScope
    let owner_uid: UInt32
    let managed_label, target_plist_path, expected_plist_sha256, observation_sha256, outcome_sha256, recorded_at_utc: String
}
struct MaatLocalOutcome: Decodable {
    let schema, attempt_id, recorded_at_utc, determination: String
}
struct MaatLocalEvent: Decodable {
    let schema, event_key, incident_key, predecessor_event_key, predecessor_body_sha256: String
    let kind, status, reason, evidence_sha256, owner_review_evidence, recorded_at_utc: String
}
struct MaatLocalDispositionReceipt: Decodable {
    let schema, plan_sha256, incident_key, occurrence_key, occurrence_body_sha256, effect, conclusion: String
    let owner_review_evidence, owner_acceptance_evidence, projection_evidence, recorded_at_utc, health: String
    let event_key: String?
    let action_authorized: Bool
    func matches(sha: String, key: String) -> Bool {
        schema == "sirsi.maat.local-prevention-disposition-receipt.v1" && plan_sha256 == sha && incident_key == key &&
        MaatCapturedPreflightIdentity.digest(occurrence_key) && MaatCapturedPreflightIdentity.digest(occurrence_body_sha256) &&
        MaatLocalPreventionEffect(rawValue: effect) != nil && !conclusion.isEmpty &&
        owner_review_evidence.hasPrefix("diagnostic-finding:sha256=") && owner_acceptance_evidence.hasPrefix("diagnostic-acceptance:sha256=") &&
        projection_evidence == "maat-local-prevention-disposition:" + sha && !recorded_at_utc.isEmpty &&
        health == "historical-known-unhealthy; current-health-unobserved" && !action_authorized &&
        (effect == MaatLocalPreventionEffect.lift.rawValue ? event_key.map(MaatCapturedPreflightIdentity.digest) == true : event_key == nil)
    }
}
struct MaatLocalFailureRead: Decodable {
    let schema, store_uuid: String
    let incident: MaatLocalIncident
    let head: MaatLocalHead
    let events: [MaatLocalEvent]
    let completion: MaatLocalCompletion
    let historical_outcome: MaatLocalOutcome
    let preflight_receipt_sha256, current_fact_reference, prevention, health: String
    let originating_preflight: MaatRetainedPreflight
    let dispositions: [MaatLocalDispositionReceipt]
    let action_authorized: Bool
    func matches(entry: MaatCase, key: String) -> Bool {
        guard let binding = entry.failureMemoryReceipt?.store_binding,
              binding.state == "initialized", binding.store_uuid == store_uuid, binding.valid else { return false }
        return schema == "sirsi.maat.local-managed-watch-failure-read.v1" && incident.key == key &&
            incident.scope == MaatLocalPreventionModel.scope && incident.guard_id == "maat.active-incident" && incident.guard_version == "v1" &&
            incident.schema == "sirsi.maat.failure-memory.v1" && MaatCapturedPreflightIdentity.digest(incident.evidence_sha256) &&
            incident.signature.namespace == "pantheon" && incident.signature.version == "local-managed-watch-failure/v1" &&
            incident.signature.failure_class == "managed-watch-postcondition" && incident.signature.operation == "install-managed-liveness-watch" &&
            incident.signature.component == "pantheon" && incident.signature.invariant == "managed-watch-present-and-loaded-after-installer-success" &&
            incident.signature.error_code == (completion.observation == "known-absent" ? "watch-absent-after-success" : "watch-unloaded-after-success") &&
            completion.schema == "sirsi.maat.liveness-completion.v1" &&
            UUID(uuidString: completion.attempt_id)?.uuidString.lowercased() == completion.attempt_id &&
            [completion.attempt_sha256, completion.action_manifest_sha256, completion.preflight_receipt_sha256,
             completion.expected_plist_sha256, completion.observation_sha256, completion.outcome_sha256].allSatisfy(MaatCapturedPreflightIdentity.digest) &&
            completion.managed_label == "ai.sirsi.liveness-watch" && completion.target_plist_path.hasPrefix("/") &&
            !completion.host.isEmpty && !completion.recorded_at_utc.isEmpty &&
            completion.producer == "pantheon.local-managed-watch/v1" && completion.mode == "direct-install" && completion.invocation == "returned-success" &&
            ["known-absent", "known-unloaded"].contains(completion.observation) && completion.owner_uid == geteuid() &&
            completion.store_uuid == store_uuid && completion.scope == MaatLocalPreventionModel.scope &&
            completion.preflight_receipt_sha256 == preflight_receipt_sha256 &&
            originating_preflight.schema == "sirsi.maat.failure-memory.v1" && originating_preflight.action == MaatLocalPreventionModel.scope &&
            originating_preflight.action_manifest_sha256 == completion.action_manifest_sha256 && originating_preflight.decision == "pass" &&
            originating_preflight.registry_snapshot_version == 2 && MaatCapturedPreflightIdentity.digest(originating_preflight.registry_snapshot_sha256) &&
            MaatCapturedPreflightIdentity.digest(head.key) && MaatCapturedPreflightIdentity.digest(head.body_sha256) &&
            ["active", "retired"].contains(head.status) && prevention == (head.status == "active" ? "active" : "accepted-not-repaired") &&
            current_fact_reference == "unavailable:historical-read-does-not-observe-current-health" &&
            historical_outcome.schema == "sirsi.maat.failure-memory.v1" && historical_outcome.attempt_id == completion.attempt_id &&
            historical_outcome.determination == "incomplete" && !action_authorized &&
            health == "historical-known-unhealthy; current-health-unobserved"
    }
}
struct MaatLocalDispositionPlan: Decodable {
    let schema, store_uuid, incident_key, occurrence_key, occurrence_body_sha256: String
    let scope: MaatPreflightScope
    let failure_evidence_sha256, preflight_receipt_sha256, owner_review_evidence, effect, conclusion, current_fact_reference, reviewed_at_utc: String
}
struct MaatLocalDispositionPreview: Decodable {
    let schema: String
    let plan: MaatLocalDispositionPlan
    let plan_sha256: String
    let can_commit: Bool
    let constraints: [String]
    let confirmation: String
    func matches(entry: MaatCase, key: String, view: MaatLocalFailureRead, effect: MaatLocalPreventionEffect, conclusion: String) -> Bool {
        schema == "sirsi.maat.local-prevention-disposition-preview.v1" && MaatCapturedPreflightIdentity.digest(plan_sha256) &&
        plan.schema == "sirsi.maat.local-prevention-disposition-plan.v1" && plan.store_uuid == view.store_uuid &&
        plan.incident_key == key && plan.occurrence_key == view.head.key && plan.occurrence_body_sha256 == view.head.body_sha256 &&
        plan.scope == MaatLocalPreventionModel.scope && plan.failure_evidence_sha256 == view.head.evidence_sha256 &&
        plan.preflight_receipt_sha256 == MaatCapturedPreflightIdentity(evidence: entry.evidence)?.receipt &&
        plan.effect == effect.rawValue && plan.conclusion == conclusion && !plan.reviewed_at_utc.isEmpty && !confirmation.isEmpty &&
        !(effect == .lift && can_commit && !constraints.isEmpty)
    }
}
struct MaatLocalRecoveryOffer: Decodable { let plan_sha256, incident_key, effect, state: String; let plan: MaatLocalDispositionPlan }
struct MaatLocalRecoveryInspection: Decodable {
    let schema, store_uuid, incident_key, registry_state: String
    let offers: [MaatLocalRecoveryOffer]
    let action_authorized: Bool
}
struct MaatLocalPreventionLaunch {
    let arguments: [String]
    let authorize: @MainActor () -> Bool
}
struct MaatLocalPreventionState {
    var generation = UUID()
    var busy = false
    var message: String?
    var view: MaatLocalFailureRead?
    var preview: MaatLocalDispositionPreview?
    var receipt: MaatLocalDispositionReceipt?
    var pendingPlanSHA: String?
    var pendingPlan: MaatLocalDispositionPlan?
    var offers: [MaatLocalRecoveryOffer] = []
    var execution: CommandExecution?
    var continuation: MaatExactHistoryContinuation?
    var nextCase: MaatCase?
    var nextEvidence: String?
    var nextContinuation: MaatExactHistoryContinuation?
}

@MainActor final class MaatLocalPreventionModel: ObservableObject {
    nonisolated static var scope: MaatPreflightScope { MaatPreflightScope(component: "pantheon", profile: "local-macos", operation: "install-managed-liveness-watch") }
    typealias Capture = @MainActor (MaatLocalPreventionLaunch) async -> CommandExecution
    typealias HistoryRun = @Sendable ([String], Int) async -> SirsiEngine.JSONCommandRead
    @Published private(set) var states: [String: MaatLocalPreventionState] = [:]
    private var tasks: [String: Task<Void, Never>] = [:]
    private var taskIDs: [String: UUID] = [:]
    private let capture: Capture
    private let historyRun: HistoryRun
    init(capture: @escaping Capture = { request in await SirsiEngine.runCaptured(args: request.arguments, timeout: 30, authorize: request.authorize) },
         historyRun: @escaping HistoryRun = { args, timeout in await SirsiEngine.runJSONRead(args: args, timeoutSeconds: timeout) }) {
        self.capture = capture; self.historyRun = historyRun
    }
    private func id(_ entry: MaatCase, _ key: String) -> String { entry.id + "|" + entry.evidence + "|" + key }
    func state(_ entry: MaatCase, _ key: String) -> MaatLocalPreventionState { states[id(entry, key)] ?? MaatLocalPreventionState() }
    func cancel(_ entry: MaatCase, _ key: String) {
        let id = id(entry, key); tasks[id]?.cancel()
        var s = state(entry, key); s.generation = UUID(); s.preview = nil
        s.message = s.pendingPlanSHA == nil ? "Inspection cancelled; prior detail is preserved." : "Cancellation requested. The retained consent may be incomplete; inspect or reconcile it. No installer runs in this flow."
        states[id] = s
    }
    func invalidate(_ entry: MaatCase, _ key: String) {
        if tasks[id(entry, key)] != nil { cancel(entry, key) } else { update(entry, key) { $0.generation = UUID(); $0.preview = nil; $0.message = nil } }
    }
    private func update(_ entry: MaatCase, _ key: String, _ mutate: (inout MaatLocalPreventionState) -> Void) {
        var s = state(entry, key); mutate(&s); states[id(entry, key)] = s
    }
    private func launch(_ entry: MaatCase, _ key: String, operation: @escaping @MainActor (UUID) async -> Void) {
        let id = id(entry, key); guard tasks[id] == nil else { return }
        let generation = UUID(); taskIDs[id] = generation
        update(entry, key) { $0.generation = generation; $0.busy = true; $0.message = nil }
        tasks[id] = Task {
            defer { if self.taskIDs[id] == generation { self.tasks[id] = nil; self.taskIDs[id] = nil; self.update(entry, key) { $0.busy = false } } }
            await operation(generation)
        }
    }
    func waitForCurrent(_ entry: MaatCase, _ key: String) async { await tasks[id(entry, key)]?.value }
    private func current(_ entry: MaatCase, _ key: String, _ generation: UUID) -> Bool { state(entry, key).generation == generation && !Task.isCancelled }
    private func run(_ args: [String], _ entry: MaatCase, _ key: String, _ generation: UUID, plan: String? = nil) async -> CommandExecution {
        await capture(MaatLocalPreventionLaunch(arguments: args, authorize: {
            self.current(entry, key, generation) && (plan == nil || self.state(entry, key).pendingPlanSHA == plan)
        }))
    }
    func inspect(_ entry: MaatCase, _ key: String) {
        guard MaatCapturedPreflightIdentity.digest(key) else { return }
        launch(entry, key) { generation in
            await self.readFailure(entry, key, generation)
            let read = await self.run(["maat", "failure-memory", "disposition", "pending", "--incident-key", key, "--json"], entry, key, generation)
            guard self.current(entry, key, generation) else { return }
            if read.complete, read.exitCode == 0, let offers = try? JSONDecoder().decode(MaatLocalRecoveryInspection.self, from: read.stdoutData),
               offers.schema == "sirsi.maat.local-prevention-recovery-inspection.v1", offers.incident_key == key,
               offers.store_uuid == entry.failureMemoryReceipt?.store_binding.store_uuid, !offers.action_authorized,
               ["verified", "unverifiable"].contains(offers.registry_state),
               offers.offers.allSatisfy({ $0.incident_key == key && MaatCapturedPreflightIdentity.digest($0.plan_sha256) && MaatLocalPreventionEffect(rawValue: $0.effect) != nil && $0.plan.incident_key == key && $0.plan.store_uuid == offers.store_uuid && $0.plan.scope == Self.scope && $0.plan.effect == $0.effect && $0.plan.schema == "sirsi.maat.local-prevention-disposition-plan.v1" && MaatCapturedPreflightIdentity.digest($0.plan.occurrence_key) && MaatCapturedPreflightIdentity.digest($0.plan.occurrence_body_sha256) }) {
                self.update(entry, key) { $0.offers = offers.offers }
            }
        }
    }
    private func readFailure(_ entry: MaatCase, _ key: String, _ generation: UUID) async {
        let read = await run(["maat", "failure-memory", "failure", "--incident-key", key, "--json"], entry, key, generation)
        guard current(entry, key, generation) else { return }
        guard read.complete, read.exitCode == 0, let view = try? JSONDecoder().decode(MaatLocalFailureRead.self, from: read.stdoutData), view.matches(entry: entry, key: key) else {
            update(entry, key) { $0.execution = read; $0.message = "The exact local occurrence could not be verified. Prior detail is preserved; inspect retained recovery references or retry. No prevention was lifted." }; return
        }
        update(entry, key) { $0.view = view; $0.execution = read }
    }
    // This separate effect may remain available after an ordinary Casebook
    // conclusion. Resolved presentation is not a memory transition. Exact
    // underlying receipt/store/scope and complete history still must match.
    nonisolated static func exactLocalHistory(_ original: MaatCase, key: String, _ read: MaatExactHistoryRead) -> Bool {
        guard read.continuation == nil, read.read.exitStatus == 0, !read.read.timedOut, read.read.launchError == nil,
              let p = MaatOperationPreflight.decodeProjection(from: read.read.data), let h = p.history,
              p.journalIntegrity.invalidCount == 0, h.scope == "all", h.countScope == "query", h.coverage == "complete",
              h.hasMoreKnown, !h.hasMore, h.candidateMatchesComplete, h.candidateMatches == 1,
              h.evidenceMatchesComplete == true, h.evidenceMatches == 1, h.totalComplete,
              h.nextCursor == nil, h.returned == 1, h.total == 1, h.pageSize >= h.returned, p.cases.count == 1,
              !h.queryID.isEmpty, !h.snapshot.isEmpty, let latest = p.cases.first,
              original.kind == "failure memory preflight", latest.id == original.id, latest.evidence == original.evidence, latest.kind == original.kind,
              latest.resource == original.resource, latest.affected == original.affected, latest.assessed == original.assessed,
              latest.determination == original.determination, latest.why == original.why, latest.time == original.time, latest.requester == original.requester,
              latest.failureMemoryIncidentKeys == original.failureMemoryIncidentKeys,
              latest.failureMemoryReceipt == original.failureMemoryReceipt else { return false }
        return latest.failureMemoryReceipt?.action == scope && MaatCapturedPreflightIdentity.digest(key) && (latest.failureMemoryIncidentKeys.isEmpty || latest.failureMemoryIncidentKeys.contains(key))
    }
    private func history(_ entry: MaatCase, _ key: String, _ generation: UUID) async -> Bool {
        let prior = state(entry, key).continuation
        let runner = historyRun
        let read = await MaatOperationPreflight.readCompleteExactHistory(evidence: entry.evidence, continuing: prior, run: runner)
        guard current(entry, key, generation) else { return false }
        update(entry, key) { $0.continuation = read.continuation }
        guard Self.exactLocalHistory(entry, key: key, read) else {
            update(entry, key) { $0.message = read.continuation == nil ? "The exact originating case changed or is unverifiable; refresh its retained history before a new conclusion." : "Exact history review is incomplete. Retry the same review to continue; no disposition ran." }; return false
        }
        return true
    }
    func preview(_ entry: MaatCase, _ key: String, effect: MaatLocalPreventionEffect, conclusion: String) {
        let conclusion = conclusion.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !conclusion.isEmpty, conclusion.utf8.count <= 4096, !conclusion.contains("\0"), state(entry, key).pendingPlanSHA == nil,
              let view = state(entry, key).view, view.head.status == "active" else { return }
        launch(entry, key) { generation in
            guard await self.history(entry, key, generation) else { return }
            let args = ["maat", "failure-memory", "disposition", "preview", "--incident-key", key, "--occurrence-key", view.head.key,
                        "--body-sha256", view.head.body_sha256, "--case-evidence", entry.evidence, "--effect", effect.rawValue, "--conclusion", conclusion, "--json"]
            let read = await self.run(args, entry, key, generation)
            guard self.current(entry, key, generation) else { return }
            guard read.complete, let preview = try? JSONDecoder().decode(MaatLocalDispositionPreview.self, from: read.stdoutData),
                  preview.matches(entry: entry, key: key, view: view, effect: effect, conclusion: conclusion),
                  (read.exitCode == 0 || (!preview.can_commit && read.exitCode != 0)) else {
                self.update(entry, key) { $0.execution = read; $0.message = "Exact disposition preview failed. Prior receipt and prevention remain unchanged." }; return
            }
            self.update(entry, key) { $0.preview = preview; $0.execution = read }
        }
    }
    func commit(_ entry: MaatCase, _ key: String, expectedSHA: String) {
        guard let preview = state(entry, key).preview, preview.can_commit, preview.plan_sha256 == expectedSHA, state(entry, key).pendingPlanSHA == nil else { return }
        launch(entry, key) { generation in
            guard await self.history(entry, key, generation) else { return }
            guard self.current(entry, key, generation), self.state(entry, key).preview?.plan_sha256 == expectedSHA else { return }
            self.update(entry, key) { $0.pendingPlanSHA = expectedSHA; $0.pendingPlan = preview.plan }
            let read = await self.run(["maat", "failure-memory", "disposition", "commit", "--plan-sha256", expectedSHA, "--confirm", "--json"], entry, key, generation, plan: expectedSHA)
            self.acceptDisposition(read, entry, key, generation, sha: expectedSHA, plan: preview.plan)
            if self.current(entry, key, generation), self.state(entry, key).pendingPlanSHA == nil { await self.readFailure(entry, key, generation) }
        }
    }
    func reconcile(_ entry: MaatCase, _ key: String, sha: String) {
        guard MaatCapturedPreflightIdentity.digest(sha), state(entry, key).pendingPlanSHA == sha || state(entry, key).offers.contains(where: { $0.plan_sha256 == sha }) else { return }
        let retainedPlan = state(entry, key).pendingPlanSHA == sha ? state(entry, key).pendingPlan : state(entry, key).offers.first(where: { $0.plan_sha256 == sha })?.plan
        guard let plan = retainedPlan else { return }
        launch(entry, key) { generation in
            self.update(entry, key) { $0.pendingPlanSHA = sha; $0.pendingPlan = plan }
            let read = await self.run(["maat", "failure-memory", "disposition", "reconcile", "--plan-sha256", sha, "--json"], entry, key, generation, plan: sha)
            self.acceptDisposition(read, entry, key, generation, sha: sha, plan: plan)
            if self.current(entry, key, generation), self.state(entry, key).pendingPlanSHA == nil { await self.readFailure(entry, key, generation) }
        }
    }
    private func acceptDisposition(_ read: CommandExecution, _ entry: MaatCase, _ key: String, _ generation: UUID, sha: String, plan: MaatLocalDispositionPlan) {
        guard current(entry, key, generation) else { return }
        guard read.complete, read.exitCode == 0, let receipt = try? JSONDecoder().decode(MaatLocalDispositionReceipt.self, from: read.stdoutData), receipt.matches(sha: sha, key: key), receipt.effect == plan.effect, receipt.conclusion == plan.conclusion, receipt.occurrence_key == plan.occurrence_key, receipt.occurrence_body_sha256 == plan.occurrence_body_sha256, receipt.owner_review_evidence == plan.owner_review_evidence else {
            update(entry, key) { $0.execution = read; $0.message = "The owner disposition is incomplete or unverifiable. Inspect/reconcile this exact retained plan; no installer will replay and no health success was inferred." }; return
        }
        update(entry, key) { $0.receipt = receipt; $0.execution = read; $0.preview = nil; $0.pendingPlanSHA = nil; $0.pendingPlan = nil; $0.offers.removeAll { $0.plan_sha256 == sha }; $0.message = "Accepted — not repaired. Review the exact occurrence receipt; current health has not been inferred." }
    }
    func recordPreventionScreen(_ entry: MaatCase) {
        launch(entry, "") { generation in
            if let evidence = self.state(entry, "").nextEvidence, self.state(entry, "").nextCase == nil { await self.readNextCase(evidence, entry, "", generation); return }
            let args = ["maat", "failure-memory", "preflight", "--component", "pantheon", "--profile", "local-macos", "--operation", "install-managed-liveness-watch", "--confirm", "--json"]
            let read = await self.run(args, entry, "", generation)
            struct Recorded: Decodable { let decision_evidence: String }
            guard self.current(entry, "", generation) else { return }
            guard read.complete, read.exitCode == 0, let row = try? JSONDecoder().decode(Recorded.self, from: read.stdoutData), MaatCapturedPreflightIdentity(evidence: row.decision_evidence) != nil else {
                self.update(entry, "") { $0.execution = read; $0.message = "The exact local prevention screen could not be retained. Inspect local failure-memory state; no repair ran." }; return
            }
            await self.readNextCase(row.decision_evidence, entry, "", generation)
        }
    }
    func reviewNewAction(_ entry: MaatCase, _ key: String) {
        launch(entry, key) { generation in
            if let evidence = self.state(entry, key).nextEvidence, self.state(entry, key).nextCase == nil { await self.readNextCase(evidence, entry, key, generation); return }
            let read = await self.run(["liveness-watch", "run", "--local"], entry, key, generation)
            struct Observed: Decodable { let decision_evidence: String; let local_record: String }
            guard self.current(entry, key, generation) else { return }
            guard read.complete, read.exitCode == 0, let row = try? JSONDecoder().decode(Observed.self, from: read.stdoutData), row.local_record == "recorded", row.decision_evidence.hasPrefix("maat-system-one:sha256=") else {
                self.update(entry, key) { $0.execution = read; $0.message = "Fresh local observation failed. The historical owner disposition remains separate; no installer ran." }; return
            }
            await self.readNextCase(row.decision_evidence, entry, key, generation)
        }
    }
    private func readNextCase(_ evidence: String, _ entry: MaatCase, _ key: String, _ generation: UUID) async {
        let prior = state(entry, key).nextEvidence == evidence ? state(entry, key).nextContinuation : nil
        update(entry, key) { $0.nextEvidence = evidence }
        let runner = historyRun
        let read = await MaatOperationPreflight.readCompleteExactHistory(evidence: evidence, continuing: prior, run: runner)
        guard current(entry, key, generation) else { return }
        update(entry, key) { $0.nextContinuation = read.continuation }
        guard read.continuation == nil, read.read.exitStatus == 0, !read.read.timedOut, read.read.launchError == nil,
              let p = MaatOperationPreflight.decodeProjection(from: read.read.data), p.journalIntegrity.invalidCount == 0, let h = p.history,
              h.scope == "all", h.countScope == "query", h.coverage == "complete", h.hasMoreKnown, !h.hasMore,
              h.candidateMatchesComplete, h.candidateMatches == 1, h.evidenceMatchesComplete == true, h.evidenceMatches == 1,
              h.totalComplete, h.total == 1, h.returned == 1, h.nextCursor == nil, h.pageSize >= h.returned,
              !h.queryID.isEmpty, !h.snapshot.isEmpty, p.cases.count == 1, let next = p.cases.first, next.evidence == evidence else {
            update(entry, key) { $0.message = "The new factual case was recorded but its complete history could not be re-read. Retry inspection; no action was authorized." }; return
        }
        update(entry, key) { $0.nextCase = next; $0.message = "A fresh factual case is ready. Any repair still requires its own review and new confirmation." }
    }
}

struct MaatLocalPreventionCollection: View {
    @ObservedObject var model: MaatLocalPreventionModel
    @ObservedObject var receiptReads: MaatPreflightReceiptReadStore
    let entry: MaatCase
    let destination: (MaatCase) -> AnyView
    var body: some View {
        let keys = entry.failureMemoryIncidentKeys.isEmpty ? receiptReads.state(entry).receipt?.receipt.incident_keys ?? [] : entry.failureMemoryIncidentKeys
        VStack(alignment: .leading, spacing: 12) {
            ForEach(keys.filter(MaatCapturedPreflightIdentity.digest), id: \.self) { key in
                MaatLocalPreventionDetail(model: model, entry: entry, key: key, destination: destination)
            }
        }
    }
}
struct MaatLocalPreventionEntry: View {
    @ObservedObject var model: MaatLocalPreventionModel
    let entry: MaatCase
    let destination: (MaatCase) -> AnyView
    @State private var confirmScreen = false
    var body: some View {
        let state = model.state(entry, "")
        VStack(alignment: .leading, spacing: 8) {
            Button("Review local recurrence prevention") { confirmScreen = true }.disabled(state.busy)
            Text("Record an exact local prevention screen, without repairing or changing health. Retained failure and unknown action debt remain binding.").font(.caption).foregroundStyle(.secondary)
            if state.busy { ProgressView("Reading and retaining exact local memory…") }
            if let message = state.message { Text(message).font(.caption) }
            if let next = state.nextCase { NavLink { destination(next) } label: { Label("Open retained local prevention", systemImage: "shield.lefthalf.filled") } }
        }.confirmationDialog("Record the current local prevention screen?", isPresented: $confirmScreen) {
            Button("Record and inspect local prevention") { model.recordPreventionScreen(entry) }
            Button("Cancel", role: .cancel) {}
        } message: { Text("Only an existing default-store preflight is retained in Casebook. No installer, model or service action runs.") }
    }
}
struct MaatLocalPreventionDetail: View {
    @ObservedObject var model: MaatLocalPreventionModel
    let entry: MaatCase
    let key: String
    let destination: (MaatCase) -> AnyView
    @State private var conclusion = ""
    @State private var effect = MaatLocalPreventionEffect.acceptOnly
    @State private var confirmSHA: String?
    var body: some View {
        let state = model.state(entry, key)
        VStack(alignment: .leading, spacing: 8) {
            Text("Local recurrence prevention").font(.headline)
            Text(key).font(.caption).textSelection(.enabled)
            Button("Inspect exact failure and owner history") { model.inspect(entry, key) }.disabled(state.busy)
            if let view = state.view {
                failureFacts(view)
                if view.head.status == "active" {
                    Picker("Effect", selection: $effect) { ForEach(MaatLocalPreventionEffect.allCases, id: \.rawValue) { choice in Text(choice.title).tag(choice) } }
                    TextEditor(text: $conclusion).frame(minHeight: 60).disabled(state.busy)
                    Button("Review exact conclusion and effect") { model.preview(entry, key, effect: effect, conclusion: conclusion) }.disabled(state.busy || conclusion.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty || state.pendingPlanSHA != nil)
                }
            }
            if let preview = state.preview {
                Text(preview.confirmation)
                ForEach(Array(preview.constraints.enumerated()), id: \.offset) { _, constraint in Text(constraint).foregroundStyle(.secondary) }
                Button("Confirm this exact reviewed effect") { confirmSHA = preview.plan_sha256 }.disabled(state.busy || !preview.can_commit)
            }
            if let receipt = state.receipt { receiptView(receipt) }
            if let pending = state.pendingPlanSHA { Button("Reconcile retained consent without repair replay") { model.reconcile(entry, key, sha: pending) }.disabled(state.busy) }
            ForEach(state.offers, id: \.plan_sha256) { offer in Button("Inspect and reconcile retained \(offer.effect)") { model.reconcile(entry, key, sha: offer.plan_sha256) }.disabled(state.busy) }
            Button("Review a new local action") { model.reviewNewAction(entry, key) }.disabled(state.busy)
            Text("This observes a fresh local case. It never replays the installer or carries old consent into a new repair.").font(.caption).foregroundStyle(.secondary)
            if let next = state.nextCase { NavLink { destination(next) } label: { Label("Open fresh local case", systemImage: "arrow.right.circle") } }
            if state.busy { ProgressView("Ma’at is verifying the exact retained workflow…"); Button("Cancel current request") { model.cancel(entry, key) } }
            if let message = state.message { Text(message).font(.caption).foregroundStyle(.secondary) }
        }
        .onChange(of: conclusion) { _ in model.invalidate(entry, key) }
        .onChange(of: effect) { _ in model.invalidate(entry, key) }
        .confirmationDialog("Confirm only this local owner disposition?", isPresented: Binding(get: { confirmSHA != nil }, set: { if !$0 { confirmSHA = nil } })) {
            if let sha = confirmSHA { Button("Confirm reviewed plan") { model.commit(entry, key, expectedSHA: sha); confirmSHA = nil } }
            Button("Cancel", role: .cancel) { confirmSHA = nil }
        } message: { Text(state.preview?.confirmation ?? "The reviewed plan changed; review it again before confirming.") }
    }
    @ViewBuilder private func failureFacts(_ view: MaatLocalFailureRead) -> some View {
        VStack(alignment: .leading, spacing: 6) {
                Text("Prevention: \(view.head.status). \(view.health)")
                Text("Occurrence \(view.head.key)\nBody \(view.head.body_sha256)\nProof \(view.head.evidence_sha256)").font(.caption)
                Text("Direct attempt \(view.completion.attempt_id): \(view.completion.invocation), \(view.completion.observation), \(view.completion.recorded_at_utc)")
                Text("Originating preflight \(view.preflight_receipt_sha256); evaluated \(view.originating_preflight.evaluated_at_utc). \(view.current_fact_reference)").font(.caption)
                ForEach(Array((view.originating_preflight.evaluated_guards ?? []).enumerated()), id: \.offset) { _, guardRecord in Text("Guard \(guardRecord.ID) / \(guardRecord.Version): \(guardRecord.SHA256)").font(.caption) }
                ForEach(Array((view.originating_preflight.measured_checks ?? []).enumerated()), id: \.offset) { _, check in Text("\(check.IncidentKey): \(check.Status) / \(check.Outcome); proof \(check.EvidenceSHA256)").font(.caption) }
                Text("Original action \(view.originating_preflight.action.component) / \(view.originating_preflight.action.profile) / \(view.originating_preflight.action.operation)\nManifest \(view.originating_preflight.action_manifest_sha256)\nRegistry v\(view.originating_preflight.registry_snapshot_version ?? 0) \(view.originating_preflight.registry_snapshot_sha256)").font(.caption)
                ForEach(Array((view.originating_preflight.incident_keys ?? []).enumerated()), id: \.offset) { _, key in Text("Original incident \(key)").font(.caption) }
                ForEach(Array((view.originating_preflight.recovery_actions ?? []).enumerated()), id: \.offset) { _, action in Text("\(action.label) [\(action.id)]: \(action.instruction)").font(.caption) }
                Text("Original recovery reference: \(view.originating_preflight.recovery_reference)").font(.caption)

                ForEach(view.events, id: \.event_key) { event in Text("\(event.recorded_at_utc): \(event.kind), \(event.status), \(event.reason)\nPredecessor \(event.predecessor_event_key)").font(.caption) }
                ForEach(view.dispositions, id: \.plan_sha256) { receipt in receiptView(receipt) }
        }
    }
    @ViewBuilder private func receiptView(_ receipt: MaatLocalDispositionReceipt) -> some View {
        VStack(alignment: .leading, spacing: 4) {
            Text("Accepted — not repaired").font(.headline)
            Text(receipt.effect == "retire-local-prevention" ? "This local prevention was lifted for the named occurrence." : "Conclusion only; this local prevention remains active.")
            Text(receipt.conclusion)
            Text("Occurrence \(receipt.occurrence_key); body \(receipt.occurrence_body_sha256)\nPlan \(receipt.plan_sha256)\n\(receipt.recorded_at_utc); \(receipt.health)").font(.caption)
        }
    }
}
