import SwiftUI

// A loading ScrollView has no useful intrinsic height. Give result sheets a
// usable viewport before their command returns, without constraining embedded
// results or growing the window to the entire report's length.
struct ResultPanelLayout: ViewModifier {
    func body(content: Content) -> some View {
        content.frame(minWidth: 560, idealWidth: 680, minHeight: 460, idealHeight: 640)
    }
}

// macOS bordered bezels are single-line controls. Result actions explain their
// consequences on a second line, so both primary and secondary surfaces must
// follow the label's full wrapped height.
struct ResultActionButtonStyle: ButtonStyle {
    let prominent: Bool
    @Environment(\.isEnabled) private var isEnabled

    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .foregroundStyle(prominent ? PantheonTheme.canvas : PantheonTheme.text)
            .padding(12)
            .frame(maxWidth: .infinity, minHeight: 52, alignment: .leading)
            .background {
                RoundedRectangle(cornerRadius: 8)
                    .fill(prominent ? gold : Color.primary.opacity(0.06))
            }
            .overlay {
                if !prominent {
                    RoundedRectangle(cornerRadius: 8).strokeBorder(Color.primary.opacity(0.22))
                }
            }
            .contentShape(RoundedRectangle(cornerRadius: 8))
            .opacity(isEnabled ? (configuration.isPressed ? 0.82 : 1) : 0.55)
    }
}

struct ResultActionLabel: View {
    let title: String
    let detail: String?
    let prominent: Bool
    let systemImage: String

    var body: some View {
        HStack(alignment: .top, spacing: 10) {
            Image(systemName: systemImage)
                .sirsiFont(.body)
                .accessibilityHidden(true)
            VStack(alignment: .leading, spacing: 5) {
                Text(title).sirsiFont(.callout, weight: .semibold)
                    .fixedSize(horizontal: false, vertical: true)
                if let detail, !detail.isEmpty {
                    Text(detail).sirsiFont(.caption)
                        .foregroundStyle(prominent ? PantheonTheme.canvas.opacity(0.9) : PantheonTheme.text.opacity(0.8))
                        .fixedSize(horizontal: false, vertical: true)
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
        }
        .multilineTextAlignment(.leading)
        .fixedSize(horizontal: false, vertical: true)
        .accessibilityElement(children: .combine)
    }
}
