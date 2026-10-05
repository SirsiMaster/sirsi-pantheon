import SwiftUI

// PantheonReleaseWorkspaceView is the native delivery desk. It deliberately
// does not turn a shell transcript into a release UI: source-contract and
// public signing-identity checks are decoded as typed Ma'at projections, while
// Stack Lab remains the recipe authority. Signing, notarization, and
// publication stay explicit confirmation-owned operations instead of hidden
// side effects of opening this screen.
struct PantheonReleaseWorkspaceView: View {
    @ObservedObject var engine: SirsiEngine
    @State private var source: MaatReleaseContractPreflight?
    @State private var credentials: MaatReleaseCredentialPreflight?
    @State private var sourceError: String?
    @State private var credentialError: String?
    @State private var readingSource = false
    @State private var readingCredentials = false

    var body: some View {
        VStack(spacing: 0) {
            BackBar(title: "Release")
            ProjectBar(engine: engine) { resetSource() }
            MaybeScroll {
                VStack(alignment: .leading, spacing: 16) {
                    header
                    sourceContract
                    signingReadiness
                    packageRecipe
                    deliveryPath
                }
                .padding(16)
            }
        }
        .task { engine.loadProjectRoot() }
    }

    private var header: some View {
        HStack(alignment: .top, spacing: 14) {
            PantheonBrandMark(size: 54)
            VStack(alignment: .leading, spacing: 8) {
                Text("Ship the Pantheon application")
                    .sirsiFont(.title3, weight: .bold)
                    .foregroundStyle(Color.white)
                Text("Review the source, package, and identity evidence for the selected checkout. Inspecting is safe; shipping stays an explicit, guided decision.")
                    .sirsiFont(.subheadline)
                    .foregroundStyle(PantheonTheme.mutedText)
                    .fixedSize(horizontal: false, vertical: true)
            }
        }
        .padding(18)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(RoundedRectangle(cornerRadius: 14).fill(PantheonTheme.panelRaised))
        .overlay(RoundedRectangle(cornerRadius: 14).stroke(PantheonTheme.gold.opacity(0.26), lineWidth: 1))
    }

    private var sourceContract: some View {
        VStack(alignment: .leading, spacing: 10) {
            sectionHeader("1", title: "Source contract", detail: "Verify the selected checkout’s release contract before a build is considered.")
            if engine.projectRoot == nil {
                missingProject("Choose a Git checkout above. Pantheon will never guess which source tree you intend to ship.")
            } else if readingSource {
                ProgressView("Reading the source contract…")
                    .sirsiFont(.caption)
            } else if let source {
                sourceResult(source)
            } else {
                Button { Task { await readSourceContract() } } label: {
                    Label("Inspect source contract", systemImage: "checklist")
                        .frame(maxWidth: .infinity)
                }
                .buttonStyle(.borderedProminent)
                .tint(emerald)
            }
            if let sourceError { recovery(sourceError) { Task { await readSourceContract() } } }
        }
        .padding(14)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(cardBackground)
    }

    private var signingReadiness: some View {
        VStack(alignment: .leading, spacing: 10) {
            sectionHeader("2", title: "Signing readiness", detail: "Inspect public local Developer ID metadata. Pantheon never renders, reads, or retains secret material here.")
            if readingCredentials {
                ProgressView("Checking public signing identities…")
                    .sirsiFont(.caption)
            } else if let credentials {
                credentialResult(credentials)
            } else {
                Button { Task { await readCredentials() } } label: {
                    Label("Check local signing identities", systemImage: "checkmark.shield")
                        .frame(maxWidth: .infinity)
                }
                .buttonStyle(.bordered)
            }
            if let credentialError { recovery(credentialError) { Task { await readCredentials() } } }
        }
        .padding(14)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(cardBackground)
    }

    private var packageRecipe: some View {
        VStack(alignment: .leading, spacing: 9) {
            sectionHeader("3", title: "Package recipe", detail: "Inspect the exact Stack Lab recipe and its evidence before producing a delivery artifact.")
            HStack(spacing: 9) {
                NavLink { StackLabCatalogView(engine: engine) } label: {
                    Label("Open Stack Lab recipes", systemImage: "square.3.layers.3d")
                }
                .buttonStyle(.bordered)
                NavLink { StackLabView(engine: engine) } label: {
                    Label("Review readiness", systemImage: "checkmark.circle")
                }
                .buttonStyle(.bordered)
            }
            Text("Recipes describe and verify the delivery path; they do not silently publish artifacts.")
                .sirsiFont(.caption)
                .foregroundStyle(.secondary)
        }
        .padding(14)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(cardBackground)
    }

    private var deliveryPath: some View {
        VStack(alignment: .leading, spacing: 9) {
            sectionHeader("4", title: "Delivery", detail: "Use a reviewed recipe and explicit owner confirmation for the signed, notarized DMG, PKG, tag, and cask update.")
            NavLink { MaatWorkspaceView(engine: engine, opensReleasePreflight: true) } label: {
                Label("Open Ma'at release evidence", systemImage: "checkmark.seal")
            }
            .buttonStyle(.borderedProminent)
            .tint(gold)
            Text("Pantheon keeps the evidence and the delivery decision together: a green source read or visible certificate is never misrepresented as a published release.")
                .sirsiFont(.caption)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
        }
        .padding(14)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(cardBackground)
    }

