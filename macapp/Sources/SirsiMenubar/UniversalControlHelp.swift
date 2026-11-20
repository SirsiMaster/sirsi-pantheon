import AppKit
import CoreWLAN
import IOBluetooth
import SwiftUI

// The supported macOS control is opened on demand. This is a guided route;
// opening settings does not claim that a peer is connected or input works.
struct UniversalControlHelp: View {
    @State private var expanded = false
    @State private var openFailed = false
    @State private var radioCheck: String?

    var body: some View {
        DisclosureGroup("Keyboard and mouse sharing", isExpanded: $expanded) {
            VStack(alignment: .leading, spacing: 8) {
                Text("On the Mac that owns your mouse, open Displays. Use the add display menu to link the affected Mac under Link Keyboard and Mouse. Both Macs must be awake and unlocked.")
                    .sirsiFont(.callout)
                    .fixedSize(horizontal: false, vertical: true)
                Text("After linking, move the pointer and type on the other Mac to confirm both work. Opening settings does not change a connection.")
                    .sirsiFont(.caption).foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
                Text("If the pointer crosses but typing does not, check which Mac the keyboard and mouse are connected to. For a switchable mouse, select the channel paired with the keyboard's Mac and try again.")
                    .sirsiFont(.caption).foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
                Text("On both Macs, check Wi-Fi, Bluetooth and Handoff are on, the same Apple Account is signed in, and both are on the same Wi-Fi network. Keep the Macs nearby.")
                    .sirsiFont(.caption).foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
                Link("Apple's Universal Control troubleshooting", destination: URL(string: "https://support.apple.com/guide/mac-help/mchl412faecf/mac")!)
                    .sirsiFont(.caption)
                Button("Check Wi-Fi and Bluetooth on this Mac") {
                    let wifi = CWWiFiClient.shared().interface().map { $0.powerOn() ? "On" : "Off" } ?? "Unavailable"
                    let bluetooth: String
                    if let state = IOBluetoothHostController.default()?.powerState {
                        switch state {
                        case kBluetoothHCIPowerStateON: bluetooth = "On"
                        case kBluetoothHCIPowerStateOFF: bluetooth = "Off"
                        default: bluetooth = "Unavailable"
                        }
                    } else {
                        bluetooth = "Unavailable"
                    }
                    radioCheck = "This Mac — Wi-Fi: \(wifi); Bluetooth: \(bluetooth). This does not check the other Mac or confirm that sharing is connected."
                }
                if let radioCheck {
                    Text(radioCheck)
                        .sirsiFont(.caption).foregroundStyle(.secondary)
                        .fixedSize(horizontal: false, vertical: true)
                }
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
