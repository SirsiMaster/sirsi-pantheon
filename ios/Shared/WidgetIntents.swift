import AppIntents
import WidgetKit
import PantheonCore

// MARK: - Anubis: Trigger Scan from Widget

struct AnubisWidgetScanIntent: AppIntent {
    static let title: LocalizedStringResource = "Scan for Waste"
    static let description: IntentDescription = "Run an Anubis infrastructure scan."
    static let isDiscoverable = false

    func perform() async throws -> some IntentResult {
        let json = MobileAnubisScan("", "")
        if let data = json.data(using: .utf8),
           let response = try? JSONDecoder().decode(WidgetScanResponse.self, from: data),
           response.ok, let scan = response.data {
            let sorted = scan.findings.sorted { $0.sizeBytes > $1.sizeBytes }
            let top = sorted.prefix(3).map { ($0.description, $0.sizeBytes) }
            SharedDataManager.saveScanResults(
                findingCount: scan.findings.count,
                totalSize: scan.totalSize,
                topFindings: top,
                rulesRan: scan.rulesRan
            )
        }
        return .result()
    }
}

// MARK: - Seba: Refresh Hardware from Widget

struct SebaWidgetRefreshIntent: AppIntent {
    static let title: LocalizedStringResource = "Refresh Hardware"
    static let description: IntentDescription = "Refresh Seba hardware profile."
    static let isDiscoverable = false

    func perform() async throws -> some IntentResult {
        let json = MobileSebaDetectHardware()
        SharedDataManager.saveHardwareJSON(json)
        return .result()
    }
}

// MARK: - Guard: Refresh Process Stats from Widget

struct GuardWidgetRefreshIntent: AppIntent {
    static let title: LocalizedStringResource = "Refresh Guard"
    static let description: IntentDescription = "Refresh process watchdog stats."
    static let isDiscoverable = false

    func perform() async throws -> some IntentResult {
        // Collect system memory stats using Mach APIs (available in app extension context).
        let totalRAM = Int64(ProcessInfo.processInfo.physicalMemory)
        let usedRAM = estimateUsedRAM(total: totalRAM)

        SharedDataManager.saveGuardStats(
            totalRAM: totalRAM,
            usedRAM: usedRAM,
            processCount: ProcessInfo.processInfo.activeProcessorCount,
            topProcesses: []
        )
        return .result()
    }

    private func estimateUsedRAM(total: Int64) -> Int64 {
        var size = mach_msg_type_number_t(
            MemoryLayout<vm_statistics64_data_t>.size / MemoryLayout<integer_t>.size
        )
        var stats = vm_statistics64_data_t()
        let result = withUnsafeMutablePointer(to: &stats) { ptr in
            ptr.withMemoryRebound(to: integer_t.self, capacity: Int(size)) { intPtr in
                host_statistics64(mach_host_self(), HOST_VM_INFO64, intPtr, &size)
            }
        }
        guard result == KERN_SUCCESS else { return 0 }

        let pageSize = Int64(vm_kernel_page_size)
        let active = Int64(stats.active_count) * pageSize
        let wired = Int64(stats.wire_count) * pageSize
        let compressed = Int64(stats.compressor_page_count) * pageSize
        return min(active + wired + compressed, total)
    }
}

// MARK: - Decodable helpers (shared between widget intents)

struct WidgetScanResponse: Decodable {
    let ok: Bool
    let data: WidgetScanData?
    let error: String?
}

struct WidgetScanData: Decodable {
    let findings: [WidgetFinding]
    let totalSize: Int64
    let rulesRan: Int

    enum CodingKeys: String, CodingKey {
        case findings
        case totalSize = "TotalSize"
        case rulesRan = "RulesRan"
    }
}

struct WidgetFinding: Decodable {
    let description: String
    let sizeBytes: Int64

    enum CodingKeys: String, CodingKey {
        case description = "Description"
        case sizeBytes = "SizeBytes"
    }
}
