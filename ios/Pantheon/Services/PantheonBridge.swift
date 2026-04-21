import Foundation
import PantheonCore  // gomobile-generated framework

/// Bridge between Swift and the Go mobile package.
/// All Go functions return JSON strings; this layer deserializes them into Swift types.
final class PantheonBridge: Sendable {

    // MARK: - Response envelope (matches mobile.Response in Go)

    struct BridgeResponse<T: Decodable>: Decodable {
        let ok: Bool
        let data: T?
        let error: String?
    }

    // MARK: - Anubis

    func anubisCategories() throws -> [ScanCategory] {
        let json = MobileAnubisCategories()
        return try decode(json)
    }

    func anubisScan(rootPath: String, categories: [String] = []) async throws -> ScanResult {
        let options = try JSONEncoder().encode(["categories": categories])
        let optionsStr = String(data: options, encoding: .utf8) ?? ""

        return try await Task.detached(priority: .userInitiated) {
            let json = MobileAnubisScan(rootPath, optionsStr)
            return try self.decode(json) as ScanResult
        }.value
    }

    // MARK: - Ka

    func kaHunt(includeSudo: Bool = false) async throws -> [GhostApp] {
        return try await Task.detached(priority: .userInitiated) {
            let json = MobileKaHunt(includeSudo)
            return try self.decode(json) as [GhostApp]
        }.value
    }

    func kaEnumerateApps() async throws -> [InstalledApp] {
        return try await Task.detached(priority: .userInitiated) {
            let json = MobileKaEnumerateApps()
            return try self.decode(json) as [InstalledApp]
        }.value
    }

    // MARK: - Thoth

    func thothInit(projectRoot: String) async throws -> ProjectInfo {
        return try await Task.detached(priority: .userInitiated) {
            let json = MobileThothInit(projectRoot)
            return try self.decode(json) as ProjectInfo
        }.value
    }

    func thothSync(root: String) async throws {
        let options = try JSONEncoder().encode(["root": root])
        let optionsStr = String(data: options, encoding: .utf8) ?? ""

        try await Task.detached(priority: .userInitiated) {
            let json = MobileThothSync(optionsStr)
            let _: [String: String] = try self.decode(json)
        }.value
    }

    func thothCompact(root: String) async throws {
        let options = try JSONEncoder().encode(["repo_root": root])
        let optionsStr = String(data: options, encoding: .utf8) ?? ""

        try await Task.detached(priority: .userInitiated) {
            let json = MobileThothCompact(optionsStr)
            let _: [String: String] = try self.decode(json)
        }.value
    }

    func thothDetectProject(root: String) throws -> ProjectInfo {
        let json = MobileThothDetectProject(root)
        return try decode(json)
    }

    // MARK: - Seba

    func sebaDetectHardware() async throws -> HardwareProfile {
        return try await Task.detached(priority: .userInitiated) {
            let json = MobileSebaDetectHardware()
            return try self.decode(json) as HardwareProfile
        }.value
    }

    func sebaDetectAccelerators() async throws -> AcceleratorProfile {
        return try await Task.detached(priority: .userInitiated) {
            let json = MobileSebaDetectAccelerators()
            return try self.decode(json) as AcceleratorProfile
        }.value
    }

    // MARK: - Seshat

    func seshatListSources() throws -> [KnowledgeSource] {
        let json = MobileSeshatListSources()
        return try decode(json)
    }

    func seshatIngest(sources: [String], sinceDays: Int = 7) async throws -> [IngestResult] {
        struct IngestOptions: Encodable {
            let sources: [String]
            let sinceDays: Int

            enum CodingKeys: String, CodingKey {
                case sources
                case sinceDays = "since_days"
            }
        }
        let options = try JSONEncoder().encode(IngestOptions(sources: sources, sinceDays: sinceDays))
        let optionsStr = String(data: options, encoding: .utf8) ?? ""

        return try await Task.detached(priority: .userInitiated) {
            let json = MobileSeshatIngest(optionsStr)
            return try self.decode(json) as [IngestResult]
        }.value
    }

    // MARK: - Brain (Inference)

    func brainClassify(filePath: String) async throws -> FileClassification {
        return try await Task.detached(priority: .userInitiated) {
            let json = MobileBrainClassify(filePath)
            return try self.decode(json) as FileClassification
        }.value
    }

    func brainClassifyBatch(paths: [String], workers: Int = 4) async throws -> BatchClassificationResult {
        let pathsData = try JSONEncoder().encode(paths)
        let pathsStr = String(data: pathsData, encoding: .utf8) ?? "[]"

        return try await Task.detached(priority: .userInitiated) {
            let json = MobileBrainClassifyBatch(pathsStr, workers)
            return try self.decode(json) as BatchClassificationResult
        }.value
    }

