import SwiftUI
import XCTest
@testable import SirsiMenubar

@MainActor
final class ApolloPlanContinuityTests: XCTestCase {
    private func window() -> PantheonWorkspaceSession {
        let name = "ApolloPlanContinuityTests." + UUID().uuidString
        let defaults = UserDefaults(suiteName: name)!
        addTeardownBlock { defaults.removePersistentDomain(forName: name) }
        return PantheonWorkspaceSession(defaults: defaults)
    }

    private func catalog(cores: Int = 18, memoryGiB: Int = 48, model: String = "resident",
                         machineID: String = "fixture-peer", route: Bool = true,
                         configured: Bool = true, neural: Bool = true) -> ApolloCatalog {
        var estates = [
            ApolloChipEstate(id: "cpu", name: "CPU", available: true, description: "fixture"),
            ApolloChipEstate(id: "gpu", name: "GPU", available: true, description: "fixture")
        ]
        if neural { estates.append(ApolloChipEstate(id: "neural-engine", name: "Neural", available: false, description: "fixture")) }
        let machine = ApolloMachine(id: machineID, name: "Fixture", cpuCores: cores,
                                    memoryBytes: Int64(memoryGiB) * 1_073_741_824,
                                    chipEstates: estates.map(\.id), estates: estates)
        return ApolloCatalog(machine: machine, machines: nil,
                             engines: route ? [ApolloEngineOption(id: "fixture-route", machineID: machine.id,
                                name: "Route", provider: "SNE", residentModel: model,
                                endpoint: nil, state: configured ? "configured" : "unconfigured")] : [], estates: estates)
    }

    private func edit(_ draft: ApolloPlannerDraft) {
        draft.selectedCores = 10; draft.selectedMemoryGiB = 25; draft.selectedSwapGiB = 1
        draft.selectedEstates = Set(["cpu", "gpu", "neural-engine"])
    }

    private func acceptCatalog(_ planner: ApolloRunPlannerView, _ value: ApolloCatalog) {
        let token = planner.beginCapacityRefresh()
        planner.receiveCatalog(value, generation: token)
    }

    private func plan(_ draft: ApolloPlannerDraft) -> ApolloPlan {
        ApolloPlan(machineID: draft.selectedMachine, engineID: draft.selectedEngine,
                   residentModel: "resident", cpuCores: draft.selectedCores,
                   memoryBytes: Int64(draft.selectedMemoryGiB) * 1_073_741_824,
                   swapBytes: Int64(draft.selectedSwapGiB) * 1_073_741_824,
                   chipEstates: draft.selectedEstates.sorted(), unavailableEstates: ["neural-engine"])
    }

    private func planData(_ draft: ApolloPlannerDraft) throws -> Data {
        try JSONSerialization.data(withJSONObject: ["machine_id": draft.selectedMachine,
            "engine_id": draft.selectedEngine, "resident_model": "resident", "cpu_cores": draft.selectedCores,
            "memory_bytes": Int64(draft.selectedMemoryGiB) * 1_073_741_824,
            "swap_bytes": Int64(draft.selectedSwapGiB) * 1_073_741_824,
            "chip_estates": draft.selectedEstates.sorted()])
    }

    private func assertSelection(_ draft: ApolloPlannerDraft, file: StaticString = #filePath, line: UInt = #line) {
        XCTAssertEqual(draft.selectedMachine, "fixture-peer", file: file, line: line)
        XCTAssertEqual(draft.selectedEngine, "fixture-route", file: file, line: line)
        XCTAssertEqual(draft.selectedCores, 10, file: file, line: line)
        XCTAssertEqual(draft.selectedMemoryGiB, 25, file: file, line: line)
        XCTAssertEqual(draft.selectedSwapGiB, 1, file: file, line: line)
        XCTAssertEqual(draft.selectedEstates, Set(["cpu", "gpu", "neural-engine"]), file: file, line: line)
    }

