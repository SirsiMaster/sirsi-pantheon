import SwiftUI

/// 𓇽 Seba — Hardware profiling view.
/// Detect device capabilities: CPU, GPU, Neural Engine, Metal.
struct SebaView: View {
    @EnvironmentObject var appState: AppState
    @State private var hardware: HardwareProfile?
    @State private var accelerators: AcceleratorProfile?
    @State private var isDetecting = false
    @State private var errorMessage: String?

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 16) {
                DeityHeader(
                    glyph: "𓇽",
                    name: "Seba",
                    subtitle: "The Star Gate",
                    description: "Profile device hardware — CPU, GPU, Neural Engine, and compute accelerators."
                )

                Button {
                    Task { await detectHardware() }
                } label: {
                    HStack {
                        Image(systemName: isDetecting ? "progress.indicator" : "cpu")
                        Text(isDetecting ? "Detecting..." : "Detect Hardware")
                    }
                    .frame(maxWidth: .infinity)
                    .padding()
                    .background(PantheonTheme.gold)
                    .foregroundStyle(.black)
                    .clipShape(RoundedRectangle(cornerRadius: 12))
                    .font(.headline)
                }
                .disabled(isDetecting)

                if let errorMessage {
                    ErrorRetryView(message: errorMessage) { await detectHardware() }
                }

                if isDetecting {
                    HardwareSkeleton()
                }

                if let hw = hardware, !isDetecting {
                    HardwareCard(profile: hw)
                }

                if let accel = accelerators, !isDetecting {
                    AcceleratorCard(profile: accel)
                }
            }
            .padding()
        }
        .background(PantheonTheme.background)
        .task { await detectHardware() }
    }

    private func detectHardware() async {
        isDetecting = true
        errorMessage = nil
        defer { isDetecting = false }

        do {
            async let hw = appState.bridge.sebaDetectHardware()
            async let accel = appState.bridge.sebaDetectAccelerators()
            hardware = try await hw
            accelerators = try await accel

            // Write hardware JSON to App Group for widgets.
            // Re-encode the profile so the widget can decode it independently.
            if let hwResult = hardware,
               let jsonData = try? JSONEncoder().encode(hwResult) {
                let envelope: [String: Any] = [
                    "ok": true,
                    "data": (try? JSONSerialization.jsonObject(with: jsonData)) as Any,
                ]
                if let envelopeData = try? JSONSerialization.data(withJSONObject: envelope),
                   let envelopeStr = String(data: envelopeData, encoding: .utf8) {
                    SharedDataManager.saveHardwareJSON(envelopeStr)
                }
            }
        } catch {
            errorMessage = error.localizedDescription
        }
    }
}

struct HardwareCard: View {
    let profile: HardwareProfile

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("Device Hardware")
                .font(.headline)
                .foregroundStyle(PantheonTheme.gold)

            InfoRow(label: "CPU", value: profile.cpuModel)
            InfoRow(label: "Architecture", value: profile.cpuArch)
            InfoRow(label: "Cores", value: "\(profile.cpuCores)")
            InfoRow(label: "RAM", value: profile.formattedRAM)

            if let gpu = profile.gpu {
                Divider().background(PantheonTheme.textSecondary.opacity(0.3))
                Text("GPU")
                    .font(.subheadline.bold())
                    .foregroundStyle(PantheonTheme.gold)
                InfoRow(label: "Type", value: gpu.type)
                InfoRow(label: "Name", value: gpu.name)
                if let metalFamily = gpu.metalFamily {
                    InfoRow(label: "Metal", value: metalFamily)
                }
            }

            if let hasNE = profile.neuralEngine, hasNE {
                Divider().background(PantheonTheme.textSecondary.opacity(0.3))
                HStack {
                    Image(systemName: "brain")
                        .foregroundStyle(PantheonTheme.success)
                    Text("Apple Neural Engine Available")
                        .font(.subheadline)
                        .foregroundStyle(PantheonTheme.success)
                }
            }
        }
        .padding()
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(PantheonTheme.surface)
        .clipShape(RoundedRectangle(cornerRadius: 12))
    }
}

struct AcceleratorCard: View {
    let profile: AcceleratorProfile

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("Compute Accelerators")
                .font(.headline)
                .foregroundStyle(PantheonTheme.gold)

            AccelRow(name: "Metal GPU", available: profile.hasMetal, detail: profile.gpuVendor)
            AccelRow(name: "Neural Engine", available: profile.hasAne, detail: profile.aneCores.map { "\($0) cores" })
            AccelRow(name: "CUDA", available: profile.hasCuda, detail: nil)
            AccelRow(name: "ROCm", available: profile.hasRocm, detail: nil)
            AccelRow(name: "CPU Cores", available: true, detail: "\(profile.cpuCores)")

            if let routing = profile.routing {
                Divider().background(PantheonTheme.textSecondary.opacity(0.3))
                InfoRow(label: "Routing", value: routing)
            }
        }
        .padding()
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(PantheonTheme.surface)
        .clipShape(RoundedRectangle(cornerRadius: 12))
    }
}

struct AccelRow: View {
    let name: String
    let available: Bool
    let detail: String?

    var body: some View {
        HStack {
            Image(systemName: available ? "checkmark.circle.fill" : "xmark.circle")
                .foregroundStyle(available ? PantheonTheme.success : PantheonTheme.textSecondary)
            Text(name)
                .font(.subheadline)
                .foregroundStyle(PantheonTheme.textPrimary)
            Spacer()
            if let detail {
                Text(detail)
                    .font(.caption)
                    .foregroundStyle(PantheonTheme.textSecondary)
            }
        }
    }
}
