import SwiftUI
import Foundation
import Darwin
import AppKit

private let sneGold = Color(red: 0.78, green: 0.66, blue: 0.32)

@MainActor
protocol SNEControlTransport {
    func data(for request: URLRequest) async throws -> (Data, URLResponse)
}

struct URLSessionSNEControlTransport: SNEControlTransport {
    func data(for request: URLRequest) async throws -> (Data, URLResponse) {
        try await URLSession.shared.data(for: request)
    }
}

private struct SNECard<Content: View>: View {
    @Environment(\.snapshotMode) private var snapshotMode
    let title: String
    @ViewBuilder let content: Content

    init(_ title: String, @ViewBuilder content: () -> Content) {
        self.title = title
        self.content = content()
    }

    var body: some View {
        if snapshotMode {
            VStack(alignment: .leading, spacing: 8) {
                Text(title).sirsiFont(.headline)
                content
            }
            .padding(10)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(Color.primary.opacity(0.06), in: RoundedRectangle(cornerRadius: 8))
        } else {
            GroupBox(title) { content }
        }
    }
}

struct SNELifecycleViewState: Decodable {
    let modelID: String?
    let runtimeID: String?
    let runtimeSHA256: String?
    let modelManifestSHA256: String?
    let profile: String?
    let state: String
    let error: String?
    let errorCode: String?
    let recovery: String?
    let prefixCachePressure: SNEPrefixCachePressureReceipt?

    enum CodingKeys: String, CodingKey {
        case modelID = "model_id"
        case runtimeID = "runtime_id"
        case runtimeSHA256 = "runtime_sha256"
        case modelManifestSHA256 = "model_manifest_sha256"
        case profile, state, error
        case errorCode = "error_code"
        case recovery
        case prefixCachePressure = "prefix_cache_pressure"
    }
}

// These values mirror Pantheon's public prefix-cache-pressure read contract.
// They deliberately do not contain SNE cache-policy fields: Pantheon measures
// and obtains an owner authorization, while SNE owns every decision, execution,
// replay, and retention result.
struct SNEPrefixCachePressureObservation: Decodable {
    let requestID: String
    let hostID: String
    let observedAtUnix: Int64
    let expiresAtUnix: Int64
    let totalRAMBytes: UInt64
    let availableRAMBytes: UInt64
    let swapUsedBytes: UInt64
    let swapLimitBytes: UInt64
    let pressure: String
    let pressureSource: String
    let swapMeasured: Bool

    enum CodingKeys: String, CodingKey {
        case requestID = "request_id"
        case hostID = "host_id"
        case observedAtUnix = "observed_at_unix"
        case expiresAtUnix = "expires_at_unix"
        case totalRAMBytes = "total_ram_bytes"
        case availableRAMBytes = "available_ram_bytes"
        case swapUsedBytes = "swap_used_bytes"
        case swapLimitBytes = "swap_limit_bytes"
        case pressure
        case pressureSource = "pressure_source"
        case swapMeasured = "swap_measured"
    }
}

struct SNEPrefixCachePressureReceipt: Decodable {
    let schema: String
    let observation: SNEPrefixCachePressureObservation
    let observationSHA256: String

    enum CodingKeys: String, CodingKey {
        case schema, observation
        case observationSHA256 = "observation_sha256"
    }
}

struct SNEPrefixCachePressureConfirmation: Decodable {
    let confirmToken: String
    let actionHash: String
    let expiresAt: Date
    let preview: String

    enum CodingKeys: String, CodingKey {
        case confirmToken = "confirm_token"
        case actionHash = "action_hash"
        case expiresAt = "expires_at"
        case preview
    }
}

struct SNEPrefixCachePressureAuthorization: Decodable {
    let state: String
    let requestID: String
    let artifactSHA256: String
    let expiresAtUnix: Int64

    enum CodingKeys: String, CodingKey {
        case state
        case requestID = "request_id"
        case artifactSHA256 = "artifact_sha256"
        case expiresAtUnix = "expires_at_unix"
    }
}

struct SNEPrefixCachePressureViewState: Decodable {
    let state: String
    let receipt: SNEPrefixCachePressureReceipt
    let confirmation: SNEPrefixCachePressureConfirmation?
    let authorization: SNEPrefixCachePressureAuthorization?
}

struct SNEPrefixCachePressureEvidenceState: Decodable {
    let state: String
    let evidenceType: String
    let identity: String
    let receipt: SNEPrefixCachePressureTerminalReceipt?

