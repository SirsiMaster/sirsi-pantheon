import Foundation
import XCTest
@testable import SirsiMenubar

final class MercuryWorkspaceTests: XCTestCase {
    func testCurrentObservationRequiresPublishedLiveStateAndSuccessfulRead() throws {
        for state in ["live", "stale", "down", "error", "unknown", ""] {
            let payload = "{\"state\":\"\(state)\",\"samples\":[{\"t\":100}]}"
            let machine = try JSONDecoder().decode(MercuryMonitorMachine.self, from: Data(payload.utf8))
            XCTAssertEqual(machine.hasCurrentObservation(at: Date(timeIntervalSince1970: 105), readSucceeded: true), state == "live")
            XCTAssertFalse(machine.hasCurrentObservation(at: Date(timeIntervalSince1970: 105), readSucceeded: false))
        }
        let missingState = try JSONDecoder().decode(MercuryMonitorMachine.self, from: Data(#"{"samples":[{"t":100}]}"#.utf8))
        XCTAssertFalse(missingState.hasCurrentObservation(at: Date(timeIntervalSince1970: 105), readSucceeded: true))
    }

    func testRetainedObservationAgesWithoutAReplacementResponse() throws {
        let machine = try JSONDecoder().decode(MercuryMonitorMachine.self, from: Data(#"{"state":"live","samples":[{"t":100}]}"#.utf8))
        XCTAssertTrue(machine.hasCurrentObservation(at: Date(timeIntervalSince1970: 110), readSucceeded: true))
        XCTAssertFalse(machine.hasCurrentObservation(at: Date(timeIntervalSince1970: 110.001), readSucceeded: true))
        XCTAssertFalse(machine.hasCurrentObservation(at: Date(timeIntervalSince1970: 90), readSucceeded: true))
    }

    func testMonitorRejectsStaleFutureAndInvalidCoreObservations() throws {
        let sample = try JSONDecoder().decode(MercuryMonitorSample.self, from: Data(#"{"t":100,"cores":[20,40]}"#.utf8))
        XCTAssertEqual(sample.meanCPU, 30)
        XCTAssertTrue(sample.isFresh(at: Date(timeIntervalSince1970: 105)))
        XCTAssertFalse(sample.isFresh(at: Date(timeIntervalSince1970: 120)))
        XCTAssertFalse(sample.isFresh(at: Date(timeIntervalSince1970: 90)))
        let invalid = try JSONDecoder().decode(MercuryMonitorSample.self, from: Data(#"{"t":100,"cores":[-1,101]}"#.utf8))
        XCTAssertNil(invalid.meanCPU)
    }

    @MainActor func testExistingLocalMonitorWhenExplicitlyRequested() async throws {
        guard ProcessInfo.processInfo.environment["PANTHEON_LIVE_MERCURY"] == "1" else {
            throw XCTSkip("Opt-in GET against the existing Mercury monitor")
        }
        let monitor = MercuryMonitor()
        await monitor.refresh()
        XCTAssertNil(monitor.error)
        let projection = try XCTUnwrap(monitor.projection)
        XCTAssertFalse(projection.machines.isEmpty)
        XCTAssertTrue(projection.machines.values.contains { $0.latest?.isFresh(at: Date()) == true })
    }

    func testDecodesCanonicalRouterPingSchemaAndSelectsMercuryExactly() throws {
        let data = Data(#"[{"agent":"hermes","verdict":"LIVE","detail":"other lane","workable":true},{"agent":"mercury","verdict":"WAKEABLE","detail":"published worker is ready","workable":true,"thread":"mercury-mercury-m1","reported_at":"2026-10-23T12:00:00Z","report_summary":"consumer completed"}]"#.utf8)

        let lane = try decodeMercuryLane(data)

        XCTAssertEqual(lane.agent, "mercury")
        XCTAssertEqual(lane.verdict, "WAKEABLE")
        XCTAssertEqual(lane.detail, "published worker is ready")
        XCTAssertEqual(lane.workable, true)
        XCTAssertEqual(lane.thread, "mercury-mercury-m1")
        XCTAssertEqual(lane.reportedAt, "2026-10-23T12:00:00Z")
        XCTAssertEqual(lane.reportSummary, "consumer completed")
    }

    func testRejectsRouterPingContainingOnlyAnotherLane() {
        let data = Data(#"[{"agent":"hermes","verdict":"LIVE","detail":"not Mercury","workable":true}]"#.utf8)

        XCTAssertThrowsError(try decodeMercuryLane(data)) { error in
            XCTAssertEqual(error as? MercuryWorkspaceDecodeError, .noMercuryLane)
        }
    }

    func testRejectsMalformedLaneAndMachineRecordsWithMissingRequiredFields() {
        let laneMissingAgent = Data(#"[{"verdict":"LIVE","detail":"missing agent","workable":true}]"#.utf8)
        let machineMissingModel = Data(#"{"machines":[{"id":"m1","registered_at":"2026-10-01","admitted_by":"ra"}]}"#.utf8)

        XCTAssertThrowsError(try decodeMercuryLane(laneMissingAgent))
        XCTAssertThrowsError(try decodeMercuryMachines(machineMissingModel))
    }

    func testRejectsValidPingPayloadWhenCLIExitStatusIsUnsuccessful() {
        let data = Data(#"[{"agent":"mercury","verdict":"LIVE","detail":"must not be accepted","workable":true}]"#.utf8)
        let read = SirsiEngine.JSONCommandRead(data: data, exitStatus: 1, timedOut: false, launchError: nil)

        XCTAssertFalse(mercuryReadSucceeded(read))
        XCTAssertThrowsError(try decodeMercuryLane(read: read)) { error in
            XCTAssertEqual(error as? MercuryWorkspaceDecodeError, .unsuccessfulRead)
        }
    }

    func testDecodesCanonicalMachinesEnvelope() throws {
        let data = Data(#"{"machines":[{"id":"m1","model":"Mac mini","chip":"M4 Pro","memory_gib":64,"tailscale":"m1.example","lan":"192.0.2.10","monitor_port":8044,"registered_at":"2026-10-01T10:00:00Z","admitted_by":"ra"}]}"#.utf8)

        let report = try decodeMercuryMachines(data)

        XCTAssertEqual(report.machines.count, 1)
        XCTAssertEqual(report.machines[0].id, "m1")
        XCTAssertEqual(report.machines[0].model, "Mac mini")
        XCTAssertEqual(report.machines[0].chip, "M4 Pro")
        XCTAssertEqual(report.machines[0].memoryGiB, 64)
        XCTAssertEqual(report.machines[0].monitorPort, 8044)
        XCTAssertEqual(report.machines[0].admittedBy, "ra")
    }

    func testDecodesCanonicalHardwareLinksReceiptAndUnknownRate() throws {
        let data = Data(#"{"collected_at":"2026-10-23T12:00:00Z","host":"m1.local","macos":"15.0","ports":[{"bus":"Thunderbolt/USB4","receptacle_id":"1","advertised_max":"Up to 120 Gb/s","link_status_raw":"0x1","connected":"yes","peer":"M5","negotiated_rate":"unknown","negotiated_rate_reason":"system_profiler reports only the ceiling"}],"port_count_known":true,"thunderbolt_interfaces":["en7"],"rdma":{"family_loaded":"yes","nvram_enabled":"no","reason":"RDMA NVRAM flag is absent"},"unknown":[],"source":"system_profiler; nvram; sysctl"}"#.utf8)

        let receipt = try decodeMercuryTransportReceipt(data)

        XCTAssertEqual(receipt.host, "m1.local")
        XCTAssertTrue(receipt.portCountKnown)
        XCTAssertEqual(receipt.ports.first?.connected, "yes")
        XCTAssertEqual(receipt.ports.first?.advertisedMax, "Up to 120 Gb/s")
        XCTAssertEqual(receipt.ports.first?.negotiatedRate, "unknown")
        XCTAssertEqual(receipt.thunderboltInterfaces, ["en7"])
        XCTAssertEqual(receipt.rdma.familyLoaded, "yes")
        XCTAssertEqual(receipt.rdma.nvramEnabled, "no")
    }
}
