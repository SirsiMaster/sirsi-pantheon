import XCTest
@testable import SirsiMenubar

final class CoreContractsTests: XCTestCase {
    func testStackLabReportTreatsNullCollectionsAsAnEmptyTypedReport() {
        let raw = #"""
        {"roster":["stacklab.wing.maat"],"findings":null}
        """#

        let report = StackLabReport.decode(Data(raw.utf8))

        XCTAssertEqual(report?.roster, ["stacklab.wing.maat"])
        XCTAssertEqual(report?.findings.count, 0)
        XCTAssertEqual(report?.unknown.count, 0)
        XCTAssertEqual(report?.clean, true)
    }

    func testStackLabReadFailureExplainsTheRealRecoveryBoundary() {
        let timedOut = SirsiEngine.JSONCommandRead(data: Data(), exitStatus: nil, timedOut: true, launchError: nil)
        XCTAssertTrue(StackLabReadFailure.message(for: timedOut).contains("20 seconds"))

        let empty = SirsiEngine.JSONCommandRead(data: Data(), exitStatus: 1, timedOut: false, launchError: nil)
        XCTAssertTrue(StackLabReadFailure.message(for: empty).contains("exit 1"))
    }

    func testFleetStatePresentationCoversCanonicalSixStatesAndUnknown() {
        let cases: [(String, String)] = [
            ("WORKING", "WORKING"),
            ("ASSIGNED", "ASSIGNED"),
            ("IDLE_WITH_WORK", "IDLE — WORK WAITING"),
            ("BLOCKED", "BLOCKED"),
            ("UNROUTABLE", "UNROUTABLE"),
            ("COMPLETE", "COMPLETE"),
            ("new_router_state", "UNKNOWN — REVIEW"),
        ]

        for (state, expected) in cases {
            XCTAssertEqual(FleetStatePresentation.label(state), expected, "state \\(state)")
        }
        // Older routers remain readable, but the app never maps an unknown
        // state to the old, incorrect "stopped" label.
        XCTAssertEqual(FleetStatePresentation.label("working"), "WORKING")
        XCTAssertEqual(FleetStatePresentation.label("stopped"), "UNKNOWN — REVIEW")
    }

    func testFleetUsesBoundedDirectRaProducerInsteadOfStartingTheDashboardServer() {
        XCTAssertEqual(SirsiEngine.fleetReadArgs, ["router", "fleet", "--json"])
        XCTAssertEqual(SirsiEngine.fleetReadTimeoutSeconds, 20)
        XCTAssertFalse(SirsiEngine.fleetReadArgs.contains("board-serve"))
    }

    func testKnownFailureProposalDecodesTheTypedLocalReviewQueue() throws {
        let raw = #"""
        [{
          "schema":"sirsi.maat.known-failure-proposal.v1",
          "id":"catalog-root-missing",
          "title":"Catalog root missing",
          "signature":"catalog missing",
          "cause":"an ambient checkout is incomplete",
          "status":"proposed",
          "created_at_utc":"2026-10-05T20:00:00Z",
          "catalog_sha256":"abc123"
        }]
        """#

        let proposals = MaatKnownFailureProposalsView.decode(raw)

        XCTAssertEqual(proposals?.count, 1)
        XCTAssertEqual(proposals?.first?.id, "catalog-root-missing")
        XCTAssertEqual(proposals?.first?.titleOrID, "Catalog root missing")
    }

    func testActivityResolutionAlwaysClosesAnAmbiguousOrFailedOutcome() {
        XCTAssertEqual(activityResolution(for: "applied"), .resolved)
        XCTAssertEqual(activityResolution(for: "The command exited successfully, but returned no readable structured result. No repair is claimed."), .evidenceOnly)
        XCTAssertEqual(activityResolution(for: "registry-police: exit status 1"), .maatReview)
        XCTAssertEqual(activityResolution(for: "Error: evidence unavailable"), .maatReview)
    }

    func testActivityMaatReviewCarriesEvidenceButNeverReplaysTheActivityCommand() {
        let entry = ActivityEntry(
            title: "LaunchAgent restore",
            command: "liveness-watch restore-disabled --confirm",
            when: "2026-10-05T20:00:00Z",
            result: "exit status 1: verification is incomplete"
        )

        XCTAssertEqual(
            activityMaatReviewArgs(for: entry),
            [
                "maat", "record-resolution",
                "--check", "Pantheon activity: LaunchAgent restore",
                "--message", "exit status 1: verification is incomplete",
                "--detail", "Retained Pantheon activity command: sirsi liveness-watch restore-disabled --confirm",
                "--confirm",
            ]
        )
    }

    func testEveryDiagnosticHasAClosedNativeResolutionRoute() {
        XCTAssertEqual(
            diagnosticResolutionRoute(resolution: nil, severity: 0, hasFix: false, hasRecommendedCommand: false),
            .accepted
        )
        XCTAssertEqual(
            diagnosticResolutionRoute(resolution: "information", severity: 1, hasFix: false, hasRecommendedCommand: false),
            .accepted
        )
        XCTAssertEqual(
            diagnosticResolutionRoute(resolution: nil, severity: 2, hasFix: false, hasRecommendedCommand: false),
            .maatReview
        )
        XCTAssertEqual(
            diagnosticResolutionRoute(resolution: "maat_review", severity: 3, hasFix: false, hasRecommendedCommand: false),
            .maatReview
        )
        XCTAssertEqual(
            diagnosticResolutionRoute(resolution: nil, severity: 3, hasFix: true, hasRecommendedCommand: false),
            .repair
        )
        XCTAssertEqual(
            diagnosticResolutionRoute(resolution: nil, severity: 1, hasFix: false, hasRecommendedCommand: true),
            .command
        )
    }

    func testFindingDisplayTitleLeadsWithTheManagedRecoveryInsteadOfTheLaunchdImplementationDetail() {
        XCTAssertEqual(findingDisplayTitle(check: "launchd Disabled Override"), "Restore managed services")
        XCTAssertEqual(findingDisplayTitle(check: "Swap Usage"), "Swap Usage")
    }

    func testMaatSystemOneCommandResultDecodesBothTypedTriageAndScreenShapes() {
        let triage = MaatSystemOneCommandResult.decode(#"""
        {
          "schema_version": 1,
          "feather_weight": 42,
          "gate": "changes",
          "confidence": 0.91,
          "subject": {"kind": "host", "ref": "M5", "head_sha": ""},
          "floor": {"passed": true, "checks": []},
          "model": {"provider": "Ma'at", "version": "1", "local": true, "latency_ms": 3},
          "findings": [],
          "snapshot_evidence": "diagnostic:sha256=abc",
          "decision_evidence": "decision:sha256=def"
        }
        """#)
        XCTAssertEqual(triage?.verdict.gate, "changes")
        XCTAssertEqual(triage?.snapshotEvidence, "diagnostic:sha256=abc")
        XCTAssertEqual(triage?.decisionEvidence, "decision:sha256=def")

        let screen = MaatSystemOneCommandResult.decode(#"""
        {
          "schema_version": 1,
          "feather_weight": 100,
          "gate": "pass",
          "confidence": 0.99,
          "subject": {"kind": "recipe", "ref": "stack-lab", "head_sha": "abc"},
          "floor": {"passed": true, "checks": []},
          "model": {"provider": "Ma'at", "version": "1", "local": true, "latency_ms": 1},
          "findings": []
        }
        """#)
        XCTAssertEqual(screen?.verdict.gate, "pass")
        XCTAssertEqual(screen?.decisionEvidence, "")
    }

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
            {"id":"m1","name":"M1","cpu_cores":8,"memory_bytes":17179869184,"estates":[{"id":"cpu","name":"M1 CPU","available":true,"description":"8 cores"}]},
            {"id":"m2","name":"M2","cpu_cores":12,"memory_bytes":34359738368,"estates":[{"id":"gpu","name":"M2 GPU","available":true,"description":"12 cores"}]}
          ],
          "engines": [
            {"id":"apollo-m1","machine_id":"m1","name":"Apollo MLX","provider":"SNE","resident_model":"Apollo Plain","state":"configured"},
            {"id":"apollo-m2","machine_id":"m2","name":"Apollo Flash","provider":"SNE","resident_model":"Apollo Flash","state":"configured"}
          ],
          "chip_estates": [{"id":"cpu","name":"Local CPU","available":false,"description":"local only"}]
        }
        """#.data(using: .utf8)!

        let catalog = try JSONDecoder().decode(ApolloCatalog.self, from: raw)

        XCTAssertEqual(catalog.residentModelOptions(for: "m1").map(\.id), ["apollo-m1"])
        XCTAssertEqual(catalog.residentModelOptions(for: "m1").map(\.residentModel), ["Apollo Plain"])
        XCTAssertEqual(catalog.residentModelOptions(for: "m2").map(\.id), ["apollo-m2"])
        XCTAssertEqual(catalog.residentModelOptions(for: "m2").map(\.residentModel), ["Apollo Flash"])
        XCTAssertEqual(catalog.route(machineID: "m1", engineID: "apollo-m1")?.residentModel, "Apollo Plain")
        XCTAssertNil(catalog.route(machineID: "m1", engineID: "apollo-m2"))
        XCTAssertEqual(catalog.estateOptions(for: "m1").map(\.name), ["M1 CPU"])
        XCTAssertEqual(catalog.estateOptions(for: "m2").map(\.name), ["M2 GPU"])
    }

