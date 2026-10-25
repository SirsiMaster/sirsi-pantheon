import Foundation
import Combine
import Darwin
import Security
import AppKit

struct LegacyRelayPlan: Codable, Equatable {
    struct Instance: Codable, Equatable {
        let label: String
        let target: String
        let pid: Int
        let started: String
    }
    let profile: String
    let original: Instance
    let legacyPayload: String
    let sha256: String
    enum CodingKeys: String, CodingKey { case profile, original, sha256; case legacyPayload = "legacy_payload" }
    var isRootProfile: Bool {
        profile == "root-relay-legacy-maintenance-v1" && original.label == "ai.sirsi.router.relay" &&
        original.target == "system/ai.sirsi.router.relay" && original.pid > 1 && !original.started.isEmpty &&
        sha256.count == 64 && sha256.utf8.allSatisfy { (48...57).contains($0) || (97...102).contains($0) }
    }
}

struct LegacyRelayTransaction: Codable, Equatable {
    let id: String
    let revision: UInt64
    let plan: LegacyRelayPlan
    let phase: String
    let preserved: Int
    let oldOutcomes: String
    let nextAction: String
    enum CodingKeys: String, CodingKey {
        case id, revision, plan, phase
        case preserved = "preserved_envelopes", oldOutcomes = "old_outcomes", nextAction = "next_action"
    }
    var isBound: Bool { !id.isEmpty && plan.isRootProfile && preserved >= 0 && oldOutcomes == "unknown_not_certified_drained" }
}

enum LegacyRelayAppHost {
    // The GUI stays unprivileged. The typed bridge independently qualifies the
    // protected engine and lets macOS own its one-shot administrator dialog.
    static func refusal() -> String? {
        guard Bundle.main.bundleIdentifier == "ai.sirsi.pantheon" else { return "The canonical signed Pantheon app host is required." }
        var code: SecCode?
        var requirement: SecRequirement?
        let text = "anchor apple generic and certificate leaf[subject.OU] = \"9D382WV988\" and identifier \"ai.sirsi.pantheon\""
        guard SecCodeCopySelf(SecCSFlags(rawValue: 0), &code) == errSecSuccess, let code,
              SecRequirementCreateWithString(text as CFString, SecCSFlags(rawValue: 0), &requirement) == errSecSuccess, let requirement,
              SecCodeCheckValidity(code, SecCSFlags(rawValue: 0), requirement) == errSecSuccess else {
            return "Pantheon's existing signed app identity was not verified. No service action was started."
        }
        return nil
    }
}

@MainActor final class LegacyRelayMaintenanceModel: ObservableObject {
    typealias Capture = @MainActor ([String], String?, URL, @escaping @MainActor () -> Bool) async -> CommandExecution
    struct Approval: Equatable { let plan: LegacyRelayPlan; let bytes: Data; let file: URL; let directory: URL; let generation: UUID;let ticketJSON:String }
    struct RestoreApproval: Equatable {
        let transaction: LegacyRelayTransaction
        let artifact: URL
        let digest: String
        let bytes: Data
        let directory: URL
        let generation: UUID
        let ticketJSON:String
    }
    struct ActionApproval:Equatable {let operation:String;let transaction:LegacyRelayTransaction;let ticketJSON:String;let directory:URL;let generation:UUID}
    struct InstallerApproval: Equatable {
        let release:String;let package:String;let digest:String;let bytes:Data;let generation:UUID
    }
    @Published private(set) var selectedFile: URL?
    @Published private(set) var preview: LegacyRelayPlan?
    @Published private(set) var transaction: LegacyRelayTransaction?
    @Published private(set) var receipt: CommandExecution?
    @Published private(set) var busy = false
    @Published private(set) var error: String?
    @Published private(set) var canConfirm = false
    @Published private(set) var restoreApproval: RestoreApproval?
    @Published private(set) var outcomeUnknown = false
    @Published private(set) var statusNotice: String?
    @Published private(set) var installer: InstallerApproval?
    @Published private(set) var installerReceipt:CommandExecution?
    @Published private(set) var actionApproval:ActionApproval?
    @Published private(set) var outstanding:LegacyRelayOutstandingOperation?
    @Published private(set) var lastResolved:LegacyRelayOutstandingOperation?
    @Published private(set) var observedTransaction:LegacyRelayTransaction?
    @Published private(set) var lifecycleDebt=false
    private var approval: Approval?
    private var generation = UUID()
    private var directory: URL
    private var transactionDirectory: URL?
    private var activeTask: Task<Void, Never>?
    private var observationTask: Task<CommandExecution,Never>?
    private let capture: Capture
    private let installerCapture: Capture
    private let openInstaller: @MainActor (URL)->Bool
    private let operationStorage:LegacyRelayOperationStorage

