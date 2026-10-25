import Foundation

struct LegacyRelayAppTicket:Decodable,Equatable {
    let schema:Int;let generation:UInt64;let id:String;let nonce:String;let operation:String
    let plan:LegacyRelayPlan;let transaction:String;let restoreSHA256:String;let restoreRevision:UInt64
    enum CodingKeys:String,CodingKey{case schema,generation,nonce,operation,plan,transaction;case id="operation_id",restoreSHA256="restore_sha256",restoreRevision="restore_revision"}
    var valid:Bool{schema==1 && generation>0 && plan.isRootProfile && Self.hex(id,32) && Self.hex(nonce,32) && Self.hex(transaction,32) &&
        ["confirm","recover","abort","restore-confirm"].contains(operation)}
    static func hex(_ s:String,_ n:Int)->Bool{s.count==n && s.utf8.allSatisfy{(48...57).contains($0)||(97...102).contains($0)}}
}
struct LegacyRelaySavedReceipt:Codable,Equatable {
    let stdout:Data;let stderr:Data;let exitCode:Int32?;let issue:String?;let unknown:Bool
    init(_ execution:CommandExecution){
        stdout=execution.stdoutData;stderr=execution.stderrData;exitCode=execution.exitCode;issue=execution.issue
        unknown=execution.childState == .exitUnknown || execution.captureError != nil || execution.cancelled || execution.timedOut
    }
}
struct LegacyRelayOutstandingOperation:Codable,Equatable {
    let operation:String;let ticketJSON:String;let originalPreview:Data
    var receipt:LegacyRelaySavedReceipt?
    var ticket:LegacyRelayAppTicket? {
        guard ticketJSON.utf8.count<=8192,let data=ticketJSON.data(using:.utf8),
              let value=try?JSONDecoder().decode(LegacyRelayAppTicket.self,from:data),value.valid,value.operation==operation else{return nil}
        return value
    }
}
struct LegacyRelaySavedOperationState:Codable{
    var schema=1
    var outstanding:LegacyRelayOutstandingOperation?
    var resolved:LegacyRelayOutstandingOperation?
    var lifecycleDebt=false
    var lastTransaction:LegacyRelayTransaction?
}
@MainActor struct LegacyRelayOperationStorage {
    let read:()->Data?
    let write:(Data)throws->Void
    static func preferences()->Self{
        let key="legacy-root-maintenance-operation-v1"
        return Self(read:{UserDefaults.standard.data(forKey:key)},write:{data in
            guard data.count<=256*1024 else{throw NSError(domain:"MaintenancePublicReceiptBound",code:1)}
            UserDefaults.standard.set(data,forKey:key)
            guard UserDefaults.standard.synchronize(),UserDefaults.standard.data(forKey:key)==data else {
                throw NSError(domain:"MaintenancePublicReceiptPersistence",code:1)
            }
        })
    }
}
