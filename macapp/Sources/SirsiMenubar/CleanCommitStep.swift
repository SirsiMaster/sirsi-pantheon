import SwiftUI

// CleanCommitStep — the final step of the Clean workflow
// (docs/prd/MENUBAR_CLEAN_WORKFLOW.md): the Clean button showing the CHECKED
// total, the in-progress spinner, and the success line with the honest Trash
// note. Built directly by claude-pantheon (the gemma worker was RAM-blocked —
// the warm broker couldn't fit; the other three step components came through
// the gemma pipeline: CleanReviewRow #254, CleanCategorySection #255,
// CleanScanStep #256).
struct CleanCommitStep: View {
    let checkedCount: Int
    let checkedSizeText: String
    let isCleaning: Bool
    let freedText: String?
    let onClean: () -> Void

    var body: some View {
        VStack(spacing: 6) {
            if let freed = freedText {
                Image(systemName: "checkmark.seal.fill").foregroundStyle(.green)
                Text("Freed \(freed)").font(.system(size: 15, weight: .semibold))
                Text("Files are in the Trash — restore any from there.")
                    .font(.caption2).foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
            } else if isCleaning {
                HStack(spacing: 8) {
                    ProgressView().controlSize(.small)
                    Text("Moving to Trash…").font(.caption).foregroundStyle(.secondary)
                }
            } else if checkedCount == 0 {
                Text("Nothing selected to clean.").font(.caption).foregroundStyle(.secondary)
            } else {
                Button {
                    onClean()
                } label: {
                    Text("Clean \(checkedCount) item\(checkedCount == 1 ? "" : "s") (\(checkedSizeText))")
                        .font(.system(size: 13, weight: .semibold))
                }
                .buttonStyle(.borderedProminent)
            }
        }
        .frame(maxWidth: .infinity)
        .padding(.vertical, 10)
    }
}
