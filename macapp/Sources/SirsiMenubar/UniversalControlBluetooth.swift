import Foundation

enum UniversalControlBluetooth {
    // nil means the report was unavailable or incomplete, not “no mouse”.
    static func mouseNames(in data: Data) -> [String]? {
        guard let root = (try? JSONSerialization.jsonObject(with: data)) as? [String: Any],
              let reports = root["SPBluetoothDataType"] as? [[String: Any]],
              !reports.isEmpty else { return nil }
        var names: [String] = []
        for report in reports {
            guard let controller = report["controller_properties"] as? [String: Any],
                  let state = controller["controller_state"] as? String,
                  !state.isEmpty else { return nil }
            guard let connected = report["device_connected"] else { return nil }
            guard let devices = connected as? [[String: Any]] else { return nil }
            for device in devices {
                guard device.count == 1 else { return nil }
                for (name, raw) in device {
                    guard let properties = raw as? [String: Any],
                          let type = properties["device_minorType"] as? String,
                          !name.isEmpty, !type.isEmpty else { return nil }
                    if type.caseInsensitiveCompare("Mouse") == .orderedSame { names.append(name) }
                }
            }
        }
        return names.sorted()
    }

    static func inspect() -> [String]? {
        let process = Process()
        process.executableURL = URL(fileURLWithPath: "/usr/sbin/system_profiler")
        process.arguments = ["SPBluetoothDataType", "-detailLevel", "mini", "-json", "-timeout", "5"]
        let output = Pipe()
        process.standardOutput = output
        process.standardError = FileHandle.nullDevice
        do { try process.run() } catch { return nil }
        let data = output.fileHandleForReading.readDataToEndOfFile()
        process.waitUntilExit()
        guard process.terminationStatus == 0 else { return nil }
        return mouseNames(in: data)
    }
}
