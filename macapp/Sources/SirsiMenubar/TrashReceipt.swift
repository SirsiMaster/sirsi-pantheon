import Foundation
import Combine

struct TrashIdentity: Codable, Equatable {
    let device: UInt64
    let inode: UInt64
    let uid: UInt32
    let mode: UInt32
    let size: Int64
    let mtime_ns: Int64
    let ctime_ns: Int64
    let type: String
}

struct TrashMember: Codable, Equatable {
    let path: String
    let identity: TrashIdentity
}

struct TrashManifest: Codable, Equatable {
    let schema: String
    let uid: UInt32
    let root: TrashIdentity
    let containers: [TrashMember]
    let entries: [TrashMember]

    var validWireShape: Bool {
        guard schema == "pantheon.trash-manifest.v1", root.type == "directory", root.mode & 0o170000 == 0o040000,
              root.inode != 0, root.uid == uid, entries.count <= 1024, containers.count <= 64 else { return false }
        var directories: Set<String> = [""]
        var containerPaths: Set<String> = []
        for (isContainer, members) in [(true, containers), (false, entries)] {
        var previous = ""
        for entry in members {
            let parts = entry.path.split(separator: "/", omittingEmptySubsequences: false)
            let parent = parts.dropLast().joined(separator: "/")
            guard !parts.isEmpty, parts.count <= 24, entry.path.utf8.count <= 8192,
                  !parts.contains(where: { $0.isEmpty || $0 == "." || $0 == ".." }),
                  !entry.path.contains("\0"), previous.utf8.lexicographicallyPrecedes(entry.path.utf8),
                  directories.contains(parent), entry.identity.uid == uid,
                  entry.identity.device == root.device, entry.identity.inode != 0,
                  entry.identity.size >= 0,
                  ["directory": UInt32(0o040000), "file": UInt32(0o100000), "symlink": UInt32(0o120000)][entry.identity.type] == entry.identity.mode & 0o170000 else { return false }
            previous = entry.path
            if entry.identity.type == "directory" { directories.insert(entry.path) }
            if isContainer {
                guard entry.identity.type == "directory", containerPaths.insert(entry.path).inserted else { return false }
            } else if containerPaths.contains(entry.path) { return false }
        }
        }
        return directories.count <= 65
    }
}

func trashWireInput<T: Encodable>(_ value: T) throws -> String {
    let data = try JSONEncoder().encode(value)
    guard data.count <= 64 * 1024, let input = String(data: data, encoding: .utf8) else { throw CocoaError(.coderInvalidValue) }
    return input
}

struct TrashBrowseRequest: Codable, Equatable {
    let schema: String
    let directory: String
    let session: UUID?
    let sequence: UInt64
    let generation: String
    let root: TrashIdentity?

    init(directory: String = ".", session: UUID? = nil, sequence: UInt64 = 0, generation: String = "", root: TrashIdentity? = nil) {
        schema = "pantheon.trash-browse.v3"; self.directory = directory
        self.session = session; self.sequence = sequence; self.generation = generation; self.root = root
    }
    enum CodingKeys: String, CodingKey { case schema, directory, session, sequence, generation, root }
    func encode(to encoder: Encoder) throws {
        var c = encoder.container(keyedBy: CodingKeys.self)
        try c.encode(schema, forKey: .schema); try c.encode(directory, forKey: .directory)
        try c.encodeIfPresent(session, forKey: .session); try c.encode(sequence, forKey: .sequence)
        try c.encode(generation, forKey: .generation)
        if let root { try c.encode(root, forKey: .root) } else { try c.encodeNil(forKey: .root) }
    }
}

struct TrashSelectionRequest: Codable {
    let schema: String
    let uid: UInt32
    let root: TrashIdentity
    let containers: [TrashMember]
    let entries: [TrashMember]
}

struct TrashBlockedMember: Codable, Equatable {
    let path: String
    let reason: String
    let type: String

