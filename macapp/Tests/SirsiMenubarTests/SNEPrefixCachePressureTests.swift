import XCTest
@testable import SirsiMenubar

final class SNEPrefixCachePressureTests: XCTestCase {
    @MainActor
    final class FixtureTransport: SNEControlTransport {
        private var responses: [Data]
        private(set) var requests: [URLRequest] = []

        init(responses: [Data]) {
            self.responses = responses
        }

        func data(for request: URLRequest) async throws -> (Data, URLResponse) {
            requests.append(request)
            guard !responses.isEmpty else { throw URLError(.badServerResponse) }
            let response = HTTPURLResponse(url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!
            return (responses.removeFirst(), response)
        }
    }

    @MainActor
    func testFixtureOnlyMeasureThenAuthorizeUsesExactBoundOwnerAction() async throws {
        let requestID = "pressure-0123456789abcdef"
        let observationSHA = String(repeating: "a", count: 64)
        let prepared = Data("""
        {"state":"owner-confirmation-required","receipt":{"schema":"pantheon.sne-prefix-cache-pressure-receipt.v1","observation":{"request_id":"\(requestID)","host_id":"fixture-host","observed_at_unix":1724846400,"expires_at_unix":1724846700,"total_ram_bytes":51539607552,"available_ram_bytes":25769803776,"swap_used_bytes":0,"swap_limit_bytes":8589934592,"pressure":"normal","pressure_source":"fixture","swap_measured":true},"observation_sha256":"\(observationSHA)"},"confirmation":{"confirm_token":"fixture-confirm-token","action_hash":"\(String(repeating: "b", count: 64))","expires_at":"2026-08-28T12:02:00Z","preview":"Fixture owner confirmation."}}
        """.utf8)
        let accepted = Data("""
        {"state":"authorization-accepted","receipt":{"schema":"pantheon.sne-prefix-cache-pressure-receipt.v1","observation":{"request_id":"\(requestID)","host_id":"fixture-host","observed_at_unix":1724846400,"expires_at_unix":1724846700,"total_ram_bytes":51539607552,"available_ram_bytes":25769803776,"swap_used_bytes":0,"swap_limit_bytes":8589934592,"pressure":"normal","pressure_source":"fixture","swap_measured":true},"observation_sha256":"\(observationSHA)"},"authorization":{"state":"accepted","request_id":"\(requestID)","artifact_sha256":"\(observationSHA)","expires_at_unix":1724846700}}
        """.utf8)
        let transport = FixtureTransport(responses: [prepared, accepted])
        let model = SNEControlModel(
            baseURL: URL(string: "http://fixture.invalid:9119")!,
            transport: transport,
            capabilityProvider: { "fixture-owner-capability-token-which-is-not-used-outside-this-test" }
        )

        await model.preparePrefixCachePressure()
        let confirmation = try XCTUnwrap(model.prefixCachePressure?.confirmation)
        XCTAssertEqual(model.prefixCachePressure?.state, "owner-confirmation-required")
        XCTAssertEqual(transport.requests.count, 1)
        XCTAssertEqual(transport.requests[0].httpMethod, "GET")
        XCTAssertEqual(transport.requests[0].url?.path, "/api/sne/prefix-cache-pressure")
        XCTAssertEqual(transport.requests[0].value(forHTTPHeaderField: "Authorization"), "Bearer fixture-owner-capability-token-which-is-not-used-outside-this-test")

        await model.authorizePrefixCachePressure(try XCTUnwrap(model.prefixCachePressure))
        XCTAssertEqual(model.prefixCachePressure?.authorization?.state, "accepted")
        XCTAssertEqual(transport.requests.count, 2)
        XCTAssertEqual(transport.requests[1].httpMethod, "POST")
        let payload = try XCTUnwrap(try JSONSerialization.jsonObject(with: try XCTUnwrap(transport.requests[1].httpBody)) as? [String: String])
        XCTAssertEqual(payload["request_id"], requestID)
        XCTAssertEqual(payload["observation_sha256"], observationSHA)
        XCTAssertEqual(payload["confirm_token"], confirmation.confirmToken)
        XCTAssertEqual(payload["action_hash"], confirmation.actionHash)
    }

    @MainActor
    func testEvidenceIdentityRejectsPathTraversal() {
        XCTAssertTrue(SNEControlModel.validEvidenceIdentity("pressure-req_20260828"))
        XCTAssertFalse(SNEControlModel.validEvidenceIdentity("../pressure"))
        XCTAssertFalse(SNEControlModel.validEvidenceIdentity("pressure/receipt"))
        XCTAssertFalse(SNEControlModel.validEvidenceIdentity(""))
    }

