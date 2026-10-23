import Foundation

// Display freshness is not operation admission. Missing, future or expired
// observations never establish current health, activity or availability.
func pantheonObservationIsCurrent(_ observed: Date?, at now: Date, maxAge: TimeInterval) -> Bool {
    guard let observed else { return false }
    let age = now.timeIntervalSince(observed)
    return age.isFinite && age >= -2 && age <= maxAge
}

func pantheonObservationDate(_ value: String?) -> Date? {
    guard let value else { return nil }
    let fractional = ISO8601DateFormatter()
    fractional.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
    return fractional.date(from: value) ?? ISO8601DateFormatter().date(from: value)
}
