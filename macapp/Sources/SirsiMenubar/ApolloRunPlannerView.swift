import SwiftUI
import Foundation

// ApolloRunPlannerView is Stack Lab's inference selector. It consumes
// typed Go observations; it does not discover models with a shell transcript or
// manufacture performance figures. Creating a plan is intentionally
// non-mutating: SNE separately admits execution against live pressure.
struct ApolloRunPlannerView: View {
    @ObservedObject var engine: SirsiEngine
    @Environment(\.snapshotMode) private var snapshotMode
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
    @State private var openTelemetry = false
    let preloadedCatalog: ApolloCatalog?

    init(engine: SirsiEngine, preloadedCatalog: ApolloCatalog? = nil) {
        self.engine = engine
        self.preloadedCatalog = preloadedCatalog
        _catalog = State(initialValue: preloadedCatalog)
        _loading = State(initialValue: preloadedCatalog == nil)
        let machine = preloadedCatalog?.machineOptions.first ?? preloadedCatalog?.machine
        let machineID = machine?.id ?? "this-mac"
        let engines = preloadedCatalog?.engines.filter { $0.machineID == nil || $0.machineID == machineID } ?? []
        let estates = preloadedCatalog?.estateOptions(for: machineID) ?? []
        _selectedMachine = State(initialValue: machineID)
        _selectedEngine = State(initialValue: engines.first(where: { $0.state == "configured" })?.id ?? engines.first?.id ?? "")
        _selectedCores = State(initialValue: max(1, (machine?.cpuCores ?? 1) / 2))
        _selectedMemoryGiB = State(initialValue: max(1, Int((machine?.memoryBytes ?? 1_073_741_824) / 1_073_741_824) / 2))
        _selectedEstates = State(initialValue: Set(estates.filter(\.available).map(\.id)))
    }

