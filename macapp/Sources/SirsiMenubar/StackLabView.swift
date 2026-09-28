import SwiftUI

// StackLabView is the native inspection surface for the same ADR-066 doctor
// the CLI exposes. It does not scrape a shell transcript or reconstruct wing
// authority in Swift: Go's typed report remains the sole authority. This view
// makes an incomplete registry actionable instead of leaving an operator with
// a command and no route through the product.
struct StackLabView: View {
    @ObservedObject var engine: SirsiEngine
    @State private var report: StackLabReport?
    @State private var loadError: String?
    @State private var loading = true

    var body: some View {
        VStack(spacing: 0) {
            BackBar(title: "Stack Lab")
            Group {
                if loading {
                    loadingState
                } else if let error = loadError {
                    failureState(error)
                } else if let report {
                    reportBody(report)
                } else {
                    failureState("Pantheon did not receive a Stack Lab report. No authority result was inferred.")
                }
            }
        }
        .task { await load() }
    }

    private var loadingState: some View {
        VStack(spacing: 10) {
            ProgressView()
            Text("Reading the canonical wing registry…")
                .sirsiFont(.subheadline)
                .foregroundStyle(.secondary)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }

    private func failureState(_ error: String) -> some View {
        VStack(alignment: .leading, spacing: 12) {
            Image(systemName: "exclamationmark.triangle.fill")
                .sirsiFont(24, weight: .semibold)
                .foregroundStyle(.orange)
            Text("Stack Lab needs a fresh read")
                .sirsiFont(.headline)
            Text(error)
                .sirsiFont(.subheadline)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
            Button("Try again") { Task { await load() } }
                .buttonStyle(.borderedProminent)
                .tint(gold)
            NavLink { MaatWorkspaceView(engine: engine) } label: {
                Label("Open Ma'at evidence", systemImage: "checkmark.seal")
            }
            .buttonStyle(.bordered)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .leading)
        .padding(20)
    }

    private func reportBody(_ report: StackLabReport) -> some View {
        VStack(spacing: 0) {
            MaybeScroll {
                VStack(alignment: .leading, spacing: 16) {
                    summary(report)
                    if report.clean {
                        cleanState(report)
                    } else {
                        findings(report)
                    }
                }
                .padding(16)
            }
            Divider()
            HStack {
                Text("Canonical origin and registry check · read only")
                    .sirsiFont(.caption)
                    .foregroundStyle(.secondary)
                Spacer()
                Button { Task { await load() } } label: {
                    Label("Refresh", systemImage: "arrow.clockwise")
                }
                .buttonStyle(.borderless)
                .disabled(loading)
            }
            .padding(.horizontal, 16)
            .padding(.vertical, 11)
        }
    }

    private func summary(_ report: StackLabReport) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Canonical recipe authority")
                .sirsiFont(.title3, weight: .bold)
            Text(summaryText(report))
                .sirsiFont(.subheadline)
                .foregroundStyle(.secondary)
            HStack(spacing: 8) {
                metric("Declared", report.roster.count, .secondary)
                metric("Findings", report.findings.count, report.findings.isEmpty ? .green : .orange)
                metric("Unreadable", report.unknown.count, report.unknown.isEmpty ? .green : .orange)
            }
        }
        .padding(14)
        .background(RoundedRectangle(cornerRadius: 10).fill(Color.primary.opacity(0.05)))
    }

    private func metric(_ title: String, _ value: Int, _ tint: Color) -> some View {
        HStack(spacing: 5) {
            Circle().fill(tint).frame(width: 7, height: 7)
            Text("\(value) \(title)")
                .sirsiFont(.caption, weight: .semibold)
        }
        .padding(.horizontal, 8)
        .padding(.vertical, 5)
        .background(Capsule().fill(tint.opacity(0.12)))
    }

    private func summaryText(_ report: StackLabReport) -> String {
        if report.clean { return "Every declared wing is built, pushed, pinned, declared, and schema-valid." }
        if !report.unknown.isEmpty { return "Authority is incomplete: one or more canonical reads could not be verified. This is not a clean result." }
        return "\(report.findings.count) declared recipe \(report.findings.count == 1 ? "needs" : "need") an evidence-bound follow-up."
    }

    @ViewBuilder private func cleanState(_ report: StackLabReport) -> some View {
        VStack(alignment: .leading, spacing: 7) {
            Label("Registry authority is current", systemImage: "checkmark.seal.fill")
                .sirsiFont(.headline)
                .foregroundStyle(.green)
            Text("This verifies Stack Lab wing records only. It does not by itself prove a product build, package, signing, or release.")
                .sirsiFont(.subheadline)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
            if !report.roster.isEmpty {
                Text(report.roster.joined(separator: "\n"))
                    .sirsiFont(.caption, design: .monospaced)
                    .foregroundStyle(.secondary)
                    .textSelection(.enabled)
            }
        }
        .padding(14)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(RoundedRectangle(cornerRadius: 10).fill(Color.green.opacity(0.08)))
    }

    @ViewBuilder private func findings(_ report: StackLabReport) -> some View {
        VStack(alignment: .leading, spacing: 10) {
            ForEach(report.findings) { finding in
                StackLabFindingCard(engine: engine, finding: finding)
            }
            if !report.unknown.isEmpty {
                VStack(alignment: .leading, spacing: 6) {
                    Text("Unreadable canonical records")
                        .sirsiFont(.headline)
                    Text("Retry when the origin or registry can be read. Pantheon will not treat an unavailable record as a clean wing.")
                        .sirsiFont(.subheadline)
                        .foregroundStyle(.secondary)
                        .fixedSize(horizontal: false, vertical: true)
                    ForEach(report.unknown, id: \.self) { item in
                        Text(item)
                            .sirsiFont(.caption, design: .monospaced)
                            .foregroundStyle(.secondary)
                            .textSelection(.enabled)
                    }
                }
                .padding(13)
                .frame(maxWidth: .infinity, alignment: .leading)
                .background(RoundedRectangle(cornerRadius: 10).fill(Color.orange.opacity(0.08)))
            }
        }
    }

    @MainActor private func load() async {
        loading = true
        loadError = nil
        let data = await SirsiEngine.runJSON(args: ["stacklab", "doctor", "--json"])
        if let decoded = try? JSONDecoder().decode(StackLabReport.self, from: data) {
            report = decoded
        } else {
            report = nil
            loadError = "Pantheon could not decode the canonical Stack Lab report. The registry was not treated as clean; retry the read or inspect Ma'at evidence."
        }
        loading = false
    }
}

