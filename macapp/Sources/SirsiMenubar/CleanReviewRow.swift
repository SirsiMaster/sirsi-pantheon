import SwiftUI

// CleanReviewRow — one finding row in the Clean-workflow Review step
// (docs/prd/MENUBAR_CLEAN_WORKFLOW.md): a per-item toggle, the name, the wrapped
// path, a safe/caution reason, and the size. Drafted by the local model (gemma,
// TASK: build — the first gemma-built SwiftUI component, once the warm broker
// restored its build path), reviewed + bound by claude-pantheon.
struct CleanReviewRow: View {
    let name: String
    let path: String
    let sizeText: String
    let reason: String
    let isCaution: Bool
    @Binding var checked: Bool

    var body: some View {
        HStack(spacing: 12) {
            Toggle("", isOn: $checked)   // label empty + labelsHidden (gemma omitted the label arg)
                .labelsHidden()
                .toggleStyle(.checkbox)

            VStack(alignment: .leading, spacing: 2) {
                Text(name)
                    .sirsiFont(13, weight: .medium)
                Text(path)
                    .sirsiFont(.caption2, design: .monospaced)
                    .foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
                Text(reason)
                    .sirsiFont(.caption2)
                    .foregroundStyle(isCaution ? .orange : .green)
                    .fixedSize(horizontal: false, vertical: true)
            }

            Spacer()

            Text(sizeText)
                .sirsiFont(.caption, design: .monospaced)
        }
        .padding(.vertical, 6)
    }
}