    var body: some View {
        VStack(spacing: 0) {
            BackBar(title: "Apollo plan")
            Group {
                if loading {
                    ProgressView("Reading the selected device capacity…")
                        .frame(maxWidth: .infinity, maxHeight: .infinity)
                } else if let error {
                    recovery(error)
                } else if let catalog {
                    planner(catalog)
                }
            }
        }
        .task {
            guard catalog == nil else { return }
            await load()
        }
        .navigationTitle("Stack Lab — Apollo")
        .navigationDestination(isPresented: $openTelemetry) {
            if let plan {
                ApolloTelemetryView(engine: engine, plan: plan)
            }
        }
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
                selectionPreview(catalog)
                planAction(catalog)
                if let plan { planReady(plan) }
            }
            .padding(16)
        }
    }

    private func header(_ catalog: ApolloCatalog) -> some View {
		let machine = selectedMachineDescriptor(catalog)
        return VStack(alignment: .leading, spacing: 7) {
            Text("Plan an Apollo run")
                .sirsiFont(.title3, weight: .bold)
            Text("Choose the resident route, machine, and resource envelope before SNE is asked to admit inference. Stack Lab writes no device state at this stage.")
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
            Label("Resident LLM & inference engine", systemImage: "cpu")
                .sirsiFont(.headline)
            if snapshotMode {
                snapshotSelection("Resident route", value: engineOptions(catalog).first(where: { $0.id == selectedEngine }).map(routeLabel) ?? "No qualified route")
            } else {
                Picker("Resident LLM and inference engine", selection: $selectedEngine) {
                    ForEach(engineOptions(catalog)) { option in
                        Text(routeLabel(option)).tag(option.id)
                    }
                }
                .labelsHidden()
                .pickerStyle(.menu)
            }
            if let engine = selectedRoute(catalog) {
                Text(engine.state == "configured" ? engineDetail(engine) : "This route has no configured resident model on the selected machine. Configure its SNE endpoint, then refresh this screen.")
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
            if snapshotMode {
                let machine = selectedMachineDescriptor(catalog)
                snapshotSelection("Machine", value: "\(machine.name) · \(machine.cpuCores) cores · \(byteLabel(machine.memoryBytes))")
            } else {
                Picker("Machine", selection: $selectedMachine) {
                    ForEach(catalog.machineOptions) { machine in
                        Text("\(machine.name) · \(machine.cpuCores) cores · \(byteLabel(machine.memoryBytes))")
                            .tag(machine.id)
                    }
                }
                .labelsHidden()
                .pickerStyle(.menu)
                .onChange(of: selectedMachine) { _ in resetSelections(catalog) }
            }
            Text("Each selectable entry has a typed capacity receipt. This device is immediately available; another Horus instance appears only after it publishes the same record through Ra/Hermes. Pantheon never guesses peer capacity.")
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
            HStack(spacing: 8) {
                capacityFact("Available CPU", "\(machine.cpuCores) cores")
                capacityFact("Available memory", byteLabel(machine.memoryBytes))
                capacityFact("Requested swap", "\(selectedSwapGiB) GiB")
            }
            if snapshotMode {
                resourceLine("CPU allocation", "\(selectedCores) of \(machine.cpuCores) cores")
                resourceLine("Unified memory", "\(selectedMemoryGiB) GiB of \(memoryCapacityGiB(machine)) GiB installed")
                resourceLine("Swap ceiling", "\(selectedSwapGiB) GiB requested")
            } else {
                Stepper(value: $selectedCores, in: 1...max(1, machine.cpuCores)) {
                    resourceLine("CPU allocation", "\(selectedCores) of \(machine.cpuCores) cores")
                }
                Stepper(value: $selectedMemoryGiB, in: 1...memoryCapacityGiB(machine)) {
                    resourceLine("Unified memory", "\(selectedMemoryGiB) GiB of \(memoryCapacityGiB(machine)) GiB installed")
                }
                Stepper(value: $selectedSwapGiB, in: 0...memoryCapacityGiB(machine)) {
                    resourceLine("Swap ceiling", "\(selectedSwapGiB) GiB requested")
                }
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
        let estates = estateOptions(catalog)
        return VStack(alignment: .leading, spacing: 9) {
            Label("Chip estates", systemImage: "square.grid.2x2")
                .sirsiFont(.headline)
            Text("Select every detected estate Apollo should observe or request. An estate that is not currently qualified remains selectable and is carried to SNE as an explicit pending request.")
                .sirsiFont(.subheadline).foregroundStyle(.secondary)
            if !snapshotMode && !estates.isEmpty {
                HStack(spacing: 8) {
                    Button("Select all detected") {
                        selectedEstates = Set(estates.map(\.id))
                    }
                    .buttonStyle(.bordered)
                    Button("Clear selection") {
                        selectedEstates.removeAll()
                    }
                    .buttonStyle(.bordered)
                }
            }
            SwiftUI.ForEach(estates, id: \.id) { estate in
                estateRow(estate)
            }
        }
        .padding(14)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(RoundedRectangle(cornerRadius: 12).fill(Color.primary.opacity(0.05)))
    }

    private func planAction(_ catalog: ApolloCatalog) -> some View {
        VStack(alignment: .leading, spacing: 9) {
            SnapshotActionButton(disabled: planning || planBlocker(catalog) != nil) {
                Task { await createPlan(catalog) }
            } label: {
                Label(planning ? "Validating selected recipe…" : "Validate recipe & open Apollo", systemImage: "play.circle")
            }
            Text("Validates the exact typed Stack Lab plan below, then transfers the same declaration to Apollo telemetry. SNE separately admits and starts inference against live capacity.")
                .sirsiFont(.caption)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
            if let blocker = planBlocker(catalog) {
                Text(blocker)
                    .sirsiFont(.caption, weight: .semibold).foregroundStyle(.orange)
                    .fixedSize(horizontal: false, vertical: true)
            }
            if let planError {
                Text(planError).sirsiFont(.caption, weight: .semibold).foregroundStyle(.red)
                    .fixedSize(horizontal: false, vertical: true)
            }
        }
    }

    // This is a reviewable declaration rather than a decorative dashboard.
    // Everything the operator is about to ask SNE to admit is visible in one
    // place before Pantheon calls the typed plan command.
    private func selectionPreview(_ catalog: ApolloCatalog) -> some View {
        let machine = selectedMachineDescriptor(catalog)
        let route = selectedRoute(catalog)
        let selectedNames = estateOptions(catalog)
            .filter { selectedEstates.contains($0.id) }
            .map(\.name)
            .sorted()
        return VStack(alignment: .leading, spacing: 9) {
            Label("Selection at a glance", systemImage: "list.bullet.rectangle")
                .sirsiFont(.headline)
            previewLine("Resident LLM", route?.residentModel?.isEmpty == false ? (route?.residentModel ?? "") : "Choose a configured resident model")
            previewLine("Engine", route.map { "\($0.name) · \($0.provider)" } ?? "Choose an inference engine")
            previewLine("Machine", "\(machine.name) · \(machine.cpuCores) cores · \(byteLabel(machine.memoryBytes))")
            previewLine("Requested", "\(selectedCores) cores · \(selectedMemoryGiB) GiB memory · \(selectedSwapGiB) GiB swap")
            previewLine("Chip estates", selectedNames.isEmpty ? "Choose at least one detected estate" : selectedNames.joined(separator: ", "))
            Text(recipeCommand(catalog))
                .sirsiFont(.caption, design: .monospaced)
                .textSelection(.enabled)
                .padding(8)
                .frame(maxWidth: .infinity, alignment: .leading)
                .background(RoundedRectangle(cornerRadius: 8).fill(Color.primary.opacity(0.06)))
            Text("Validate this declaration to hand it directly to Apollo telemetry. SNE remains the authority that can admit an inference session against current pressure.")
                .sirsiFont(.caption)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
        }
        .padding(14)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(RoundedRectangle(cornerRadius: 12).fill(gold.opacity(0.09)))
    }

    private func planReady(_ plan: ApolloPlan) -> some View {
        VStack(alignment: .leading, spacing: 9) {
            Label("Run plan is ready", systemImage: "checkmark.seal.fill")
                .sirsiFont(.headline).foregroundStyle(.green)
            Text(planSummary(plan))
                .sirsiFont(.subheadline).foregroundStyle(.secondary)
            Text("SNE must still admit execution against current system pressure. Opening Apollo now shows live telemetry when a session is active.")
                .sirsiFont(.caption).foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
            if !plan.unqualifiedEstates.isEmpty {
                Label("Awaiting SNE qualification: \(plan.unqualifiedEstates.joined(separator: ", "))", systemImage: "exclamationmark.triangle.fill")
                    .sirsiFont(.caption, weight: .semibold)
                    .foregroundStyle(.orange)
                    .fixedSize(horizontal: false, vertical: true)
                Text("Pantheon retained your requested estate selection, but it cannot start inference or present it as admitted until SNE qualifies it.")
                    .sirsiFont(.caption)
                    .foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
            }
            NavLink { ApolloTelemetryView(engine: engine, plan: plan) } label: {
                Label("Open Apollo telemetry", systemImage: "waveform.path.ecg")
            }
            .buttonStyle(.borderedProminent).tint(gold)
        }
        .padding(14)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(RoundedRectangle(cornerRadius: 12).fill(Color.green.opacity(0.08)))
    }

    private func planSummary(_ plan: ApolloPlan) -> String {
        let model = plan.residentModel ?? "Configured resident model"
        return "\(model) · \(plan.machineID) · \(plan.engineID) · \(plan.cpuCores) cores · \(byteLabel(plan.memoryBytes)) memory · \(byteLabel(plan.swapBytes)) swap ceiling"
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

    private func capacityFact(_ title: String, _ detail: String) -> some View {
        VStack(alignment: .leading, spacing: 2) {
            Text(title).sirsiFont(.caption).foregroundStyle(.secondary)
            Text(detail).sirsiFont(.caption, weight: .semibold).lineLimit(1)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(8)
        .background(RoundedRectangle(cornerRadius: 8).fill(Color.primary.opacity(0.05)))
    }

    private func previewLine(_ label: String, _ value: String) -> some View {
        HStack(alignment: .firstTextBaseline, spacing: 8) {
            Text(label).sirsiFont(.caption, weight: .semibold).foregroundStyle(.secondary)
                .frame(width: 76, alignment: .leading)
            Text(value).sirsiFont(.subheadline, weight: .semibold)
                .fixedSize(horizontal: false, vertical: true)
            Spacer(minLength: 0)
        }
    }

    @ViewBuilder private func estateRow(_ estate: ApolloChipEstate) -> some View {
        if snapshotMode {
            HStack(alignment: .top, spacing: 8) {
                Image(systemName: selectedEstates.contains(estate.id) ? "checkmark.square.fill" : "square")
                    .foregroundStyle(estate.available ? gold : .orange)
                VStack(alignment: .leading, spacing: 2) {
                    Text(estate.name).sirsiFont(.subheadline, weight: .semibold)
                    Text(estateDetail(estate)).sirsiFont(.caption).foregroundStyle(estate.available ? Color.secondary : .orange)
                }
            }
        } else {
            Toggle(isOn: binding(for: estate)) {
                VStack(alignment: .leading, spacing: 2) {
                    Text(estate.name).sirsiFont(.subheadline, weight: .semibold)
                    Text(estateDetail(estate)).sirsiFont(.caption).foregroundStyle(estate.available ? Color.secondary : .orange)
                }
            }
            .toggleStyle(.checkbox)
        }
    }

    private func estateDetail(_ estate: ApolloChipEstate) -> String {
        estate.available ? estate.description : "Not currently SNE-qualified · \(estate.description)"
    }

    private func snapshotSelection(_ label: String, value: String) -> some View {
        HStack(alignment: .top, spacing: 8) {
            Text(label).sirsiFont(.caption, weight: .semibold).foregroundStyle(.secondary)
            Text(value).sirsiFont(.subheadline, weight: .semibold)
            Spacer(minLength: 0)
            Image(systemName: "chevron.up.chevron.down").sirsiFont(.caption).foregroundStyle(gold)
        }
        .padding(.horizontal, 10)
        .padding(.vertical, 8)
        .background(RoundedRectangle(cornerRadius: 8).fill(Color.primary.opacity(0.07)))
    }

    private func engineDetail(_ engine: ApolloEngineOption) -> String {
        let route = engine.endpoint?.isEmpty == false ? " · \(engine.endpoint!)" : ""
        return "\(engine.name) · \(engine.provider)\(route)"
    }

    private func routeLabel(_ engine: ApolloEngineOption) -> String {
        let model = engine.residentModel?.trimmingCharacters(in: .whitespacesAndNewlines)
        let name = (model?.isEmpty == false) ? model! : "No resident model"
        return "\(name) · \(engine.name)"
    }

    private func selectedMachineDescriptor(_ catalog: ApolloCatalog) -> ApolloMachine {
        catalog.machineOptions.first(where: { $0.id == selectedMachine }) ?? catalog.machine
    }
    private func engineOptions(_ catalog: ApolloCatalog) -> [ApolloEngineOption] {
        catalog.residentModelOptions(for: selectedMachine)
    }
    private func selectedRoute(_ catalog: ApolloCatalog) -> ApolloEngineOption? {
        catalog.route(machineID: selectedMachine, engineID: selectedEngine)
    }
    private func planBlocker(_ catalog: ApolloCatalog) -> String? {
        guard !selectedEstates.isEmpty else {
            return "Choose at least one detected chip estate to continue."
        }
        guard let route = selectedRoute(catalog) else {
            return "Choose an Apollo inference route for the selected machine."
        }
        guard route.state == "configured" else {
            return "This route is visible but not configured. Choose a configured resident model route, then run the recipe."
        }
        guard route.residentModel?.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty == false else {
            return "This route has no declared resident LLM. Refresh after SNE publishes its configured model."
        }
        return nil
    }
    private func estateOptions(_ catalog: ApolloCatalog) -> [ApolloChipEstate] {
        catalog.estateOptions(for: selectedMachine)
    }
    private func recipeCommand(_ catalog: ApolloCatalog) -> String {
        let estates = estateOptions(catalog)
            .filter { selectedEstates.contains($0.id) }
            .map(\.id)
            .sorted()
            .joined(separator: ",")
        return "sirsi apollo plan --machine \(selectedMachine) --engine \(selectedEngine) --cores \(selectedCores) --memory-gib \(selectedMemoryGiB) --swap-gib \(selectedSwapGiB) --estates \(estates) --json"
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
        let data = await catalogData
        _ = await vitals
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
            openTelemetry = true
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
    @State private var isRefreshing = false
    private static let telemetryRefreshIntervalNanoseconds: UInt64 = 5_000_000_000

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
        // This page is a live instrument, not a snapshot with a decorative
        // Refresh button. The task is automatically cancelled when the view
        // leaves the navigation stack; it does no router work and uses a
        // deliberately bounded cadence to avoid becoming another pressure source.
        .task {
            await refresh()
            while !Task.isCancelled {
                do {
                    try await Task.sleep(nanoseconds: Self.telemetryRefreshIntervalNanoseconds)
                } catch {
                    return
                }
                guard !Task.isCancelled else { return }
                await refresh()
            }
        }
        .navigationTitle("Apollo telemetry")
    }

    private var sessionSummary: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text(sessionTitle)
                .sirsiFont(.title3, weight: .bold)
                .foregroundStyle(sessionMatchesPlan == true || (session?.state != "active" && engine.localLLM?.healthy == true) ? .green : .orange)
            Text(sessionDetail)
                .sirsiFont(.subheadline).foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
            Label("Selected: \(planSelectionLabel)", systemImage: "slider.horizontal.3")
                .sirsiFont(.caption, weight: .semibold)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
            Button {
                Task { await refresh() }
            } label: {
                Label(isRefreshing ? "Refreshing telemetry…" : "Refresh telemetry", systemImage: "arrow.clockwise")
            }
                .buttonStyle(.bordered).tint(gold)
                .disabled(isRefreshing)
            Text("Live refresh every 5 seconds while this page is open.")
                .sirsiFont(.caption)
                .foregroundStyle(.secondary)
            if sessionMatchesPlan == false {
                NavLink { ApolloRunPlannerView(engine: engine) } label: {
                    Label("Return to selected Apollo plan", systemImage: "slider.horizontal.3")
                }
                .buttonStyle(.borderedProminent)
                .tint(gold)
            }
            if sessionMatchesPlan != true {
                NavLink { MaatWorkspaceView(engine: engine) } label: {
                    Label("Check Apollo readiness in Ma'at", systemImage: "checklist")
                }
                .buttonStyle(.bordered)
                Text("Ma'at reweighs the actual local route, capacity, and evidence before recommending the next repair or admission step. It does not fabricate a running session.")
                    .sirsiFont(.caption)
                    .foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
            }
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
            Label("Chip estate telemetry", systemImage: "square.grid.2x2")
                .sirsiFont(.headline)
            Text("Every selected estate remains visible. When an active Apollo session reports additional estates, they are shown here too rather than being silently hidden.")
                .sirsiFont(.caption).foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
            ForEach(displayedEstateIDs, id: \.self) { estate in
                VStack(alignment: .leading, spacing: 3) {
                    HStack {
                        Text(estate.replacingOccurrences(of: "-", with: " ").capitalized).sirsiFont(.subheadline, weight: .semibold)
                        Spacer()
                        Text(estateTelemetry(estate)).sirsiFont(.caption).foregroundStyle(.secondary)
                    }
                    Text(plan.chipEstates.contains(estate) ? "Selected for this run plan" : "Reported by the active session; not selected in this plan")
                        .sirsiFont(.caption)
                        .foregroundStyle(plan.chipEstates.contains(estate) ? Color.secondary : .orange)
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
    private var planSelectionLabel: String {
        let model = plan.residentModel ?? "configured resident model"
        return "\(model) · \(plan.engineID) · \(plan.machineID)"
    }
    private var tokensTelemetry: String { metric(selectedTelemetry?.tokensPerSec, suffix: " tok/s", precision: 1) }
    private var bandwidthTelemetry: String {
        guard let bytes = selectedTelemetry?.bandwidthBps else { return unavailable }
        return "\(byteLabel(bytes))/s"
    }
    private var networkTelemetry: String { metric(selectedTelemetry?.networkPct, suffix: "%", precision: 1) }
    private var gpuTelemetry: String { metric(selectedTelemetry?.gpuResidency, suffix: "%", precision: 1) }
    private var cpuTelemetry: String { metric(selectedTelemetry?.cpuResidency, suffix: "%", precision: 1) }
    private var memoryTelemetry: String {
        if let bytes = selectedTelemetry?.memoryBytes { return byteLabel(bytes) + " session" }
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
        guard let estate = selectedTelemetry?.estates.first(where: { $0.id == id }) else { return unavailable }
        var parts: [String] = []
        if let utilization = estate.utilizationPct { parts.append(String(format: "%.1f%% util", utilization)) }
        if let residency = estate.residencyPct { parts.append(String(format: "%.1f%% resident", residency)) }
        if let memory = estate.memoryBytes { parts.append(byteLabel(memory)) }
        return parts.isEmpty ? unavailable : parts.joined(separator: " · ")
    }
    private var displayedEstateIDs: [String] {
        var seen = Set<String>()
        let reported = session?.telemetry?.estates.map(\.id) ?? []
        return (plan.chipEstates + reported).filter { seen.insert($0).inserted }
    }
    private func byteLabel(_ bytes: Int64) -> String { bytes == 0 ? "0 GiB" : String(format: "%.1f GiB", Double(bytes) / 1_073_741_824) }
    @MainActor private func refresh() async {
        guard !isRefreshing else { return }
        isRefreshing = true
        defer { isRefreshing = false }
        telemetryError = nil
        async let a: Void = engine.fetchVitals()
        async let telemetryData = SirsiEngine.runJSON(args: ["apollo", "telemetry", "--json"])
        let data = await telemetryData
        _ = await a
        if let read = try? JSONDecoder().decode(ApolloTelemetryRead.self, from: data) {
            session = read
        } else {
            session = nil
            telemetryError = "Pantheon could not decode an Apollo session sample. It was not treated as active telemetry."
        }
    }

    private var sessionMatchesPlan: Bool? {
        guard session?.state == "active", let telemetry = session?.telemetry else { return nil }
        return telemetry.matches(plan: plan)
    }

    private var selectedTelemetry: ApolloSessionTelemetry? {
        sessionMatchesPlan == true ? session?.telemetry : nil
    }

    private var sessionTitle: String {
        if sessionMatchesPlan == true { return "Apollo session is active" }
        if sessionMatchesPlan == false { return "A different Apollo session is active" }
        return engine.localLLM?.healthy == true ? "Apollo local route is online" : "No active Apollo session"
    }

    private var sessionDetail: String {
        if sessionMatchesPlan == true { return "SNE published a bounded session sample for the engine selected in this plan." }
        if let observed = session?.telemetry, sessionMatchesPlan == false {
            let observedMachine = observed.machineID ?? "legacy this-mac"
            return "The active SNE sample belongs to \(observed.engineID) on \(observedMachine), not \(plan.engineID) on \(plan.machineID). Its metrics are withheld; return to the plan to choose the matching route or wait for SNE to publish the selected session."
        }
        if engine.localLLM?.healthy == true { return "The local SNE conduit is reachable. Metrics below update when SNE publishes a sample for this selected engine." }
        return "The selected plan is ready for SNE admission. Return to the plan to recheck its resource envelope, then refresh after SNE publishes a selected-engine session sample."
    }
}

struct ApolloCatalog: Decodable {
    let machine: ApolloMachine
	let machines: [ApolloMachine]?
    let engines: [ApolloEngineOption]
    let estates: [ApolloChipEstate]

	// The Go contract intentionally calls this chip_estates. Keep the native
	// projection explicit so a valid live catalog cannot silently fall back to
	// the preview or fail before the selector becomes usable.
	enum CodingKeys: String, CodingKey {
		case machine, machines, engines
		case estates = "chip_estates"
	}
	var machineOptions: [ApolloMachine] { machines?.isEmpty == false ? machines! : [machine] }
}

extension ApolloCatalog {
    // A resident model is never selected independently from its SNE engine:
    // the pair is the executable route. Listing routes this way lets Stack Lab
    // show every typed local LLM choice without allowing a model string to be
    // paired with a different engine or machine by the UI.
    func residentModelOptions(for machineID: String) -> [ApolloEngineOption] {
        engines.filter { $0.machineID == nil || $0.machineID == machineID }
    }

    // A route is the indivisible resident-model/engine/machine choice. Keeping
    // this lookup in the typed catalog prevents a stale UI selection from
    // presenting another machine's engine as executable on the current one.
    func route(machineID: String, engineID: String) -> ApolloEngineOption? {
        residentModelOptions(for: machineID).first { $0.id == engineID }
    }

    // A selected Horus instance owns its estate details. Fall back only for a
    // legacy catalog whose single top-level estate list predates per-machine
    // records; never render this Mac's labels or availability for a peer with
    // a typed estate receipt of its own.
    func estateOptions(for machineID: String) -> [ApolloChipEstate] {
        let selected = machineOptions.first(where: { $0.id == machineID }) ?? machine
        if let estates = selected.estates, !estates.isEmpty { return estates }
        let ids = Set(selected.chipEstates ?? [])
        return estates.filter { ids.isEmpty || ids.contains($0.id) }
    }

    // Used only when an installed CLI predates the source checkout running the
    // native visual walk. This preview has no route, credential, or telemetry
    // authority; the live planner always requires a typed CLI catalog.
    static let snapshotPreview = ApolloCatalog(
        machine: ApolloMachine(id: "this-mac", name: "This Mac", cpuCores: 12,
                               memoryBytes: 32 * 1_073_741_824,
                               chipEstates: ["cpu", "gpu", "neural-engine"], estates: nil),
        machines: nil,
        engines: [ApolloEngineOption(id: "apollo-local", machineID: "this-mac",
                                     name: "Apollo local", provider: "Apollo",
                                     residentModel: "qualified resident model",
                                     endpoint: nil, state: "configured")],
        estates: [
            ApolloChipEstate(id: "cpu", name: "CPU", available: true,
                             description: "General-purpose local compute."),
            ApolloChipEstate(id: "gpu", name: "GPU", available: true,
                             description: "Apple GPU estate when SNE qualifies it."),
            ApolloChipEstate(id: "neural-engine", name: "Neural Engine", available: false,
                             description: "Visible until a local SNE receipt makes it available."),
        ]
    )
}
struct ApolloMachine: Decodable, Identifiable { let id: String; let name: String; let cpuCores: Int; let memoryBytes: Int64; let chipEstates: [String]?; let estates: [ApolloChipEstate]?; enum CodingKeys: String, CodingKey { case id, name, estates; case cpuCores = "cpu_cores"; case memoryBytes = "memory_bytes"; case chipEstates = "chip_estates" } }
struct ApolloEngineOption: Decodable, Identifiable { let id: String; let machineID: String?; let name: String; let provider: String; let residentModel: String?; let endpoint: String?; let state: String; enum CodingKeys: String, CodingKey { case id, name, provider, endpoint, state; case machineID = "machine_id"; case residentModel = "resident_model" } }
struct ApolloChipEstate: Decodable, Identifiable { let id: String; let name: String; let available: Bool; let description: String }
struct ApolloPlan: Decodable {
    let machineID: String
    let engineID: String
    let residentModel: String?
    let cpuCores: Int
    let memoryBytes: Int64
    let swapBytes: Int64
    let chipEstates: [String]
    let unavailableEstates: [String]?

    enum CodingKeys: String, CodingKey {
        case machineID = "machine_id"
        case engineID = "engine_id"
        case residentModel = "resident_model"
        case cpuCores = "cpu_cores"
        case memoryBytes = "memory_bytes"
        case swapBytes = "swap_bytes"
        case chipEstates = "chip_estates"
        case unavailableEstates = "unavailable_chip_estates"
    }

    // Older local CLI binaries did not emit this optional planning disclosure.
    // Treat its absence as no unqualified selection, never as a decode failure.
    var unqualifiedEstates: [String] { unavailableEstates ?? [] }
}
struct ApolloTelemetryRead: Decodable { let state: String; let telemetry: ApolloSessionTelemetry?; let reason: String? }
struct ApolloSessionTelemetry: Decodable {
    let engineID: String
    let machineID: String?
    let tokensPerSec: Double?
    let bandwidthBps: Int64?
    let memoryBytes: Int64?
    let networkPct: Double?
    let cpuResidency: Double?
    let gpuResidency: Double?
    let estates: [ApolloEstateTelemetry]

    enum CodingKeys: String, CodingKey {
        case engineID = "engine_id"
        case machineID = "machine_id"
        case tokensPerSec = "tokens_per_second"
        case bandwidthBps = "bandwidth_bytes_per_second"
        case memoryBytes = "memory_bytes"
        case networkPct = "network_saturation_percent"
        case cpuResidency = "cpu_residency_percent"
        case gpuResidency = "gpu_residency_percent"
        case estates = "chip_estates"
    }
}
extension ApolloSessionTelemetry {
    // An engine name alone is not a session identity once Stack Lab can select
    // another Horus instance. Existing local SNE publishers predate machine_id,
    // so their samples remain compatible only with this-mac plans.
    func matches(plan: ApolloPlan) -> Bool {
        guard engineID == plan.engineID else { return false }
        guard plan.machineID != "this-mac" else {
            return machineID == nil || machineID == plan.machineID
        }
        return machineID == plan.machineID
    }
}
struct ApolloEstateTelemetry: Decodable { let id: String; let residencyPct: Double?; let memoryBytes: Int64?; let utilizationPct: Double?; enum CodingKeys: String, CodingKey { case id; case residencyPct = "residency_percent"; case memoryBytes = "memory_bytes"; case utilizationPct = "utilization_percent" } }
