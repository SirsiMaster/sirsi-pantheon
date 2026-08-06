import SwiftUI

// CleanCategorySection — groups one cleanup category's rows in the Clean-workflow
// Review step (docs/prd/MENUBAR_CLEAN_WORKFLOW.md): a header with the category
// title, its total size, and a per-category select-all toggle, over the rows the
// parent supplies (composed CleanReviewRows). Drafted by the local model (gemma,
// TASK: build) and Tier-2 bound — gemma's output was correct this pass (it used
// Toggle("", isOn:) after the CleanReviewRow fix); added .toggleStyle(.checkbox)
// for visual consistency with the rows.
struct CleanCategorySection: View {
    let title: String
    let totalText: String
    @Binding var selectAll: Bool
    let content: () -> AnyView

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            HStack {
                Text(title)
                    .sirsiFont(12, weight: .semibold)
                Spacer()
                Text(totalText)
                    .sirsiFont(.caption, design: .monospaced)
                    .foregroundStyle(.secondary)
                Toggle("", isOn: $selectAll)
                    .labelsHidden()
                    .toggleStyle(.checkbox)
                    .controlSize(.small)
            }

            VStack(alignment: .leading, spacing: 0) {
                content()
            }
        }
        .padding(.vertical, 4)
    }
}
