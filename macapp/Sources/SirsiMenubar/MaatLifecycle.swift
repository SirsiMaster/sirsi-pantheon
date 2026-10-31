import SwiftUI
import Combine

struct MaatLifecycleObservation: Decodable {
    struct State: Decodable {
        let status: String
        let reason: String
        let epoch: Int
        let declared: Int
        let verified: Int
        let missing: Int
        let coverageKnown: Bool
        enum CodingKeys: String, CodingKey {
            case status, reason, epoch
            case declared = "declared_receipts", verified = "verified_receipts", missing = "missing_outcomes"
            case coverageKnown = "coverage_known"
        }
    }
    let schema: String
    let receiptID: String
    let receiptSHA: String
    let repo: String
    let head: String
    let profileID: String
    let acceptance: String
    let recorded: Bool
    let duplicate: Bool
    let state: State
    enum CodingKeys: String, CodingKey {
        case schema, repo, acceptance, recorded, duplicate, state
        case receiptID = "receipt_id", receiptSHA = "receipt_sha256", head = "head_sha", profileID = "profile_id"
    }
    func matches(id: String, repo: String, head: String) -> Bool {
        schema == "maat.source-lifecycle-receipt.v1" && receiptID == id && self.repo == repo && self.head == head &&
        receiptSHA.count == 64 && receiptSHA.allSatisfy { $0.isHexDigit } &&
        profileID.count == 64 && profileID.allSatisfy { $0.isHexDigit } &&
        ["unqualified", "qualified", "requalification_required"].contains(state.status) && !state.reason.isEmpty &&
        state.epoch >= 0 && state.declared >= 0 && state.verified >= 0 && state.missing >= 0
    }
}

struct MaatLifecycleAdoptionObservation: Decodable {
    struct Profile: Decodable {
        let producer: String
        let artifactSHA: String
        let modelVersion: String
        let promptSHA: String
        let recipeSHA: String
        let policySHA: String
        enum CodingKeys: String, CodingKey {
            case producer
            case artifactSHA = "artifact_sha256"
            case modelVersion = "model_version"
            case promptSHA = "prompt_sha256"
            case recipeSHA = "recipe_sha256"
            case policySHA = "policy_sha256"
        }
        var isValid: Bool {
            !producer.isEmpty && !modelVersion.isEmpty &&
            [artifactSHA, promptSHA, recipeSHA, policySHA].allSatisfy(Self.isSHA256)
        }
        private static func isSHA256(_ value: String) -> Bool {
            value.count == 64 && value.allSatisfy { $0.isHexDigit }
        }
    }
    struct Adoption: Decodable {
        let reviewID: String
        let reviewSHA: String
        let originReceiptID: String
        let originReceiptSHA: String
        let repo: String
        let headSHA: String
        let profile: Profile
        let screenSHA: String
        let candidateGate: String
        let finalGate: String
        let producer: String
        let reviewerClaim: String
        let originIssuerClaim: String
        enum CodingKeys: String, CodingKey {
            case repo, profile, producer
            case reviewID = "review_id"
            case reviewSHA = "review_sha256"
            case originReceiptID = "origin_receipt_id"
            case originReceiptSHA = "origin_receipt_sha256"
            case headSHA = "head_sha"
            case screenSHA = "screen_sha256"
            case candidateGate = "candidate_gate"
            case finalGate = "final_gate"
            case reviewerClaim = "reviewer_claim"
            case originIssuerClaim = "origin_issuer_claim"
        }
        func matches(reviewID: String, originID: String, repo: String, head: String) -> Bool {
            self.reviewID == reviewID && originReceiptID == originID && self.repo == repo && headSHA == head &&
            [reviewSHA, originReceiptSHA, screenSHA].allSatisfy { $0.count == 64 && $0.allSatisfy { $0.isHexDigit } } &&
            profile.isValid && producer == profile.producer &&
            !reviewerClaim.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty &&
            reviewerClaim == reviewerClaim.trimmingCharacters(in: .whitespacesAndNewlines) && reviewerClaim != producer &&
            !originIssuerClaim.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty &&
            ["pass", "changes"].contains(candidateGate) && ["pass", "changes", "block"].contains(finalGate)
        }
    }
    let schema: String
    let adoption: Adoption
    let adoptionSHA: String
    let recorded: Bool
    let duplicate: Bool
    let provenance: String
    enum CodingKeys: String, CodingKey {
        case schema, adoption, recorded, duplicate, provenance
        case adoptionSHA = "adoption_sha256"
    }
    func matches(reviewID: String, originID: String, repo: String, head: String) -> Bool {
        schema == "maat.source-lifecycle-owner-adoption.v1" &&
        adoption.matches(reviewID: reviewID, originID: originID, repo: repo, head: head) &&
        adoptionSHA.count == 64 && adoptionSHA.allSatisfy { $0.isHexDigit } &&
        provenance.contains("not authenticated reviewer identity")
    }
}

