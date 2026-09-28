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
}
