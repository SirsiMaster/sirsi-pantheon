import Foundation
import SwiftUI

struct MaatPreparedPreflightTarget:Decodable,Equatable {
    let schema:String
    let reference:MaatPreflightReceiptReference
    let store_device:UInt64
    let store_inode:UInt64
    let intent_key:String
    let byte_length:Int
    var valid:Bool {
        schema=="sirsi.maat.prepared-preflight-receipt.v1" && store_inode>0 && byte_length>0 && byte_length<=8*1024*1024 &&
        reference.registry_snapshot_version==2 && reference.store_binding.valid &&
        [intent_key,reference.receipt_sha256,reference.action_sha256,reference.registry_sha256].allSatisfy(MaatCapturedPreflightIdentity.digest)
    }
    func matches(_ read:MaatRetainedPreflightRead)->Bool {
        valid && read.schema=="sirsi.maat.failure-memory-receipt-read.v1" && read.receipt.schema=="sirsi.maat.failure-memory.v1" &&
        ["pass","reject","unverifiable"].contains(read.receipt.decision) && ["same","changed","unverifiable"].contains(read.registry_relation) &&
        read.receipt_sha256==reference.receipt_sha256 && read.receipt.action_manifest_sha256==reference.action_sha256 &&
        read.receipt.registry_snapshot_sha256==reference.registry_sha256 && read.receipt.registry_snapshot_version==2 &&
        read.receipt.action==reference.action && read.store_binding==reference.store_binding &&
        read.freshness=="historical" && !read.action_authorized
    }
}

struct MaatPreparedPreflight:Decodable,Equatable {
    let schema:String
    let target:MaatPreparedPreflightTarget
    let target_json:String
    let read_json:String
    var read:MaatRetainedPreflightRead?{
        guard let data=read_json.data(using:.utf8) else{return nil}
        return MaatRetainedPreflightRead.decode(data)
    }
    func valid(storeUUID:String?)->Bool {
        guard schema==target.schema,target.valid,target_json.utf8.count<=8192,
              let raw=target_json.data(using:.utf8),let decoded=try?JSONDecoder().decode(MaatPreparedPreflightTarget.self,from:raw),decoded==target,
              target.reference.store_binding.state=="initialized",target.reference.store_binding.store_uuid==storeUUID,
              let read, target.matches(read) else{return false}
        return !read.receipt.evaluated_at_utc.isEmpty && !read.read_at_utc.isEmpty
    }
    var recoveryArguments:[String]{["maat","failure-memory","receipt","--sha256",target.reference.receipt_sha256,"--json","--recover","--confirm","--prepared-target",Data(target_json.utf8).base64EncodedString()]}
}