    func testDesktopOwnerReconstructsUnbuiltAndTelemetryReturnsWithoutDefaults() throws {
        let engine = SirsiEngine(), owner = window()
        let nav = owner.navigation(for: .apollo)
        var root: ApolloRunPlannerView? = owner.apolloPlanner(engine: engine)
        acceptCatalog(root!, catalog())
        XCTAssertEqual(root?.draft.selectedCores, 9)
        XCTAssertEqual(root?.draft.selectedMemoryGiB, 24)
        edit(root!.draft)
        let draft = root!.draft
        nav.push(Text("Another screen"))
        root!.leavePlanner(); root = nil // destroy the removed root, not just Nav.pop
        nav.pop()
        root = owner.apolloPlanner(engine: engine) // exact production desktop-root factory
        XCTAssertTrue(root!.draft === draft)
        acceptCatalog(root!, catalog())
        assertSelection(root!.draft) // edits survive before any recipe is built
        let telemetry = root!.telemetryDestination(for: plan(draft))
        root!.leavePlanner(); nav.push(telemetry); root = nil
        nav.pop()
        let returned = owner.apolloPlanner(engine: engine)
        acceptCatalog(returned, catalog(cores: 20, memoryGiB: 64))
        assertSelection(returned.draft)
        XCTAssertNil(returned.draft.revisionNotice)
        XCTAssertFalse(returned.draft.planning, "Back must not submit a plan")
        XCTAssertNil(returned.draft.plan)
        let otherWindow = window().apolloPlanner(engine: engine)
        XCTAssertFalse(otherWindow.draft === draft)
        acceptCatalog(otherWindow, catalog())
        XCTAssertEqual(otherWindow.draft.selectedCores, 9)
        let newJourney = ApolloRunPlannerView(engine: engine, preloadedCatalog: catalog())
        XCTAssertFalse(newJourney.draft === draft)
        XCTAssertEqual(newJourney.draft.selectedSwapGiB, 0)
        engine.projectRoot = "/fixture/project-b" // UI identity only, no disk or command
        let changed = owner.apolloPlanner(engine: engine)
        acceptCatalog(changed, catalog())
        XCTAssertEqual(changed.draft.selectedCores, 9)
        XCTAssertEqual(changed.draft.selectedMemoryGiB, 24)
        XCTAssertEqual(changed.draft.selectedSwapGiB, 0)
    }

    func testPushedJourneyAndCancelledRevisionHaveIndependentOwners() throws {
        let engine = SirsiEngine(), nav = Nav()
        var pushed: ApolloRunPlannerView? = ApolloRunPlannerView(engine: engine, preloadedCatalog: catalog())
        edit(pushed!.draft)
        weak var retained = pushed!.draft
        nav.push(pushed!)
        let frame = try XCTUnwrap(nav.stack.last?.id)
        var telemetry: ApolloTelemetryView? = pushed!.telemetryDestination(for: plan(pushed!.draft))
        pushed!.leavePlanner(); nav.push(telemetry!); pushed = nil
        let review = telemetry!.reviewDestination()
        XCTAssertFalse(review.draft === retained, "revision must not alias the origin")
        acceptCatalog(review, catalog())
        assertSelection(review.draft)
        nav.push(review)
        review.draft.selectedCores = 12; review.draft.selectedSwapGiB = 4
        review.leavePlanner(); nav.pop() // cancel revision
        nav.pop(); telemetry = nil
        XCTAssertEqual(nav.stack.last?.id, frame)
        let returned = ApolloRunPlannerView(engine: engine, draft: try XCTUnwrap(retained))
        acceptCatalog(returned, catalog())
        assertSelection(returned.draft)
        returned.draft.plan = plan(returned.draft)
        XCTAssertNotNil(returned.draft.retainedPlan)
        returned.draft.selectedSwapGiB = 2
        XCTAssertNil(returned.draft.retainedPlan, "edited draft must not show an old plan ready")
    }

