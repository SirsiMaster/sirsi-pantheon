import SwiftUI

// CleanScanStep — the first step of the Clean workflow (docs/prd/MENUBAR_CLEAN_WORKFLOW.md):
// the Scan trigger with in-place progress, empty, and error states (no premature
// navigation). Drafted by the local model (gemma, TASK: build) and Tier-2 bound —
// gemma's output was correct; modernized .foregroundColor → .foregroundStyle to
// match the codebase convention.
struct CleanScanStep: View {
    let isScanning: Bool
    let errorText: String?
    let onScan: () -> Void

    var body: some View {
        VStack(spacing: 8) {
            if let error = errorText {
                VStack(spacing: 8) {
                    Image(systemName: "exclamationmark.triangle.fill")
                        .foregroundStyle(.orange)
                    Text(error)
                        .font(.caption)
                        .fixedSize(horizontal: false, vertical: true)
                    Button("Try again") {
                        onScan()
                    }
                }
            } else if isScanning {
                VStack(spacing: 8) {
                    ProgressView()
                    Text("Searching for leftover apps, caches, logs, and snapshots.")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                        .fixedSize(horizontal: false, vertical: true)
                }
            } else {
                VStack(spacing: 8) {
                    Text("Scan for cleanup")
                        .font(.system(size: 15, weight: .semibold))
                    Text("One tap finds caches, logs, leftover apps, and snapshots.")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                        .fixedSize(horizontal: false, vertical: true)
                    Button {
                        onScan()
                    } label: {
                        Text("Scan")
                            .font(.system(size: 13, weight: .semibold))
                    }
                    .buttonStyle(.borderedProminent)
                }
            }
        }
        .frame(maxWidth: .infinity)
        .padding(.vertical, 24)
    }
}
