import Foundation
import WidgetKit

/// Manages shared data between the main app and widget extensions via App Group container.
/// Widgets cannot call PantheonCore directly in the background — the app writes scan results
/// to the shared container, and widgets read from it.
enum SharedDataManager {
    static let appGroupID = "group.ai.sirsi.pantheon"

    private static var defaults: UserDefaults? {
        UserDefaults(suiteName: appGroupID)
    }

    // MARK: - Keys

    private enum Key {
        static let lastScanDate = "lastScanDate"
        static let scanFindingCount = "scanFindingCount"
        static let scanTotalSize = "scanTotalSize"
        static let scanTopFindings = "scanTopFindings"
        static let scanRulesRan = "scanRulesRan"
        static let hardwareJSON = "hardwareJSON"
        static let guardTotalRAM = "guardTotalRAM"
        static let guardUsedRAM = "guardUsedRAM"
        static let guardProcessCount = "guardProcessCount"
        static let guardTopProcesses = "guardTopProcesses"
        static let guardLastUpdate = "guardLastUpdate"
        static let appVersion = "appVersion"
    }

    // MARK: - Write: Scan Results (Main App → Widget)

    static func saveScanResults(
        findingCount: Int,
        totalSize: Int64,
        topFindings: [(name: String, size: Int64)],
        rulesRan: Int
    ) {
        guard let defaults else { return }
        defaults.set(Date(), forKey: Key.lastScanDate)
        defaults.set(findingCount, forKey: Key.scanFindingCount)
        defaults.set(totalSize, forKey: Key.scanTotalSize)
        defaults.set(rulesRan, forKey: Key.scanRulesRan)

        let encoded = topFindings.map { ["name": $0.name, "size": String($0.size)] }
        defaults.set(encoded, forKey: Key.scanTopFindings)

        WidgetCenter.shared.reloadTimelines(ofKind: "ai.sirsi.pantheon.anubis")
    }

    // MARK: - Write: Hardware JSON (Main App → Widget)

    static func saveHardwareJSON(_ json: String) {
        defaults?.set(json, forKey: Key.hardwareJSON)
        WidgetCenter.shared.reloadTimelines(ofKind: "ai.sirsi.pantheon.seba")
    }

    // MARK: - Write: Guard Stats (Main App → Widget)

    static func saveGuardStats(
        totalRAM: Int64,
        usedRAM: Int64,
        processCount: Int,
        topProcesses: [ProcessSnapshot]
    ) {
        guard let defaults else { return }
        defaults.set(totalRAM, forKey: Key.guardTotalRAM)
        defaults.set(usedRAM, forKey: Key.guardUsedRAM)
        defaults.set(processCount, forKey: Key.guardProcessCount)
        defaults.set(Date(), forKey: Key.guardLastUpdate)

        let encoded = topProcesses.prefix(5).map { proc in
            [
                "name": proc.name,
                "ram": String(proc.ramBytes),
                "pid": String(proc.pid),
            ]
        }
        defaults.set(encoded, forKey: Key.guardTopProcesses)

        WidgetCenter.shared.reloadTimelines(ofKind: "ai.sirsi.pantheon.guard")
    }

    // MARK: - Write: App Version (Main App → Widget)

    static func saveAppVersion(_ version: String) {
        defaults?.set(version, forKey: Key.appVersion)
    }

    // MARK: - Read: Scan Snapshot (Widget)

    struct ScanSnapshot {
        let date: Date?
        let findingCount: Int
        let totalSize: Int64
        let topFindings: [(name: String, size: Int64)]
        let rulesRan: Int
        var isStale: Bool {
            guard let date else { return true }
            return Date().timeIntervalSince(date) > 3600
        }
    }

    static func loadScanSnapshot() -> ScanSnapshot? {
        guard let defaults else { return nil }
        guard defaults.object(forKey: Key.scanFindingCount) != nil else { return nil }

        let findings: [(String, Int64)]
        if let raw = defaults.array(forKey: Key.scanTopFindings) as? [[String: String]] {
            findings = raw.compactMap { dict in
                guard let name = dict["name"],
                      let sizeStr = dict["size"],
                      let size = Int64(sizeStr) else { return nil }
                return (name, size)
            }
        } else {
            findings = []
        }

        return ScanSnapshot(
            date: defaults.object(forKey: Key.lastScanDate) as? Date,
            findingCount: defaults.integer(forKey: Key.scanFindingCount),
            totalSize: Int64(defaults.integer(forKey: Key.scanTotalSize)),
            topFindings: findings,
            rulesRan: defaults.integer(forKey: Key.scanRulesRan)
        )
    }

    // MARK: - Read: Hardware JSON (Widget)

    static func loadHardwareJSON() -> String? {
        defaults?.string(forKey: Key.hardwareJSON)
    }

    // MARK: - Read: Guard Snapshot (Widget)

    struct GuardSnapshot {
        let totalRAM: Int64
        let usedRAM: Int64
        let processCount: Int
        let topProcesses: [ProcessSnapshot]
        let lastUpdate: Date?

        var ramUsagePercent: Double {
            guard totalRAM > 0 else { return 0 }
            return Double(usedRAM) / Double(totalRAM)
        }

        var isStale: Bool {
            guard let lastUpdate else { return true }
            return Date().timeIntervalSince(lastUpdate) > 3600
        }
    }

    static func loadGuardSnapshot() -> GuardSnapshot? {
        guard let defaults else { return nil }
        guard defaults.object(forKey: Key.guardTotalRAM) != nil else { return nil }

        let topProcesses: [ProcessSnapshot]
        if let raw = defaults.array(forKey: Key.guardTopProcesses) as? [[String: String]] {
            topProcesses = raw.compactMap { dict in
                guard let name = dict["name"],
                      let ramStr = dict["ram"],
                      let ram = Int64(ramStr),
                      let pidStr = dict["pid"],
                      let pid = Int32(pidStr) else { return nil }
                return ProcessSnapshot(name: name, ramBytes: ram, pid: pid)
            }
        } else {
            topProcesses = []
        }

        return GuardSnapshot(
            totalRAM: Int64(defaults.integer(forKey: Key.guardTotalRAM)),
            usedRAM: Int64(defaults.integer(forKey: Key.guardUsedRAM)),
            processCount: defaults.integer(forKey: Key.guardProcessCount),
            topProcesses: topProcesses,
            lastUpdate: defaults.object(forKey: Key.guardLastUpdate) as? Date
        )
    }
}

// MARK: - Shared Models

/// A snapshot of a single process for the Guard widget.
struct ProcessSnapshot: Equatable {
    let name: String
    let ramBytes: Int64
    let pid: Int32

    var formattedRAM: String {
        ByteCountFormatter.string(fromByteCount: ramBytes, countStyle: .memory)
    }
}