    enum CodingKeys: String, CodingKey {
        case state
        case evidenceType = "evidence_type"
        case identity
        case receipt
    }
}

struct SNEPrefixCachePressureTerminalReceipt: Decodable {
    let status: String?
    let errorCode: String?
    let removedRequestIDs: [String]?
    let retainedRequestIDs: [String]?

    enum CodingKeys: String, CodingKey {
        case status
        case errorCode = "error_code"
        case removedRequestIDs = "removed_request_ids"
        case retainedRequestIDs = "retained_request_ids"
    }
}

struct SNERuntimeCatalogViewState: Decodable {
    let state: String
    let versionSHA256: String?
    let retainedVersions: [String]?
    let rollbackAvailable: Bool
    let updateFeedConfigured: Bool

    enum CodingKeys: String, CodingKey {
        case state
        case versionSHA256 = "version_sha256"
        case retainedVersions = "retained_versions"
        case rollbackAvailable = "rollback_available"
        case updateFeedConfigured = "update_feed_configured"
    }
}

struct SNECatalogViewItem: Decodable, Identifiable {
    var id: String { catalogEntry }
    let catalogEntry: String
    let modelID: String
    let runtimeID: String?
    let parameterClass: String
    let executionMode: String
    let weightFormat: String
    let weightBits: Int
    let memoryBytes: UInt64
    let supportStatus: String
    let nextGate: String?
    let installed: Bool
    let active: Bool
    let actionLabel: String
    let actionKind: String?
    let actionEnabled: Bool
    let removalEnabled: Bool
    let removalReason: String?
    let reason: String?
    let licenseID: String?
    let licenseURL: String?
    let licenseRequired: Bool
    let cacheTopology: String?
    let servingCacheCapacity: Int?

    enum CodingKeys: String, CodingKey {
        case catalogEntry = "catalog_entry"
        case modelID = "model_id"
        case runtimeID = "runtime_id"
        case parameterClass = "parameter_class"
        case executionMode = "execution_mode"
        case weightFormat = "weight_format"
        case weightBits = "weight_bits"
        case memoryBytes = "memory_bytes"
        case supportStatus = "support_status"
        case nextGate = "next_gate"
        case installed, active
        case actionLabel = "action_label"
        case actionKind = "action_kind"
        case actionEnabled = "action_enabled"
        case removalEnabled = "removal_enabled"
        case removalReason = "removal_reason"
        case reason
        case licenseID = "license_id"
        case licenseURL = "license_url"
        case licenseRequired = "license_acceptance_required"
        case cacheTopology = "cache_topology"
        case servingCacheCapacity = "serving_cache_capacity"
    }
}

struct SNEReadViewState: Decodable {
    let configured: Bool
    let ready: Bool
    let serviceState: String
    let activeModel: String?
    let deviceFamily: String?
    let unifiedMemoryBytes: UInt64?
    let catalog: [SNECatalogViewItem]
    let runtimeCatalog: SNERuntimeCatalogViewState
    let lifecycle: SNELifecycleViewState
    let recovery: String?
    let lifecycleToolsReady: Bool
    let lifecycleToolsStatus: String?

    enum CodingKeys: String, CodingKey {
        case configured, ready
        case serviceState = "service_state"
        case activeModel = "active_model"
        case deviceFamily = "device_family"
        case unifiedMemoryBytes = "unified_memory_bytes"
        case catalog
        case runtimeCatalog = "runtime_catalog"
        case lifecycle, recovery
        case lifecycleToolsReady = "lifecycle_tools_ready"
        case lifecycleToolsStatus = "lifecycle_tools_status"
    }
}

@MainActor
final class SNEControlModel: ObservableObject {
    @Published var state: SNEReadViewState?
    @Published var loading = false
    @Published var actionInFlight = false
    @Published var message: String?
    @Published var failure: String?
    @Published var prefixCachePressure: SNEPrefixCachePressureViewState?
    @Published var executionEvidence: SNEPrefixCachePressureEvidenceState?
    @Published var retentionEvidence: SNEPrefixCachePressureEvidenceState?

    private let baseURL: URL
    private let transport: any SNEControlTransport
    private let capabilityProvider: @MainActor () throws -> String
    private let decoder = JSONDecoder()

    convenience init(state: SNEReadViewState? = nil) {
        self.init(
            state: state,
            baseURL: URL(string: "http://127.0.0.1:9119")!,
            transport: URLSessionSNEControlTransport(),
            capabilityProvider: SNEControlModel.localCapability
        )
    }

