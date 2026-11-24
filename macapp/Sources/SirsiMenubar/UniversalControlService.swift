import Darwin
import Foundation

enum UniversalControlServiceState: Equatable {
    case running
    case notRunning
    case unknown
}

enum UniversalControlService {
    // A running service is not evidence of a working peer connection.
    static func parse(_ output: String, complete: Bool, exitCode: Int32?) -> UniversalControlServiceState {
        guard complete, exitCode == 0 else { return .unknown }
        let lines = output.split(separator: "\n").map(String.init)
        guard lines.contains("\tpath = /System/Library/LaunchAgents/com.apple.ensemble.plist"),
              lines.contains("\tprogram = /System/Library/CoreServices/UniversalControl.app/Contents/MacOS/UniversalControl") else {
            return .unknown
        }
        let states = lines.filter { $0.hasPrefix("\tstate = ") }
        guard states.count == 1 else { return .unknown }
        switch states[0] {
        case "\tstate = running": return .running
        case "\tstate = waiting": return .notRunning
        default: return .unknown
        }
    }

    static func inspect() async -> UniversalControlServiceState {
        let uid = getuid()
        let result = await SirsiEngine.captureProcess(executable: "/bin/launchctl",
            args: ["print", "gui/\(uid)/com.apple.ensemble"], timeout: 4, outputLimit: 32 * 1024,
            environment: ["PATH": "/usr/bin:/bin:/usr/sbin:/sbin", "LC_ALL": "C"])
        return parse(result.stdout, complete: result.complete, exitCode: result.exitCode)
    }
}
