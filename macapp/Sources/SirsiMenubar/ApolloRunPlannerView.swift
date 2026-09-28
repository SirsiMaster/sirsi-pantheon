import SwiftUI
import Foundation

// ApolloRunPlannerView is Stack Lab's local inference selector. It consumes
// typed Go observations; it does not discover models with a shell transcript or
// manufacture performance figures. Creating a plan is intentionally
// non-mutating: SNE separately admits execution against live pressure.
struct ApolloRunPlannerView: View {
    @ObservedObject var engine: SirsiEngine
    @State private var catalog: ApolloCatalog?
    @State private var loading = true
    @State private var error: String?
    @State private var selectedEngine = ""
    @State private var selectedMachine = "this-mac"
    @State private var selectedCores = 1
    @State private var selectedMemoryGiB = 1
    @State private var selectedSwapGiB = 0
    @State private var selectedEstates = Set<String>()
    @State private var plan: ApolloPlan?
    @State private var planning = false
    @State private var planError: String?

    var body: some View {
        VStack(spacing: 0) {
            BackBar(title: "Apollo plan")
            Group {
                if loading {
                    ProgressView("Reading this Mac’s local capacity…")
                        .frame(maxWidth: .infinity, maxHeight: .infinity)
                } else if let error {
                    recovery(error)
                } else if let catalog {
                    planner(catalog)
                }
            }
        }
        .task { await load() }
        .navigationTitle("Stack Lab — Apollo")
    }

    @ViewBuilder private func recovery(_ message: String) -> some View {
        VStack(alignment: .leading, spacing: 12) {
            Image(systemName: "antenna.radiowaves.left.and.right.slash")
                .sirsiFont(24, weight: .semibold).foregroundStyle(.orange)
            Text("Apollo needs a measured capacity read")
                .sirsiFont(.headline)
            Text(message)
                .sirsiFont(.subheadline).foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
            Button("Read capacity again") { Task { await load() } }
                .buttonStyle(.borderedProminent).tint(gold)
            NavLink { MaatWorkspaceView(engine: engine) } label: {
                Label("Open Ma'at evidence", systemImage: "checkmark.seal")
            }
            .buttonStyle(.bordered)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .leading)
        .padding(20)
    }

    private func planner(_ catalog: ApolloCatalog) -> some View {
        MaybeScroll {
            VStack(alignment: .leading, spacing: 16) {
                header(catalog)
                machinePicker(catalog)
                enginePicker(catalog)
                resourceEnvelope(catalog)
                estatePicker(catalog)
                planAction(catalog)
                if let plan { planReady(plan) }
            }
            .padding(16)
        }
    }

    private func header(_ catalog: ApolloCatalog) -> some View {
        VStack(alignment: .leading, spacing: 7) {
            Text("Plan a local Apollo run")
                .sirsiFont(.title3, weight: .bold)
            Text("Choose the resident route and the resource envelope before SNE is asked to admit inference. Stack Lab writes no device state at this stage.")
                .sirsiFont(.subheadline).foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
            HStack(spacing: 7) {
                fact("Machine", catalog.machine.name)
                fact("CPU", "\(catalog.machine.cpuCores) cores")
                fact("Memory", byteLabel(catalog.machine.memoryBytes))
            }
        }
        .padding(14)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(RoundedRectangle(cornerRadius: 12).fill(Color.primary.opacity(0.05)))
    }