    init(
        state: SNEReadViewState? = nil,
        baseURL: URL,
        transport: any SNEControlTransport,
        capabilityProvider: @escaping @MainActor () throws -> String,
        prefixCachePressure: SNEPrefixCachePressureViewState? = nil,
        executionEvidence: SNEPrefixCachePressureEvidenceState? = nil,
        retentionEvidence: SNEPrefixCachePressureEvidenceState? = nil
    ) {
        decoder.dateDecodingStrategy = .iso8601
        self.state = state
        self.baseURL = baseURL
        self.transport = transport
        self.capabilityProvider = capabilityProvider
        self.prefixCachePressure = prefixCachePressure
        self.executionEvidence = executionEvidence
        self.retentionEvidence = retentionEvidence
    }

    func refresh() async {
        loading = true
        defer { loading = false }
        do {
            state = try await request(path: "/api/sne", method: "GET", body: nil, authorized: false)
            failure = nil
        } catch {
            failure = error.localizedDescription
        }
    }

    func start(_ item: SNECatalogViewItem) async {
        await mutate(path: "/api/sne/start", body: ["model_id": item.modelID, "runtime_id": item.runtimeID ?? ""], success: "SNE start admitted.")
    }

    func retry() async {
        guard let lifecycle = state?.lifecycle, let modelID = lifecycle.modelID else { return }
        await mutate(path: "/api/sne/start", body: ["model_id": modelID, "runtime_id": lifecycle.runtimeID ?? ""], success: "SNE retry admitted.")
    }

    func retryIfWaitingForUnlock() async {
        guard state?.serviceState == "waiting-for-unlock",
              state?.lifecycle.errorCode == "metal_session_locked",
              !loading, !actionInFlight else { return }
        await retry()
    }

    func stop() async {
        await mutate(path: "/api/sne/stop", body: [:], success: "SNE stopped safely.")
    }

    func preparePrefixCachePressure() async {
        actionInFlight = true
        failure = nil
        defer { actionInFlight = false }
        do {
            prefixCachePressure = try await request(path: "/api/sne/prefix-cache-pressure", method: "GET", body: nil, authorized: true)
        } catch {
            failure = error.localizedDescription
        }
    }

    func authorizePrefixCachePressure(_ view: SNEPrefixCachePressureViewState) async {
        guard let confirmation = view.confirmation else { return }
        actionInFlight = true
        failure = nil
        defer { actionInFlight = false }
        do {
            prefixCachePressure = try await request(path: "/api/sne/prefix-cache-pressure", method: "POST", body: [
                "request_id": view.receipt.observation.requestID,
                "observation_sha256": view.receipt.observationSHA256,
                "confirm_token": confirmation.confirmToken,
                "action_hash": confirmation.actionHash,
            ], authorized: true)
            message = "Owner authorization recorded. SNE remains the sole decision and execution owner."
        } catch {
            failure = error.localizedDescription
        }
    }

    func loadPrefixCachePressureEvidence(kind: String, identity: String) async {
        guard Self.validEvidenceIdentity(identity), (kind == "receipts" || kind == "retention") else {
            failure = "Enter an exact receipt identity before reading SNE-owned evidence."
            return
        }
        actionInFlight = true
        failure = nil
        defer { actionInFlight = false }
        do {
            let evidence: SNEPrefixCachePressureEvidenceState = try await request(path: "/api/sne/prefix-cache-pressure/\(kind)/\(identity)", method: "GET", body: nil, authorized: false)
            if kind == "receipts" { executionEvidence = evidence } else { retentionEvidence = evidence }
        } catch {
            failure = error.localizedDescription
        }
    }

    static func validEvidenceIdentity(_ value: String) -> Bool {
        let allowed = CharacterSet.alphanumerics.union(CharacterSet(charactersIn: ".-_"))
        guard (1...128).contains(value.count), value.unicodeScalars.allSatisfy({ allowed.contains($0) }) else { return false }
        return value.unicodeScalars.first.map { CharacterSet.alphanumerics.contains($0) } ?? false
    }

    func install(_ item: SNECatalogViewItem) async {
        await mutate(path: "/api/sne/install", body: ["catalog_entry": item.catalogEntry, "accept_license": true, "allow_research": false], success: "Verified installation started.")
    }

