import AppKit
import SwiftUI

// The supported macOS control is opened on demand. This is a guided route;
// opening settings does not claim that a peer is connected or input works.
struct UniversalControlHelp: View {
    @State private var expanded = false
    @State private var openFailed = false

    var body: some View {
        DisclosureGroup("Keyboard and mouse sharing", isExpanded: $expanded) {
            VStack(alignment: .leading, spacing: 8) {
                Text("On the Mac that owns your mouse, open Displays. Use the add display menu to link the affected Mac under Link Keyboard and Mouse. Both Macs must be awake and unlocked.")
                    .sirsiFont(.callout)
                    .fixedSize(horizontal: false, vertical: true)
                Text("After linking, move the pointer and type on the other Mac to confirm both work. Opening settings does not change a connection.")
                    .sirsiFont(.caption).foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
                Button("Open Displays Settings") {
                    guard let url = URL(string: "x-apple.systempreferences:com.apple.Displays-Settings.extension") else {
                        openFailed = true
                        return
                    }
                    openFailed = !NSWorkspace.shared.open(url)
                }
                if openFailed {
                    Text("Could not open Displays. Open System Settings and choose Displays.")
                        .sirsiFont(.caption).foregroundStyle(.secondary)
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .padding(.top, 8)
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 10)
    }
}