@MainActor final class MaatLifecycleModel: ObservableObject {
    struct Approval: Equatable { let id: String, repo: String, head: String, sha: String, profile: String, generation: UUID }
    struct AdoptionApproval: Equatable {
        let reviewID: String, originID: String, repo: String, head: String
        let reviewSHA: String, originSHA: String, adoptionSHA: String, generation: UUID
    }
    typealias Capture = @MainActor ([String]) async -> CommandExecution
    @Published var receiptID = "" { didSet { if oldValue != receiptID { invalidate() } } }
    @Published var originReceiptID = "" { didSet { if oldValue != originReceiptID { invalidate() } } }
    @Published var repo = "" { didSet { if oldValue != repo { invalidate() } } }
    @Published var head = "" { didSet { if oldValue != head { invalidate() } } }
    @Published private(set) var approval: Approval?
    @Published private(set) var adoptionApproval: AdoptionApproval?
    @Published private(set) var projection: MaatLifecycleObservation?
    @Published private(set) var adoptionProjection: MaatLifecycleAdoptionObservation?
    @Published private(set) var receipt: CommandExecution?
    @Published private(set) var adoptionReceipt: CommandExecution?
    @Published private(set) var issue: String?
    @Published private(set) var busy = false
    private var generation = UUID()
    private var launchApproval: Approval?
    private var launchAdoptionApproval: AdoptionApproval?

    func invalidate() {
        generation = UUID(); approval = nil; launchApproval = nil; adoptionApproval = nil
        launchAdoptionApproval = nil; projection = nil; adoptionProjection = nil; issue = nil
    }
    private func isCurrent(_ approved: Approval) -> Bool {
        generation == approved.generation && receiptID == approved.id && repo == approved.repo && head == approved.head
    }
    private func authorizesLaunch(_ approved: Approval) -> Bool {
        isCurrent(approved) && launchApproval == approved
    }
    private func isCurrent(_ approved: AdoptionApproval) -> Bool {
        generation == approved.generation && receiptID == approved.reviewID && originReceiptID == approved.originID &&
        repo == approved.repo && head == approved.head
    }
    private func authorizesAdoptionLaunch(_ approved: AdoptionApproval) -> Bool {
        isCurrent(approved) && launchAdoptionApproval == approved
    }
    private func arguments(id: String, repo: String, head: String) -> [String] {
        ["maat", "lifecycle", "--receipt-id", id, "--expect-repo", repo, "--expect-head", head, "--json"]
    }
    private func decoded(_ capture: CommandExecution, id: String, repo: String, head: String) -> MaatLifecycleObservation? {
        guard capture.complete, capture.exitCode == 0,
              let view = try? JSONDecoder().decode(MaatLifecycleObservation.self, from: capture.stdoutData),
              view.matches(id: id, repo: repo, head: head) else { return nil }
        return view
    }
    private func adoptionArguments(_ approved: AdoptionApproval, preview: Bool) -> [String] {
        var args = ["maat", "lifecycle", "--receipt-id", approved.reviewID, "--origin-receipt-id", approved.originID,
                    "--expect-repo", approved.repo, "--expect-head", approved.head, "--owner-adopt-review", "--json"]
        if preview { args.append("--preview") }
        else {
            args += ["--expect-review-sha256", approved.reviewSHA, "--expect-origin-sha256", approved.originSHA,
                     "--expect-adoption-sha256", approved.adoptionSHA, "--confirm"]
        }
        return args
    }
    private func decodedAdoption(_ capture: CommandExecution, reviewID: String, originID: String,
                                 repo: String, head: String) -> MaatLifecycleAdoptionObservation? {
        guard capture.complete, capture.exitCode == 0,
              let value = try? JSONDecoder().decode(MaatLifecycleAdoptionObservation.self, from: capture.stdoutData),
              value.matches(reviewID: reviewID, originID: originID, repo: repo, head: head) else { return nil }
        return value
    }
    func preview(capture: Capture) async {
        guard !busy else { return }
        approval = nil; issue = nil; projection = nil
        guard !receiptID.isEmpty, !repo.isEmpty, head.count == 40, head.allSatisfy({ $0.isHexDigit }) else {
            issue = "Choose an exact existing receipt ID, repository and independently selected full head."; return
        }
        busy = true; defer { busy = false }
        let token = generation, id = receiptID, scope = repo, selectedHead = head
        let read = await capture(arguments(id: id, repo: scope, head: selectedHead) + ["--preview"])
        guard generation == token, !Task.isCancelled else { return }
        guard let view = decoded(read, id: id, repo: scope, head: selectedHead), !view.recorded else {
            issue = read.issue ?? "Preview unavailable or malformed. Correct the receipt/repository/head and preview again; nothing was recorded."; return
        }
        projection = view
        approval = Approval(id: id, repo: scope, head: selectedHead, sha: view.receiptSHA,
                            profile: view.profileID, generation: token)
    }
    @discardableResult func beginConfirm(capture: Capture? = nil) -> Task<Void, Never>? {
        guard !busy, let frozen = approval, isCurrent(frozen) else { return nil }
        approval = nil; launchApproval = frozen; busy = true; issue = nil
        let args = arguments(id: frozen.id, repo: frozen.repo, head: frozen.head) + ["--expect-receipt-sha256", frozen.sha, "--confirm"]
        return Task { @MainActor in
            let result: CommandExecution
            if !self.authorizesLaunch(frozen) {
                result = CommandExecution(launchError: "Selection changed after confirmation. Nothing launched; preview again.")
            } else if let capture {
                result = await capture(args)
            } else {
                result = await SirsiEngine.runCaptured(args: args, authorize: { self.authorizesLaunch(frozen) })
            }
            self.receipt = result; self.busy = false
            if self.launchApproval == frozen { self.launchApproval = nil }
            guard self.isCurrent(frozen), !Task.isCancelled else { return }
            guard let view = self.decoded(result, id: frozen.id, repo: frozen.repo, head: frozen.head),
                  view.receiptSHA == frozen.sha, view.profileID == frozen.profile, view.recorded || view.duplicate else {
                self.issue = result.issue ?? "Recording/readback was not verified. Keep this receipt; a new explicit preview is required."; return
            }
            self.projection = view
        }
    }
    func confirm(capture: Capture? = nil) async {
        guard let task = beginConfirm(capture: capture) else { return }
        await task.value
    }