    init(directory: URL, capture: Capture? = nil, installerCapture: Capture? = nil, openInstaller: (@MainActor (URL)->Bool)? = nil,storage:LegacyRelayOperationStorage? = nil) {
        self.directory = directory
        let stateStorage=storage ?? .preferences()
        operationStorage=stateStorage
        self.capture = capture ?? { args, stdin, _, authorize in
            await LegacyRelayPrivilegedBridge().capture(args: args, stdin: stdin, authorize: authorize)
        }
        self.installerCapture = installerCapture ?? {args,_,_,authorize in
            if let reason=LegacyRelayAppHost.refusal(){return CommandExecution(launchError:reason)}
            guard let executable=Bundle.main.executableURL,let release=Bundle.main.object(forInfoDictionaryKey:"CFBundleShortVersionString") as? String,!release.isEmpty else {
                return CommandExecution(launchError:"Canonical app release metadata is unavailable.")
            }
            let cli=executable.deletingLastPathComponent().appendingPathComponent("sirsi")
            guard SirsiEngine.isTrustedBundledCLI(cli) else {return CommandExecution(launchError:"The app's update client is missing.")}
            return await SirsiEngine.captureProcess(executable:cli.path,args:args,directory:URL(fileURLWithPath:NSHomeDirectory()),
                timeout:130,outputLimit:16*1024,environment:["PATH":"/usr/bin:/bin:/usr/sbin:/sbin","HOME":NSHomeDirectory(),"TMPDIR":NSTemporaryDirectory(),"LC_ALL":"C"],authorize:authorize)
        }
        self.openInstaller=openInstaller ?? {NSWorkspace.shared.open($0)}
        if let data=stateStorage.read() {
            if data.count<=256*1024,let saved=try?JSONDecoder().decode(LegacyRelaySavedOperationState.self,from:data),saved.schema==1,
               saved.outstanding == nil || saved.outstanding?.ticket != nil {
                outstanding=saved.outstanding;lastResolved=saved.resolved;outcomeUnknown=saved.outstanding != nil
                lifecycleDebt=saved.lifecycleDebt;transaction=saved.lastTransaction;transactionDirectory=directory
                if let old=saved.outstanding?.receipt {receipt=CommandExecution(stdout:old.stdout,stderr:old.stderr,exitCode:old.exitCode,captureError:old.issue ?? "Restored uncertain operation; check result and fence late arrival",childState:.exitUnknown)}
            }else{outcomeUnknown=true;error="Saved public operation binding is unreadable. Read controller status before any new review."}
        }
    }

    func select(_ file: URL) { selectedFile = file; cancelReview() }
    func changeDirectory(_ directory: URL) { if self.directory != directory { self.directory = directory; cancelReview() } }
    func cancelReview() { generation = UUID(); approval = nil; restoreApproval = nil; actionApproval=nil;canConfirm = false; preview = nil }

