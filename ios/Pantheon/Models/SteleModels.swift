import Foundation

// MARK: - Stele (Event Ledger)

struct SteleEntry: Codable, Identifiable {
    let seq: Int64
    let prev: String
    let deity: String
    let type: String
    let scope: String
    let data: [String: String]?
    let ts: String
    let hash: String

    var id: Int64 { seq }

    /// Human-readable relative timestamp.
    var relativeTime: String {
        guard let date = ISO8601DateFormatter().date(from: ts) else { return ts }
        let interval = Date().timeIntervalSince(date)
        switch interval {
        case ..<60:       return "just now"
        case ..<3600:     return "\(Int(interval / 60))m ago"
        case ..<86400:    return "\(Int(interval / 3600))h ago"
        case ..<604800:   return "\(Int(interval / 86400))d ago"
        default:          return "\(Int(interval / 604800))w ago"
        }
    }

    /// Deity glyph for display.
    var deityGlyph: String {
        SteleEntry.deityGlyphs[deity.lowercased()] ?? deity
    }

    /// Deity display name (capitalized).
    var deityName: String {
        deity.prefix(1).uppercased() + deity.dropFirst()
    }

    /// Event type color name for theming.
    var typeColorName: String {
        switch type {
        case "governance", "maat_weigh", "maat_pulse", "maat_audit", "maat_heal":
            return "governance"
        case "commit":
            return "commit"
        case "tool_use":
            return "toolUse"
        case "deploy_start", "deploy_end":
            return "deploy"
        case "drift_check", "neith_drift":
            return "driftCheck"
        case "failed":
            return "error"
        default:
            return "default"
        }
    }

    static let deityGlyphs: [String: String] = [
        "anubis": "\u{130E3}",
        "maat": "\u{13184}",
        "thoth": "\u{1305F}",
        "ra": "\u{131F6}",
        "neith": "\u{1306F}",
        "seba": "\u{131FD}",
        "seshat": "\u{13046}",
        "isis": "\u{13050}",
        "osiris": "\u{13079}",
        "horus": "\u{13080}",
        "ka": "\u{13093}",
        "rtk": "\u{26A1}",
        "vault": "\u{1F3DB}\u{FE0F}",
    ]
}

struct SteleStats: Codable {
    let totalEntries: Int
    let deityCounts: [String: Int]
    let typeCounts: [String: Int]
    let firstTs: String?
    let lastTs: String?

    /// Time span description.
    var timeSpan: String {
        guard let first = firstTs, let last = lastTs,
              let firstDate = ISO8601DateFormatter().date(from: first),
              let lastDate = ISO8601DateFormatter().date(from: last) else {
            return "No events"
        }
        let interval = lastDate.timeIntervalSince(firstDate)
        if interval < 3600 { return "\(Int(interval / 60)) minutes" }
        if interval < 86400 { return "\(Int(interval / 3600)) hours" }
        return "\(Int(interval / 86400)) days"
    }
}

struct SteleVerifyResult: Codable {
    let status: String
    let chainLength: Int
    let totalCount: Int
    let breaks: [String]
    let verifiedAt: String

    var isVerified: Bool { status == "verified" }
}