    var explanation: String {
        switch reason {
        case "stat_failed": return "Metadata could not be read"
        case "unsupported_type": return "Unsupported item type"
        case "unowned": return "Not owned by the current user"
        case "cross_device": return "On a different device"
        default: return "Name could not be safely represented"
        }
    }
}

struct TrashBrowseWork: Codable, Equatable {
    let native_calls: Int
    let bytes: Int
    let inspected: Int
}

struct TrashPage: Codable, Equatable {
    let schema: String
    let status: String
    let uid: UInt32
    let root: TrashIdentity
    let directory: String
    let containers: [TrashMember]
    let entries: [TrashMember]
    let blocked: [TrashBlockedMember]
    let work: TrashBrowseWork
    let generation: String
    let more: Bool
    let next: TrashBrowseRequest?
    let errors: [String]

    var validWireShape: Bool {
        let paths = entries.map(\.path)
        guard schema == "pantheon.trash-browse.v3", status == "page", entries.count + blocked.count <= 32,
              (1...34).contains(work.native_calls), (0...(34 * 1024)).contains(work.bytes),
              work.inspected == entries.count + blocked.count, work.native_calls >= work.inspected,
              blocked.allSatisfy({ $0.path.utf8.count <= 8192 && !$0.path.contains("\0") &&
                  ["stat_failed", "unsupported_type", "unowned", "cross_device", "invalid_path", "name_unavailable"].contains($0.reason) }),
              errors.isEmpty, generation.utf8.count == 64,
              Set(paths).count == paths.count,
              paths == paths.sorted(by: { $0.utf8.lexicographicallyPrecedes($1.utf8) }),
              TrashManifest(schema: "pantheon.trash-manifest.v1", uid: uid, root: root,
                            containers: containers, entries: []).validWireShape,
              entries.allSatisfy({ TrashManifest(schema: "pantheon.trash-manifest.v1", uid: uid, root: root,
                                                containers: containers, entries: [$0]).validWireShape }),
              directory == (containers.last?.path ?? "."),
              entries.allSatisfy({ member in
                  let parent = member.path.split(separator: "/").dropLast().joined(separator: "/")
                  return (parent.isEmpty ? "." : parent) == directory
              }), more == (next != nil) else { return false }
        if let next {
            return next.schema == schema && next.directory == directory && next.root == root &&
                next.generation == generation && next.session != nil && next.sequence != 0
        }
        return true
    }
}

struct TrashReport: Codable, Equatable {
    let schema: String
    let status: String
    let preview: TrashManifest?
    let attempted: Int
    let deleted: [String]
    let logical_bytes: Int64
    let errors: [String]

    var validWireShape: Bool {
        guard schema == "pantheon.trash-report.v1", attempted >= 0, attempted <= 1024,
              deleted.count <= attempted, Set(deleted).count == deleted.count,
              logical_bytes >= 0 else { return false }
        switch status {
        case "ready", "empty":
            guard let preview, preview.validWireShape, attempted == 0,
                  deleted.isEmpty, errors.isEmpty, logical_bytes == 0 else { return false }
            return (status == "empty") == preview.entries.isEmpty
        case "completed": return preview == nil && errors.isEmpty && deleted.count == attempted
        case "review_required": return preview == nil && attempted == 0 && deleted.isEmpty && !errors.isEmpty
        case "partial": return preview == nil && !deleted.isEmpty && !errors.isEmpty
        case "failed": return preview == nil && deleted.isEmpty && !errors.isEmpty
        default: return false
        }
    }

    func matches(_ manifest: TrashManifest) -> Bool {
        let ordered = Array(manifest.entries.reversed())
        guard attempted <= ordered.count, deleted == ordered.prefix(deleted.count).map(\.path),
              status != "completed" || deleted.count == ordered.count else { return false }
        var bytes: Int64 = 0
        for entry in ordered.prefix(deleted.count) where entry.identity.type != "directory" {
            let sum = bytes.addingReportingOverflow(entry.identity.size)
            guard !sum.overflow else { return false }; bytes = sum.partialValue
        }
        return bytes == logical_bytes
    }
}

