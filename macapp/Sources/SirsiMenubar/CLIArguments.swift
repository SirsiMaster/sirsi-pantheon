import Foundation

enum SnapshotAppearance: Equatable {
    case light
    case dark
}

enum MenubarCommand: Equatable {
    case launch
    case help
    case snapshot(directory: String, width: Double, appearance: SnapshotAppearance)
}

enum MenubarCommandError: Error, Equatable {
    case unknownFlag(String)
    case snapshotRequiresDirectory
    case widthRequiresNumber
    case appearanceRequiresValue
    case snapshotRequired(String)

    var message: String {
        switch self {
        case let .unknownFlag(flag): return "unknown flag \(flag)"
        case .snapshotRequiresDirectory: return "--snapshot requires a directory"
        case .widthRequiresNumber: return "--width requires a number"
        case .appearanceRequiresValue: return "--appearance requires light or dark"
        case let .snapshotRequired(flag): return "\(flag) requires --snapshot"
        }
    }
}

enum MenubarCommandParser {
    static let usage = """
    Usage: SirsiMenubar [--snapshot <dir> [--width <pt>] [--appearance light|dark]]

    With no arguments, launches the menubar surface (accessory app, no Dock icon).

      --snapshot <dir>          render the popover's key screens to PNGs and exit
      --width <pt>              snapshot width in points (default 380)
      --appearance light|dark   snapshot appearance (default dark)
      -h, --help                print this message and exit
    """

    static func parse(_ argv: [String]) throws -> MenubarCommand {
        let arguments = Array(argv.dropFirst())
        let knownFlags: Set<String> = ["--snapshot", "--width", "--appearance"]
        let flags = arguments.filter { $0.hasPrefix("-") }

        if flags.contains("--help") || flags.contains("-h") {
            return .help
        }
        if let unknown = flags.first(where: { !knownFlags.contains($0) }) {
            throw MenubarCommandError.unknownFlag(unknown)
        }

        guard let snapshotIndex = arguments.firstIndex(of: "--snapshot") else {
            if let orphan = flags.first {
                throw MenubarCommandError.snapshotRequired(orphan)
            }
            return .launch
        }
        guard snapshotIndex + 1 < arguments.count, !arguments[snapshotIndex + 1].hasPrefix("-") else {
            throw MenubarCommandError.snapshotRequiresDirectory
        }

        var width = 380.0
        if let widthIndex = arguments.firstIndex(of: "--width") {
            guard widthIndex + 1 < arguments.count, let value = Double(arguments[widthIndex + 1]) else {
                throw MenubarCommandError.widthRequiresNumber
            }
            width = value
        }

        var appearance: SnapshotAppearance = .dark
        if let appearanceIndex = arguments.firstIndex(of: "--appearance") {
            guard appearanceIndex + 1 < arguments.count else {
                throw MenubarCommandError.appearanceRequiresValue
            }
            switch arguments[appearanceIndex + 1] {
            case "light": appearance = .light
            case "dark": appearance = .dark
            default: throw MenubarCommandError.appearanceRequiresValue
            }
        }

        return .snapshot(directory: arguments[snapshotIndex + 1], width: width, appearance: appearance)
    }
}