    func testApolloPlanPreservesRequestedUnqualifiedEstate() throws {
        let raw = #"""
        {
          "machine_id": "this-mac",
          "engine_id": "apollo-local",
          "resident_model": "Apollo Plain",
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
        XCTAssertEqual(plan.residentModel, "Apollo Plain")
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

    func testApolloTelemetryRequiresMachineMatchForASelectedPeer() throws {
        let plan = try JSONDecoder().decode(ApolloPlan.self, from: #"""
        {"machine_id":"horus-m1","engine_id":"apollo-m1","cpu_cores":4,"memory_bytes":8589934592,"swap_bytes":0,"chip_estates":["cpu"]}
        """#.data(using: .utf8)!)
        let matching = try JSONDecoder().decode(ApolloSessionTelemetry.self, from: #"""
        {"engine_id":"apollo-m1","machine_id":"horus-m1","chip_estates":[]}
        """#.data(using: .utf8)!)
        let ambiguousLegacy = try JSONDecoder().decode(ApolloSessionTelemetry.self, from: #"""
        {"engine_id":"apollo-m1","chip_estates":[]}
        """#.data(using: .utf8)!)

        XCTAssertTrue(matching.matches(plan: plan))
        XCTAssertFalse(ambiguousLegacy.matches(plan: plan))
    }

    func testApolloTelemetryRecoveryAlwaysClosesAwaitingStates() throws {
        let awaiting = try JSONDecoder().decode(ApolloTelemetryRead.self, from: #"""
        {"state":"awaiting_session","reason":"SNE has not published a sample"}
        """#.data(using: .utf8)!)
        let different = try JSONDecoder().decode(ApolloTelemetryRead.self, from: #"""
        {"state":"active","telemetry":{"engine_id":"other-engine","chip_estates":[]}}
        """#.data(using: .utf8)!)

        XCTAssertEqual(
            apolloTelemetryResolution(read: awaiting, matchesSelectedPlan: nil, localRouteHealthy: true, telemetryDecodeFailed: false),
            .awaitingAdmission
        )
        XCTAssertEqual(
            apolloTelemetryResolution(read: awaiting, matchesSelectedPlan: nil, localRouteHealthy: false, telemetryDecodeFailed: false),
            .routeUnavailable
        )
        XCTAssertEqual(
            apolloTelemetryResolution(read: different, matchesSelectedPlan: false, localRouteHealthy: true, telemetryDecodeFailed: false),
            .differentSession
        )
        XCTAssertEqual(
            apolloTelemetryResolution(read: nil, matchesSelectedPlan: nil, localRouteHealthy: false, telemetryDecodeFailed: true),
            .unreadableEvidence
        )
    }

    func testMaatCredentialPreflightDecodesObservedNonDeveloperIdentityTypes() throws {
        let raw = #"""
        {
          "team_id": "9D382WV988",
          "fingerprint": "sha256=fixture",
          "developer_identities": [],
          "observed_non_developer_identity_types": ["Apple Distribution"],
          "notarization_observed": false,
          "verdict": {
            "schema_version": "maat-system-one/v1",
            "feather_weight": 0,
            "gate": "block",
            "confidence": 1,
            "subject": {"kind": "host", "ref": "release-credentials", "head_sha": "fixture"},
            "floor": {"passed": false, "checks": []},
            "model": {"provider": "local:test", "version": "v1", "local": true, "latency_ms": 0},
            "findings": []
          }
        }
        """#.data(using: .utf8)!

        let preflight = try JSONDecoder().decode(MaatReleaseCredentialPreflight.self, from: raw)

        XCTAssertEqual(preflight.observedNonDeveloperIdentityTypes, ["Apple Distribution"])
    }

    func testCredentialPreflightAlwaysProvidesAProtectedRecoveryPlan() throws {
        let raw = #"""
        {
          "team_id": "9D382WV988",
          "fingerprint": "sha256=fixture",
          "developer_identities": [],
          "notarization_observed": false,
          "verdict": {
            "schema_version": "maat-system-one/v1",
            "feather_weight": 0,
            "gate": "block",
            "confidence": 1,
            "subject": {"kind": "host", "ref": "release-credentials", "head_sha": "fixture"},
            "floor": {"passed": false, "checks": []},
            "model": {"provider": "local:test", "version": "v1", "local": true, "latency_ms": 0},
            "findings": []
          }
        }
        """#.data(using: .utf8)!

        let preflight = try JSONDecoder().decode(MaatReleaseCredentialPreflight.self, from: raw)
        let steps = protectedReleaseRecoverySteps(for: preflight)

        XCTAssertTrue(steps.contains { $0.contains("Developer ID Application") && $0.contains("9D382WV988") })
        XCTAssertTrue(steps.contains { $0.contains("Developer ID Installer") && $0.contains("9D382WV988") })
        XCTAssertTrue(steps.contains { $0.contains("notarization credential") })
        XCTAssertTrue(steps.last?.contains("Recheck readiness") == true)
    }

    func testCredentialPreflightPrefersCanonicalRecoveryPlan() throws {
        let raw = #"""
        {
          "team_id": "9D382WV988",
          "fingerprint": "sha256=fixture",
          "developer_identities": [],
          "notarization_observed": false,
          "recovery_plan": ["Use the protected workflow.", "Recheck readiness."],
          "verdict": {
            "schema_version": "maat-system-one/v1",
            "feather_weight": 0,
            "gate": "block",
            "confidence": 1,
            "subject": {"kind": "host", "ref": "release-credentials", "head_sha": "fixture"},
            "floor": {"passed": false, "checks": []},
            "model": {"provider": "local:test", "version": "v1", "local": true, "latency_ms": 0},
            "findings": []
          }
        }
        """#.data(using: .utf8)!

        let preflight = try JSONDecoder().decode(MaatReleaseCredentialPreflight.self, from: raw)

        XCTAssertEqual(protectedReleaseRecoverySteps(for: preflight), ["Use the protected workflow.", "Recheck readiness."])
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

    func testMaatCasebookExecutesOnlyClosedNativeRepairReferences() {
        XCTAssertEqual(
            maatCasebookRepairArguments(actionID: "launchd-disabled"),
            ["maat", "repair", "launchd-disabled", "--confirm"]
        )
        XCTAssertEqual(
            maatCasebookRepairArguments(actionID: "liveness-watch"),
            ["maat", "repair", "liveness-watch", "--confirm"]
        )
        XCTAssertNil(maatCasebookRepairArguments(actionID: "launchctl bootstrap gui/501/untrusted"))
    }

    func testMaatCasebookIntegrityKeepsValidCasesVisibleAndRoutesRepair() throws {
        let raw = #"""
        {
          "cases": [],
          "summary": {"total":0,"open":0,"urgent":0,"high":0,"resolved":0},
          "journal_integrity": {
            "invalid_count": 1,
            "issues": [
              {"line": 14, "digest": "sha256=fixture-a", "reason": "requester is required"},
              {"line": 15, "digest": "sha256=fixture-b", "reason": "host is required"}
            ]
          }
        }
        """#.data(using: .utf8)!

        let projection = try JSONDecoder().decode(MaatCasebookProjection.self, from: raw)

        XCTAssertEqual(projection.cases.count, 0)
        XCTAssertEqual(projection.journalIntegrity.invalidCount, 1)
        XCTAssertEqual(projection.journalIntegrity.issues.first?.line, 14)
        XCTAssertEqual(projection.journalIntegrity.issues.first?.reason, "requester is required")
        XCTAssertEqual(Set(projection.journalIntegrity.issues.map(\.id)).count, 2)
    }

    func testDesktopEnginePrefersTheBundledCLIOverAHostInstall() throws {
        let root = URL(fileURLWithPath: NSTemporaryDirectory())
            .appendingPathComponent("sirsi-bundled-cli-\(UUID().uuidString)", isDirectory: true)
        let macOS = root.appendingPathComponent("Pantheon.app/Contents/MacOS", isDirectory: true)
        try FileManager.default.createDirectory(at: macOS, withIntermediateDirectories: true)
        defer { try? FileManager.default.removeItem(at: root) }
        let menubar = macOS.appendingPathComponent("sirsi-menubar")
        let cli = macOS.appendingPathComponent("sirsi")
        try Data().write(to: menubar)
        try Data().write(to: cli)
        try FileManager.default.setAttributes([.posixPermissions: 0o755], ofItemAtPath: cli.path)

        let selected = SirsiEngine.sirsiBinary(bundleExecutableURL: menubar, homeDirectory: "/does-not-exist")

        XCTAssertEqual(selected, cli.path)

        try FileManager.default.removeItem(at: cli)
        try FileManager.default.createSymbolicLink(atPath: cli.path, withDestinationPath: "/bin/true")
        XCTAssertNil(SirsiEngine.sirsiBinary(bundleExecutableURL: menubar, homeDirectory: "/does-not-exist"))

        try FileManager.default.removeItem(at: cli)
        XCTAssertNil(SirsiEngine.sirsiBinary(bundleExecutableURL: menubar, homeDirectory: "/does-not-exist"))
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

    @MainActor
    func testMenubarCleanupLabelNamesTheActionInsteadOfShowingAnAmbiguousByteCount() throws {
        let finding = try JSONDecoder().decode(Finding.self, from: #"""
        {"path":"/tmp/reclaimable","size_bytes":43379169689,"severity":"safe","description":"reclaimable data"}
        """#.data(using: .utf8)!)
        let engine = SirsiEngine()
        engine.findings = [finding]

        XCTAssertEqual(engine.titleLabel(), "Clean 40.4 GB")
        XCTAssertFalse(engine.titleLabel().hasPrefix("40"))
    }

    func testProjectRootAdmissionAcceptsGitWorktreesAndRejectsPlainFolders() throws {
        let root = URL(fileURLWithPath: NSTemporaryDirectory())
            .appendingPathComponent("sirsi-project-root-\(UUID().uuidString)", isDirectory: true)
        defer { try? FileManager.default.removeItem(at: root) }
        try FileManager.default.createDirectory(at: root, withIntermediateDirectories: true)

        XCTAssertNil(SirsiEngine.projectRootPath(root.path))

        // A linked worktree represents .git as a file rather than a directory.
        try "gitdir: /tmp/fixture".write(to: root.appendingPathComponent(".git"), atomically: true, encoding: .utf8)
        XCTAssertEqual(SirsiEngine.projectRootPath(root.path), root.path)
    }
}