    func prepareInstaller() async {
        guard !busy else {return}
        busy=true;error=nil;installer=nil
        let token=generation,scope=directory
        let result=await observe(["launchd","maintenance-installer"],scope,{self.generation==token},using:installerCapture)
        busy=false
        guard !Task.isCancelled,generation==token else{return}
        installerReceipt=result
        guard result.complete,result.exitCode==0,let plan=try?JSONDecoder().decode(InstallerEnvelope.self,from:result.stdoutData),plan.valid else {error=detail(result);return}
        installer=InstallerApproval(release:plan.release,package:plan.package,digest:plan.sha256,bytes:result.stdoutData,generation:token)
        statusNotice="Compatible signed/notarized PKG verified. Review and open normal Installer, then recheck the installed app/helper pair. No service action was performed."
    }
    func install(_ approved:InstallerApproval)->Task<Void,Never>? {
        guard !busy,installer==approved,generation==approved.generation else{return nil}
        installer=nil;busy=true;error=nil
        let task=Task{@MainActor in
            let encoded=approved.bytes.map{String(format:"%02x",$0)}.joined()
            let result=await self.installerCapture(["launchd","maintenance-installer","--verify-selection",encoded],nil,self.directory,
                {self.generation==approved.generation})
            self.installerReceipt=result
            if result.complete,result.exitCode==0,
               let plan=try?JSONDecoder().decode(InstallerEnvelope.self,from:result.stdoutData),plan.valid,
               plan.package==approved.package,plan.sha256==approved.digest,plan.release==approved.release,
               self.generation==approved.generation,!Task.isCancelled {
                if self.openInstaller(URL(fileURLWithPath:plan.package)) {
                    self.statusNotice="Normal Installer opened. Complete its signed PKG workflow, then choose Recheck installed pair. If Pantheon was replaced, reopen the one installed app before maintenance."
                }else{self.error="Installer did not open. Check the release again; no maintenance or installation was claimed."}
            }else{self.error=self.detail(result)}
            self.busy=false
        }
        activeTask=task;return task
    }
    func recheckInstalledPair()async {
        guard !busy else{statusNotice="A request is pending; no concurrent installation or root action was started.";return}
        busy=true;let scope=directory;let token=generation
        let result=await observe(["launchd","legacy-maintenance","status"],scope,{self.generation==token})
        busy=false
        guard !Task.isCancelled,generation==token else{return}
        acceptStatus(result,scope:scope)
        if result.complete,result.exitCode==0,!outcomeUnknown {statusNotice="Installed pair/protocol qualified and current transaction read. No service action was started."}
    }
    private struct InstallerEnvelope:Decodable {
        let schema:Int;let release:String;let package:String;let sha256:String;let device:UInt64;let inode:UInt64;let implementation:String
        var valid:Bool {
            let url=URL(fileURLWithPath:package)
            return schema==1 && !release.isEmpty && package.hasPrefix("/") &&
                url.deletingLastPathComponent().lastPathComponent.hasPrefix("sirsi-maintenance-installer-") &&
                url.lastPathComponent=="SirsiPantheon-\(release)-arm64.pkg" && inode>0 &&
                sha256.count==64 && implementation.count==64 &&
                sha256.utf8.allSatisfy{(48...57).contains($0)||(97...102).contains($0)}
        }
    }

    func review() async {
        guard !busy, !outcomeUnknown,!lifecycleDebt, let file = selectedFile else { return }
        cancelReview(); let token = generation; let scope = directory
        busy = true; error = nil
        let result = await observe(["launchd", "legacy-maintenance", "preview", "--legacy-payload", file.path], scope, { self.generation == token && self.selectedFile == file && self.directory == scope })
        busy = false
        guard !Task.isCancelled, generation == token, selectedFile == file, directory == scope else { return }
        receipt = result
        guard result.complete, result.exitCode == 0,
              let response = try? JSONDecoder().decode(PreviewEnvelope.self, from: result.stdoutData),
              response.plan.isRootProfile,response.plan.legacyPayload == file.path,
              response.ticket.valid,response.ticket.operation=="confirm",response.ticket.plan==response.plan,
              Self.ticket(response.ticketJSON)==response.ticket else { error = detail(result); return }
        preview=response.plan;approval=Approval(plan:response.plan,bytes:result.stdoutData,file:file,directory:scope,generation:token,ticketJSON:response.ticketJSON);canConfirm=true
    }

