import Foundation
import XCTest
@testable import SirsiMenubar

final class MercuryWorkspaceTests: XCTestCase {
    func testCanonicalReadUsesGPUWorkAndProducerProjectedLinkUnits() throws {
        let projection = try decodeProjection(#"{"state":"ok","message":"Mercury snapshot · fixture","snapshot":{"nodes":[{"id":"m1","name":"M1","chip":"Fixture silicon","state":"live","observed_at":1792339199.7,"gpu":{"work_pct":42.5},"cpu":{"busy_pct":21.25}}]},"scene":{"cables":[{"id":"m1:en1|m2:en1","state":"up","iface":"en1","directions":[{"from":"M1","to":"M2","rate":"12.5 MB/s"}]}]}}"#)
        let node = try XCTUnwrap(projection.snapshot?.nodes.first)

        XCTAssertEqual(projection.gpuWork(for: node, readSucceeded: true), 42.5)
        XCTAssertEqual(projection.cpuBusy(for: node, readSucceeded: true), 21.25)
        XCTAssertEqual(projection.scene?.cables.first?.directions.first?.rate, "12.5 MB/s")
    }

    func testStalePartialAndFailedReadsWithholdRetainedMeasurements() throws {
        for state in ["stale", "partial"] {
            let projection = try decodeProjection("""
            {"state":"\(state)","message":"not current","snapshot":{"nodes":[{"id":"m1","name":"M1","state":"live","gpu":{"work_pct":42.5},"cpu":{"busy_pct":21.25}}]}}
            """)
            let node = try XCTUnwrap(projection.snapshot?.nodes.first)
            XCTAssertNil(projection.gpuWork(for: node, readSucceeded: true))
            XCTAssertNil(projection.cpuBusy(for: node, readSucceeded: true))
        }

        let current = try decodeProjection(#"{"state":"ok","message":"current","snapshot":{"nodes":[{"id":"m1","name":"M1","state":"live","gpu":{"work_pct":42.5},"cpu":{"busy_pct":21.25}}]} }"#)
        let node = try XCTUnwrap(current.snapshot?.nodes.first)
        XCTAssertNil(current.gpuWork(for: node, readSucceeded: false))
        XCTAssertNil(current.cpuBusy(for: node, readSucceeded: false))
    }

    func testUnknownCalibrationAndNonLiveNodeStayUnknown() throws {
        let projection = try decodeProjection(#"{"state":"ok","message":"current","snapshot":{"nodes":[{"id":"m1","name":"M1","state":"live","gpu":{"work_pct":null},"cpu":{"busy_pct":null}},{"id":"m2","name":"M2","state":"stale","gpu":{"work_pct":50},"cpu":{"busy_pct":60}}]}}"#)
        let nodes = try XCTUnwrap(projection.snapshot?.nodes)

        XCTAssertNil(projection.gpuWork(for: nodes[0], readSucceeded: true))
        XCTAssertNil(projection.cpuBusy(for: nodes[0], readSucceeded: true))
        XCTAssertNil(projection.gpuWork(for: nodes[1], readSucceeded: true))
        XCTAssertNil(projection.cpuBusy(for: nodes[1], readSucceeded: true))
    }

    func testCanonicalMonitorRejectsUnsuccessfulCLIProvenance() throws {
        let data = Data(#"{"state":"ok","message":"must not be accepted"}"#.utf8)
        let failed = SirsiEngine.JSONCommandRead(data: data, exitStatus: 1, timedOut: false, launchError: nil)
        let timeout = SirsiEngine.JSONCommandRead(data: data, exitStatus: 0, timedOut: true, launchError: nil)
        let launchFailure = SirsiEngine.JSONCommandRead(data: data, exitStatus: 0, timedOut: false, launchError: "unavailable")

        XCTAssertThrowsError(try decodeMercuryMonitor(read: failed))
        XCTAssertThrowsError(try decodeMercuryMonitor(read: timeout))
        XCTAssertThrowsError(try decodeMercuryMonitor(read: launchFailure))
    }

    func testLegacyNativeMonitorEnvelopeIsNotAcceptedAsCanonicalRead() {
        let data = Data(#"{"now":1792339200,"local":"m1","machines":{"m1":{"state":"live","samples":[]}}}"#.utf8)
        let read = SirsiEngine.JSONCommandRead(data: data, exitStatus: 0, timedOut: false, launchError: nil)

        XCTAssertThrowsError(try decodeMercuryMonitor(read: read))
    }

    private func decodeProjection(_ json: String) throws -> MercuryMonitorProjection {
        let data = Data(json.utf8)
        let read = SirsiEngine.JSONCommandRead(data: data, exitStatus: 0, timedOut: false, launchError: nil)
        return try decodeMercuryMonitor(read: read)
    }

    @MainActor func testExistingCanonicalMonitorWhenExplicitlyRequested() async throws {
        guard ProcessInfo.processInfo.environment["PANTHEON_LIVE_MERCURY"] == "1" else {
            throw XCTSkip("Opt-in canonical read through the existing Mercury CLI reader")
        }
        let monitor = MercuryMonitor()
        await monitor.refresh()
        XCTAssertNil(monitor.error)
        let projection = try XCTUnwrap(monitor.projection)
        XCTAssertTrue(try XCTUnwrap(projection.snapshot?.nodes).contains { $0.state == "live" })
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
