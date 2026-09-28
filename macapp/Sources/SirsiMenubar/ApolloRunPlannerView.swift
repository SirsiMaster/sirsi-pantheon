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
		let machine = selectedMachineDescriptor(catalog)
        return VStack(alignment: .leading, spacing: 7) {
            Text("Plan a local Apollo run")
                .sirsiFont(.title3, weight: .bold)
            Text("Choose the resident route and the resource envelope before SNE is asked to admit inference. Stack Lab writes no device state at this stage.")
                .sirsiFont(.subheadline).foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
            HStack(spacing: 7) {
                fact("Machine", machine.name)
                fact("CPU", "\(machine.cpuCores) cores")
                fact("Memory", byteLabel(machine.memoryBytes))
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
                ForEach(engineOptions(catalog)) { option in
                    Text(option.name).tag(option.id)
                }
            }
            .labelsHidden()
            .pickerStyle(.menu)
            if let engine = engineOptions(catalog).first(where: { $0.id == selectedEngine }) {
                Text(engine.state == "configured" ? engineDetail(engine) : "This route is not configured on the selected machine. Configure its SNE endpoint, then refresh this screen.")
                    .sirsiFont(.subheadline)
                    .foregroundStyle(engine.state == "configured" ? Color.secondary : Color.orange)
                    .fixedSize(horizontal: false, vertical: true)
            } else {
                Text("No resident inference route has a typed receipt for this machine. Choose another measured machine or add an SNE-qualified route.")
                    .sirsiFont(.subheadline).foregroundStyle(.orange)
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
                ForEach(catalog.machineOptions) { machine in
                    Text("\(machine.name) · \(machine.cpuCores) cores · \(byteLabel(machine.memoryBytes))")
                        .tag(machine.id)
                }
            }
            .labelsHidden()
            .pickerStyle(.menu)
            .onChange(of: selectedMachine) { _ in resetSelections(catalog) }
            Text("Each selectable entry has a typed capacity receipt. Ra/Hermes peers appear only after they publish the same record; Pantheon will not invent remote capacity.")
                .sirsiFont(.subheadline).foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
        }
        .padding(14)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(RoundedRectangle(cornerRadius: 12).fill(Color.primary.opacity(0.05)))
    }

    private func resourceEnvelope(_ catalog: ApolloCatalog) -> some View {
		let machine = selectedMachineDescriptor(catalog)
        return VStack(alignment: .leading, spacing: 10) {
            Label("Resource envelope", systemImage: "slider.horizontal.3")
                .sirsiFont(.headline)
            Stepper(value: $selectedCores, in: 1...max(1, machine.cpuCores)) {
                resourceLine("CPU allocation", "\(selectedCores) of \(machine.cpuCores) cores")
            }
            Stepper(value: $selectedMemoryGiB, in: 1...memoryCapacityGiB(machine)) {
                resourceLine("Unified memory", "\(selectedMemoryGiB) GiB of \(memoryCapacityGiB(machine)) GiB installed")
            }
            Stepper(value: $selectedSwapGiB, in: 0...memoryCapacityGiB(machine)) {
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
            ForEach(estateOptions(catalog)) { estate in
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

    private func selectedMachineDescriptor(_ catalog: ApolloCatalog) -> ApolloMachine {
        catalog.machineOptions.first(where: { $0.id == selectedMachine }) ?? catalog.machine
    }
    private func engineOptions(_ catalog: ApolloCatalog) -> [ApolloEngineOption] {
        catalog.engines.filter { $0.machineID == nil || $0.machineID == selectedMachine }
    }
    private func estateOptions(_ catalog: ApolloCatalog) -> [ApolloChipEstate] {
        let machineEstates = Set(selectedMachineDescriptor(catalog).chipEstates ?? [])
        return catalog.estates.filter { machineEstates.isEmpty || machineEstates.contains($0.id) }
    }
    private func resetSelections(_ catalog: ApolloCatalog) {
        selectedEngine = engineOptions(catalog).first(where: { $0.state == "configured" })?.id ?? engineOptions(catalog).first?.id ?? ""
        selectedEstates = Set(estateOptions(catalog).filter(\.available).map(\.id))
        resetEnvelope(catalog)
    }
    private func resetEnvelope(_ catalog: ApolloCatalog) {
        let machine = selectedMachineDescriptor(catalog)
        selectedCores = max(1, min(machine.cpuCores, max(1, machine.cpuCores / 2)))
        selectedMemoryGiB = max(1, min(memoryCapacityGiB(machine), max(1, memoryCapacityGiB(machine) / 2)))
        selectedSwapGiB = 0
        plan = nil
        planError = nil
    }
    private func memoryCapacityGiB(_ machine: ApolloMachine) -> Int { max(1, Int(machine.memoryBytes / 1_073_741_824)) }
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
        selectedMachine = decoded.machineOptions.first?.id ?? decoded.machine.id
        resetSelections(decoded)
        selectedSwapGiB = 0
        loading = false
    }

    @MainActor private func createPlan(_ catalog: ApolloCatalog) async {
        planning = true; planError = nil; plan = nil
        let estates = selectedEstates.sorted().joined(separator: ",")
        let data = await SirsiEngine.runJSON(args: ["apollo", "plan", "--machine", selectedMachine, "--engine", selectedEngine, "--cores", "\(selectedCores)", "--memory-gib", "\(selectedMemoryGiB)", "--swap-gib", "\(selectedSwapGiB)", "--estates", estates, "--json"])
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
    @State private var session: ApolloTelemetryRead?
    @State private var telemetryError: String?

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
            Text(session?.state == "active" ? "Apollo session is active" : (engine.localLLM?.healthy == true ? "Apollo local route is online" : "No active Apollo session"))
                .sirsiFont(.title3, weight: .bold)
                .foregroundStyle(session?.state == "active" || engine.localLLM?.healthy == true ? .green : .orange)
            Text(session?.state == "active" ? "SNE published a bounded session sample for this screen." : (engine.localLLM?.healthy == true ? "The local SNE conduit is reachable. Metrics below update when Apollo publishes a session sample." : "The selected plan is saved in this screen only. Ask SNE to admit a run, then return here for live session telemetry."))
                .sirsiFont(.subheadline).foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
            Button("Refresh telemetry") { Task { await refresh() } }
                .buttonStyle(.bordered).tint(gold)
            if let telemetryError {
                Text(telemetryError).sirsiFont(.caption, weight: .semibold).foregroundStyle(.orange)
                    .fixedSize(horizontal: false, vertical: true)
            }
        }
        .padding(14).frame(maxWidth: .infinity, alignment: .leading)
        .background(RoundedRectangle(cornerRadius: 12).fill(Color.primary.opacity(0.05)))
    }

    private var telemetryGrid: some View {
        VStack(alignment: .leading, spacing: 8) {
            Label("Live estate telemetry", systemImage: "waveform.path.ecg")
                .sirsiFont(.headline)
            LazyVGrid(columns: [GridItem(.flexible()), GridItem(.flexible())], spacing: 8) {
                telemetry("Tokens / second", tokensTelemetry)
                telemetry("Bandwidth", bandwidthTelemetry)
                telemetry("Memory", memoryTelemetry)
                telemetry("Network saturation", networkTelemetry)
                telemetry("GPU residency", gpuTelemetry)
                telemetry("CPU residency", cpuTelemetry)
            }
        }
    }

    private var estateSummary: some View {
        VStack(alignment: .leading, spacing: 8) {
            Label("Selected chip estates", systemImage: "square.grid.2x2")
                .sirsiFont(.headline)
            ForEach(plan.chipEstates, id: \.self) { estate in
                HStack {
                    Text(estate.replacingOccurrences(of: "-", with: " ").capitalized).sirsiFont(.subheadline, weight: .semibold)
                    Spacer()
                    Text(estateTelemetry(estate)).sirsiFont(.caption).foregroundStyle(.secondary)
                }
            }
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
    private var tokensTelemetry: String { metric(session?.telemetry?.tokensPerSec, suffix: " tok/s", precision: 1) }
    private var bandwidthTelemetry: String {
        guard let bytes = session?.telemetry?.bandwidthBps else { return unavailable }
        return "\(byteLabel(bytes))/s"
    }
    private var networkTelemetry: String { metric(session?.telemetry?.networkPct, suffix: "%", precision: 1) }
    private var gpuTelemetry: String { metric(session?.telemetry?.gpuResidency, suffix: "%", precision: 1) }
    private var cpuTelemetry: String { metric(session?.telemetry?.cpuResidency, suffix: "%", precision: 1) }
    private var memoryTelemetry: String {
        if let bytes = session?.telemetry?.memoryBytes { return byteLabel(bytes) + " session" }
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
    private func metric(_ value: Double?, suffix: String, precision: Int) -> String {
        guard let value else { return unavailable }
        return String(format: "%.*f%@", precision, value, suffix)
    }
    private func estateTelemetry(_ id: String) -> String {
        guard let estate = session?.telemetry?.estates.first(where: { $0.id == id }) else { return unavailable }
        var parts: [String] = []
        if let utilization = estate.utilizationPct { parts.append(String(format: "%.1f%% util", utilization)) }
        if let residency = estate.residencyPct { parts.append(String(format: "%.1f%% resident", residency)) }
        if let memory = estate.memoryBytes { parts.append(byteLabel(memory)) }
        return parts.isEmpty ? unavailable : parts.joined(separator: " · ")
    }
    private func byteLabel(_ bytes: Int64) -> String { bytes == 0 ? "0 GiB" : String(format: "%.1f GiB", Double(bytes) / 1_073_741_824) }
    @MainActor private func refresh() async {
        telemetryError = nil
        async let a: Void = engine.fetchVitals()
        async let b: Void = engine.loadRouterBoard()
        async let telemetryData = SirsiEngine.runJSON(args: ["apollo", "telemetry", "--json"])
        let data = await telemetryData
        _ = await (a, b)
        if let read = try? JSONDecoder().decode(ApolloTelemetryRead.self, from: data) {
            session = read
        } else {
            session = nil
            telemetryError = "Pantheon could not decode an Apollo session sample. It was not treated as active telemetry."
        }
    }
}

private struct ApolloCatalog: Decodable {
    let machine: ApolloMachine
	let machines: [ApolloMachine]?
    let engines: [ApolloEngineOption]
    let estates: [ApolloChipEstate]
	var machineOptions: [ApolloMachine] { machines?.isEmpty == false ? machines! : [machine] }
}
private struct ApolloMachine: Decodable, Identifiable { let id: String; let name: String; let cpuCores: Int; let memoryBytes: Int64; let chipEstates: [String]?; enum CodingKeys: String, CodingKey { case id, name; case cpuCores = "cpu_cores"; case memoryBytes = "memory_bytes"; case chipEstates = "chip_estates" } }
private struct ApolloEngineOption: Decodable, Identifiable { let id: String; let machineID: String?; let name: String; let provider: String; let residentModel: String?; let endpoint: String?; let state: String; enum CodingKeys: String, CodingKey { case id, name, provider, endpoint, state; case machineID = "machine_id"; case residentModel = "resident_model" } }
private struct ApolloChipEstate: Decodable, Identifiable { let id: String; let name: String; let available: Bool; let description: String }
struct ApolloPlan: Decodable { let cpuCores: Int; let memoryBytes: Int64; let swapBytes: Int64; let chipEstates: [String]; enum CodingKeys: String, CodingKey { case cpuCores = "cpu_cores"; case memoryBytes = "memory_bytes"; case swapBytes = "swap_bytes"; case chipEstates = "chip_estates" } }
private struct ApolloTelemetryRead: Decodable { let state: String; let telemetry: ApolloSessionTelemetry?; let reason: String? }
private struct ApolloSessionTelemetry: Decodable { let tokensPerSec: Double?; let bandwidthBps: Int64?; let memoryBytes: Int64?; let networkPct: Double?; let cpuResidency: Double?; let gpuResidency: Double?; let estates: [ApolloEstateTelemetry]; enum CodingKeys: String, CodingKey { case tokensPerSec = "tokens_per_second"; case bandwidthBps = "bandwidth_bytes_per_second"; case memoryBytes = "memory_bytes"; case networkPct = "network_saturation_percent"; case cpuResidency = "cpu_residency_percent"; case gpuResidency = "gpu_residency_percent"; case estates = "chip_estates" } }
private struct ApolloEstateTelemetry: Decodable { let id: String; let residencyPct: Double?; let memoryBytes: Int64?; let utilizationPct: Double?; enum CodingKeys: String, CodingKey { case id; case residencyPct = "residency_percent"; case memoryBytes = "memory_bytes"; case utilizationPct = "utilization_percent" } }
