import SwiftUI

struct DiagnosticRepairResultSheet<SheetContent: View>: ViewModifier {
    @Binding var item: DiagnosticRepairPresentation?
    let content: (DiagnosticRepairPresentation) -> SheetContent

    init(item: Binding<DiagnosticRepairPresentation?>,
         @ViewBuilder content: @escaping (DiagnosticRepairPresentation) -> SheetContent) {
        _item = item
        self.content = content
    }

    func body(content base: Content) -> some View {
        base.sheet(item: $item, content: content)
    }
}

// Only this audited command is a read-only preview. Other diagnostics retain
// explicit consent; absence of --confirm alone never proves a command is safe.
func diagnosticOpensMemoryPreview(_ args: [String]) -> Bool {
    args == ["relieve", "--memory"]
}

func actionReviewScope(arguments: [String], directory: URL) -> String {
    // The readable command is supplemented by unambiguous argument boundaries;
    // spaces/newlines in a path or operand must not disguise the reviewed argv.
    let bytes = (try? JSONEncoder().encode(arguments)) ?? Data()
    let exact = String(data: bytes, encoding: .utf8) ?? "unavailable"
    return "Command: sirsi \(arguments.joined(separator: " "))\n\nExact arguments: \(exact)\n\nProject: \(directory.path)"
}

// Native confirmationDialog may omit all but the first message Text on macOS.
// Reviewable scope belongs in a real, scrollable view with persistent controls.
struct ActionReviewSheet: View {
    let title: String
    let detail: String
    let scope: String
    let confirmLabel: String
    let cancel: () -> Void
    let confirm: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            Text(title).sirsiFont(.title2, weight: .semibold).padding(20)
            Divider()
            ScrollView {
                VStack(alignment: .leading, spacing: 16) {
                    Text(detail).sirsiFont(.body)
                        .fixedSize(horizontal: false, vertical: true)
                    Text(scope).sirsiFont(.callout, design: .monospaced)
                        .fixedSize(horizontal: false, vertical: true)
                        .textSelection(.enabled)
                }
                .frame(maxWidth: .infinity, alignment: .leading)
                .padding(20)
            }
            Divider()
            HStack {
                Button("Cancel", action: cancel).keyboardShortcut(.cancelAction)
                Spacer()
                Button(confirmLabel, action: confirm)
                    .buttonStyle(.borderedProminent).tint(gold)
            }.padding(16)
        }
        .background(PantheonTheme.canvas)
        .modifier(ResultPanelLayout())
    }
}