    func remove(_ item: SNECatalogViewItem) async {
        await mutate(path: "/api/sne/remove", body: ["catalog_entry": item.catalogEntry, "model_id": item.modelID], success: "Governed model removal completed.")
    }

    func rollback(to version: String) async {
        await mutate(path: "/api/sne/catalog/rollback", body: ["version_sha256": version], success: "Signed catalog rollback completed.")
    }

    func removeCatalog(version: String) async {
        await mutate(path: "/api/sne/catalog/remove", body: ["version_sha256": version], success: "Inactive catalog version removed.")
    }

    private func mutate(path: String, body: [String: Any], success: String) async {
        actionInFlight = true
        failure = nil
        defer { actionInFlight = false }
        do {
            let _: SNEMutationResponse = try await request(path: path, method: "POST", body: body, authorized: true)
            message = success
            try? await Task.sleep(nanoseconds: 300_000_000)
            await refresh()
        } catch {
            failure = error.localizedDescription
        }
    }

    private func request<T: Decodable>(path: String, method: String, body: [String: Any]?, authorized: Bool) async throws -> T {
        var request = URLRequest(url: baseURL.appendingPathComponent(path))
        request.httpMethod = method
        request.timeoutInterval = 30
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        if authorized {
            request.setValue("Bearer \(try capabilityProvider())", forHTTPHeaderField: "Authorization")
        }
        if let body { request.httpBody = try JSONSerialization.data(withJSONObject: body) }
        let (data, response) = try await transport.data(for: request)
        guard let http = response as? HTTPURLResponse else { throw SNEControlError.invalidResponse }
        guard (200..<300).contains(http.statusCode) else {
            let object = (try? JSONSerialization.jsonObject(with: data)) as? [String: Any]
            throw SNEControlError.server((object?["error"] as? String) ?? "HTTP \(http.statusCode)")
        }
        return try decoder.decode(T.self, from: data)
    }

    fileprivate static func localCapability() throws -> String {
        let path = ("~/Library/Application Support/Sirsi/Pantheon/sne-local-api.token" as NSString).expandingTildeInPath
        var info = stat()
        guard lstat(path, &info) == 0, (info.st_mode & S_IFMT) == S_IFREG, (info.st_mode & 0o077) == 0 else {
            throw SNEControlError.capabilityUnavailable
        }
        let token = try String(contentsOfFile: path, encoding: .utf8).trimmingCharacters(in: .whitespacesAndNewlines)
        guard token.count >= 32, token.rangeOfCharacter(from: .whitespacesAndNewlines) == nil else {
            throw SNEControlError.capabilityUnavailable
        }
        return token
    }
}

private struct SNEMutationResponse: Decodable {}

enum SNEControlError: LocalizedError {
    case invalidResponse
    case capabilityUnavailable
    case server(String)

    var errorDescription: String? {
        switch self {
        case .invalidResponse: return "Pantheon returned an invalid local response."
        case .capabilityUnavailable: return "Pantheon's private SNE control capability is unavailable or insecure. No SNE state changed; reopen this control after the required owner action."
        case .server(let message): return message
        }
    }
}

struct SNEControlView: View {
    @Environment(\.snapshotMode) private var snapshotMode
    @StateObject private var model: SNEControlModel
    @State private var confirmation: SNEConfirmation?
    @State private var executionReceiptID = ""
    @State private var retentionReceiptID = ""

    init(
        preloaded: SNEReadViewState? = nil,
        prefixCachePressure: SNEPrefixCachePressureViewState? = nil,
        executionEvidence: SNEPrefixCachePressureEvidenceState? = nil,
        retentionEvidence: SNEPrefixCachePressureEvidenceState? = nil
    ) {
        _model = StateObject(wrappedValue: SNEControlModel(
            state: preloaded,
            baseURL: URL(string: "http://127.0.0.1:9119")!,
            transport: URLSessionSNEControlTransport(),
            capabilityProvider: SNEControlModel.localCapability,
            prefixCachePressure: prefixCachePressure,
            executionEvidence: executionEvidence,
            retentionEvidence: retentionEvidence
        ))
    }

