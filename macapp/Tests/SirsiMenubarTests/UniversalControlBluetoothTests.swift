import XCTest
@testable import SirsiMenubar

final class UniversalControlBluetoothTests: XCTestCase {
    func testIncompleteAndFailedCaptureRemainUnknown() {
        let output = #"{"SPBluetoothDataType":[{"controller_properties":{"controller_state":"attrib_on"},"device_connected":[]}]}"#
        XCTAssertNil(UniversalControlBluetooth.parseCapture(output, complete: false, exitCode: 0))
        XCTAssertNil(UniversalControlBluetooth.parseCapture(output, complete: true, exitCode: 1))
        XCTAssertNil(UniversalControlBluetooth.parseCapture(output, complete: true, exitCode: nil))
        XCTAssertEqual(UniversalControlBluetooth.parseCapture(output, complete: true, exitCode: 0), [])
    }

    func testConnectedBLEMouseIsReportedWithoutOtherDevices() {
        let data = #"{"SPBluetoothDataType":[{"controller_properties":{"controller_state":"attrib_on"},"device_connected":[{"MX Master 3":{"device_minorType":"Mouse"}},{"Headphones":{"device_minorType":"Headphones"}}]}]}"#.data(using: .utf8)!
        XCTAssertEqual(UniversalControlBluetooth.mouseNames(in: data), ["MX Master 3"])
    }

    func testReportWithoutConnectedDevicesFieldIsUnknown() {
        let data = #"{"SPBluetoothDataType":[{"controller_properties":{"controller_state":"attrib_on"},"device_not_connected":[{"Paired Mouse":{"device_minorType":"Mouse"}}]}]}"#.data(using: .utf8)!
        XCTAssertNil(UniversalControlBluetooth.mouseNames(in: data))
    }

    func testExplicitEmptyConnectedDevicesIsEmpty() {
        let data = #"{"SPBluetoothDataType":[{"controller_properties":{"controller_state":"attrib_on"},"device_connected":[]}]}"#.data(using: .utf8)!
        XCTAssertEqual(UniversalControlBluetooth.mouseNames(in: data), [])
    }

    func testMissingOrMalformedReportIsUnknown() {
        for input in ["{}", #"{"SPBluetoothDataType":[]}"#, #"{"SPBluetoothDataType":[{"device_connected":[] }]}"#, #"{"SPBluetoothDataType":[{"controller_properties":{}}]}"#, #"{"SPBluetoothDataType":[{"controller_properties":{"controller_state":"attrib_on"},"device_connected":"bad"}]}"#, #"{"SPBluetoothDataType":[{"controller_properties":{"controller_state":"attrib_on"},"device_connected":[{}]}]}"#, #"{"SPBluetoothDataType":[{"controller_properties":{"controller_state":"attrib_on"},"device_connected":[{"device":{}}]}]}"#] {
            XCTAssertNil(UniversalControlBluetooth.mouseNames(in: input.data(using: .utf8)!))
        }
    }
}