// Decode only a complete bounded capture. A valid nonzero failure report may
// describe a partial prefix, but never becomes confirmed success.
struct TrashCommandRead {
    let report: TrashReport?
    let issue: String?

    init(_ execution: CommandExecution, preview: Bool) {
        guard execution.complete else {
            report = nil; issue = execution.issue ?? "Command completion is unknown. Do not repeat deletion."
            return
        }
        guard let decoded = try? JSONDecoder().decode(TrashReport.self, from: execution.stdoutData),
              decoded.validWireShape else {
            report = nil; issue = "Pantheon could not verify the Trash receipt. Review remaining items; do not repeat deletion."
            return
        }
        let successStatuses = preview ? ["ready", "empty"] : ["completed"]
        if successStatuses.contains(decoded.status) && execution.exitCode != 0 {
            report = nil; issue = "The command failed despite a success-shaped report. Outcome is unverified."
        } else if preview && !["ready", "empty", "failed"].contains(decoded.status) ||
                    !preview && !["completed", "review_required", "partial", "failed"].contains(decoded.status) {
            report = nil; issue = "Unexpected Trash receipt state. Review remaining items."
        } else {
            report = decoded
            issue = successStatuses.contains(decoded.status) ? nil : decoded.errors.joined(separator: "\n")
        }
    }
}

struct TrashApproval: Equatable {
    let id = UUID()
    let manifest: TrashManifest
    let stdin: String
    let args = ["anubis", "empty-trash", "apply", "--confirm"]

    init(_ manifest: TrashManifest) throws {
        guard manifest.validWireShape else { throw CocoaError(.coderInvalidValue) }
        self.manifest = manifest; self.stdin = try trashWireInput(manifest)
    }
}

enum TrashPhase: Equatable { case loading, error, empty, browsing, ready, applying, result }

@MainActor final class TrashWorkflow: ObservableObject {
    let browser = TrashBrowseController()
    @Published private(set) var phase: TrashPhase = .loading
    @Published private(set) var preview: TrashManifest?
    @Published private(set) var approval: TrashApproval?
    @Published private(set) var message: String?
    @Published private(set) var browseFailure: TrashBrowseFailure?
    @Published private(set) var operation: TrashReport?
    @Published private(set) var execution: CommandExecution?
    @Published private(set) var page: TrashPage?
    @Published private(set) var selected: Set<String> = []
    private var generation = UUID()
    private var initialReadStarted = false
    private var browseActive = true
    private var activeApproval: UUID?
    private var explicitBrowseRequired = false

    var canReviewAccess: Bool { phase == .error && browseFailure?.offersAccessGuide == true }

    func beginAccessReview() {
        guard canReviewAccess else { return }
        cancelBrowse()
        explicitBrowseRequired = true; page = nil
    }

    var confirmedSuccess: Bool { operation?.status == "completed" && execution?.complete == true && execution?.exitCode == 0 }

    func loadInitial(capture: () async -> CommandExecution) async {
        guard browseActive, !explicitBrowseRequired, !Task.isCancelled, !initialReadStarted else { return }
        initialReadStarted = true
        await loadPage(capture: capture)
    }

    var browseRequest: TrashBrowseRequest {
        TrashBrowseRequest(directory: page?.directory ?? ".", root: page?.root)
    }

    func cancelBrowse() {
        browseActive = false; browser.invalidate(); generation = UUID(); activeApproval = nil
        guard phase != .applying else { return }
        approval = nil; preview = nil; selected = []
    }

    func activateBrowse() {
        guard !browseActive else { return }
        browseActive = true; initialReadStarted = false
    }