    private func enginePicker(_ catalog: ApolloCatalog) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            Label("Resident inference route", systemImage: "cpu")
                .sirsiFont(.headline)
            Picker("Inference engine", selection: $selectedEngine) {
                ForEach(catalog.engines) { option in
                    Text(option.name).tag(option.id)
                }
            }
            .labelsHidden()
            .pickerStyle(.menu)
            if let engine = catalog.engines.first(where: { $0.id == selectedEngine }) {
                Text(engine.state == "configured" ? engineDetail(engine) : "This route is not configured on this Mac. Configure an SNE local endpoint, then refresh this screen.")
                    .sirsiFont(.subheadline)
                    .foregroundStyle(engine.state == "configured" ? .secondary : .orange)
                    .fixedSize(horizontal: false, vertical: true)
            }
        }
        .padding(14)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(RoundedRectangle(cornerRadius: 12).fill(Color.primary.opacity(0.05)))
    }

    private func machinePicker(_ catalog: ApolloCatalog) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            Label("Machine", systemImage: "laptopcomputer")
                .sirsiFont(.headline)
            Picker("Machine", selection: $selectedMachine) {
                Text(catalog.machine.name).tag(catalog.machine.id)
            }
            .labelsHidden()
            .pickerStyle(.menu)
            .disabled(true)
            Text("This Mac is the only machine with a measured Apollo capability receipt. Ra/Hermes machines appear here only after they publish the same typed capacity record; Pantheon will not invent remote capacity.")
                .sirsiFont(.subheadline).foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
        }
        .padding(14)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(RoundedRectangle(cornerRadius: 12).fill(Color.primary.opacity(0.05)))
    }

    private func resourceEnvelope(_ catalog: ApolloCatalog) -> some View {
        VStack(alignment: .leading, spacing: 10) {
            Label("Resource envelope", systemImage: "slider.horizontal.3")
                .sirsiFont(.headline)
            Stepper(value: $selectedCores, in: 1...max(1, catalog.machine.cpuCores)) {
                resourceLine("CPU allocation", "\(selectedCores) of \(catalog.machine.cpuCores) cores")
            }
            Stepper(value: $selectedMemoryGiB, in: 1...memoryCapacityGiB(catalog)) {
                resourceLine("Unified memory", "\(selectedMemoryGiB) GiB of \(memoryCapacityGiB(catalog)) GiB installed")
            }
            Stepper(value: $selectedSwapGiB, in: 0...memoryCapacityGiB(catalog)) {
                resourceLine("Swap ceiling", "\(selectedSwapGiB) GiB requested")
            }
            Text("A swap ceiling is a request, not reserved capacity. Apollo rechecks current pressure and swap before it starts inference.")
                .sirsiFont(.caption).foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
        }
        .padding(14)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(RoundedRectangle(cornerRadius: 12).fill(Color.primary.opacity(0.05)))
    }

    private func estatePicker(_ catalog: ApolloCatalog) -> some View {
        VStack(alignment: .leading, spacing: 9) {
            Label("Chip estates", systemImage: "square.grid.2x2")
                .sirsiFont(.headline)
            Text("Select every estate Apollo may observe and use. Unavailable estates stay visible and cannot be selected.")
                .sirsiFont(.subheadline).foregroundStyle(.secondary)
            ForEach(catalog.estates) { estate in
                Toggle(isOn: binding(for: estate)) {
                    VStack(alignment: .leading, spacing: 2) {
                        Text(estate.name).sirsiFont(.subheadline, weight: .semibold)
                        Text(estate.description).sirsiFont(.caption).foregroundStyle(.secondary)
                    }
                }
                .toggleStyle(.checkbox)
                .disabled(!estate.available)
                .opacity(estate.available ? 1 : 0.55)
            }
        }
        .padding(14)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(RoundedRectangle(cornerRadius: 12).fill(Color.primary.opacity(0.05)))
    }

    private func planAction(_ catalog: ApolloCatalog) -> some View {
        VStack(alignment: .leading, spacing: 9) {
            Button {
                Task { await createPlan(catalog) }
            } label: {
                Label(planning ? "Validating plan…" : "Create Apollo run plan", systemImage: "checkmark.circle")
            }
            .buttonStyle(.borderedProminent).tint(gold)
            .disabled(planning || selectedEstates.isEmpty || selectedEngine.isEmpty)
            if selectedEstates.isEmpty {
                Text("Choose at least one available chip estate to continue.")
                    .sirsiFont(.caption, weight: .semibold).foregroundStyle(.orange)
            }
            if let planError {
                Text(planError).sirsiFont(.caption, weight: .semibold).foregroundStyle(.red)
                    .fixedSize(horizontal: false, vertical: true)
            }
        }
    }

    private func planReady(_ plan: ApolloPlan) -> some View {
        VStack(alignment: .leading, spacing: 9) {
            Label("Run plan is ready", systemImage: "checkmark.seal.fill")
                .sirsiFont(.headline).foregroundStyle(.green)
            Text("\(plan.cpuCores) cores · \(byteLabel(plan.memoryBytes)) memory · \(byteLabel(plan.swapBytes)) swap ceiling")
                .sirsiFont(.subheadline).foregroundStyle(.secondary)
            Text("SNE must still admit execution against current system pressure. Opening Apollo now shows live telemetry when a session is active.")
                .sirsiFont(.caption).foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
            NavLink { ApolloTelemetryView(engine: engine, plan: plan) } label: {
                Label("Open Apollo telemetry", systemImage: "waveform.path.ecg")
            }
            .buttonStyle(.borderedProminent).tint(gold)
        }
        .padding(14)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(RoundedRectangle(cornerRadius: 12).fill(Color.green.opacity(0.08)))
    }

    private func binding(for estate: ApolloChipEstate) -> Binding<Bool> {
        Binding(get: { selectedEstates.contains(estate.id) }, set: { selected in
            if selected { selectedEstates.insert(estate.id) } else { selectedEstates.remove(estate.id) }
        })
    }

    private func fact(_ label: String, _ value: String) -> some View {
        VStack(alignment: .leading, spacing: 2) {
            Text(label).sirsiFont(.caption).foregroundStyle(.secondary)
            Text(value).sirsiFont(.caption, weight: .semibold).lineLimit(1)
        }
        .padding(.horizontal, 8).padding(.vertical, 6)
        .background(Capsule().fill(Color.primary.opacity(0.07)))
    }

    private func resourceLine(_ title: String, _ detail: String) -> some View {
        HStack { Text(title).sirsiFont(.subheadline, weight: .semibold); Spacer(); Text(detail).sirsiFont(.caption).foregroundStyle(.secondary) }
    }

    private func engineDetail(_ engine: ApolloEngineOption) -> String {
        let model = engine.residentModel?.isEmpty == false ? engine.residentModel! : "model reported by SNE at session start"
        let route = engine.endpoint?.isEmpty == false ? " · \(engine.endpoint!)" : ""
        return "\(engine.provider) · \(model)\(route)"
    }

    private func memoryCapacityGiB(_ catalog: ApolloCatalog) -> Int { max(1, Int(catalog.machine.memoryBytes / 1_073_741_824)) }
    private func byteLabel(_ bytes: Int64) -> String { bytes == 0 ? "0 GiB" : String(format: "%.0f GiB", Double(bytes) / 1_073_741_824) }

    @MainActor private func load() async {
        loading = true; error = nil; plan = nil
        async let catalogData = SirsiEngine.runJSON(args: ["apollo", "catalog", "--json"])
        async let vitals: Void = engine.fetchVitals()
        async let board: Void = engine.loadRouterBoard()
        let data = await catalogData
        _ = await (vitals, board)
        guard let decoded = try? JSONDecoder().decode(ApolloCatalog.self, from: data) else {
            error = "Pantheon could not read a typed Apollo capacity catalog. No engine, machine, or resource limits were inferred. Retry the read or inspect Ma'at evidence."
            loading = false; return
        }
        catalog = decoded
        selectedMachine = decoded.machine.id
        selectedEngine = decoded.engines.first(where: { $0.state == "configured" })?.id ?? decoded.engines.first?.id ?? ""
        selectedCores = max(1, min(decoded.machine.cpuCores, max(1, decoded.machine.cpuCores / 2)))
        selectedMemoryGiB = max(1, min(memoryCapacityGiB(decoded), max(1, memoryCapacityGiB(decoded) / 2)))
        selectedSwapGiB = 0
        selectedEstates = Set(decoded.estates.filter(\.available).map(\.id))
        loading = false
    }

    @MainActor private func createPlan(_ catalog: ApolloCatalog) async {
        planning = true; planError = nil; plan = nil
        let estates = selectedEstates.sorted().joined(separator: ",")
        let data = await SirsiEngine.runJSON(args: ["apollo", "plan", "--engine", selectedEngine, "--cores", "\(selectedCores)", "--memory-gib", "\(selectedMemoryGiB)", "--swap-gib", "\(selectedSwapGiB)", "--estates", estates, "--json"])
        if let decoded = try? JSONDecoder().decode(ApolloPlan.self, from: data) {
            plan = decoded
        } else {
            planError = SirsiEngine.firstMeaningful(String(data: data, encoding: .utf8) ?? "")
            if planError?.isEmpty != false { planError = "Apollo rejected the selected plan. Adjust the resource envelope and retry." }
        }
        planning = false
    }
}

