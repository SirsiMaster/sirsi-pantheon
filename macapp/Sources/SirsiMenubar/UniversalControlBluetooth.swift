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

    static func parseCapture(_ output: String, complete: Bool, exitCode: Int32?) -> [String]? {
        guard complete, exitCode == 0 else { return nil }
        return mouseNames(in: Data(output.utf8))
    }

    static func inspect() async -> [String]? {
        let result = await SirsiEngine.captureProcess(executable: "/usr/sbin/system_profiler",
            args: ["SPBluetoothDataType", "-detailLevel", "mini", "-json", "-timeout", "5"],
            timeout: 7, outputLimit: 256 * 1024,
            environment: ["PATH": "/usr/bin:/bin:/usr/sbin:/sbin", "LC_ALL": "C"])
        return parseCapture(result.stdout, complete: result.complete, exitCode: result.exitCode)
    }
}