@MainActor final class MaatPreparedPreflightRecovery:ObservableObject {
    typealias Reader=@MainActor ([String])async->SirsiEngine.JSONCommandRead
    struct Approval:Equatable{let prepared:MaatPreparedPreflight;let generation:UUID}
    @Published private(set)var current:MaatPreparedPreflight?
    @Published private(set)var retained:MaatPreparedPreflight?
    @Published private(set)var recovered:MaatRetainedPreflightRead?
    @Published private(set)var inspection:MaatFailureMemoryStatus?
    @Published private(set)var summary="Local failure memory has not been inspected. No health or empty-history claim is available."
    @Published private(set)var busy=false
    @Published private(set)var message:String?
    private var generation=UUID()
    private var readTask:Task<SirsiEngine.JSONCommandRead,Never>?
    private let reader:Reader
    var canReviewFirstUseInitialization:Bool {
        guard let inspection else{return false}
        return inspection.state=="uninitialized" && inspection.journalReadiness=="ready" && !inspection.incompleteAttempt && !inspection.projectionDebt &&
            inspection.preparedPreflight==nil && inspection.receiptRecoveryError==nil
    }
    init(reader:Reader?=nil){self.reader=reader ?? {await SirsiEngine.runJSONRead(args:$0)}}
    func inspectionUnavailable(){
        guard !busy else{return};generation=UUID();current=nil;inspection=nil
        summary="Local failure memory inspection is unavailable. Prior historical detail remains; no approval is available."
        if retained==nil && recovered==nil { message="Local memory inspection is unavailable. Prior historical detail remains; retry inspection before any recovery." }
    }
    func cancelRecovery(){readTask?.cancel()}
    func accept(_ status:MaatFailureMemoryStatus){
        guard !busy else{return}
        generation=UUID();current=nil;inspection=status;summary=Self.summary(for:status)
        if status.state=="incomplete",status.journalReadiness=="ready",!status.incompleteAttempt,!status.projectionDebt,status.receiptRecoveryError==nil,
           let prepared=status.preparedPreflight,prepared.valid(storeUUID:status.storeUUID){
            current=prepared;retained=prepared
            if recovered==nil { message="Complete prepared historical receipt verified. Review exact recovery; no action or Casebook projection is authorized." }
        }else if status.preparedPreflight != nil || status.receiptRecoveryError != nil{
            if retained==nil && recovered==nil { message="Prepared receipt debt is unavailable or does not match current custody. Reinspect; missing, changed or unrelated bytes will not be reconstructed." }
        }
    }
    private static func summary(for status:MaatFailureMemoryStatus)->String {
        let attempt=status.incompleteAttempt ? "present" : "none"
        let projection=status.projectionDebt ? "present" : "none"
        let debt="Incomplete attempt: \(attempt) · projection debt: \(projection)."
        let receiptError=status.receiptRecoveryError.map{" Receipt recovery error: \($0)."} ?? ""
        let journalStatus:String
        switch status.journalReadiness {
        case "ready": journalStatus="Strict journal readiness: ready."
        case "invalid": journalStatus="Strict journal readiness: invalid; at least one record is rejected."
        case "unavailable": journalStatus="Strict journal readiness: unavailable."
        default: journalStatus="Strict journal readiness: unknown."
        }
        switch status.state {
        case "uninitialized":
            return "Local failure memory: not created. \(debt) \(journalStatus) No health or empty-history claim is available.\(receiptError)"
        case "ready":
            return "Local failure memory: ready. \(debt) \(journalStatus) Readiness is not a health or empty-history claim.\(receiptError)"
        case "incomplete":
            return "Local failure memory: incomplete. \(debt) \(journalStatus) Only an exact verified prepared receipt can be reviewed; no health or empty-history claim is available.\(receiptError)"
        default:
            return "Local failure memory state is unknown (\(status.state)). \(debt) \(journalStatus) No approval is available.\(receiptError)"
        }
    }
    func inspect()async {
        guard !busy else{return};busy=true
        generation=UUID();current=nil;inspection=nil
        summary="Inspecting local failure memory… no approval is available until this read completes."
        let task=Task{@MainActor in await self.reader(["maat","repair","liveness-watch","--inspect-failure-memory","--json"])}
        readTask=task
        let result=await withTaskCancellationHandler(operation:{await task.value},onCancel:{task.cancel()})
        readTask=nil
        busy=false
        guard !task.isCancelled,!Task.isCancelled,result.exitStatus==0,!result.timedOut,result.launchError==nil,
              let status=try?JSONDecoder().decode(MaatFailureMemoryStatus.self,from:result.data)else{
            generation=UUID();current=nil;inspection=nil
            summary="Local failure memory inspection failed or was canceled. Prior historical detail remains; no approval is available."
            if retained==nil && recovered==nil { message="Prepared receipt inspection failed or was canceled. Prior detail is retained; no recovery was attempted." };return
        }
        accept(status)
    }
    func confirmation()->Approval?{guard !busy,let current else{return nil};return Approval(prepared:current,generation:generation)}
    @discardableResult func recover(_ approved:Approval)async->Bool{
        guard !busy,generation==approved.generation,current==approved.prepared,!Task.isCancelled else{return false}
        generation=UUID();current=nil;busy=true
        defer{busy=false}
        let task=Task{@MainActor in await self.reader(approved.prepared.recoveryArguments)}
        readTask=task
        let result=await withTaskCancellationHandler(operation:{await task.value},onCancel:{task.cancel()})
        readTask=nil
        guard !task.isCancelled,!Task.isCancelled,result.exitStatus==0,!result.timedOut,result.launchError==nil,
              let historical=MaatRetainedPreflightRead.decode(result.data),approved.prepared.target.matches(historical),
              historical.receipt.evaluated_at_utc==approved.prepared.read?.receipt.evaluated_at_utc else{
            message="Exact receipt recovery failed, was canceled or could not be verified. Prior detail and debt remain; inspect again before another confirmation.";return false
        }
        recovered=historical
        message="Historical receipt write recovered. No Casebook row, decision, attempt or installation was created. Refresh local state, then use fresh normal assessment and new action consent."
        return true
    }
}

