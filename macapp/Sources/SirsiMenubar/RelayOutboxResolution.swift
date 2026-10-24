import SwiftUI

// Navigation evidence is not consent. Keep the selected row and its root scope,
// not a new lookup by agent that could silently retarget a different spool.
struct RelayOutboxResolutionContext {
    let outbox: RBOutbox
    let sourceTitle = "Ra Fabric"
    var returnTitle: String { "Back to \(sourceTitle)" }
    var location: String {
        guard let root = outbox.scope?.canonicalSpool else {
            return "Exact outbox location unavailable in the selected observation"
        }
        return "\(root)/\(outbox.agent)/outbox"
    }
}

struct RelayOutboxResolutionProjection {
    enum State: Equatable {
        case refreshRequired, scopeChanged, missing, serviceManaged, repairAvailable, observed, unavailable
    }
    let state: State
    let observation: String
    let guidance: String

    @MainActor
    init(context: RelayOutboxResolutionContext, engine: SirsiEngine,
         freshPreflight: Bool, now: Date = Date()) {
        let selected = context.outbox
        observation = selected.observationText(at: now)
        guard freshPreflight, engine.routerObservationIsCurrent(at: now) else {
            state = .refreshRequired
            guidance = engine.routerReadError ?? "Read fresh relay metadata before reviewing a repair. The selected Ra observation is retained context, not authorization."
            return
        }
        let rows = (engine.routerBoard?.outbox ?? []).filter { $0.agent == selected.agent }
        guard rows.count == 1, let current = rows.first else {
            state = .missing
            guidance = "The selected outbox is missing or ambiguous in the fresh read. Return to Ra and select its current observation; absence from this inventory does not mean an empty queue."
            return
        }
        guard current.scope == selected.scope else {
            state = .scopeChanged
            guidance = "The selected spool/root identity changed. No repair is offered for a different scope. Return to Ra, refresh and review the current outbox."
            return
        }
        if current.observerAccess == "service-private" {
            state = .serviceManaged
            guidance = current.serviceObservationIsCurrent(at: now)
                ? "Current relay-producer metadata is available; payload access remains private. Retry counts do not prove delivery. Re-observe here; no permission repair is needed for private service ownership."
                : "The relay owner must qualify/update this selected service through its supported lifecycle so it can publish current health metadata. Re-observe after that update. This guide cannot restart the service, widen access or infer zero retries."
        } else if engine.relayOutboxRepairApproval(for: selected) != nil {
            state = .repairAvailable
            guidance = "A same-user metadata preflight offers a bounded repair for this exact agent/root. Review and confirm it below; queued messages will not be removed or replayed."
        } else if current.observerAccess == "observed", current.unreadable == false,
                  current.retryIsCurrent(at: now) {
            state = .observed
            guidance = "Fresh direct metadata is readable. No repair is offered. This observation does not prove delivery or that an earlier action caused recovery."
        } else {
            state = .unavailable
            guidance = "No supported same-user repair is available for this scope. Re-observe, or ask the selected relay's operator to inspect its lifecycle and metadata policy. Pantheon will not change ownership, ACLs or Full Disk Access."
        }
    }
}

@MainActor
final class RelayOutboxResolutionModel: ObservableObject {
    let context: RelayOutboxResolutionContext
    @Published private(set) var reading = false
    @Published private(set) var applying = false
    @Published private(set) var freshPreflight = false
    @Published private(set) var approval: RelayOutboxRepairApproval?
    @Published private(set) var operationResult: String?
    private var generation = 0

    init(context: RelayOutboxResolutionContext) { self.context = context }

    func cancelReview() {
        generation += 1
        approval = nil
        freshPreflight = false
        // An already confirmed engine invocation is not claimed stopped here.
    }

    func refresh(engine: SirsiEngine, review: Bool = false,
                 read: (() async -> Bool)? = nil) async {
        guard !reading, !applying else { return }
        generation += 1
        let token = generation
        let started = Date()
        approval = nil
        freshPreflight = false
        reading = true
        defer { reading = false }
        // The returned global summary concerns all private-service rows. This
        // guide projects the selected row, not that aggregate success string.
        var readOK = false
        _ = await engine.refreshRelayOutboxObservation(refresh: {
            if let read { readOK = await read() }
            else { readOK = await engine.loadRouterBoard(force: true) }
            return readOK
        })
        guard token == generation, !Task.isCancelled else { return }
        freshPreflight = readOK && engine.routerReadError == nil &&
            engine.routerReadAt.map { $0 >= started } == true && engine.routerObservationIsCurrent(at: Date())
        if review, freshPreflight {
            approval = engine.relayOutboxRepairApproval(for: context.outbox)
        }
        // Re-observation never clears/replaces an operation receipt.
    }

