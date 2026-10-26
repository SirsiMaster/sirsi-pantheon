import XCTest
@testable import SirsiMenubar

@MainActor final class StackLabLocalTests: XCTestCase {
    private func reply(_ text: String, exit: Int32 = 0) -> SirsiEngine.JSONCommandRead {
        .init(data: Data(text.utf8), exitStatus: exit, timedOut: false, launchError: nil)
    }
    func testLocalFirstInspectEnableAllProviderKindsAndDisableUsesClosedArguments() async {
        var calls: [[String]] = []
        let model = StackLabLocalModel { args in
            calls.append(args)
            let enabled = args.contains("enable")
            let provider = args.firstIndex(of: "--provider").map { args[$0 + 1] } ?? "local"
            return self.reply("{\"schema\":\"sirsi.stacklab.local.v1\",\"enabled\":\(enabled),\"provider\":\"\(provider)\"}")
        }
        await model.inspect()
        XCTAssertEqual(model.state?.enabled, false)
        for provider in ["local", "frontier", "self-hosted"] {
            await model.setEnabled(true, provider: provider)
            XCTAssertEqual(model.state?.provider, provider)
            XCTAssertEqual(calls.last, ["stacklab", "local", "enable", "--provider", provider, "--json"])
        }
        await model.setEnabled(false, provider: "self-hosted")
        XCTAssertEqual(calls.last, ["stacklab", "local", "disable", "--json"])
        XCTAssertEqual(model.state?.enabled, false)
    }
    func testUnknownStateAndInvalidProviderNeverBecomeEnablementOrDoctor() async {
        var calls: [[String]] = []
        let model = StackLabLocalModel { args in calls.append(args); return self.reply("{}", exit: 1) }
        await model.inspect()
        XCTAssertNil(model.state)
        XCTAssertNotNil(model.error)
        await model.setEnabled(true, provider: "invalid")
        await model.inspectRecipes()
        XCTAssertEqual(calls, [["stacklab", "local", "inspect", "--json"]])
        XCTAssertFalse(model.busy)
    }
    func testEnabledCatalogueReadsOnlyLocalCommandAndClearsOnDisable() async {
        var calls: [[String]] = []
        let model = StackLabLocalModel { args in
            calls.append(args)
            if args.contains("catalog") {
                return self.reply(#"{"entries":[{"id":"stacklab.recipe.fixture","kind":"recipe","purpose":"owned fixture"}]}"#)
            }
            return self.reply("{\"schema\":\"sirsi.stacklab.local.v1\",\"enabled\":\(!args.contains("disable")),\"provider\":\"self-hosted\"}")
        }
        await model.inspect()
        await model.inspectRecipes()
        XCTAssertEqual(model.catalog?.entries.first?.id, "stacklab.recipe.fixture")
        XCTAssertEqual(calls[1], ["stacklab", "local", "catalog", "--json"])
        await model.setEnabled(false, provider: "self-hosted")
        XCTAssertNil(model.catalog)
        XCTAssertEqual(model.state?.enabled, false)
        XCTAssertFalse(calls.contains { $0.contains("doctor") })
    }
}