// ApolloTelemetryView renders actual observations from the conduit and Vitals.
// Throughput/network/residency are consciously unavailable until an Apollo/SNE
// session publishes them; a blank measurement is safer than a synthetic zero.
struct ApolloTelemetryView: View {
    @ObservedObject var engine: SirsiEngine
    let plan: ApolloPlan

    var body: some View {
        VStack(spacing: 0) {
            BackBar(title: "Apollo telemetry")
            MaybeScroll {
                VStack(alignment: .leading, spacing: 16) {
                    sessionSummary
                    telemetryGrid
                    estateSummary
                    evidenceNote
                }
                .padding(16)
            }
        }
        .task { await refresh() }
        .navigationTitle("Apollo telemetry")
    }

    private var sessionSummary: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text(engine.localLLM?.healthy == true ? "Apollo local route is online" : "No active Apollo session")
                .sirsiFont(.title3, weight: .bold)
                .foregroundStyle(engine.localLLM?.healthy == true ? .green : .orange)
            Text(engine.localLLM?.healthy == true ? "The local SNE conduit is reachable. Metrics below update when Apollo publishes a session sample." : "The selected plan is saved in this screen only. Ask SNE to admit a run, then return here for live session telemetry.")
                .sirsiFont(.subheadline).foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
            Button("Refresh telemetry") { Task { await refresh() } }
                .buttonStyle(.bordered).tint(gold)
        }
        .padding(14).frame(maxWidth: .infinity, alignment: .leading)
        .background(RoundedRectangle(cornerRadius: 12).fill(Color.primary.opacity(0.05)))
    }

    private var telemetryGrid: some View {
        VStack(alignment: .leading, spacing: 8) {
            Label("Live estate telemetry", systemImage: "waveform.path.ecg")
                .sirsiFont(.headline)
            LazyVGrid(columns: [GridItem(.flexible()), GridItem(.flexible())], spacing: 8) {
                telemetry("Tokens / second", unavailable)
                telemetry("Bandwidth", unavailable)
                telemetry("Memory", memoryTelemetry)
                telemetry("Network saturation", unavailable)
                telemetry("GPU residency", unavailable)
                telemetry("CPU residency", unavailable)
            }
        }
    }

    private var estateSummary: some View {
        VStack(alignment: .leading, spacing: 8) {
            Label("Selected chip estates", systemImage: "square.grid.2x2")
                .sirsiFont(.headline)
            Text(plan.chipEstates.joined(separator: " · "))
                .sirsiFont(.subheadline, weight: .semibold)
            Text("Requested: \(plan.cpuCores) cores · \(byteLabel(plan.memoryBytes)) memory · \(byteLabel(plan.swapBytes)) swap ceiling")
                .sirsiFont(.caption).foregroundStyle(.secondary)
        }
        .padding(14).frame(maxWidth: .infinity, alignment: .leading)
        .background(RoundedRectangle(cornerRadius: 12).fill(Color.primary.opacity(0.05)))
    }

    private var evidenceNote: some View {
        Text("Measured now: memory and swap come from Sirsi Vitals; the conduit reports local model state. Tokens, bandwidth, network saturation, and CPU/GPU residency appear only after Apollo/SNE emits an authenticated session sample. They are not estimated from the chosen plan.")
            .sirsiFont(.caption).foregroundStyle(.secondary)
            .fixedSize(horizontal: false, vertical: true)
    }

    private var unavailable: String { "Awaiting session" }
    private var memoryTelemetry: String {
        guard let vitals = engine.vitals else { return unavailable }
        return "\(byteLabel(vitals.usedBytes)) used · \(byteLabel(vitals.swapUsedBytes)) swap"
    }
    private func telemetry(_ title: String, _ value: String) -> some View {
        VStack(alignment: .leading, spacing: 4) {
            Text(title).sirsiFont(.caption).foregroundStyle(.secondary)
            Text(value).sirsiFont(.subheadline, weight: .semibold).lineLimit(2)
        }
        .frame(maxWidth: .infinity, minHeight: 54, alignment: .leading)
        .padding(10).background(RoundedRectangle(cornerRadius: 10).fill(Color.primary.opacity(0.05)))
    }
    private func byteLabel(_ bytes: Int64) -> String { bytes == 0 ? "0 GiB" : String(format: "%.1f GiB", Double(bytes) / 1_073_741_824) }
    private func refresh() async { async let a: Void = engine.fetchVitals(); async let b: Void = engine.loadRouterBoard(); _ = await (a, b) }
}

private struct ApolloCatalog: Decodable {
    let machine: ApolloMachine
    let engines: [ApolloEngineOption]
    let estates: [ApolloChipEstate]
}
private struct ApolloMachine: Decodable { let id: String; let name: String; let cpuCores: Int; let memoryBytes: Int64; enum CodingKeys: String, CodingKey { case id, name; case cpuCores = "cpu_cores"; case memoryBytes = "memory_bytes" } }
private struct ApolloEngineOption: Decodable, Identifiable { let id: String; let name: String; let provider: String; let residentModel: String?; let endpoint: String?; let state: String; enum CodingKeys: String, CodingKey { case id, name, provider, endpoint, state; case residentModel = "resident_model" } }
private struct ApolloChipEstate: Decodable, Identifiable { let id: String; let name: String; let available: Bool; let description: String }
struct ApolloPlan: Decodable { let cpuCores: Int; let memoryBytes: Int64; let swapBytes: Int64; let chipEstates: [String]; enum CodingKeys: String, CodingKey { case cpuCores = "cpu_cores"; case memoryBytes = "memory_bytes"; case swapBytes = "swap_bytes"; case chipEstates = "chip_estates" } }
