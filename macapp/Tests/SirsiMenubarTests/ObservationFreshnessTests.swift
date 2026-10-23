import XCTest
@testable import SirsiMenubar

final class ObservationFreshnessTests: XCTestCase {
    func testObservationExpiryMissingAndFutureTimes() {
        let now = Date(timeIntervalSince1970: 1_792_760_000)
        XCTAssertTrue(pantheonObservationIsCurrent(now, at: now, maxAge: 30))
        XCTAssertTrue(pantheonObservationIsCurrent(now.addingTimeInterval(-30), at: now, maxAge: 30))
        XCTAssertFalse(pantheonObservationIsCurrent(now.addingTimeInterval(-31), at: now, maxAge: 30))
        XCTAssertFalse(pantheonObservationIsCurrent(now.addingTimeInterval(3), at: now, maxAge: 30))
        XCTAssertFalse(pantheonObservationIsCurrent(nil, at: now, maxAge: 30))
    }

    func testApolloMetricsRequireProducerTimestampNotRepeatedReadTime() throws {
        let now = Date(timeIntervalSince1970: 1_792_760_000)
        let date = ISO8601DateFormatter()
        func sample(_ time: String?) throws -> ApolloSessionTelemetry {
            var json: [String: Any] = ["engine_id": "apollo-local", "chip_estates": []]
            if let time { json["emitted_at"] = time }
            return try JSONDecoder().decode(ApolloSessionTelemetry.self, from: JSONSerialization.data(withJSONObject: json))
        }
        XCTAssertTrue(try sample(date.string(from: now)).isFresh(at: now))
        XCTAssertFalse(try sample(date.string(from: now.addingTimeInterval(-16))).isFresh(at: now))
        XCTAssertFalse(try sample(date.string(from: now.addingTimeInterval(3))).isFresh(at: now))
        XCTAssertFalse(try sample(nil).isFresh(at: now))
        XCTAssertFalse(try sample("invalid").isFresh(at: now))
        XCTAssertEqual(apolloTelemetryResolution(read: nil, matchesSelectedPlan: true,
            localRouteHealthy: true, telemetryDecodeFailed: false, telemetryStale: true), .staleEvidence)
    }

    @MainActor func testRaFailedReadCannotRenewOrEstablishCurrentHealth() {
        let engine = SirsiEngine()
        let now = Date()
        engine.routerReadAt = now
        XCTAssertTrue(engine.routerObservationIsCurrent(at: now))
        engine.routerReadError = "read failed"
        XCTAssertFalse(engine.routerObservationIsCurrent(at: now))
        XCTAssertEqual(engine.routerReadAt, now)
        engine.routerReadError = nil
        XCTAssertFalse(engine.routerObservationIsCurrent(at: now.addingTimeInterval(31)))
    }

    @MainActor func testExplicitRaRefreshBypassesEvenFreshCacheAndExpiredCacheIsRejected() {
        let now = Date()
        XCTAssertTrue(SirsiEngine.routerCacheIsUsable(modified: now, at: now, force: false))
        XCTAssertFalse(SirsiEngine.routerCacheIsUsable(modified: now, at: now, force: true))
        XCTAssertFalse(SirsiEngine.routerCacheIsUsable(modified: now.addingTimeInterval(-60), at: now, force: false))
    }
}
