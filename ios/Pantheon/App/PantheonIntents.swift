import AppIntents
import PantheonCore

// MARK: - Anubis Scan Shortcut

struct AnubisScanIntent: AppIntent {
    static let title: LocalizedStringResource = "Run Pantheon Scan"
    static let description: IntentDescription = "Scan for infrastructure waste using Anubis."
    static let openAppWhenRun = false

    func perform() async throws -> some IntentResult & ReturnsValue<String> {
        let json = MobileAnubisScan("", "")
        guard let data = json.data(using: .utf8),
              let response = try? JSONDecoder().decode(QuickResponse.self, from: data),
              response.ok else {
            return .result(value: "Scan failed")
        }

        guard let resultData = response.data,
              let scan = try? JSONDecoder().decode(ScanSummary.self, from: resultData) else {
            return .result(value: "No findings")
        }

        let size = ByteCountFormatter.string(fromByteCount: scan.totalSize, countStyle: .file)
        return .result(value: "𓁢 Anubis: \(scan.findingCount) findings, \(size) reclaimable")
    }
}

// MARK: - Thoth Sync Shortcut

struct ThothSyncIntent: AppIntent {
    static let title: LocalizedStringResource = "Sync Project Memory"
    static let description: IntentDescription = "Sync Thoth project memory for the selected project."
    static let openAppWhenRun = false

    @Parameter(title: "Project Path")
    var projectPath: String

    func perform() async throws -> some IntentResult & ReturnsValue<String> {
        let options = "{\"repo_root\":\"\(projectPath)\"}"
        let json = MobileThothSync(options)
        guard let data = json.data(using: .utf8),
              let response = try? JSONDecoder().decode(QuickResponse.self, from: data),
              response.ok else {
            return .result(value: "𓁟 Thoth sync failed")
        }

        return .result(value: "𓁟 Thoth: project memory synced")
    }
}

// MARK: - Seba Hardware Shortcut

struct SebaHardwareIntent: AppIntent {
    static let title: LocalizedStringResource = "Get Device Hardware"
    static let description: IntentDescription = "Detect device hardware using Seba."
    static let openAppWhenRun = false

    func perform() async throws -> some IntentResult & ReturnsValue<String> {
        let json = MobileSebaDetectHardware()
        guard let data = json.data(using: .utf8),
              let response = try? JSONDecoder().decode(QuickResponse.self, from: data),
              response.ok,
              let resultData = response.data,
              let hw = try? JSONDecoder().decode(HWSummary.self, from: resultData) else {
            return .result(value: "Hardware detection failed")
        }

        let ram = ByteCountFormatter.string(fromByteCount: hw.totalRam, countStyle: .memory)
        return .result(value: "𓇽 \(hw.cpuModel) · \(hw.cpuCores) cores · \(ram)")
    }
}

// MARK: - Shortcuts Provider

struct PantheonShortcutsProvider: AppShortcutsProvider {
    static var appShortcuts: [AppShortcut] {
        AppShortcut(
            intent: AnubisScanIntent(),
            phrases: [
                "Run \(.applicationName) scan",
                "Scan with \(.applicationName)",
                "Check infrastructure with \(.applicationName)"
            ],
            shortTitle: "𓁢 Scan",
            systemImageName: "magnifyingglass.circle.fill"
        )
        AppShortcut(
            intent: SebaHardwareIntent(),
            phrases: [
                "Get hardware info from \(.applicationName)",
                "\(.applicationName) hardware profile"
            ],
            shortTitle: "𓇽 Hardware",
            systemImageName: "cpu.fill"
        )
    }
}

// MARK: - Decodable helpers

private struct QuickResponse: Decodable {
    let ok: Bool
    let data: Data?
    let error: String?

    enum CodingKeys: String, CodingKey {
        case ok, data, error
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        ok = try container.decode(Bool.self, forKey: .ok)
        error = try container.decodeIfPresent(String.self, forKey: .error)
        // Capture raw JSON for nested decode
        if let raw = try? container.decode(AnyCodable.self, forKey: .data) {
            data = try? JSONEncoder().encode(raw)
        } else {
            data = nil
        }
    }
}

private struct AnyCodable: Codable {
    let value: Any

    init(from decoder: Decoder) throws {
        let container = try decoder.singleValueContainer()
        if let dict = try? container.decode([String: AnyCodable].self) {
            value = dict.mapValues(\.value)
        } else if let arr = try? container.decode([AnyCodable].self) {
            value = arr.map(\.value)
        } else if let str = try? container.decode(String.self) {
            value = str
        } else if let num = try? container.decode(Double.self) {
            value = num
        } else if let bool = try? container.decode(Bool.self) {
            value = bool
        } else {
            value = ""
        }
    }

    func encode(to encoder: Encoder) throws {
        var container = encoder.singleValueContainer()
        if let dict = value as? [String: Any] {
            let codable = dict.mapValues { AnyCodable(value: $0) }
            try container.encode(codable)
        } else if let arr = value as? [Any] {
            let codable = arr.map { AnyCodable(value: $0) }
            try container.encode(codable)
        } else if let str = value as? String {
            try container.encode(str)
        } else if let num = value as? Double {
            try container.encode(num)
        } else if let bool = value as? Bool {
            try container.encode(bool)
        }
    }

    init(value: Any) { self.value = value }
}

private struct ScanSummary: Decodable {
    let findingCount: Int
    let totalSize: Int64

    enum CodingKeys: String, CodingKey {
        case findingCount = "RulesWithFindings"
        case totalSize = "TotalSize"
    }
}

private struct HWSummary: Decodable {
    let cpuModel: String
    let cpuCores: Int
    let totalRam: Int64

    enum CodingKeys: String, CodingKey {
        case cpuModel = "cpu_model"
        case cpuCores = "cpu_cores"
        case totalRam = "total_ram"
    }
}
