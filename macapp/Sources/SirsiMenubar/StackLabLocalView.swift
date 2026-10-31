import SwiftUI

struct StackLabLocalState: Decodable, Equatable {
    let schema: String
    let enabled: Bool
    let provider: String
    var valid: Bool { schema == "sirsi.stacklab.local.v1" && ["local", "frontier", "self-hosted"].contains(provider) }
}

struct StackLabLocalCatalog: Decodable {
    struct Entry: Decodable, Identifiable { let id: String; let kind: String; let purpose: String? }
    let entries: [Entry]
    let unknown: [String]?
}

@MainActor final class StackLabLocalModel: ObservableObject {
    @Published private(set) var state: StackLabLocalState?
    @Published private(set) var catalog: StackLabLocalCatalog?
    @Published private(set) var error: String?
    @Published private(set) var busy = false
    private let read: @MainActor ([String]) async -> SirsiEngine.JSONCommandRead
    init(read: @escaping @MainActor ([String]) async -> SirsiEngine.JSONCommandRead = { await SirsiEngine.runJSONRead(args: $0) }) { self.read = read }
    func inspect() async { await stateCommand(["inspect"]) }
    func setEnabled(_ enabled: Bool, provider: String) async {
        guard ["local", "frontier", "self-hosted"].contains(provider) else { error = "Unsupported provider kind. No setting was changed."; return }
        await stateCommand(enabled ? ["enable", "--provider", provider] : ["disable"])
    }
    private func stateCommand(_ operation: [String]) async {
        guard !busy else { return }
        busy = true
        defer { busy = false }
        let result = await read(["stacklab", "local"] + operation + ["--json"])
        guard !Task.isCancelled else { return }
        guard result.exitStatus == 0, !result.timedOut, result.launchError == nil,
              let value = try? JSONDecoder().decode(StackLabLocalState.self, from: result.data), value.valid else {
            error = "Local preferences could not be verified. No enablement or provider change is claimed. Inspect again; retained prior state is not a fresh result."
            return
        }
        state = value; error = nil
        if !value.enabled { catalog = nil }
    }
    func inspectRecipes() async {
        guard !busy, state?.enabled == true, error == nil else { return }
        busy = true; defer { busy = false }
        let result = await read(["stacklab", "local", "catalog", "--json"])
        guard !Task.isCancelled else { return }
        guard result.exitStatus == 0, !result.timedOut, result.launchError == nil,
              let value = try? JSONDecoder().decode(StackLabLocalCatalog.self, from: result.data) else {
            error = "Installed recipes could not be read. Inspect again or open the release workspace to check the installed payload."
            return
        }
        catalog = value; error = nil
    }
}

// The normal root is local and off by default. Fabric doctor stays available as
// a deliberate separate navigation, never an automatic request on opening this.
struct StackLabView: View {
    @ObservedObject var engine: SirsiEngine
    var body: some View { StackLabLocalFrame(engine: engine, model: engine.localStackLab) }
}

private struct StackLabLocalFrame: View {
    @ObservedObject var engine: SirsiEngine
    @ObservedObject var model: StackLabLocalModel
    @State private var provider = "local"
    var body: some View {
        MaybeScroll {
        VStack(alignment: .leading, spacing: 14) {
            BackBar(title: "Stack Lab")
            Text("Local workspace").sirsiFont(.title2, weight: .semibold)
            Text(model.state.map { (model.error == nil ? "" : "Last observed: ") + ($0.enabled ? "Enabled on this Mac" : "Off on this Mac") } ?? "Local state has not been verified")
                .sirsiFont(.headline)
            Text("Stack Lab ships with Pantheon and stays off until you enable it. Provider kind organizes local recipes and evidence; selecting one never invokes a model or connects an account.")
                .sirsiFont(.subheadline).foregroundStyle(.secondary)
            if let error = model.error { Text(error).sirsiFont(.subheadline).foregroundStyle(.orange) }
            Picker("Provider kind", selection: $provider) {
                Text("Local").tag("local")
                Text("Frontier").tag("frontier")
                Text("Self-hosted").tag("self-hosted")
            }.disabled(model.busy)
            HStack {
                Button("Inspect local state") { Task { await model.inspect(); provider = model.state?.provider ?? provider } }
                Button(model.state?.enabled == true ? "Save provider kind" : "Enable local Stack Lab") { Task { await model.setEnabled(true, provider: provider) } }
                    .disabled(model.busy || model.state == nil || model.error != nil)
                if model.state?.enabled == true {
                    Button("Disable") { Task { await model.setEnabled(false, provider: provider) } }.disabled(model.busy)
                }
            }.buttonStyle(.bordered)
            Button("Inspect installed recipes") { Task { await model.inspectRecipes() } }
                .buttonStyle(.bordered).disabled(model.busy || model.state?.enabled != true || model.error != nil)
            if model.busy { ProgressView("Reading local Stack Lab…") }
            if let catalog = model.catalog {
                Text(model.error == nil ? "Local structural projection · not remote pin or execution proof" : "Retained local catalogue · freshness unverified").sirsiFont(.caption).foregroundStyle(.secondary)
                ForEach(catalog.entries) { entry in
                    VStack(alignment: .leading) {
                        Text(entry.id).sirsiFont(.subheadline, weight: .semibold)
                        if let purpose = entry.purpose { Text(purpose).sirsiFont(.caption).foregroundStyle(.secondary) }
                    }
                }
                ForEach(catalog.unknown ?? [], id: \.self) { Text($0).sirsiFont(.caption).foregroundStyle(.orange) }
            }
            NavLink { StackLabDoctorView(engine: engine) } label: { Label("Fabric wing checks", systemImage: "network") }
            NavLink { PantheonReleaseWorkspaceView(engine: engine) } label: { Label("Open release workspace", systemImage: "shippingbox") }
            Spacer(minLength: 0)
        }
        .padding(20)
        .task { await model.inspect(); provider = model.state?.provider ?? "local" }
        }
    }
}
