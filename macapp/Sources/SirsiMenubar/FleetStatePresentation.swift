import SwiftUI

// FleetStatePresentation is the native rendering contract for the six Ra
// supervision states.  The producer deliberately emits uppercase canonical
// values; accepting a legacy lowercase spelling here keeps an installed older
// router readable without inventing a stopped/complete state.  Everything
// else remains visibly UNKNOWN so a new router state cannot be quietly
// misrepresented as a healthy lane.
enum FleetStatePresentation {
    private static func canonical(_ raw: String) -> String {
        raw.trimmingCharacters(in: .whitespacesAndNewlines).uppercased()
    }

    static func label(_ raw: String) -> String {
        switch canonical(raw) {
        case "WORKING": return "WORKING"
        case "ASSIGNED": return "ASSIGNED"
        case "IDLE_WITH_WORK": return "IDLE — WORK WAITING"
        case "BLOCKED": return "BLOCKED"
        case "UNROUTABLE": return "UNROUTABLE"
        case "COMPLETE": return "COMPLETE"
        default: return "UNKNOWN — REVIEW"
        }
    }

    static func color(_ raw: String) -> Color {
        switch canonical(raw) {
        case "WORKING": return .green
        case "ASSIGNED": return .blue
        case "IDLE_WITH_WORK", "BLOCKED", "UNROUTABLE": return .orange
        case "COMPLETE": return .secondary
        default: return .orange
        }
    }
}