    func previewAdoption(capture: Capture) async {
        guard !busy else { return }
        approval = nil; adoptionApproval = nil; issue = nil; adoptionProjection = nil
        guard !receiptID.isEmpty, !originReceiptID.isEmpty, !repo.isEmpty,
              head.count == 40, head.allSatisfy({ $0.isHexDigit }) else {
            issue = "Choose exact review and originating evaluation IDs, repository and independently selected full head."; return
        }
        busy = true; defer { busy = false }
        let token = generation, reviewID = receiptID, originID = originReceiptID, scope = repo, selectedHead = head
        let request = AdoptionApproval(reviewID: reviewID, originID: originID, repo: scope, head: selectedHead,
                                       reviewSHA: "", originSHA: "", adoptionSHA: "", generation: token)
        let read = await capture(adoptionArguments(request, preview: true))
        guard generation == token, !Task.isCancelled else { return }
        guard let value = decodedAdoption(read, reviewID: reviewID, originID: originID, repo: scope, head: selectedHead),
              !value.recorded, !value.duplicate else {
            issue = read.issue ?? "Owner-adoption preview unavailable or malformed. Correct the exact IDs/scope and preview again; nothing was recorded."; return
        }
        adoptionProjection = value
        adoptionApproval = AdoptionApproval(reviewID: reviewID, originID: originID, repo: scope, head: selectedHead,
            reviewSHA: value.adoption.reviewSHA, originSHA: value.adoption.originReceiptSHA,
            adoptionSHA: value.adoptionSHA, generation: token)
    }

    @discardableResult func beginAdoptionConfirm(capture: Capture? = nil) -> Task<Void, Never>? {
        guard !busy, let frozen = adoptionApproval, isCurrent(frozen) else { return nil }
        adoptionApproval = nil; launchAdoptionApproval = frozen; busy = true; issue = nil
        let args = adoptionArguments(frozen, preview: false)
        return Task { @MainActor in
            let result: CommandExecution
            if !self.authorizesAdoptionLaunch(frozen) {
                result = CommandExecution(launchError: "Selection changed after owner-adoption confirmation. Nothing launched; preview again.")
            } else if let capture {
                result = await capture(args)
            } else {
                result = await SirsiEngine.runCaptured(args: args, authorize: { self.authorizesAdoptionLaunch(frozen) })
            }
            self.adoptionReceipt = result; self.busy = false
            if self.launchAdoptionApproval == frozen { self.launchAdoptionApproval = nil }
            guard self.isCurrent(frozen), !Task.isCancelled else { return }
            guard let value = self.decodedAdoption(result, reviewID: frozen.reviewID, originID: frozen.originID,
                repo: frozen.repo, head: frozen.head), value.adoption.reviewSHA == frozen.reviewSHA,
                value.adoption.originReceiptSHA == frozen.originSHA, value.adoptionSHA == frozen.adoptionSHA,
                (value.recorded || value.duplicate) else {
                self.issue = result.issue ?? "Owner adoption was not verified. Keep this operation receipt; explicit preview is required before retrying."; return
            }
            self.adoptionProjection = value
        }
    }

