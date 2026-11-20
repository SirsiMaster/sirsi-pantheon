import XCTest
@testable import SirsiMenubar

final class UniversalControlBluetoothTests: XCTestCase {
    func testConnectedBLEMouseIsReportedWithoutOtherDevices() {
        let data = #"{"SPBluetoothDataType":[{"controller_properties":{"controller_state":"attrib_on"},"device_connected":[{"MX Master 3":{"device_minorType":"Mouse"}},{"Headphones":{"device_minorType":"Headphones"}}]}]}"#.data(using: .utf8)!
        XCTAssertEqual(UniversalControlBluetooth.mouseNames(in: data), ["MX Master 3"])
    }

    func testValidReportWithoutConnectedDevicesIsEmpty() {
        let data = #"{"SPBluetoothDataType":[{"controller_properties":{"controller_state":"attrib_on"}}]}"#.data(using: .utf8)!
        XCTAssertEqual(UniversalControlBluetooth.mouseNames(in: data), [])
    }

    func testMissingOrMalformedReportIsUnknown() {
        for input in ["{}", #"{"SPBluetoothDataType":[]}"#, #"{"SPBluetoothDataType":[{"device_connected":[] }]}"#, #"{"SPBluetoothDataType":[{"controller_properties":{}}]}"#, #"{"SPBluetoothDataType":[{"controller_properties":{"controller_state":"attrib_on"},"device_connected":"bad"}]}"#, #"{"SPBluetoothDataType":[{"controller_properties":{"controller_state":"attrib_on"},"device_connected":[{}]}]}"#, #"{"SPBluetoothDataType":[{"controller_properties":{"controller_state":"attrib_on"},"device_connected":[{"device":{}}]}]}"#] {
            XCTAssertNil(UniversalControlBluetooth.mouseNames(in: input.data(using: .utf8)!))
        }
    }
}