    func loadPage(capture: () async -> CommandExecution) async {
        guard browseActive, phase != .applying else { return }
        let token = UUID(); generation = token
        approval = nil; preview = nil; selected = []; page = nil
        browseFailure = nil; explicitBrowseRequired = false; message = nil; phase = .loading
        let execution = await capture()
        guard generation == token else { return }
        guard !Task.isCancelled else {
            cancelBrowse(); phase = .error; message = "Browse cancelled. Start a fresh browse."; return
        }
        guard execution.complete, execution.exitCode == 0,
              let decoded = try? JSONDecoder().decode(TrashPage.self, from: execution.stdoutData), decoded.validWireShape else {
            browseFailure = TrashBrowseFailure.decode(execution)
            phase = .error; message = browseFailure?.message ?? execution.issue ?? "This Trash page could not be verified. Retry browsing; no deletion was authorized."
            return
        }
        page = decoded; phase = decoded.entries.isEmpty && decoded.blocked.isEmpty && !decoded.more ? .empty : .browsing
    }

    func toggle(_ member: TrashMember) {
        guard phase == .browsing, page?.entries.contains(member) == true else { return }
        if selected.contains(member.path) { selected.remove(member.path) } else { selected.insert(member.path) }
    }

    func showPage() {
        guard phase != .applying, let page else { return }
        generation = UUID(); approval = nil; preview = nil; selected = []; message = nil; browseFailure = nil
        phase = page.entries.isEmpty && page.blocked.isEmpty && !page.more ? .empty : .browsing
    }

    func reviewSelection(capture: (String) async -> CommandExecution) async {
        guard phase == .browsing, let page, !selected.isEmpty else { return }
        let request = TrashSelectionRequest(schema: "pantheon.trash-selection.v1", uid: page.uid, root: page.root,
                                            containers: page.containers, entries: page.entries.filter { selected.contains($0.path) })
        guard let input = try? trashWireInput(request) else {
            message = "This selection exceeds the 64 KiB input budget. Select fewer entries or open a folder."; return
        }
        await loadPreview { await capture(input) }
    }

    // Always read-only. A failed refresh preserves the original operation and
    // capture, including a partial prefix or unknown child-exit debt.
    func loadPreview(capture: () async -> CommandExecution) async {
        guard phase != .applying else { return }
        let token = UUID(); generation = token
        approval = nil; preview = nil; message = nil; browseFailure = nil; phase = .loading
        let read = TrashCommandRead(await capture(), preview: true)
        guard token == generation else { return }
        guard let report = read.report, let manifest = report.preview,
              ["ready", "empty"].contains(report.status) else {
            phase = .error; message = read.issue ?? "Trash could not be read. Nothing is confirmed empty."
            return
        }
        guard (try? trashWireInput(manifest)) != nil else {
            phase = .error; message = "The encoded review exceeds 64 KiB. Browse and choose fewer entries or folder descendants."
            return
        }
        preview = manifest; phase = manifest.entries.isEmpty ? .empty : .ready
    }

    func prepareApproval() {
        guard phase == .ready, let preview, !preview.entries.isEmpty else { return }
        do { approval = try TrashApproval(preview) }
        catch { message = "This review exceeds the 64 KiB input budget. Browse and choose a smaller batch."; approval = nil }
    }

    func cancelApproval() { approval = nil }

    func permitsLaunch(_ captured: TrashApproval) -> Bool {
        phase == .applying && activeApproval == captured.id && preview == captured.manifest
    }

    func apply(capture: (TrashApproval) async -> CommandExecution) async {
        guard phase == .ready, let captured = approval, captured.manifest == preview else { return }
        // Consume the frozen approval and set the latch BEFORE any await.
        approval = nil; activeApproval = captured.id; generation = UUID(); phase = .applying
        operation = nil; execution = nil; message = nil; browseFailure = nil
        await browser.close()
        let receipt: CommandExecution
        if permitsLaunch(captured) { receipt = await capture(captured) }
        else { receipt = CommandExecution(launchError: "The review was dismissed before launch. No deletion started.") }
        activeApproval = nil; execution = receipt
        let read = TrashCommandRead(receipt, preview: false)
        if let report = read.report, report.matches(captured.manifest) {
            operation = report; message = read.issue
        } else {
            operation = nil; message = read.issue ?? "The returned deletion prefix does not match this review. Outcome is unverified."
        }
        phase = .result
    }
}