    // Consume consent before creating the task. Navigation may invalidate the
    // preview but cannot replay or cancel a once-started root transaction.
    func confirmation() -> Approval? { guard !busy,let approval,current(approval) else{return nil};return approval }
    func confirm(_ approved: Approval) -> Task<Void, Never>? {
        guard !busy, approval == approved, current(approved) else { return nil }
        guard remember(operation:"confirm",ticketJSON:approved.ticketJSON,preview:approved.bytes) else{return nil}
        approval = nil; canConfirm = false; busy = true; error = nil
        let task = Task { @MainActor in
            let result = await self.capture(["launchd", "legacy-maintenance", "confirm", "--legacy-payload", approved.file.path,
                                            "--expected-plan-sha256", approved.plan.sha256, "--confirm-maintenance"]+Self.ticketArgs(approved.ticketJSON), nil, approved.directory, { self.current(approved) })
            self.acceptMutation(result,ticketJSON:approved.ticketJSON,scope:approved.directory);self.busy=false
        }
        activeTask=task;return task
    }

    func readStatus() async {
        guard !busy else {
            statusNotice="A reviewed request is pending authorization or execution. No new privileged status read was started. Cancel Request remains available; cancellation does not prove rollback."
            return
        }
        statusNotice=nil;busy = true; let scope = directory;let token=generation
        let result = await observe(["launchd", "legacy-maintenance", "status"], scope, { self.directory == scope })
        busy = false
        guard !Task.isCancelled, directory == scope, generation == token else { return }
        acceptStatus(result,scope:scope)
    }

    func resume(_ expected: LegacyRelayTransaction) -> Task<Void, Never>? { perform("recover",expected:expected) }
    func abortBeforeRetirement(_ expected: LegacyRelayTransaction) -> Task<Void, Never>? { perform("abort",expected:expected) }
    func cancelInvocation(){cancelReview();observationTask?.cancel();activeTask?.cancel()}

    private func perform(_ operation: String,expected:LegacyRelayTransaction) -> Task<Void, Never>? {
        guard !busy,!outcomeUnknown,let consent=actionApproval,consent.operation==operation,consent.transaction==expected,
              consent.directory==directory,consent.generation==generation,
              let approved = transaction, approved == expected, approved.isBound, transactionDirectory == directory else { return nil }
        guard remember(operation:operation,ticketJSON:consent.ticketJSON,preview:Data(consent.ticketJSON.utf8)) else{return nil}
        actionApproval=nil
        let scope = directory; busy = true; error = nil
        let task = Task { @MainActor in
            let result = await self.capture(["launchd", "legacy-maintenance", operation, "--transaction", approved.id,
                                            "--expected-plan-sha256", approved.plan.sha256, "--confirm-maintenance"]+Self.ticketArgs(consent.ticketJSON), nil, scope,
                                           { self.directory == scope && self.transaction == approved })
            self.acceptMutation(result,ticketJSON:consent.ticketJSON,scope:scope);self.busy=false
        }
        activeTask=task;return task
    }
    func reviewAction(_ operation:String,expected:LegacyRelayTransaction)async{
        guard !busy,!outcomeUnknown,["recover","abort"].contains(operation),transaction==expected else{return}
        busy=true;actionApproval=nil;let token=generation;let scope=directory
        let args=["launchd","legacy-maintenance","prepare-operation","--operation",operation,"--transaction",expected.id,"--expected-plan-sha256",expected.plan.sha256]
        let result=await observe(args,scope,{self.generation==token&&self.transaction==expected})
        busy=false
        guard !Task.isCancelled,generation==token,transaction==expected else{return}
        receipt=result
        guard result.complete,result.exitCode==0,let response=try?JSONDecoder().decode(PreviewEnvelope.self,from:result.stdoutData),
              response.ticket.operation==operation,response.ticket.transaction==expected.id,response.ticket.plan==expected.plan,
              Self.ticket(response.ticketJSON)==response.ticket else{error=detail(result);return}
        actionApproval=ActionApproval(operation:operation,transaction:expected,ticketJSON:response.ticketJSON,directory:scope,generation:token)
    }

