import SwiftUI

// Native bordered buttons use a single-line bezel height on macOS. Diagnostic
// actions contain a title AND a command, so their surface must follow the full
// label's intrinsic height, including the operator's text-size preference.
struct DiagnosticRepairButtonStyle: ButtonStyle {
    @Environment(\.isEnabled) private var isEnabled

    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .foregroundStyle(PantheonTheme.canvas)
            .padding(12)
            .frame(maxWidth: .infinity, minHeight: 52, alignment: .leading)
            .background {
                RoundedRectangle(cornerRadius: 8)
                    .fill(gold.opacity(configuration.isPressed ? 0.82 : 1))
            }
            .contentShape(RoundedRectangle(cornerRadius: 8))
            .opacity(isEnabled ? 1 : 0.55)
    }
}

struct DiagnosticRepairActionLabel: View {
    let title: String
    let command: String
    let systemImage: String

    var body: some View {
        HStack(alignment: .top, spacing: 10) {
            Image(systemName: systemImage)
                .sirsiFont(.body)
                .accessibilityHidden(true)
            VStack(alignment: .leading, spacing: 5) {
                Text(title)
                    .sirsiFont(.callout, weight: .semibold)
                    .fixedSize(horizontal: false, vertical: true)
                Text(command)
                    .sirsiFont(.caption, design: .monospaced)
                    .foregroundStyle(PantheonTheme.canvas.opacity(0.9))
                    .fixedSize(horizontal: false, vertical: true)
            }
            .frame(maxWidth: .infinity, alignment: .leading)
        }
        .multilineTextAlignment(.leading)
        .fixedSize(horizontal: false, vertical: true)
        .accessibilityElement(children: .combine)
    }
}
