import XCTest
@testable import SirsiMenubar

final class MaatDiagnosticPriorityTests:XCTestCase{
    func testNativeDetailShowsBlockersFirstWithoutChangingRetainedWireOrder()throws{
        let raw=Data(#"[{"id":"a-major","severity":"major","claim":"warning"},{"id":"z-block","severity":"block","claim":"critical"},{"id":"b-block","severity":"block","claim":"critical tie"}]"#.utf8)
        let retained=try JSONDecoder().decode([MaatSystemOneFinding].self,from:raw)
        XCTAssertEqual(retained.map(\.id),["a-major","z-block","b-block"])
        let display=maatSystemOneDisplayFindings(retained)
        XCTAssertEqual(display.map(\.id),["b-block","z-block","a-major"])
        XCTAssertEqual(retained.map(\.id),["a-major","z-block","b-block"])
        XCTAssertEqual(display.first?.claim,"critical tie")
    }
}
