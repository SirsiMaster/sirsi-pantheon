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
}