    var body: some View {
        VStack(spacing: 0) {
            BackBar(title: "SNE — Local AI")
            MaybeScroll {
                VStack(alignment: .leading, spacing: 14) {
                    statusCard
                    if let state = model.state {
                        lifecycleCard(state)
                        prefixCachePressureCard(state)
                        modelCatalog(state)
                        runtimeCatalog(state)
                    }
                    if let message = model.message { Text(message).foregroundStyle(.green).sirsiFont(.callout) }
                    if let failure = model.failure {
                        Text(failure).foregroundStyle(.red).sirsiFont(.callout).accessibilityLabel("SNE error: \(failure)")
                    }
                }
                .padding(16)
            }
        }
        .task { if !snapshotMode { await model.refresh() } }
        .refreshable { await model.refresh() }
        .onReceive(NSWorkspace.shared.notificationCenter.publisher(for: NSWorkspace.sessionDidBecomeActiveNotification)) { _ in
            guard !snapshotMode else { return }
            Task { await model.retryIfWaitingForUnlock() }
        }
        .alert(confirmation?.title ?? "Confirm SNE action", isPresented: Binding(get: { confirmation != nil }, set: { if !$0 { confirmation = nil } })) {
            Button("Cancel", role: .cancel) { confirmation = nil }
            Button(confirmation?.button ?? "Continue", role: confirmation?.destructive == true ? .destructive : nil) {
                guard let action = confirmation else { return }
                confirmation = nil
                Task {
                    switch action {
                    case .install(let item): await model.install(item)
                    case .remove(let item): await model.remove(item)
                    case .rollback(let version): await model.rollback(to: version)
                    case .prefixCachePressure(let view): await model.authorizePrefixCachePressure(view)
                    }
                }
            }
        } message: { Text(confirmation?.message ?? "") }
    }

    private var statusCard: some View {
        SNECard("Engine status") {
            HStack {
                Circle().fill(model.state?.ready == true ? Color.green : Color.orange).frame(width: 10, height: 10)
                    .accessibilityHidden(true)
                Text(statusTitle)
                Spacer()
                if model.loading || model.actionInFlight {
                    ProgressView().controlSize(.small).accessibilityLabel("SNE operation in progress")
                }
                Button("Refresh") { Task { await model.refresh() } }
                    .disabled(model.loading || model.actionInFlight)
                    .accessibilityIdentifier("sne.refresh")
            }
            .padding(.vertical, 4)
            .accessibilityElement(children: .contain)
            .accessibilityLabel("SNE engine status, \(statusTitle)")
            .accessibilityIdentifier("sne.engine.status")
        }
    }

    private var statusTitle: String {
        if model.state?.ready == true { return "Ready" }
        if model.state?.serviceState == "waiting-for-unlock" { return "Waiting for unlock" }
        return model.state?.serviceState ?? "Checking"
    }

