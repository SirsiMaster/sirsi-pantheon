import AppKit
import CoreWLAN
import IOBluetooth
import SwiftUI

// The supported macOS control is opened on demand. This is a guided route;
// opening settings does not claim that a peer is connected or input works.
struct UniversalControlHelp: View {
    @Environment(\.snapshotMode) private var snapshotMode
    @State private var expanded = false
    @State private var openFailed = false
    @State private var radioCheck: String?
    @State private var mouseCheck: String?
    @State private var checkingMouse = false
    @State private var keyboardCheck: String?
    @State private var serviceCheck: String?

    init(initialExpanded: Bool = false) {
        _expanded = State(initialValue: initialExpanded)
    }

    var body: some View {
        DisclosureGroup("Keyboard and mouse sharing", isExpanded: $expanded) {
            MaybeScroll {
            VStack(alignment: .leading, spacing: 8) {
                Button(checkingMouse ? "Checking this Mac…" : "Check this Mac") {
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
                    mouseCheck = nil
                    keyboardCheck = nil
                    serviceCheck = nil
                    checkingMouse = true
                    Task {
                        async let mouse = UniversalControlBluetooth.inspect()
                        let keyboard = await Task.detached(priority: .utility) {
                            UniversalControlUSBKeyboard.inspect()
                        }.value
                        let result = (mouse: await mouse, keyboard: keyboard)
                        if let names = result.mouse {
                            mouseCheck = names.isEmpty
                                ? "No Bluetooth mouse was reported on this Mac. USB receivers and unavailable data may not appear."
                                : "Bluetooth mouse reported on this Mac: \(names.joined(separator: ", ")). Verify the keyboard is attached to the same Mac."
                        } else {
                            mouseCheck = "The Bluetooth device report was unavailable. Mouse ownership is unknown."
                        }
                        if let names = result.keyboard {
                            keyboardCheck = names.isEmpty
                                ? "No USB keyboard was reported on this Mac. Other connection types or restricted devices may not appear."
                                : "USB keyboard reported on this Mac: \(names.joined(separator: ", ")). This does not verify that typing works."
                        } else {
                            keyboardCheck = "The USB keyboard report was unavailable. Keyboard ownership is unknown."
                        }
                        switch await UniversalControlService.inspect() {
                        case .running:
                            serviceCheck = "Universal Control service: running on this Mac. This does not confirm that the other Mac is linked or that input works."
                        case .notRunning:
                            serviceCheck = "Universal Control service: waiting on this Mac. Its connection state is unknown."
                        case .unknown:
                            serviceCheck = "Universal Control service status is unavailable on this Mac."
                        }
                        checkingMouse = false
                    }
                }
                .disabled(checkingMouse)
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
                        .fixedSize(horizontal: false, vertical: true)
                }
                if let radioCheck {
                    Text(radioCheck)
                        .sirsiFont(.caption).foregroundStyle(.secondary)
                        .fixedSize(horizontal: false, vertical: true)
                }
                if let mouseCheck {
                    Text(mouseCheck)
                        .sirsiFont(.caption).foregroundStyle(.secondary)
                        .fixedSize(horizontal: false, vertical: true)
                }
                if let keyboardCheck {
                    Text(keyboardCheck)
                        .sirsiFont(.caption).foregroundStyle(.secondary)
                        .fixedSize(horizontal: false, vertical: true)
                }
                if let serviceCheck {
                    Text(serviceCheck)
                        .sirsiFont(.caption).foregroundStyle(.secondary)
                        .fixedSize(horizontal: false, vertical: true)
                }
                Text("On the Mac that owns your mouse, open Displays and link the other Mac under Link Keyboard and Mouse. Keep both awake and unlocked, then test the pointer and keyboard.")
                    .sirsiFont(.callout)
                    .fixedSize(horizontal: false, vertical: true)
                Text("If the pointer crosses but typing does not, connect the mouse and keyboard to the same Mac. For a switchable mouse, select the channel paired with the keyboard's Mac.")
                    .sirsiFont(.caption).foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
                Text("On both Macs, check Wi-Fi, Bluetooth and Handoff are on, the same Apple Account is signed in, and both are on the same Wi-Fi network. Keep the Macs nearby.")
                    .sirsiFont(.caption).foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
                Button("Apple's troubleshooting guide") {
                    if let url = URL(string: "https://support.apple.com/guide/mac-help/mchl412faecf/mac") {
                        NSWorkspace.shared.open(url)
                    }
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .padding(.top, 8)
            }
            .frame(maxHeight: snapshotMode ? nil : 260)
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 10)
    }
}