    func testDecodesOwnerConfirmationWithoutInventingAnSNEAction() throws {
        let data = Data("""
        {
          "state":"owner-confirmation-required",
          "receipt":{
            "schema":"pantheon.sne-prefix-cache-pressure-receipt.v1",
            "observation":{
              "request_id":"pressure-0123456789abcdef",
              "host_id":"m5",
              "observed_at_unix":1724846400,
              "expires_at_unix":1724846700,
              "total_ram_bytes":51539607552,
              "available_ram_bytes":25769803776,
              "swap_used_bytes":0,
              "swap_limit_bytes":8589934592,
              "pressure":"normal",
              "pressure_source":"vm_stat",
              "swap_measured":true
            },
            "observation_sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
          },
          "confirmation":{
            "confirm_token":"opaque-single-use-token",
            "action_hash":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
            "expires_at":"2026-08-28T12:02:00Z",
            "preview":"Review measured prefix-cache pressure."
          }
        }
        """.utf8)
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .iso8601
        let view = try decoder.decode(SNEPrefixCachePressureViewState.self, from: data)

        XCTAssertEqual(view.state, "owner-confirmation-required")
        XCTAssertEqual(view.receipt.observation.hostID, "m5")
        XCTAssertEqual(view.receipt.observation.requestID, "pressure-0123456789abcdef")
        XCTAssertEqual(view.receipt.observationSHA256.count, 64)
        XCTAssertNotNil(view.confirmation)
        XCTAssertNil(view.authorization)
    }

    func testDecodesUnavailableEvidenceAsNoReceipt() throws {
        let data = Data("""
        {"state":"unavailable","evidence_type":"execution","identity":"pressure-0123456789abcdef"}
        """.utf8)
        let evidence = try JSONDecoder().decode(SNEPrefixCachePressureEvidenceState.self, from: data)

        XCTAssertEqual(evidence.state, "unavailable")
        XCTAssertEqual(evidence.evidenceType, "execution")
        XCTAssertNil(evidence.receipt)
    }

    func testDecodesAcceptedAuthorizationBoundToTheObservedRequest() throws {
        let data = Data("""
        {
          "state":"authorization-accepted",
          "receipt":{
            "schema":"pantheon.sne-prefix-cache-pressure-receipt.v1",
            "observation":{"request_id":"pressure-0123456789abcdef","host_id":"m5","observed_at_unix":1724846400,"expires_at_unix":1724846700,"total_ram_bytes":51539607552,"available_ram_bytes":25769803776,"swap_used_bytes":0,"swap_limit_bytes":8589934592,"pressure":"normal","pressure_source":"vm_stat","swap_measured":true},
            "observation_sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
          },
          "authorization":{"state":"accepted","request_id":"pressure-0123456789abcdef","artifact_sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","expires_at_unix":1724846700}
        }
        """.utf8)
        let view = try JSONDecoder().decode(SNEPrefixCachePressureViewState.self, from: data)

        XCTAssertEqual(view.authorization?.state, "accepted")
        XCTAssertEqual(view.authorization?.requestID, view.receipt.observation.requestID)
        XCTAssertEqual(view.authorization?.artifactSHA256, view.receipt.observationSHA256)
    }

    func testDecodesExecutionAndRetentionReceiptsWithoutTreatingThemAsLiveMetrics() throws {
        let execution = try JSONDecoder().decode(SNEPrefixCachePressureEvidenceState.self, from: Data("""
        {"state":"available","evidence_type":"execution","identity":"pressure-0123456789abcdef","receipt":{"status":"failed","error_code":"cache_pressure_execution_interrupted"}}
        """.utf8))
        let retention = try JSONDecoder().decode(SNEPrefixCachePressureEvidenceState.self, from: Data("""
        {"state":"available","evidence_type":"retention","identity":"cleanup-0123456789abcdef","receipt":{"removed_request_ids":["pressure-old"],"retained_request_ids":["pressure-new"]}}
        """.utf8))

        XCTAssertEqual(execution.receipt?.status, "failed")
        XCTAssertEqual(execution.receipt?.errorCode, "cache_pressure_execution_interrupted")
        XCTAssertEqual(retention.receipt?.removedRequestIDs, ["pressure-old"])
        XCTAssertEqual(retention.receipt?.retainedRequestIDs, ["pressure-new"])
    }
}