private struct StackLabFindingCard: View {
    @ObservedObject var engine: SirsiEngine
    let finding: StackLabFinding

    var body: some View {
        VStack(alignment: .leading, spacing: 7) {
            HStack(alignment: .top, spacing: 7) {
                Image(systemName: "exclamationmark.triangle.fill")
                    .foregroundStyle(.orange)
                VStack(alignment: .leading, spacing: 2) {
                    Text(finding.wingID)
                        .sirsiFont(.headline)
                    Text(finding.finding)
                        .sirsiFont(.caption, weight: .semibold)
                        .foregroundStyle(.orange)
                }
            }
            Text(finding.detail)
                .sirsiFont(.subheadline)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
            Text(nextStep(for: finding.finding))
                .sirsiFont(.caption, weight: .semibold)
                .foregroundStyle(gold)
            NavLink { MaatWorkspaceView(engine: engine) } label: {
                Label("Review evidence in Ma'at", systemImage: "checkmark.seal")
            }
            .buttonStyle(.bordered)
        }
        .padding(13)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(RoundedRectangle(cornerRadius: 10).fill(Color.primary.opacity(0.05)))
    }

    private func nextStep(for kind: String) -> String {
        switch kind {
        case "stranded/unbuilt": return "Next: author and review the missing source wing record."
        case "unpushed/stranded": return "Next: review and publish the existing wing record to its owning origin/main."
        case "unpinned": return "Next: validate the origin record, then request an exact registry pin."
        case "undeclared": return "Next: reconcile the registry record with the declared peer roster."
        case "invalid": return "Next: repair the structural wing contract and repeat the canonical read."
        default: return "Next: retain this finding in Ma'at and resolve it against canonical evidence."
        }
    }
}

private struct StackLabReport: Decodable {
    let roster: [String]
    let findings: [StackLabFinding]
    let unknown: [String]

    enum CodingKeys: String, CodingKey { case roster, findings, unknown }

    init(from decoder: Decoder) throws {
        let values = try decoder.container(keyedBy: CodingKeys.self)
        roster = try values.decodeIfPresent([String].self, forKey: .roster) ?? []
        findings = try values.decodeIfPresent([StackLabFinding].self, forKey: .findings) ?? []
        unknown = try values.decodeIfPresent([String].self, forKey: .unknown) ?? []
    }

    var clean: Bool { findings.isEmpty && unknown.isEmpty }
}

private struct StackLabFinding: Decodable, Identifiable {
    let wingID: String
    let finding: String
    let detail: String

    enum CodingKeys: String, CodingKey {
        case wingID = "wing_id"
        case finding, detail
    }

    var id: String { wingID + "\u{0}" + finding + "\u{0}" + detail }
}