    private var cardBackground: some View {
        RoundedRectangle(cornerRadius: 14).fill(PantheonTheme.panel)
    }

    private func sectionHeader(_ ordinal: String, title: String, detail: String) -> some View {
        HStack(alignment: .top, spacing: 9) {
            Text(ordinal)
                .sirsiFont(.caption, weight: .bold)
                .foregroundStyle(.black)
                .frame(width: 22, height: 22)
                .background(Circle().fill(title == "Delivery" ? gold : PantheonTheme.panelRaised))
                .overlay(Circle().stroke(title == "Delivery" ? gold.opacity(0.7) : emerald.opacity(0.8), lineWidth: 1))
            VStack(alignment: .leading, spacing: 2) {
                Text(title).sirsiFont(.headline)
                Text(detail).sirsiFont(.caption).foregroundStyle(PantheonTheme.mutedText)
                    .fixedSize(horizontal: false, vertical: true)
            }
        }
    }

    private func missingProject(_ message: String) -> some View {
        Label(message, systemImage: "folder.badge.questionmark")
            .sirsiFont(.subheadline)
            .foregroundStyle(.orange)
            .fixedSize(horizontal: false, vertical: true)
    }

    private func sourceResult(_ result: MaatReleaseContractPreflight) -> some View {
        let passed = result.verdict.floor.passed
        return VStack(alignment: .leading, spacing: 7) {
            Label(passed ? "Source contract verified" : "Source contract needs resolution", systemImage: passed ? "checkmark.seal.fill" : "exclamationmark.triangle.fill")
                .sirsiFont(.subheadline, weight: .semibold)
                .foregroundStyle(passed ? emerald : .orange)
            ForEach(result.verdict.floor.checks, id: \.name) { check in
                Label(check.detail, systemImage: check.passed ? "checkmark.circle.fill" : "xmark.octagon.fill")
                    .sirsiFont(.caption)
                .foregroundStyle(check.passed ? PantheonTheme.mutedText : Color.orange)
                    .fixedSize(horizontal: false, vertical: true)
            }
            Text("Evidence fingerprint: \(result.fingerprint)")
                .sirsiFont(.caption2, design: .monospaced)
                .foregroundStyle(PantheonTheme.mutedText)
                .textSelection(.enabled)
            Button("Read again") { Task { await readSourceContract() } }
                .buttonStyle(.bordered)
        }
    }

    private func credentialResult(_ result: MaatReleaseCredentialPreflight) -> some View {
        let identities = result.developerIdentities
        return VStack(alignment: .leading, spacing: 7) {
            Label("Team \(result.teamID) public identity check", systemImage: "checkmark.shield")
                .sirsiFont(.subheadline, weight: .semibold)
                .foregroundStyle(identities.isEmpty ? .orange : emerald)
            if identities.isEmpty {
                Text("No required Developer ID identity was observed in the local public metadata.")
                    .sirsiFont(.caption)
                    .foregroundStyle(.orange)
            } else {
                ForEach(identities) { identity in
                    Label(identity.name, systemImage: identity.kind == "installer" ? "shippingbox" : "app.badge.checkmark")
                        .sirsiFont(.caption)
                        .foregroundStyle(PantheonTheme.mutedText)
                        .fixedSize(horizontal: false, vertical: true)
                }
            }
            if !result.notarizationObserved {
                Text("Notarization is deliberately checked only by the protected release workflow; no secret is requested or displayed in this app.")
                    .sirsiFont(.caption)
                    .foregroundStyle(PantheonTheme.mutedText)
                    .fixedSize(horizontal: false, vertical: true)
            }
            Button("Read again") { Task { await readCredentials() } }
                .buttonStyle(.bordered)
        }
    }

    private func recovery(_ message: String, retry: @escaping () -> Void) -> some View {
        VStack(alignment: .leading, spacing: 7) {
            Label(message, systemImage: "exclamationmark.triangle.fill")
                .sirsiFont(.caption)
                .foregroundStyle(.orange)
                .fixedSize(horizontal: false, vertical: true)
            Button("Try again", action: retry).buttonStyle(.bordered)
            NavLink { MaatWorkspaceView(engine: engine) } label: {
                Label("Open Ma'at evidence", systemImage: "checkmark.seal")
            }
            .buttonStyle(.bordered)
        }
    }

    @MainActor private func readSourceContract() async {
        guard let root = engine.projectRoot else { return }
        readingSource = true
        sourceError = nil
        source = nil
        let raw = await SirsiEngine.run(args: ["maat", "preflight", "release", "--root", root, "--json"], stdin: nil)
        source = MaatReleaseContractPreflight.decode(raw)
        if source == nil {
            sourceError = "Pantheon could not decode the typed source-contract result. No build or release action ran. Retry this read or open Ma'at evidence."
        }
        readingSource = false
    }

    @MainActor private func readCredentials() async {
        readingCredentials = true
        credentialError = nil
        credentials = nil
        let raw = await SirsiEngine.run(args: ["maat", "preflight", "credentials", "--json"], stdin: nil)
        credentials = MaatReleaseCredentialPreflight.decode(raw)
        if credentials == nil {
            credentialError = "Pantheon could not decode the typed public identity result. No signing, notarization, or publication ran. Retry the read or inspect Ma'at evidence."
        }
        readingCredentials = false
    }

    private func resetSource() {
        source = nil
        sourceError = nil
    }
}