    func reviewRestore(artifact: URL, expected: LegacyRelayTransaction) async {
        guard !busy, expected == transaction, expected.isBound, transactionDirectory == directory else { return }
        restoreApproval = nil
        let token = generation
        let scope = directory
        busy = true; error = nil
        defer{busy=false}
        let result = await observe(["launchd", "legacy-maintenance", "restore-preview", "--transaction", expected.id,
                                    "--expected-plan-sha256", expected.plan.sha256, "--artifact", artifact.path], scope,
                                   { self.generation == token && self.directory == scope && self.transaction == expected })
        guard !Task.isCancelled, generation == token, directory == scope, transaction == expected else { return }
        receipt = result
        guard result.complete, result.exitCode == 0, result.stdoutData.count <= 8192,
              let value = try? JSONDecoder().decode(RestoreEnvelope.self, from: result.stdoutData),
              value.transaction == expected.id, value.revision == expected.revision,
              value.maintenanceSHA256 == expected.plan.sha256, value.artifact == artifact.path,
              Self.digest(value.sha256) else { error = detail(result);busy=false;return }
        let prepared=await observe(["launchd","legacy-maintenance","prepare-operation","--operation","restore-confirm",
                                    "--transaction",expected.id,"--expected-plan-sha256",expected.plan.sha256],
                                   scope,{self.generation==token&&self.transaction==expected},
                                   stdin:String(decoding:result.stdoutData,as:UTF8.self))
        busy=false
        guard !Task.isCancelled,generation==token,transaction==expected,prepared.complete,prepared.exitCode==0,
              let response=try?JSONDecoder().decode(PreviewEnvelope.self,from:prepared.stdoutData),
              response.ticket.operation=="restore-confirm",response.ticket.transaction==expected.id,
              response.ticket.restoreSHA256==value.sha256,response.ticket.restoreRevision==expected.revision,
              Self.ticket(response.ticketJSON)==response.ticket else {error=detail(prepared);return}
        restoreApproval = RestoreApproval(transaction: expected, artifact: artifact, digest: value.sha256,
                                          bytes: result.stdoutData, directory: scope, generation: token,ticketJSON:response.ticketJSON)
    }

    func restore(_ approved: RestoreApproval) -> Task<Void, Never>? {
        guard !busy, restoreApproval == approved, transaction == approved.transaction,
              directory == approved.directory, generation == approved.generation else { return nil }
        guard remember(operation:"restore-confirm",ticketJSON:approved.ticketJSON,preview:approved.bytes) else{return nil}
        restoreApproval = nil; busy = true; error = nil
        let task = Task { @MainActor in
            let result = await self.capture(["launchd", "legacy-maintenance", "restore-confirm",
                                            "--expected-restore-sha256", approved.digest, "--confirm-maintenance"]+Self.ticketArgs(approved.ticketJSON),
                                           String(decoding: approved.bytes, as: UTF8.self), approved.directory,
                                           { self.directory == approved.directory && self.generation == approved.generation &&
                                               self.transaction == approved.transaction })
            self.acceptMutation(result,ticketJSON:approved.ticketJSON,scope:approved.directory)
            self.busy = false
        }
        activeTask = task; return task
    }
    func resolveOutstanding(_ approved:LegacyRelayOutstandingOperation)->Task<Void,Never>?{
        guard !busy,outstanding==approved,approved.ticket != nil else{return nil}
        let scope=directory;busy=true
        let task=Task{@MainActor in
            let result=await self.capture(["launchd","legacy-maintenance","resolve-operation"]+Self.ticketArgs(approved.ticketJSON),nil,scope,
                {self.outstanding==approved})
            defer{self.busy=false}
            guard result.complete,result.exitCode==0,
                  let response=try?JSONDecoder().decode(ResolutionEnvelope.self,from:result.stdoutData),
                  response.ticket==approved.ticket,
                  ["matching-terminal","future-execution-fenced","fenced-interrupted-needs-recovery","future-fenced-past-detail-not-retained"].contains(response.disposition) else{
                self.error=self.detail(result);return
            }
            self.lastResolved=approved;self.outstanding=nil;self.outcomeUnknown=false
            self.lifecycleDebt = !response.safeForNewPreview
            if response.status.isBound,response.status.id==response.ticket.transaction,response.status.plan==response.ticket.plan{
                self.transaction=response.status;self.transactionDirectory=scope
            }
            self.statusNotice="\(response.disposition) · past outcome: \(response.pastOutcome). Old receipt retained. " +
                (response.safeForNewPreview ? "A fresh preview and new consent are available." : "Use the exact retained transaction's recovery controls.")
            self.error=nil
            if !self.persistPublicState(){
                self.outstanding=approved;self.lastResolved=nil;self.outcomeUnknown=true;self.lifecycleDebt=true
            }
        }
        activeTask=task;return task
    }

