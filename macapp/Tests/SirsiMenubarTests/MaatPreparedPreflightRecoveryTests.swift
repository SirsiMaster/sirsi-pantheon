import XCTest
@testable import SirsiMenubar

final class MaatPreparedPreflightRecoveryTests:XCTestCase {
    private let uuid="b3c515a2-4773-4653-86d1-8a5734c2ee45"
    private let sha=String(repeating:"a",count:64)
    private func fixture()throws->(Data,Data,MaatPreparedPreflight){
        let scope=["component":"pantheon","profile":"local-macos","operation":"install-managed-liveness-watch"]
        let binding=["state":"initialized","store_uuid":uuid]
        let reference:[String:Any]=["receipt_sha256":sha,"action_sha256":String(repeating:"b",count:64),"registry_sha256":String(repeating:"c",count:64),"registry_snapshot_version":2,"action":scope,"store_binding":binding]
        let target:[String:Any]=["schema":"sirsi.maat.prepared-preflight-receipt.v1","reference":reference,"store_device":1,"store_inode":2,"intent_key":String(repeating:"d",count:64),"byte_length":512]
        let receipt:[String:Any]=["schema":"sirsi.maat.failure-memory.v1","action":scope,"action_manifest_sha256":String(repeating:"b",count:64),"registry_snapshot_sha256":String(repeating:"c",count:64),"registry_snapshot_version":2,"evaluated_guards":NSNull(),"measured_checks":NSNull(),"decision":"pass","incident_keys":NSNull(),"recovery_actions":NSNull(),"recovery_reference":"","evaluated_at_utc":"2026-10-24T00:00:00Z"]
        let read=try JSONSerialization.data(withJSONObject:["schema":"sirsi.maat.failure-memory-receipt-read.v1","receipt_sha256":sha,"receipt":receipt,"store_binding":binding,"read_at_utc":"2026-10-25T00:00:00Z","freshness":"historical","registry_relation":"unverifiable","action_authorized":false])
        let targetJSON=try JSONSerialization.data(withJSONObject:target)
        let prepared:[String:Any]=["schema":"sirsi.maat.prepared-preflight-receipt.v1","target":target,"target_json":String(decoding:targetJSON,as:UTF8.self),"read_json":String(decoding:read,as:UTF8.self)]
        let object=try JSONSerialization.data(withJSONObject:prepared)
        let status=try JSONSerialization.data(withJSONObject:["state":"incomplete","store_uuid":uuid,"detail":"prepared receipt","incomplete_attempt":false,"projection_debt":false,"prepared_preflight":prepared])
        return(status,read,try JSONDecoder().decode(MaatPreparedPreflight.self,from:object))
    }
    private func response(_ data:Data,exit:Int32=0)->SirsiEngine.JSONCommandRead{
        .init(data:data,exitStatus:exit,timedOut:false,launchError:nil)
    }

    @MainActor func testInspectorAndConfirmedRecoveryNeedNoCasebookRow()async throws{
        let (status,read,prepared)=try fixture()
        var arguments:[[String]]=[]
        let model=MaatPreparedPreflightRecovery{args in
            arguments.append(args)
            return self.response(arguments.count==1 ? status:read)
        }
        // No MaatCase or projected receipt row exists in this entire flow.
        XCTAssertNil(model.confirmation())
        await model.inspect()
        XCTAssertEqual(arguments,[["maat","repair","liveness-watch","--inspect-failure-memory","--json"]])
        XCTAssertEqual(model.current,prepared)
        XCTAssertNotNil(model.retained?.read)
        let approval=try XCTUnwrap(model.confirmation())
        XCTAssertEqual(arguments.count,1,"review is not mutation")
        let recovered=await model.recover(approval)
        XCTAssertTrue(recovered)
        XCTAssertEqual(arguments.last,prepared.recoveryArguments)
        XCTAssertFalse(prepared.recoveryArguments.contains("--store"))
        XCTAssertEqual(model.recovered?.receipt.evaluated_at_utc,"2026-10-24T00:00:00Z")
        XCTAssertEqual(model.recovered?.action_authorized,false)
        XCTAssertTrue(model.message?.contains("fresh normal assessment") == true)
        XCTAssertFalse(model.busy)
        XCTAssertNil(model.confirmation())
        let repeated=await model.recover(approval)
        XCTAssertFalse(repeated)
        XCTAssertEqual(arguments.count,2,"old consent replayed")
    }