    func confirmAdoption(capture: Capture? = nil) async {
        guard let task = beginAdoptionConfirm(capture: capture) else { return }
        await task.value
    }
}

@MainActor struct MaatLifecycleControl: View {
    @ObservedObject var engine: SirsiEngine
    @StateObject private var model = MaatLifecycleModel()
    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Source qualification evidence").sirsiFont(.headline)
            Text("Preview an existing structured canonical receipt. Canonical custody alone does not authenticate a reviewer or qualify a model. Recording preserves this observation only; it does not activate a gate or run a model.")
                .sirsiFont(.caption).foregroundStyle(.secondary).fixedSize(horizontal: false, vertical: true)
            TextField("Canonical receipt ID", text: $model.receiptID)
            TextField("Repository", text: $model.repo)
            TextField("Independently selected full head", text: $model.head)
            TextField("Originating evaluation receipt ID for owner adoption", text: $model.originReceiptID)
            Button("Preview lifecycle evidence") { Task { await model.preview { await SirsiEngine.runCaptured(args: $0) } } }.disabled(model.busy)
            if let view = model.projection {
                Text("\(view.state.status) · epoch \(view.state.epoch)").sirsiFont(.subheadline)
                Text(view.state.reason).sirsiFont(.caption).fixedSize(horizontal: false, vertical: true)
                Text(view.state.coverageKnown ? "Declared \(view.state.declared) · verified \(view.state.verified) · missing outcomes \(view.state.missing)" : "Declared \(view.state.declared) · verified \(view.state.verified) · sample coverage unknown")
                    .sirsiFont(.caption).foregroundStyle(.secondary)
                Text(view.acceptance).sirsiFont(.caption).fixedSize(horizontal: false, vertical: true)
            }
            if let approval = model.approval {
                Text("Confirm observation for \(approval.repo) @ \(approval.head)\nProfile \(approval.profile)\nReceipt SHA256 \(approval.sha)")
                    .sirsiFont(.caption).textSelection(.enabled).fixedSize(horizontal: false, vertical: true)
                Button("Record this exact observation") { _ = model.beginConfirm() }.disabled(model.busy)
            }
            Button("Preview exact review owner adoption") {
                Task { await model.previewAdoption { await SirsiEngine.runCaptured(args: $0) } }
            }.disabled(model.busy)
            if let adoption = model.adoptionProjection {
                Text("Owner-adoption review · \(adoption.adoption.candidateGate) → \(adoption.adoption.finalGate)")
                    .sirsiFont(.subheadline)
                Text("Review \(adoption.adoption.reviewID) · origin \(adoption.adoption.originReceiptID)\n\(adoption.adoption.repo) @ \(adoption.adoption.headSHA)\nProfile \(adoption.adoption.profile.producer) / \(adoption.adoption.profile.modelVersion)\nArtifact \(adoption.adoption.profile.artifactSHA) · prompt \(adoption.adoption.profile.promptSHA)\nRecipe \(adoption.adoption.profile.recipeSHA) · policy \(adoption.adoption.profile.policySHA)\nReviewer claim: \(adoption.adoption.reviewerClaim)")
                    .sirsiFont(.caption).textSelection(.enabled).fixedSize(horizontal: false, vertical: true)
                Text(adoption.provenance).sirsiFont(.caption).foregroundStyle(.secondary).fixedSize(horizontal: false, vertical: true)
            }
            if let adoption = model.adoptionApproval {
                Text("Confirm local adoption of review \(adoption.reviewID) and origin \(adoption.originID)\n\(adoption.repo) @ \(adoption.head)\nReview SHA256 \(adoption.reviewSHA)\nOrigin SHA256 \(adoption.originSHA)\nAdoption SHA256 \(adoption.adoptionSHA)\nThis explicit local record does not authenticate the reviewer or qualify a model.")
                    .sirsiFont(.caption).textSelection(.enabled).fixedSize(horizontal: false, vertical: true)
                Button("Adopt this exact review locally") { _ = model.beginAdoptionConfirm() }.disabled(model.busy)
            }
            if let issue = model.issue { Text(issue).sirsiFont(.caption).foregroundStyle(.orange) }
            if let receipt = model.receipt {
                DisclosureGroup("Retained lifecycle operation receipt") {
                    Text(receipt.displayText).sirsiFont(.caption).textSelection(.enabled)
                }
            }
            if let receipt = model.adoptionReceipt {
                DisclosureGroup("Retained owner-adoption operation receipt") {
                    Text(receipt.displayText).sirsiFont(.caption).textSelection(.enabled)
                }
            }
        }.onDisappear { model.invalidate() }
    }
}