    private func lifecycleCard(_ state: SNEReadViewState) -> some View {
        SNECard("Active runtime") {
            VStack(alignment: .leading, spacing: 7) {
                Text(state.activeModel ?? "No model active").sirsiFont(.headline)
                Text("Lifecycle: \(state.lifecycle.state) · \(state.lifecycle.profile ?? "no profile")").foregroundStyle(.secondary)
                if let runtime = state.lifecycle.runtimeSHA256 { Text("Runtime \(runtime.prefix(12))…").sirsiFont(.caption, design: .monospaced) }
                if let manifest = state.lifecycle.modelManifestSHA256 { Text("Manifest \(manifest.prefix(12))…").sirsiFont(.caption, design: .monospaced) }
                if let recovery = state.lifecycle.recovery ?? state.recovery { Text(recovery).foregroundStyle(sneGold) }
                HStack {
                    if state.lifecycle.state == "failed", state.lifecycle.modelID != nil {
                        Button(state.lifecycle.errorCode == "metal_session_locked" ? "Retry after unlocking" : "Retry when safe") { Task { await model.retry() } }
                            .accessibilityLabel("Retry \(state.activeModel ?? "SNE model") when safe")
                            .accessibilityHint(state.lifecycle.errorCode == "metal_session_locked" ? "Unlock this Mac first; Pantheon also retries automatically when the session becomes active" : "Pantheon rechecks admission and recovery conditions before starting")
                            .accessibilityIdentifier("sne.lifecycle.retry")
                    }
                    if state.activeModel != nil {
                        Button("Stop safely") { Task { await model.stop() } }
                            .accessibilityLabel("Stop \(state.activeModel ?? "SNE model") safely")
                            .accessibilityHint("Stops local inference without removing the model")
                    }
                }
                .disabled(model.actionInFlight)
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .accessibilityIdentifier("sne.active.runtime")
        }
    }

    private func prefixCachePressureCard(_ state: SNEReadViewState) -> some View {
        SNECard("Prefix-cache pressure") {
            VStack(alignment: .leading, spacing: 8) {
                if let observation = state.lifecycle.prefixCachePressure?.observation {
                    Text("Observed \(observation.pressure) pressure on \(observation.hostID)")
                        .sirsiFont(.headline)
                    Text("Request \(observation.requestID) · source \(observation.pressureSource)")
                        .sirsiFont(.caption, design: .monospaced).foregroundStyle(.secondary)
                } else {
                    Text("No measured pressure observation").foregroundStyle(.secondary)
                }

                if let view = model.prefixCachePressure {
                    Text(prefixCachePressureLabel(view)).foregroundStyle(sneGold)
                    Text("Observation \(view.receipt.observationSHA256.prefix(12))… · expires \(Date(timeIntervalSince1970: TimeInterval(view.receipt.observation.expiresAtUnix)).formatted(date: .omitted, time: .shortened))")
                        .sirsiFont(.caption, design: .monospaced).foregroundStyle(.secondary)
                    if let confirmation = view.confirmation {
                        Text(confirmation.preview).sirsiFont(.caption).foregroundStyle(.secondary)
                        Button("Authorize SNE evaluation") { self.confirmation = .prefixCachePressure(view) }
                            .accessibilityIdentifier("sne.prefix-pressure.authorize")
                            .accessibilityLabel("Authorize SNE to evaluate the measured prefix-cache pressure")
                            .accessibilityHint("Requires this visible confirmation. Pantheon does not decide or execute the cache action.")
                    }
                } else {
                    Button("Measure cache pressure") { Task { await model.preparePrefixCachePressure() } }
                        .accessibilityIdentifier("sne.prefix-pressure.measure")
                        .accessibilityHint("Measures this host and prepares an owner confirmation. It does not start SNE or change its cache.")
                }

                evidenceReader(kind: "receipts", title: "Execution receipt", evidence: model.executionEvidence)
                evidenceReader(kind: "retention", title: "Retention receipt", evidence: model.retentionEvidence)
                Text("Unavailable means unknown. Pantheon does not discover receipt paths, infer SNE execution, retry, or alter retention.")
                    .sirsiFont(.caption).foregroundStyle(.secondary)
            }
            .disabled(model.actionInFlight)
            .frame(maxWidth: .infinity, alignment: .leading)
            .accessibilityIdentifier("sne.prefix-pressure")
        }
    }

    @ViewBuilder
    private func evidenceReader(kind: String, title: String, evidence: SNEPrefixCachePressureEvidenceState?) -> some View {
        if snapshotMode {
            // ImageRenderer cannot faithfully render editable AppKit-backed
            // text fields. A fixture image must show the truthful precondition,
            // not a malformed yellow editor that looks like an alert.
            HStack {
                Text(title).sirsiFont(.caption)
                Text("Exact ID required before read")
                    .sirsiFont(.caption)
                    .foregroundStyle(.secondary)
            }
            .accessibilityLabel("\(title), exact SNE receipt identity required before read")
        } else {
            HStack {
                Text(title).sirsiFont(.caption)
                TextField("Exact ID", text: Binding(get: {
                    kind == "receipts" ? executionReceiptID : retentionReceiptID
                }, set: { value in
                    if kind == "receipts" { executionReceiptID = value } else { retentionReceiptID = value }
                }))
                .textFieldStyle(.roundedBorder)
                .accessibilityLabel("Exact SNE \(title.lowercased()) identity")
                Button("Read") {
                    let id = kind == "receipts" ? executionReceiptID : retentionReceiptID
                    Task { await model.loadPrefixCachePressureEvidence(kind: kind, identity: id) }
                }
                .accessibilityLabel("Read SNE \(title.lowercased())")
            }
        }
        if let evidence {
            Text(prefixCachePressureEvidenceLabel(evidence)).sirsiFont(.caption).foregroundStyle(.secondary)
                .accessibilityLabel("\(title), \(prefixCachePressureEvidenceLabel(evidence))")
        }
    }

    private func prefixCachePressureLabel(_ view: SNEPrefixCachePressureViewState) -> String {
        switch view.state {
        case "owner-confirmation-required": return "Owner confirmation required — SNE has not been authorized."
        case "authorization-accepted": return "Authorization accepted — SNE decision and execution evidence remain external."
        default: return "Prefix-cache pressure state: \(view.state)"
        }
    }

    private func prefixCachePressureEvidenceLabel(_ evidence: SNEPrefixCachePressureEvidenceState) -> String {
        guard evidence.state == "available", let receipt = evidence.receipt else {
            return "Unavailable — no SNE-owned state is inferred."
        }
        if evidence.evidenceType == "retention" {
            return "Retention receipt available: \(receipt.removedRequestIDs?.count ?? 0) removed, \(receipt.retainedRequestIDs?.count ?? 0) retained."
        }
        if receipt.status == "failed", receipt.errorCode == "cache_pressure_execution_interrupted" {
            return "Interrupted execution recovered to failed."
        }
        return "Execution receipt: \(receipt.status ?? "unavailable")."
    }

    private func modelCatalog(_ state: SNEReadViewState) -> some View {
        SNECard("Supported models") {
            VStack(spacing: 10) {
                ForEach(state.catalog) { item in
                    VStack(alignment: .leading, spacing: 5) {
                        HStack {
                            VStack(alignment: .leading) {
                                Text(item.modelID).sirsiFont(.headline)
                                Text("\(item.parameterClass) · \(item.weightFormat.uppercased()) \(item.weightBits)-bit · \(item.executionMode)").foregroundStyle(.secondary).sirsiFont(.caption)
                            }
                            Spacer()
                            Text(item.supportStatus).foregroundStyle(item.supportStatus == "release-supported" ? Color.green : sneGold).sirsiFont(.caption)
                        }
                        .accessibilityElement(children: .combine)
                        .accessibilityLabel("\(item.modelID), \(item.parameterClass), \(item.weightFormat) \(item.weightBits)-bit, \(item.executionMode), \(item.supportStatus)")
                        Text("\(ByteCountFormatter.string(fromByteCount: Int64(item.memoryBytes), countStyle: .memory)) · cache \(item.cacheTopology ?? "undeclared") · \(item.servingCacheCapacity ?? 0) positions").sirsiFont(.caption).foregroundStyle(.secondary)
                        if let reason = item.reason { Text(reason).sirsiFont(.caption).foregroundStyle(.secondary) }
                        HStack {
                            if item.actionEnabled {
                                Button(item.actionLabel) {
                                    if item.actionKind == "install" { confirmation = .install(item) }
                                    else { Task { await model.start(item) } }
                                }
                                .accessibilityLabel("\(item.actionLabel) \(item.modelID)")
                            }
                            if item.installed && !item.active {
                                Button("Remove model", role: .destructive) { confirmation = .remove(item) }
                                    .disabled(!item.removalEnabled)
                                    .help(item.removalReason ?? "Remove this governed model")
                                    .accessibilityLabel("Remove \(item.modelID)")
                                    .accessibilityHint(item.removalReason ?? "Removes this governed model while retaining shared objects")
                            }
                            if let url = item.licenseURL, let licenseURL = URL(string: url) {
                                if snapshotMode {
                                    Text("License terms available").sirsiFont(.caption).foregroundStyle(.secondary)
                                } else {
                                    Link("License terms", destination: licenseURL)
                                        .accessibilityLabel("License terms for \(item.modelID)")
                                }
                            }
                        }.disabled(model.actionInFlight)
                    }
                    .accessibilityIdentifier("sne.model.\(item.catalogEntry)")
                    if item.id != state.catalog.last?.id { Divider() }
                }
            }.frame(maxWidth: .infinity, alignment: .leading)
        }
    }

    private func runtimeCatalog(_ state: SNEReadViewState) -> some View {
        SNECard("Signed runtime catalog") {
            VStack(alignment: .leading, spacing: 7) {
                Text("\(state.runtimeCatalog.state) · current \(state.runtimeCatalog.versionSHA256?.prefix(12) ?? "none")")
                ForEach(state.runtimeCatalog.retainedVersions ?? [], id: \.self) { version in
                    if version != state.runtimeCatalog.versionSHA256 {
                        HStack {
                            Text(String(version.prefix(12)) + "…").sirsiFont(.caption, design: .monospaced)
                            Spacer()
                            Button("Roll back") { confirmation = .rollback(version) }
                                .disabled(state.activeModel != nil || !state.runtimeCatalog.rollbackAvailable)
                            Button("Remove", role: .destructive) { Task { await model.removeCatalog(version: version) } }
                                .disabled(state.activeModel != nil)
                        }
                    }
                }
                Text(state.lifecycleToolsReady ? (state.lifecycleToolsStatus ?? "Lifecycle tools ready") : "Lifecycle tools unavailable")
                    .sirsiFont(.caption).foregroundStyle(state.lifecycleToolsReady ? Color.secondary : Color.red)
                    .accessibilityLabel("SNE lifecycle tools, \(state.lifecycleToolsReady ? "ready" : "unavailable"). \(state.lifecycleToolsStatus ?? "")")
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .accessibilityIdentifier("sne.runtime.catalog")
        }
    }
}

extension SNEReadViewState {
    static let snapshotFixture = SNEReadViewState(
        configured: true,
        ready: true,
        serviceState: "ready",
        activeModel: "gemma-4-12b-it-affine-8",
        deviceFamily: "Apple M5 Max",
        unifiedMemoryBytes: 48 << 30,
        catalog: [
            SNECatalogViewItem(
                catalogEntry: "gemma4-12b-affine8-mtp", modelID: "gemma-4-12b-it-affine-8",
                runtimeID: "sne-v2-api4096", parameterClass: "12B dense", executionMode: "mtp",
                weightFormat: "affine", weightBits: 8, memoryBytes: 24 << 30,
                supportStatus: "release-supported", nextGate: "complete", installed: true,
                active: true, actionLabel: "Running", actionKind: "start", actionEnabled: false,
                removalEnabled: false, removalReason: "Stop SNE before removal", reason: nil,
                licenseID: "Gemma Terms", licenseURL: "https://ai.google.dev/gemma/terms",
                licenseRequired: true, cacheTopology: "paged-ring-4096", servingCacheCapacity: 4096),
            SNECatalogViewItem(
                catalogEntry: "gemma4-31b-affine4", modelID: "gemma-4-31b-it-affine-4",
                runtimeID: "sne-v2-31b", parameterClass: "31B dense", executionMode: "plain",
                weightFormat: "affine", weightBits: 4, memoryBytes: 21 << 30,
                supportStatus: "qualified", nextGate: "clean-host", installed: false,
                active: false, actionLabel: "Install", actionKind: "install", actionEnabled: true,
                removalEnabled: false, removalReason: nil,
                reason: "Separately qualified device tuple; performance is not projected from M5.",
                licenseID: "Gemma Terms", licenseURL: "https://ai.google.dev/gemma/terms",
                licenseRequired: true, cacheTopology: "paged-ring-4096", servingCacheCapacity: 4096),
        ],
        runtimeCatalog: SNERuntimeCatalogViewState(
            state: "verified", versionSHA256: String(repeating: "a", count: 64),
            retainedVersions: [String(repeating: "a", count: 64), String(repeating: "b", count: 64)],
            rollbackAvailable: true, updateFeedConfigured: true),
        lifecycle: SNELifecycleViewState(
            modelID: "gemma-4-12b-it-affine-8", runtimeID: "sne-v2-api4096",
            runtimeSHA256: String(repeating: "c", count: 64),
            modelManifestSHA256: String(repeating: "d", count: 64), profile: "interactive",
            state: "ready", error: nil, errorCode: nil, recovery: nil, prefixCachePressure: nil),
        recovery: nil, lifecycleToolsReady: true,
        lifecycleToolsStatus: "Checkout, recovery, and removal tools verified")
}

private enum SNEConfirmation {
    case install(SNECatalogViewItem)
    case remove(SNECatalogViewItem)
    case rollback(String)
    case prefixCachePressure(SNEPrefixCachePressureViewState)

    var title: String {
        switch self {
        case .install: return "Install verified local model?"
        case .remove: return "Remove installed model?"
        case .rollback: return "Roll back signed catalog?"
        case .prefixCachePressure: return "Authorize SNE prefix-cache evaluation?"
        }
    }

    var button: String {
        switch self {
        case .install: return "Accept license and install"
        case .remove: return "Remove model"
        case .rollback: return "Roll back"
        case .prefixCachePressure: return "Authorize evaluation"
        }
    }

    var destructive: Bool {
        if case .remove = self { return true }
        return false
    }

    var message: String {
        switch self {
        case .install(let item):
            return "Accept \(item.licenseID ?? "the displayed license") and install \(item.modelID). Pantheon records acceptance in the verified checkout receipt."
        case .remove:
            return "Shared objects used by another installed model are retained. The model can be installed again later."
        case .rollback:
            return "SNE must be stopped. The selected signed version becomes active atomically."
        case .prefixCachePressure(let view):
            return "Authorize SNE to calculate its own bounded cache action for request \(view.receipt.observation.requestID). Pantheon does not execute, retry, or retain SNE cache data."
        }
    }
}