struct MaatPreparedPreflightRecoveryView:View {
    @ObservedObject var model:MaatPreparedPreflightRecovery
    let refresh:()async->Void
    @State private var approved:MaatPreparedPreflightRecovery.Approval?
    @State private var confirming=false
    func recoverConfirmed(_ approval: MaatPreparedPreflightRecovery.Approval) async {
        if await model.recover(approval) { await refresh() }
    }
    var body:some View{
        VStack(alignment:.leading,spacing:8){
            if let prepared=model.retained {
                Text("Retained prepared preflight · \(prepared.target.reference.action.component) / \(prepared.target.reference.action.operation)").sirsiFont(.caption,weight:.semibold)
                Text("Store \(prepared.target.reference.store_binding.store_uuid ?? "unknown") · receipt \(prepared.target.reference.receipt_sha256)\nIntent \(prepared.target.intent_key) · \(prepared.target.byte_length) bytes").sirsiFont(.caption).textSelection(.enabled)
                if let read=prepared.read {
                    Text("Historical \(read.receipt.decision) · observed \(read.receipt.evaluated_at_utc) · registry \(read.registry_relation)").sirsiFont(.caption)
                    DisclosureGroup("Complete verified historical receipt") {
                        ScrollView {
                            Text(prepared.read_json).sirsiFont(.caption).textSelection(.enabled).frame(maxWidth:.infinity,alignment:.leading)
                        }.frame(maxHeight:240)
                    }.sirsiFont(.caption)
                }
            }
            if let message=model.message{Text(message).sirsiFont(.caption).foregroundStyle(.secondary)}
            if model.current != nil {
                Button("Review prepared receipt recovery"){approved=model.confirmation();confirming=approved != nil}.buttonStyle(.bordered).disabled(model.busy)
            }
            if model.busy{ProgressView("Recovering exact historical receipt…");Button("Cancel receipt recovery"){model.cancelRecovery()}.buttonStyle(.bordered)}
            if model.retained != nil || model.message != nil {
                Button("Reinspect local memory"){Task{await refresh()}}.buttonStyle(.bordered).disabled(model.busy)
            }
        }
        .confirmationDialog("Complete this exact prepared receipt write?",isPresented:$confirming){
            Button("Complete exact historical receipt"){if let approved{Task{await recoverConfirmed(approved)}};approved=nil}
            Button("Cancel",role:.cancel){approved=nil}
        }message:{Text("Receipt: \(approved?.prepared.target.reference.receipt_sha256 ?? "unavailable")\nStore: \(approved?.prepared.target.reference.store_binding.store_uuid ?? "unavailable")\nOnly the captured complete receipt/intent/store tuple is eligible. No missing bytes, projections or actions are manufactured. A fresh normal assessment and new consent remain required.")}
    }
}