    func testFreshCatalogRefusesChangedBasisWithoutClampingAndFailureStaysBlocked() {
        let variants = [catalog(cores: 8, memoryGiB: 16), catalog(model: "changed"),
                        catalog(machineID: "removed-peer"), catalog(route: false),
                        catalog(configured: false), catalog(neural: false)]
        for current in variants {
            let planner = ApolloRunPlannerView(engine: SirsiEngine(), preloadedCatalog: catalog())
            edit(planner.draft)
            acceptCatalog(planner, current)
            assertSelection(planner.draft)
            XCTAssertNotNil(planner.planBlocker(current))
        }
        let planner = ApolloRunPlannerView(engine: SirsiEngine(), preloadedCatalog: catalog())
        edit(planner.draft)
        let cancelled = planner.beginCapacityRefresh()
        planner.leavePlanner()
        let current = planner.beginCapacityRefresh()
        planner.receiveCatalog(catalog(cores: 8), generation: cancelled)
        XCTAssertTrue(planner.draft.loading, "late cancelled read must not finish a newer read")
        XCTAssertEqual(planner.draft.catalog?.machine.cpuCores, 18)
        planner.receiveCapacityData(Data(), generation: current)
        assertSelection(planner.draft)
        XCTAssertNotNil(planner.draft.error)
        XCTAssertNotNil(planner.planBlocker(catalog()), "failed refresh cannot authorize a stale build")
        acceptCatalog(planner, catalog(cores: 8, memoryGiB: 16))
        assertSelection(planner.draft)
        planner.acceptCurrentCapacity()
        XCTAssertNil(planner.draft.revisionNotice)
        XCTAssertEqual(planner.draft.selectedCores, 4)
        XCTAssertEqual(planner.draft.selectedMemoryGiB, 8)
        XCTAssertEqual(planner.draft.selectedSwapGiB, 0)
    }

    func testLateCancelledPlanCannotNavigateOrClearANewerAttempt() async throws {
        let engine = SirsiEngine(), nav = Nav()
        let planner = ApolloRunPlannerView(engine: engine, preloadedCatalog: catalog())
        edit(planner.draft); nav.push(planner)
        let bytes = try planData(planner.draft)
        let first = DeferredPlanRead(), second = DeferredPlanRead()
        var opened = 0
        let oldTask = Task { await planner.createPlan(catalog(), read: first.read) { view in
            opened += 1; nav.push(view)
        } }
        planner.draft.planningTask = oldTask
        await first.waitUntilEntered()
        XCTAssertEqual(first.args, ["apollo", "plan", "--machine", "fixture-peer", "--engine", "fixture-route", "--cores", "10", "--memory-gib", "25", "--swap-gib", "1", "--estates", "cpu,gpu,neural-engine", "--json"])
        planner.leavePlanner()
        let nextTask = Task { await planner.createPlan(catalog(), read: second.read) { view in
            opened += 1; nav.push(view)
        } }
        planner.draft.planningTask = nextTask
        await second.waitUntilEntered()
        first.deliver(bytes); await oldTask.value
        XCTAssertTrue(planner.draft.planning)
        XCTAssertEqual(opened, 0); XCTAssertEqual(nav.stack.count, 1)
        assertSelection(planner.draft)
        second.deliver(bytes); await nextTask.value
        XCTAssertFalse(planner.draft.planning)
        XCTAssertEqual(opened, 1); XCTAssertEqual(nav.stack.count, 2)
        XCTAssertEqual(planner.draft.retainedPlan?.cpuCores, 10)
    }
}

@MainActor
private final class DeferredPlanRead {
    private var response: CheckedContinuation<Data, Never>?
    private var entered: CheckedContinuation<Void, Never>?
    private(set) var args: [String] = []

    func read(_ args: [String]) async -> Data {
        self.args = args
        return await withCheckedContinuation { continuation in
            response = continuation
            entered?.resume(); entered = nil
        }
    }

    func waitUntilEntered() async {
        if response != nil { return }
        await withCheckedContinuation { entered = $0 }
    }

    func deliver(_ data: Data) { response?.resume(returning: data); response = nil }
}