    func brainModelInfo() throws -> ModelInfo {
        let json = MobileBrainModelInfo()
        return try decode(json)
    }

    // MARK: - RTK

    func rtkDefaultConfig() throws -> FilterConfig {
        let json = MobileRtkDefaultConfig()
        return try decode(json)
    }

    func rtkFilter(rawOutput: String, configJSON: String = "") async throws -> FilterResult {
        return try await Task.detached(priority: .userInitiated) {
            let json = MobileRtkFilter(rawOutput, configJSON)
            return try self.decode(json) as FilterResult
        }.value
    }

    // MARK: - Vault

    func vaultStore(source: String, tag: String, content: String, tokens: Int) async throws -> VaultEntry {
        return try await Task.detached(priority: .userInitiated) {
            let json = MobileVaultStore(source, tag, content, Int64(tokens))
            return try self.decode(json) as VaultEntry
        }.value
    }

    func vaultSearch(query: String, limit: Int = 10) async throws -> VaultSearchResult {
        return try await Task.detached(priority: .userInitiated) {
            let json = MobileVaultSearch(query, Int64(limit))
            return try self.decode(json) as VaultSearchResult
        }.value
    }

    func vaultGet(id: Int64) async throws -> VaultEntry {
        return try await Task.detached(priority: .userInitiated) {
            let json = MobileVaultGet(id)
            return try self.decode(json) as VaultEntry
        }.value
    }

    func vaultStats() async throws -> VaultStoreStats {
        return try await Task.detached(priority: .userInitiated) {
            let json = MobileVaultStats()
            return try self.decode(json) as VaultStoreStats
        }.value
    }

    func vaultPrune(olderThanHours: Int) async throws -> VaultPruneResult {
        return try await Task.detached(priority: .userInitiated) {
            let json = MobileVaultPrune(Int64(olderThanHours))
            return try self.decode(json) as VaultPruneResult
        }.value
    }

    // MARK: - Horus

    func horusParseDir(root: String) async throws -> HorusSymbolGraph {
        return try await Task.detached(priority: .userInitiated) {
            let json = MobileHorusParseDir(root)
            return try self.decode(json) as HorusSymbolGraph
        }.value
    }

    func horusFileOutline(root: String, filePath: String) async throws -> HorusOutlineResult {
        return try await Task.detached(priority: .userInitiated) {
            let json = MobileHorusFileOutline(root, filePath)
            return try self.decode(json) as HorusOutlineResult
        }.value
    }

    func horusContextFor(root: String, symbolName: String) async throws -> HorusContextResult {
        return try await Task.detached(priority: .userInitiated) {
            let json = MobileHorusContextFor(root, symbolName)
            return try self.decode(json) as HorusContextResult
        }.value
    }

    func horusMatchSymbols(root: String, pattern: String) async throws -> [HorusSymbol] {
        return try await Task.detached(priority: .userInitiated) {
            let json = MobileHorusMatchSymbols(root, pattern)
            return try self.decode(json) as [HorusSymbol]
        }.value
    }

    // MARK: - Stele

    func steleReadRecent(count: Int) async throws -> [SteleEntry] {
        return try await Task.detached(priority: .userInitiated) {
            let json = MobileSteleReadRecent(Int64(count))
            return try self.decode(json) as [SteleEntry]
        }.value
    }

    func steleStats() throws -> SteleStats {
        let json = MobileSteleStats()
        return try decode(json)
    }

    func steleVerify() async throws -> SteleVerifyResult {
        return try await Task.detached(priority: .userInitiated) {
            let json = MobileSteleVerify()
            return try self.decode(json) as SteleVerifyResult
        }.value
    }

    // MARK: - Version

    func version() -> String {
        return MobileVersion()
    }

    // MARK: - JSON Decoding

    private func decode<T: Decodable>(_ json: String) throws -> T {
        guard let data = json.data(using: .utf8) else {
            throw BridgeError.invalidJSON
        }

        let decoder = JSONDecoder()
        decoder.keyDecodingStrategy = .convertFromSnakeCase

        let response = try decoder.decode(BridgeResponse<T>.self, from: data)

        guard response.ok, let result = response.data else {
            throw BridgeError.goError(response.error ?? "unknown error")
        }

        return result
    }
}

// MARK: - Bridge Errors

enum BridgeError: LocalizedError {
    case invalidJSON
    case goError(String)

    var errorDescription: String? {
        switch self {
        case .invalidJSON:
            return "Invalid JSON response from Pantheon core"
        case .goError(let message):
            return "Pantheon: \(message)"
        }
    }
}