    private struct RestoreEnvelope: Decodable {
        let transaction: String; let revision: UInt64; let artifact: String
        let maintenanceSHA256: String; let sha256: String
        enum CodingKeys: String, CodingKey { case transaction, revision, artifact, sha256; case maintenanceSHA256 = "maintenance_sha256" }
    }
    private func observe(_ args:[String],_ scope:URL,_ authorize:@escaping @MainActor()->Bool,using override:Capture? = nil,stdin:String? = nil) async -> CommandExecution {
        let execute=override ?? capture
        let request=Task { @MainActor in await execute(args,stdin,scope,authorize) }
        observationTask=request
        let result=await withTaskCancellationHandler(operation:{await request.value},onCancel:{request.cancel()})
        observationTask=nil
        if request.isCancelled || Task.isCancelled {
            return CommandExecution(stdout:result.stdoutData,stderr:result.stderrData,exitCode:result.exitCode,
                launchError:result.launchError,timedOut:result.timedOut,cancelled:true,overflow:result.overflow,
                captureError:result.captureError,childState:result.childState,termSent:result.termSent)
        }
        return result
    }
    private static func digest(_ value: String) -> Bool { value.count == 64 && value.utf8.allSatisfy { (48...57).contains($0) || (97...102).contains($0) } }
    private func current(_ value: Approval) -> Bool { value.generation == generation && value.file == selectedFile && value.directory == directory && value.plan == preview }
    private struct PreviewEnvelope:Decodable{
        let plan:LegacyRelayPlan;let ticket:LegacyRelayAppTicket;let ticketJSON:String
        enum CodingKeys:String,CodingKey{case plan,ticket;case ticketJSON="ticket_json"}
    }
    private struct MutationEnvelope:Decodable{
        let ticketJSON:String;let status:LegacyRelayTransaction;let terminal:Bool;let safeForNewPreview:Bool
        enum CodingKeys:String,CodingKey{case status,terminal;case ticketJSON="ticket_json",safeForNewPreview="safe_for_new_preview"}
    }
    private struct ResolutionEnvelope:Decodable{
        let ticket:LegacyRelayAppTicket;let disposition:String;let status:LegacyRelayTransaction;let safeForNewPreview:Bool;let pastOutcome:String
        enum CodingKeys:String,CodingKey{case ticket,disposition,status;case safeForNewPreview="safe_for_new_preview",pastOutcome="past_outcome"}
    }
    private struct StatusEnvelope:Decodable{
        struct Admission:Decodable{let state:String}
        let status:LegacyRelayTransaction;let admission:Admission;let ticketJSON:String
        enum CodingKeys:String,CodingKey{case status,admission;case ticketJSON="ticket_json"}
    }
    private static func ticket(_ raw:String)->LegacyRelayAppTicket?{
        guard raw.utf8.count<=8192,let data=raw.data(using:.utf8),let ticket=try?JSONDecoder().decode(LegacyRelayAppTicket.self,from:data),ticket.valid else{return nil}
        return ticket
    }
    private static func ticketArgs(_ raw:String)->[String]{["--app-ticket",Data(raw.utf8).base64EncodedString()]}
    @discardableResult private func persistPublicState()->Bool{
        do{
            let saved=LegacyRelaySavedOperationState(outstanding:outstanding,resolved:lastResolved,lifecycleDebt:lifecycleDebt,lastTransaction:transaction)
            let data=try JSONEncoder().encode(saved);guard data.count<=256*1024 else{throw NSError(domain:"MaintenancePublicReceiptBound",code:1)}
            try operationStorage.write(data);return true
        }catch{self.outcomeUnknown=true;self.approval=nil;self.actionApproval=nil;self.restoreApproval=nil;self.canConfirm=false
            self.error="Public operation binding/receipt was not durably retained. No new submission is authorized; read matching controller state and resolve the retained operation.";return false}
    }
    private func remember(operation:String,ticketJSON:String,preview:Data)->Bool{
        guard outstanding==nil,let value=Self.ticket(ticketJSON),value.operation==operation else{return false}
        outstanding=LegacyRelayOutstandingOperation(operation:operation,ticketJSON:ticketJSON,originalPreview:preview)
        outcomeUnknown=true
        return persistPublicState()
    }
    private func acceptMutation(_ result:CommandExecution,ticketJSON:String,scope:URL){
        guard var retained=outstanding,retained.ticketJSON==ticketJSON else{error="Operation binding changed; read controller state";return}
        receipt=result;retained.receipt=LegacyRelaySavedReceipt(result);outstanding=retained
        if result.childState == .notStarted,result.captureError == nil{
            lastResolved=retained;outstanding=nil;outcomeUnknown=false
            statusNotice="The submitted operation did not start. Its unused ticket may be superseded by a new reviewed preview."
        }else if result.complete,let response=try?JSONDecoder().decode(MutationEnvelope.self,from:result.stdoutData),
                 response.ticketJSON==ticketJSON,response.terminal {
            lastResolved=retained;outstanding=nil;outcomeUnknown=false;lifecycleDebt = !response.safeForNewPreview
            if response.status.isBound,response.status.id==retained.ticket?.transaction,response.status.plan==retained.ticket?.plan{
                transaction=response.status;transactionDirectory=scope
            }
        }else{outcomeUnknown=true;statusNotice="Exact operation \(retained.ticket?.id ?? "unknown") remains unresolved. Check result and prevent a delayed submission."}
        error=result.complete && result.exitCode==0 && !outcomeUnknown ? nil : detail(result)
        if !persistPublicState(){outstanding=retained;outcomeUnknown=true;lifecycleDebt=true}
    }
    private func acceptStatus(_ result:CommandExecution,scope:URL){
        guard result.complete,let response=try?JSONDecoder().decode(StatusEnvelope.self,from:result.stdoutData)else{error=detail(result);return}
        observedTransaction=response.status.isBound ? response.status:nil
        if let retained=outstanding{
            // Status is context, never implicit resolve-or-revoke. In particular
            // oldA and an empty journal cannot consume B's unknown receipt.
            statusNotice="Status observed without resolving operation \(retained.ticket?.id ?? "unknown"). Use Check result and prevent delayed submission."
            error=nil;return
        }
        if (["running","fenced"].contains(response.admission.state) || outcomeUnknown && ["reserved","terminal"].contains(response.admission.state)),let t=Self.ticket(response.ticketJSON){
            outstanding=LegacyRelayOutstandingOperation(operation:t.operation,ticketJSON:response.ticketJSON,originalPreview:result.stdoutData)
            outcomeUnknown=true;statusNotice="Controller-owned outstanding operation restored. An earlier local receipt is not fabricated."
            persistPublicState();return
        }
        receipt=result
        if response.status.isBound{transaction=response.status;transactionDirectory=scope}
        statusNotice=response.status.isBound ? "Existing transaction observed; its recorded phase is not a new completion claim.":"No lifecycle transaction observed. This read does not revoke an uncertain submission."
        error=result.exitCode==0 ? nil:detail(result)
    }
    private func detail(_ value: CommandExecution) -> String {
        value.issue ?? (value.stderr.isEmpty ? "No verified matching transaction result. Read current status; do not repeat automatically." : String(value.stderr.prefix(1200)))
    }
}
