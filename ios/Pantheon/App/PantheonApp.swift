import SwiftUI
import WidgetKit

@main
struct PantheonApp: App {
    @StateObject private var appState = AppState()

    var body: some Scene {
        WindowGroup {
            ContentView()
                .environmentObject(appState)
                .preferredColorScheme(.dark)
                .onOpenURL { url in
                    appState.handleDeepLink(url)
                }
                .task {
                    writeSharedDataOnLaunch()
                }
        }
    }

    /// Populate App Group container with initial data so widgets have something to show.
    private func writeSharedDataOnLaunch() {
        // Save app version
        let version = appState.bridge.version()
        SharedDataManager.saveAppVersion(version)

        // Collect and save guard (process/memory) stats from iOS system APIs.
        let totalRAM = Int64(ProcessInfo.processInfo.physicalMemory)
        // iOS does not expose per-process RAM; use host memory pressure as an approximation.
        let usedRAM = estimateUsedRAM(total: totalRAM)
        let processCount = ProcessInfo.processInfo.activeProcessorCount // proxy on iOS

        SharedDataManager.saveGuardStats(
            totalRAM: totalRAM,
            usedRAM: usedRAM,
            processCount: processCount,
            topProcesses: collectTopProcesses()
        )

        // Reload all widget timelines so they pick up fresh data.
        WidgetCenter.shared.reloadAllTimelines()
    }

    /// Estimate used RAM via host_statistics (available on iOS without entitlements).
    private func estimateUsedRAM(total: Int64) -> Int64 {
        var size = mach_msg_type_number_t(MemoryLayout<vm_statistics64_data_t>.size / MemoryLayout<integer_t>.size)
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

    /// Collect a rough process list. On iOS sandboxed apps, we can only see our own process.
    /// We report the current app plus system-level metrics as synthetic entries.
    private func collectTopProcesses() -> [ProcessSnapshot] {
        var processes: [ProcessSnapshot] = []

        // Current app memory footprint
        var info = mach_task_basic_info()
        var count = mach_msg_type_number_t(MemoryLayout<mach_task_basic_info>.size) / 4
        let result = withUnsafeMutablePointer(to: &info) { ptr in
            ptr.withMemoryRebound(to: integer_t.self, capacity: Int(count)) { intPtr in
                task_info(mach_task_self_, task_flavor_t(MACH_TASK_BASIC_INFO), intPtr, &count)
            }
        }
        if result == KERN_SUCCESS {
            processes.append(ProcessSnapshot(
                name: "Pantheon",
                ramBytes: Int64(info.resident_size),
                pid: ProcessInfo.processInfo.processIdentifier
            ))
        }

        return processes
    }
}
