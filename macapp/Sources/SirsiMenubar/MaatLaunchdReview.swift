import Foundation
import SwiftUI

let maatLaunchdPreviewArguments = ["maat", "repair", "launchd-disabled", "--json"]

func maatLaunchdRepairArguments(previewBinding: String) -> [String] {
    ["maat", "repair", "launchd-disabled", "--preview-binding", previewBinding, "--confirm", "--json"]
}

struct MaatLaunchdDisabledPreview: Decodable, Equatable, Sendable {
    let schema: String
    let targets: [MaatLaunchdPreviewTarget]
    let binding: String

    var isValid: Bool {
        guard schema == "sirsi.maat.launchd-disabled-preview.v1",
              !targets.isEmpty, targets.count <= 64,
              binding.isLowercaseSHA256,
              targets.map(\.label) == targets.map(\.label).sorted(),
              Set(targets.map(\.label)).count == targets.count else { return false }
        return targets.allSatisfy { target in
            let prefix = target.label.hasPrefix("ai.sirsi.") ? "ai.sirsi." : "actions.runner."
            let suffix = target.label.dropFirst(prefix.count)
            let validLabelBytes = target.label.utf8.count <= 255 && target.label.utf8.allSatisfy { byte in
                (byte >= 65 && byte <= 90) || (byte >= 97 && byte <= 122) ||
                    (byte >= 48 && byte <= 57) || byte == 46 || byte == 95 || byte == 45
            }
            let allowed = !suffix.isEmpty && !target.label.contains("/") && !target.label.contains("\\") &&
                !target.label.contains(where: { $0.isWhitespace }) &&
                (target.label.hasPrefix("ai.sirsi.") || target.label.hasPrefix("actions.runner.")) && validLabelBytes
            return allowed && target.plist == target.label + ".plist" && target.sha256.isLowercaseSHA256
        }
    }

    var reviewDetails: String {
        let rows = targets.map { target in
            "\(target.label) · \(target.wasLoaded ? "loaded; enable only" : "unloaded; enable and bootstrap") · plist sha256 \(target.sha256)"
        }.joined(separator: "\n")
        return "Exact reviewed managed targets:\n\(rows)\nPreview binding: \(binding)"
    }

    enum CodingKeys: String, CodingKey {
        case schema, targets, binding
    }

    static func decode(_ raw: String) -> MaatLaunchdDisabledPreview? {
        guard let start = raw.firstIndex(of: "{") else { return nil }
        return try? JSONDecoder().decode(Self.self, from: Data(raw[start...].utf8))
    }
}

struct MaatLaunchdPreviewTarget: Decodable, Equatable, Identifiable, Sendable {
    let label: String
    let plist: String
    let sha256: String
    let wasLoaded: Bool
    var id: String { label }

    enum CodingKeys: String, CodingKey {
        case label, plist, sha256
        case wasLoaded = "was_loaded"
    }
}

enum MaatCasebookLaunchdApply<Value> {
    case historyRejected
    case submitted(Value)
}

extension MaatCasebookLaunchdApply: Sendable where Value: Sendable {}

struct MaatCasebookLaunchdConsent: Sendable {
    let preview: MaatLaunchdDisabledPreview

    init?(preview: MaatLaunchdDisabledPreview?) {
        guard let preview, preview.isValid else { return nil }
        self.preview = preview
    }

    var repairArguments: [String] {
        maatLaunchdRepairArguments(previewBinding: preview.binding)
    }

    @MainActor
    func apply<Value>(afterHistoryRead: () async -> Bool,
                      run: ([String]) async -> Value) async -> MaatCasebookLaunchdApply<Value> {
        let frozenArguments = repairArguments
        guard preview.isValid, await afterHistoryRead() else { return .historyRejected }
        return .submitted(await run(frozenArguments))
    }
}

private extension String {
    var isLowercaseSHA256: Bool {
        count == 64 && allSatisfy { ("0"..."9").contains($0) || ("a"..."f").contains($0) }
    }
}