    @MainActor func testChangedInspectorBindingInvalidatesOldConsentWithoutSubmission()async throws{
        let (status,_,prepared)=try fixture()
        var calls=0
        let model=MaatPreparedPreflightRecovery{_ in calls+=1;return self.response(Data(),exit:1)}
        model.accept(try JSONDecoder().decode(MaatFailureMemoryStatus.self,from:status))
        let approval=try XCTUnwrap(model.confirmation())
        let changed=try JSONSerialization.data(withJSONObject:["state":"incomplete","store_uuid":"c4d626b3-5884-4764-97e2-9b684d3dff56","receipt_recovery_error":"prepared-receipt-unavailable"])
        model.accept(try JSONDecoder().decode(MaatFailureMemoryStatus.self,from:changed))
        let result=await model.recover(approval)
        XCTAssertFalse(result)
        XCTAssertEqual(calls,0)
        XCTAssertEqual(model.retained,prepared)
        XCTAssertNil(model.confirmation())
    }

    @MainActor func testMissingTruncatedAndUnrelatedDebtRemainUnavailable()async throws{
        let (status,_,prepared)=try fixture()
        var calls=0
        let model=MaatPreparedPreflightRecovery{_ in calls+=1;return self.response(Data(),exit:1)}
        for code in ["prepared-receipt-unavailable","prepared-inspection-unavailable"]{
            model.accept(try JSONDecoder().decode(MaatFailureMemoryStatus.self,from:status))
            let unavailable=try JSONSerialization.data(withJSONObject:["state":"incomplete","store_uuid":uuid,"receipt_recovery_error":code])
            model.accept(try JSONDecoder().decode(MaatFailureMemoryStatus.self,from:unavailable))
            XCTAssertNil(model.confirmation())
            XCTAssertEqual(model.retained,prepared)
        }
        XCTAssertEqual(calls,0)
    }

    @MainActor func testFailedOrMismatchedRecoveryRetainsDetailAndConsumesConsent()async throws{
        let (status,read,prepared)=try fixture()
        for mode in ["nonzero","truncated","mismatch"]{
            var calls=0
            let model=MaatPreparedPreflightRecovery{_ in
                calls+=1
                if mode=="nonzero"{return self.response(read,exit:1)}
                if mode=="truncated"{return self.response(Data("{".utf8))}
                return self.response(Data(String(decoding:read,as:UTF8.self).replacingOccurrences(of:self.sha,with:String(repeating:"f",count:64)).utf8))
            }
            model.accept(try JSONDecoder().decode(MaatFailureMemoryStatus.self,from:status))
            let approval=try XCTUnwrap(model.confirmation())
            let result=await model.recover(approval)
            XCTAssertFalse(result,mode)
            XCTAssertEqual(model.retained,prepared,mode)
            XCTAssertNil(model.recovered,mode)
            XCTAssertNil(model.confirmation(),mode)
            XCTAssertFalse(model.busy,mode)
            XCTAssertEqual(calls,1,mode)
        }
    }

    @MainActor func testCancelJoinsOwnedReadAndRequiresNewInspection()async throws{
        let (status,read,prepared)=try fixture()
        let entered=expectation(description:"owned canonical read entered")
        var joined=false
        let model=MaatPreparedPreflightRecovery{_ in
            entered.fulfill()
            do{try await Task.sleep(nanoseconds:2_000_000_000)}catch{}
            joined=true
            return self.response(read)
        }
        model.accept(try JSONDecoder().decode(MaatFailureMemoryStatus.self,from:status))
        let approval=try XCTUnwrap(model.confirmation())
        let operation=Task{@MainActor in await model.recover(approval)}
        await fulfillment(of:[entered],timeout:1)
        model.cancelRecovery()
        let result=await operation.value
        XCTAssertTrue(joined)
        XCTAssertFalse(result)
        XCTAssertFalse(model.busy)
        XCTAssertNil(model.recovered)
        XCTAssertNil(model.confirmation())
        XCTAssertEqual(model.retained,prepared)
    }

    @MainActor func testPreparedRecoveryBelongsToExistingOperationLifetime(){
        let operations=MaatCaseOperationStore()
        XCTAssertTrue(operations.preparedPreflightRecovery === operations.preparedPreflightRecovery)
    }
}