    func confirm(engine: SirsiEngine,
                 execute: (([String]) async -> CommandExecution)? = nil,
                 readback: (() async -> Bool)? = nil,
                 record: ((String) -> Void)? = nil) -> Task<Void, Never>? {
        guard !applying, !reading, !Task.isCancelled, let frozen = approval else { return nil }
        // Consume synchronously in the button action, before SwiftUI dismisses
        // the dialog or a second click can schedule another invocation.
        approval = nil
        applying = true
        freshPreflight = false
        // Engine independently checks the offer again before capture/launch and
        // retains typed exit + readback honesty. A new repair needs new review.
        return Task {
            defer { applying = false }
            operationResult = await engine.repairRelayOutbox(approval: frozen, execute: execute,
                                                           refresh: readback, record: record)
        }
    }
}

struct RelayOutboxResolutionView: View {
    @ObservedObject var engine: SirsiEngine
    @EnvironmentObject private var nav: Nav
    @StateObject private var model: RelayOutboxResolutionModel

    init(engine: SirsiEngine, context: RelayOutboxResolutionContext) {
        self.engine = engine
        _model = StateObject(wrappedValue: RelayOutboxResolutionModel(context: context))
    }

    var body: some View {
        let context = model.context
        let projection = RelayOutboxResolutionProjection(context: context, engine: engine,
                                                         freshPreflight: model.freshPreflight)
        VStack(alignment: .leading, spacing: 0) {
            MaybeScroll {
                VStack(alignment: .leading, spacing: 14) {
                    Text("Resolve relay observation").sirsiFont(.title2, weight: .semibold)
                    Text("Selected in \(context.sourceTitle): \(context.outbox.agent)")
                        .sirsiFont(.subheadline, weight: .semibold)
                    Text(context.location).sirsiFont(.caption, design: .monospaced)
                        .textSelection(.enabled).fixedSize(horizontal: false, vertical: true)
                    if let scope = context.outbox.scope {
                        Text("Selected root identity: \(scope.rootDevice):\(scope.rootInode)")
                            .sirsiFont(.caption).foregroundStyle(PantheonTheme.mutedText)
                    }
                    Text("Selected observation · \(projection.observation)")
                        .sirsiFont(.caption).foregroundStyle(PantheonTheme.mutedText)
                        .fixedSize(horizontal: false, vertical: true)
                    if let error = context.outbox.error {
                        Text(error).sirsiFont(.caption).textSelection(.enabled)
                            .fixedSize(horizontal: false, vertical: true)
                    }
                    Divider()
                    if model.reading {
                        ProgressView("Reading selected relay metadata…")
                    } else if model.applying {
                        ProgressView("Repairing approved scope and re-observing…")
                    }
                    Text(projection.guidance).sirsiFont(.subheadline)
                        .fixedSize(horizontal: false, vertical: true)
                    if model.freshPreflight,
                       let row = engine.routerBoard?.outbox?.first(where: { $0.agent == context.outbox.agent }) {
                        Text("Latest read · \(row.observationText(at: Date()))")
                            .sirsiFont(.caption).foregroundStyle(PantheonTheme.mutedText)
                            .fixedSize(horizontal: false, vertical: true)
                    }
                    if projection.state == .repairAvailable {
                        Button("Review repair for \(context.outbox.agent)") {
                            Task { await model.refresh(engine: engine, review: true) }
                        }.buttonStyle(.borderedProminent).tint(gold)
                            .disabled(model.reading || model.applying || engine.busy)
                    }
                    Button("Re-observe selected outbox") {
                        Task { await model.refresh(engine: engine) }
                    }.buttonStyle(.bordered).disabled(model.reading || model.applying || engine.routerLoading || engine.busy)
                    if let result = model.operationResult {
                        Divider()
                        Text("Last confirmed operation").sirsiFont(.subheadline, weight: .semibold)
                        Text(result).sirsiFont(.caption).textSelection(.enabled)
                            .fixedSize(horizontal: false, vertical: true)
                    }
                    Text("No queued payload is opened, removed or replayed. App inventory and this guide do not grant repair authority. Unsupported service changes remain operator-managed.")
                        .sirsiFont(.caption).foregroundStyle(PantheonTheme.mutedText)
                        .fixedSize(horizontal: false, vertical: true)
                }.padding(16).frame(maxWidth: .infinity, alignment: .leading)
            }
            Divider()
            Button(context.returnTitle) { model.cancelReview(); nav.pop() }
                .buttonStyle(.bordered).padding(12)
        }
        .task { await model.refresh(engine: engine) }
        .onDisappear { model.cancelReview() }
        .confirmationDialog("Repair selected relay outbox?", isPresented: Binding(
            get: { model.approval != nil },
            set: { if !$0 { model.cancelReview() } }
        ), titleVisibility: .visible) {
            Button("Confirm repair for \(model.context.outbox.agent)") {
                _ = model.confirm(engine: engine)
            }
            Button("Cancel", role: .cancel) { model.cancelReview() }
        } message: {
            if let approval = model.approval {
                Text("Restore private mode for \(approval.agent) under \(approval.scope.canonicalSpool), root \(approval.scope.rootDevice):\(approval.scope.rootInode). Changed scope is refused. Messages are not changed; success requires a fresh same-scope readback.")
            }
        }
    }
}
