import XCTest
@testable import SirsiMenubar

final class CoreContractsTests: XCTestCase {
    func testDiagnosticFindingDecodesExplicitMaatReviewRoute() throws {
        let raw = #"""
        {
          "check": "Kernel Panics (7d)",
          "severity": 3,
          "message": "Two recent kernel panics need review",
          "resolution": "maat_review"
        }
        """#.data(using: .utf8)!

        let finding = try JSONDecoder().decode(DiagFinding.self, from: raw)

        XCTAssertEqual(finding.check, "Kernel Panics (7d)")
        XCTAssertEqual(finding.resolution, "maat_review")
        XCTAssertNil(finding.fix)
    }

    func testApolloSnapshotCatalogMakesAllDetectedEstatesVisible() {
        let catalog = ApolloCatalog.snapshotPreview

        XCTAssertEqual(catalog.machine.id, "this-mac")
        XCTAssertGreaterThan(catalog.machine.cpuCores, 0)
        XCTAssertGreaterThan(catalog.machine.memoryBytes, 0)
        XCTAssertEqual(Set(catalog.machine.chipEstates ?? []), Set(catalog.estates.map(\.id)))
        XCTAssertTrue(catalog.estates.contains { $0.id == "neural-engine" && !$0.available })
    }

    func testApolloCatalogKeepsResidentModelChoicesBoundToTheirEngineAndMachine() throws {
        let raw = #"""
        {
          "machine": {"id":"m1","name":"M1","cpu_cores":8,"memory_bytes":17179869184},
          "machines": [
            {"id":"m1","name":"M1","cpu_cores":8,"memory_bytes":17179869184},
            {"id":"m2","name":"M2","cpu_cores":12,"memory_bytes":34359738368}
          ],
          "engines": [
            {"id":"apollo-m1","machine_id":"m1","name":"Apollo MLX","provider":"SNE","resident_model":"Apollo Plain","state":"configured"},
            {"id":"apollo-m2","machine_id":"m2","name":"Apollo Flash","provider":"SNE","resident_model":"Apollo Flash","state":"configured"}
          ],
          "chip_estates": []
        }
        """#.data(using: .utf8)!

        let catalog = try JSONDecoder().decode(ApolloCatalog.self, from: raw)

        XCTAssertEqual(catalog.residentModelOptions(for: "m1").map(\.id), ["apollo-m1"])
        XCTAssertEqual(catalog.residentModelOptions(for: "m1").map(\.residentModel), ["Apollo Plain"])
        XCTAssertEqual(catalog.residentModelOptions(for: "m2").map(\.id), ["apollo-m2"])
        XCTAssertEqual(catalog.residentModelOptions(for: "m2").map(\.residentModel), ["Apollo Flash"])
    }

    func testApolloPlanPreservesRequestedUnqualifiedEstate() throws {
        let raw = #"""
        {
          "machine_id": "this-mac",
          "engine_id": "apollo-local",
          "cpu_cores": 4,
          "memory_bytes": 8589934592,
          "swap_bytes": 0,
          "chip_estates": ["cpu", "neural-engine"],
          "unavailable_chip_estates": ["neural-engine"]
        }
        """#.data(using: .utf8)!

        let plan = try JSONDecoder().decode(ApolloPlan.self, from: raw)

        XCTAssertEqual(plan.chipEstates, ["cpu", "neural-engine"])
        XCTAssertEqual(plan.unqualifiedEstates, ["neural-engine"])
    }

    func testApolloTelemetryDecodesSelectedAndAdditionalReportedEstates() throws {
        let raw = #"""
        {
          "state": "active",
          "telemetry": {
            "engine_id": "apollo-local",
            "tokens_per_second": 31.5,
            "bandwidth_bytes_per_second": 1048576,
            "memory_bytes": 8589934592,
            "network_saturation_percent": 12.5,
            "cpu_residency_percent": 43.0,
            "gpu_residency_percent": 66.0,
            "chip_estates": [
              {"id": "gpu", "residency_percent": 66.0, "utilization_percent": 51.0, "memory_bytes": 4294967296},
              {"id": "neural-engine", "residency_percent": 8.0, "utilization_percent": 5.0, "memory_bytes": 0}
            ]
          }
        }
        """#.data(using: .utf8)!

        let read = try JSONDecoder().decode(ApolloTelemetryRead.self, from: raw)

        XCTAssertEqual(read.state, "active")
        XCTAssertEqual(read.telemetry?.tokensPerSec, 31.5)
        XCTAssertEqual(read.telemetry?.estates.map(\.id), ["gpu", "neural-engine"])
    }

    func testMaatClosedRepairActionDecodesWithoutAcceptingACommand() throws {
        let raw = #"""
        {
          "kind": "maat_repair",
          "action_id": "launchd-disabled",
          "title": "Restore the managed launchd labels",
          "detail": "bounded repair",
          "evidence": "maat-system-one:sha256=fixture",
          "requires_confirmation": true
        }
        """#.data(using: .utf8)!

        let action = try JSONDecoder().decode(MaatCaseNextAction.self, from: raw)

        XCTAssertEqual(action.kind, "maat_repair")
        XCTAssertEqual(action.actionID, "launchd-disabled")
        XCTAssertFalse(action.detail.contains("launchctl"))
    }

    func testMenubarLeaseAllowsOneLocalProcessAtATime() throws {
        let root = URL(fileURLWithPath: NSTemporaryDirectory())
            .appendingPathComponent("sirsi-menubar-lease-\(UUID().uuidString)", isDirectory: true)
        try FileManager.default.createDirectory(at: root, withIntermediateDirectories: true)
        defer { try? FileManager.default.removeItem(at: root) }
        let path = root.appendingPathComponent("menubar.instance.lock").path

        var first: MenubarInstanceLease? = MenubarInstanceLease.acquire(at: path)
        XCTAssertNotNil(first)
        XCTAssertNil(MenubarInstanceLease.acquire(at: path))
        first = nil
        XCTAssertNotNil(MenubarInstanceLease.acquire(at: path))
    }
}