private struct MaatLaunchdPreviewReadFailure: Error {
    let message: String
}

@MainActor
private enum MaatLaunchdPreviewReader {
    static func read() async -> Result<MaatLaunchdDisabledPreview, MaatLaunchdPreviewReadFailure> {
        let capture = await SirsiEngine.runCaptured(args: maatLaunchdPreviewArguments, timeout: 30)
        guard capture.complete, capture.exitCode == 0,
              let preview = MaatLaunchdDisabledPreview.decode(capture.stdout), preview.isValid else {
            if capture.complete {
                let message = capture.result?.summary ?? (capture.stderr.isEmpty
                    ? "Ma'at could not produce a valid exact-target preview. No service state was changed."
                    : capture.stderr)
                return .failure(MaatLaunchdPreviewReadFailure(message: SirsiEngine.stripANSI(message)))
            }
            return .failure(MaatLaunchdPreviewReadFailure(
                message: SirsiEngine.stripANSI(capture.issue ?? "The managed-target preview was incomplete. No service state was changed.")
            ))
        }
        return .success(preview)
    }
}

struct MaatLaunchdPreviewReviewControls: View {
    @Binding var preview: MaatLaunchdDisabledPreview?
    @Binding var loading: Bool
    @Binding var error: String?
    let confirmLabel: String
    let canConfirm: Bool
    var canRefresh = true
    let onConfirm: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 9) {
            Button {
                Task { await loadPreview() }
            } label: {
                Label(loading ? "Refreshing managed-target preview…" : "Preview exact managed targets", systemImage: "magnifyingglass")
                    .frame(maxWidth: .infinity)
            }
            .buttonStyle(.bordered)
            .disabled(loading || !canRefresh)
            .accessibilityHint("Reads the exact eligible managed labels and plist hashes. It does not change launchd state.")

            if let preview, preview.isValid {
                MaatLaunchdPreviewTargets(preview: preview)
                Button(action: onConfirm) {
                    Label(confirmLabel, systemImage: "wrench.and.screwdriver.fill")
                        .frame(maxWidth: .infinity)
                }
                .buttonStyle(.borderedProminent)
                .disabled(loading || !canConfirm)
                .accessibilityHint("Ma'at rechecks this preview and the exact plist contents before changing service state.")
            }

            if let error {
                Label(error, systemImage: "exclamationmark.triangle.fill")
                    .sirsiFont(.caption)
                    .foregroundStyle(.orange)
                    .fixedSize(horizontal: false, vertical: true)
            }
        }
    }

    @MainActor private func loadPreview() async {
        loading = true
        error = nil
        preview = nil
        defer { loading = false }
        switch await MaatLaunchdPreviewReader.read() {
        case .success(let value): preview = value
        case .failure(let failure): error = failure.message
        }
    }
}

struct MaatLaunchdPreviewTargets: View {
    let preview: MaatLaunchdDisabledPreview

    var body: some View {
        VStack(alignment: .leading, spacing: 7) {
            Text("Exact eligible managed targets")
                .sirsiFont(.subheadline, weight: .semibold)
            ForEach(preview.targets) { target in
                VStack(alignment: .leading, spacing: 2) {
                    Text(target.label).sirsiFont(.callout, design: .monospaced)
                        .textSelection(.enabled)
                    Text("\(target.wasLoaded ? "Loaded · will enable only" : "Not loaded · will enable and bootstrap") · plist sha256 \(target.sha256)")
                        .sirsiFont(.caption, design: .monospaced)
                        .fixedSize(horizontal: false, vertical: true)
                        .textSelection(.enabled)
                }
            }
            Text("Preview binding \(preview.binding)")
                .sirsiFont(.caption2, design: .monospaced)
                .textSelection(.enabled)
        }
        .padding(10)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(RoundedRectangle(cornerRadius: 8).fill(Color.primary.opacity(0.045)))
    }
}
