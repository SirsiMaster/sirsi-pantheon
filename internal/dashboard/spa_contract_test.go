package dashboard

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
)

// fetchSPA returns the single-page-app HTML, which carries every view's
// JavaScript inline. All views ship in this one document, so this is the whole
// client surface.
func fetchSPA(t *testing.T) string {
	t.Helper()
	srv := testServer(t, Config{})
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(b)
}

// TestGuardView_ReadsDoctorJSONKeysNotGoFieldNames pins the seam that broke:
// the Guard view consumes /api/doctor, which marshals guard.Report through its
// json tags (lowercase). The view read the Go field names instead — so
// rpt.Score was undefined and rpt.Findings fell through `||[]`, discarding
// every diagnostic. The page still rendered, still returned 200, and still
// looked like a working health screen reporting nothing wrong.
//
// Go's compiler cannot catch this: the client is a string literal. So the
// assertion is derived from a real marshaled Report rather than hardcoded —
// rename a json tag and this test fails instead of the UI silently emptying.
func TestGuardView_ReadsDoctorJSONKeysNotGoFieldNames(t *testing.T) {
	raw, err := json.Marshal(guard.DoctorReport{
		Score:    91,
		Findings: []guard.DiagnosticFinding{{Check: "RAM Pressure", Message: "healthy", Severity: 0}},
	})
	if err != nil {
		t.Fatalf("marshal report: %v", err)
	}
	var report map[string]any
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatalf("unmarshal report: %v", err)
	}

	page := fetchSPA(t)

	for _, key := range []string{"score", "findings"} {
		if _, ok := report[key]; !ok {
			t.Fatalf("guard.DoctorReport no longer marshals a %q key — update the Guard view to match", key)
		}
		if !strings.Contains(page, "rpt."+key) {
			t.Errorf("Guard view never reads rpt.%s, but /api/doctor emits it", key)
		}
	}

	findings, _ := report["findings"].([]any)
	if len(findings) != 1 {
		t.Fatalf("expected one marshaled finding, got %d", len(findings))
	}
	finding, _ := findings[0].(map[string]any)
	for _, key := range []string{"check", "message", "severity"} {
		if _, ok := finding[key]; !ok {
			t.Fatalf("guard.DiagnosticFinding no longer marshals a %q key — update the Guard view to match", key)
		}
		if !strings.Contains(page, "f."+key) {
			t.Errorf("Guard view never reads f.%s, but findings carry it", key)
		}
	}

	// The exact defect, stated as an anti-assertion so it cannot come back by
	// someone "restoring" what looks like the Go field name.
	for _, bad := range []string{"rpt.Score", "rpt.Findings", "f.Severity", "f.Check", "f.Message"} {
		if strings.Contains(page, bad) {
			t.Errorf("Guard view reads %q — that is the Go field name; /api/doctor emits lowercase json tags, so this is always undefined", bad)
		}
	}
}

func TestEngineViewDistinguishesLiveCheckedCapabilities(t *testing.T) {
	page := fetchSPA(t)
	for _, want := range []string{
		"connector.contextual_capabilities",
		"live check before use",
		"Selection changes policy only; availability is proved when a session opens.",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("engine view is missing contextual capability contract %q", want)
		}
	}
}

func TestGuardViewProvidesSeveritySummaryAndProgressiveDisclosure(t *testing.T) {
	page := fetchSPA(t)

	for _, want := range []string{
		"guard-summary",
		"Needs attention",
		"Show all '+fs.length+' checks",
		"guard-finding-message",
		"Health score",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("Guard view missing structured summary contract %q", want)
		}
	}
}

func TestFleetViewRendersCanonicalWorkerControlEnvelope(t *testing.T) {
	page := fetchSPA(t)
	for _, want := range []string{
		"fetch('/api/control')",
		"r.status===404",
		"d.schema!=='pantheon.worker-control/v1'",
		"Canonical worker authority — M5",
		"envelope.revision", "const observedAge=ago(envelope.generated_at)||'time unavailable'",
		"timestamp ahead of local clock", "time unavailable",
		"envelope.state_sha256",
		"state.activity",
		"state.fleet",
		"state.threads",
		"state.tasks",
		"state.evidence",
		"item.url",
		"complete envelope",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("Fleet view is missing canonical worker-state rendering %q", want)
		}
	}
	actionAt := strings.Index(page, "renderCanonicalWorkerActionForm(envelope)")
	activityAt := strings.Index(page, "out('ACTIVITY — canonical router events'")
	if actionAt < 0 || activityAt < 0 || actionAt > activityAt {
		t.Fatal("canonical worker actions must appear after the live summary and before long ledger sections")
	}
}

func TestFleetActionComposerCoversCanonicalVerbsAndKeepsLeaseBound(t *testing.T) {
	page := fetchSPA(t)
	for _, want := range []string{
		"message:[['from'", "review_request:[['from'", "delegate:[['agent'", "claim:[['agent'",
		"cancel_handback:[['agent'", "result_return:[['agent'", "fetch('/api/control/action'",
		"receipt.lease||{}", "lease.lease_id", "Matching active lease filled from this page session",
		"const workerControlLeases=Object.create(null)", "function leaseKey(agent,taskID)",
		"function activeLease(lease,expected)", "!Number.isSafeInteger(lease.attempt)",
		"typeof lease.expires!=='string'", "!Number.isFinite(expiry)||expiry<=Date.now()",
		"expected.worker!==undefined", "receipt.task_id!==lease.task_id",
		"thread_id:lease.thread_id,token:lease.lease_id", "attempt:lease.attempt",
		"The stored lease is expired or no longer matches this task.",
		"autocomplete=kind==='password'?'new-password':'off'", "M5 credential stays server-side",
		"pantheon.worker-control-failure/v1", "Failure receipt SHA-256 ",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("Fleet action composer is missing %q", want)
		}
	}
	if strings.Contains(page, "out('Lease token") || strings.Contains(page, "out('lease_id '+lease.lease_id") {
		t.Error("Fleet action composer writes the lease credential into visible output")
	}
}

func TestHomePromptRequiresConfirmedConfiguredRoute(t *testing.T) {
	page := fetchSPA(t)
	for _, want := range []string{
		"let promptRouteReady=false",
		"const selected=(data.connectors||[]).find(function(connector){return connector.kind===data.preferred&&connector.variant===data.preferred_variant})",
		"if(!selected){",
		"Choose a configured SNE, MLX, or oMLX route before submitting a prompt.",
		"Prompt submission is unavailable until Pantheon can confirm a configured route.",
		"function setPromptRouteReady(ready){promptRouteReady=ready;input.disabled=!ready;syncSubmitControl()}",
		"termSubmit.disabled=!input.value.trim()||asking||selectionPending||!promptRouteReady",
		".term-input-bar.home-composer .term-input:disabled{border-color:#c5ceca;background:#e4e7e1;color:#53636a;cursor:not-allowed;opacity:1}",
		"if(!promptRouteReady){out('Choose a configured engine route before submitting a prompt.','t-err');return}",
		"Availability is confirmed when the session opens.",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("Home prompt route gate is missing %q", want)
		}
	}
}

func TestHomeRoutePolicyIgnoresResponsesFromStaleViews(t *testing.T) {
	page := fetchSPA(t)
	request := strings.Index(page, "const routeRequest=++homeRouteRequest")
	fetch := strings.Index(page, "fetch('/api/engine').then(function(r){return readJSONResponse(r,'Engine policy')})")
	fence := "if(currentView!=='home'||routeRequest!==homeRouteRequest)return;"
	firstFence := strings.Index(page, fence)
	lastFence := strings.LastIndex(page, fence)
	if request < 0 || fetch < 0 || firstFence < fetch || lastFence <= firstFence || strings.Count(page, fence) != 2 {
		t.Fatal("engine-policy callbacks are not fenced to the current Home view and request generation")
	}
	if update := strings.Index(page[firstFence:], "routeValue.textContent="); update < 0 {
		t.Fatal("current Home policy response does not update the route view")
	}
	if catch := strings.Index(page, ".catch(function(e){if(currentView!=='home'||routeRequest!==homeRouteRequest)return;"); catch <= firstFence || catch >= lastFence {
		t.Fatal("engine-policy error callback is not protected by the current-view fence")
	}
}

// TestHomeView_CommandsAreClickable keeps every tool reachable while the
// first-question path stays focused on the selected route and prompt.
func TestHomeView_CommandsAreClickable(t *testing.T) {
	page := fetchSPA(t)

	for _, action := range []string{
		"doctor','Check system health", "scan','Scan infrastructure", "guard','Review system controls",
		"ghosts','Find application remnants", "network','Audit network security", "hardware','Inspect hardware",
		"quality','Check code governance", "dedup','Find duplicate files",
	} {
		if !strings.Contains(page, action) {
			t.Errorf("Home tool missing %q", action)
		}
	}
	if !strings.Contains(page, "moreSummary.textContent='Other system tools'") || !strings.Contains(page, "makeHomeAction") {
		t.Error("secondary Home tools are not grouped under the tools disclosure")
	}
	if !strings.Contains(page, "button.type='button'") || !strings.Contains(page, "button.addEventListener('click',function(){input.value='';exec(item[0])})") {
		t.Error("Home actions are not real buttons dispatched through the shared command handler")
	}
	for _, want := range []string{"Work with Sirsi Pantheon", "Ask a question", "home-route", "Change route", "Response and route evidence"} {
		if !strings.Contains(page, want) {
			t.Errorf("home workflow missing %q", want)
		}
	}
	for _, want := range []string{"home-workbench", "View worker state", "home-secondary-action"} {
		if !strings.Contains(page, want) {
			t.Errorf("home action missing %q", want)
		}
	}
	for _, want := range []string{
		"document.createElement('h1')",
		"request.setAttribute('aria-labelledby','home-request-title')",
		"tools.setAttribute('aria-label','Additional Pantheon tools')",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("Home accessible naming contract missing %q", want)
		}
	}
	for _, want := range []string{
		"routeAction.addEventListener('click',function(){switchView('engine')})",
		"fetch('/api/engine')",
		"fetch('/api/ask'",
		"View request and route receipt",
		"completion SHA-256",
		"The current diagnostics did not return a finding for this question.",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("Home live workflow missing %q", want)
		}
	}
	for _, unwanted := range []string{"sampleQuestion", "Use sample question", "home-sample-note"} {
		if strings.Contains(page, unwanted) {
			t.Errorf("Home contains scripted prompt affordance %q", unwanted)
		}
	}
}

func TestHomeViewKeepsOneCanonicalPromptWorkflow(t *testing.T) {
	page := fetchSPA(t)

	for _, want := range []string{
		"routeLabel.textContent='Current route policy'",
		"routeValue.textContent=selected?engineDisplayName(selected):(routeID||'No route selected')",
		"function engineDisplayName(connector){return connector.display_name||engineRouteID(connector)}",
		"const boundary=selected&&selected.data_boundary==='on-device'?'a loopback endpoint'",
		"routeDetail.textContent='Configured route: '+routeID+'. Endpoint boundary: '+boundary+'. Availability is confirmed when the session opens.'+fallbackNote",
		"Fallback is enabled and may change the destination; inspect the receipt.",
		"View worker state",
		"Response and route evidence",
		"routeAction.addEventListener('click',function(){switchView('engine')})",
		"workbench.appendChild(route)",
		"fleet.addEventListener('click',function(){switchView('fleet')})",
		"moreSummary.textContent='Other system tools'",
		"Submitting through the selected route…",
		"identity.engine||'unknown'",
		"receipt.request_sha256",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("Home workflow missing %q", want)
		}
	}
	if strings.Contains(page, "routeLabel.textContent='Session route'") {
		t.Error("Home labels a configured preference as an already-open session route")
	}
	if count := strings.Count(page, "fleet.textContent='View worker state'"); count != 1 {
		t.Errorf("Home defines %d worker-state actions, want exactly one", count)
	}
}

func TestEnginePolicyHTTPFailuresDoNotLeakJSONParserErrors(t *testing.T) {
	page := fetchSPA(t)
	for _, want := range []string{
		"function readJSONResponse(response,label)",
		"label+' request returned HTTP '+response.status",
		"label+' response was not valid JSON'",
		"fetch('/api/engine').then(function(r){return readJSONResponse(r,'Engine policy')})",
		"readJSONResponse(r,'Route change')",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("dashboard response handling is missing %q", want)
		}
	}
}

func TestAskViewExposesBoundRouteAndExecutionIdentity(t *testing.T) {
	page := fetchSPA(t)
	for _, want := range []string{
		"sending your question and this machine’s diagnostic context through the selected route",
		`id="ask-cancel"`,
		"new AbortController()",
		"signal:controller.signal",
		"e.key==='Escape'&&askController",
		"route '+requested+' → '+selected",
		"route.fallback?' · fallback':' · no fallback'",
		"route.data_boundary==='on-device'",
		"configured endpoint: '+actualBoundary",
		"identity.engine||'unknown'",
		"identity.variant||'unknown'",
		"d.receipt.session_id||'unavailable'",
		"identity digest '+(d.receipt.identity_digest||'unavailable')",
		"request SHA-256 '+(d.receipt.request_sha256||'unavailable')",
		"completion SHA-256 '+(d.receipt.completion_sha256||'unavailable')",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("Ask route proof is missing %q", want)
		}
	}
	if strings.Contains(page, "nothing leaves this machine") || strings.Contains(page, "asking the local engine") {
		t.Fatal("prompt UI makes an unverified locality promise")
	}
}

func TestAskSubmissionPreservesDraftWhileRequestIsInFlight(t *testing.T) {
	page := fetchSPA(t)
	guard := "if(asking){out('A question is already running. Your draft remains in the input; cancel or wait before sending it.','t-dim');return}"
	guardAt := strings.Index(page, guard)
	clearAt := strings.Index(page, "const raw=input.value.trim();if(!raw){syncSubmitControl();return}")
	if guardAt < 0 || clearAt < 0 || guardAt > clearAt {
		t.Fatal("submitting during an active question can clear the operator's unsent draft")
	}
}

func TestCommandInputHasExplicitSubmitControlUsingSharedDispatch(t *testing.T) {
	page := fetchSPA(t)
	for _, want := range []string{
		`id="term-submit" aria-label="Submit the question or command" aria-controls="terminal" disabled>Submit</button>`,
		"termSubmit.addEventListener('click',submitCurrentInput)",
		"e.preventDefault();submitCurrentInput();",
		"input.addEventListener('input',function(){syncSubmitControl();fitInput()})",
		"function syncSubmitControl(){termSubmit.disabled=!input.value.trim()||asking||selectionPending||!promptRouteReady}",
		`<textarea class="term-input" id="term-input" aria-label="Ask a question about this machine or enter a Pantheon command"`,
		"if(e.key==='Enter'&&e.shiftKey)return",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("command input Submit control is missing shared behavior %q", want)
		}
	}
}

func TestDashboardDoesNotStealFocusWhenWindowReturns(t *testing.T) {
	page := fetchSPA(t)
	if strings.Contains(page, "window.addEventListener('focus',function(){input.focus()})") {
		t.Fatal("dashboard steals focus from the user's restored control when the browser window regains focus")
	}
	if !strings.Contains(page, "input.focus();") {
		t.Fatal("initial command-field focus was removed")
	}
}

func TestReturningHomeAnnouncesWorkspaceWithoutEnablingLivePromptOutput(t *testing.T) {
	page := fetchSPA(t)
	for _, want := range []string{
		`<div class="sr-only" id="view-announcement" role="status" aria-live="polite" aria-atomic="true"></div>`,
		`if(view==='home')document.getElementById('view-announcement').textContent='Home workspace opened.';`,
		`T.setAttribute('aria-live',view==='home'?'off':'polite');`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("home navigation announcement contract missing %q", want)
		}
	}
}

func TestEngineViewSelectsAndDisplaysExactVariant(t *testing.T) {
	page := fetchSPA(t)
	for _, want := range []string{
		"connector.kind===data.preferred&&connector.variant===data.preferred_variant",
		"function engineRouteID(connector){return connector.kind.toUpperCase()+' · '+connector.variant}",
		"function engineDisplayName(connector){",
		"connector.display_name||engineRouteID(connector)",
		"label.textContent=displayName",
		"card.setAttribute('aria-label',displayName+' route; '+engineRouteID(connector))",
		"'Use '+displayName",
		"choose.setAttribute('aria-pressed',selected?'true':'false')",
		"choose.setAttribute('aria-label',(selected?'Selected ':'Use ')+displayName+' route '+engineRouteID(connector))",
		"status.textContent=(selected?'Preferred policy':'Configured')+' · '+engineRouteID(connector)",
		"connector.data_boundary",
		"Configured endpoint: ",
		"data.allow_fallback?' Fallback is enabled and may change the destination; inspect the receipt.'",
		"selectEngine(connector.kind,connector.variant,choose)",
		"preferred_variant:variant",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("engine variant selection contract missing %q", want)
		}
	}
}

func TestEngineSelectionRefreshStaysOnCurrentView(t *testing.T) {
	page := fetchSPA(t)
	for _, want := range []string{
		"Choose the preferred route for this dashboard.",
		"CLI choices are explicit per invocation.",
		"Dashboard route selected:",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("engine selection scope is not explained accurately: missing %q", want)
		}
	}
	if strings.Contains(page, "Choose the preferred engine once; every surface") {
		t.Fatal("engine selection copy overstates persistence across product surfaces")
	}
	if !strings.Contains(page, "if(currentView==='engine'){\n   switchView('engine');") {
		t.Fatal("successful engine selection does not refresh the policy view in place")
	}
	if !strings.Contains(page, "}else if(currentView==='home'){\n   viewHome();\n  }") {
		t.Fatal("successful engine selection does not refresh Home's route policy when Home is active")
	}
	if strings.Contains(page, "setTimeout(viewEngine,250)") {
		t.Fatal("engine selection can navigate back to Engine after the operator has left")
	}
	for _, want := range []string{
		"let currentView=viewFromLocation();let running=false;let selectionPending=false;let engineViewRequest=0;",
		"const requestID=++engineViewRequest;",
		"if(currentView!=='engine'||requestID!==engineViewRequest)return;",
		"if(currentView==='engine'&&requestID===engineViewRequest)out('Engine selection unavailable: '+e.message,'t-err')",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("Engine view response is not fenced to its current request: missing %q", want)
		}
	}
}

func TestDashboardNavigationSupportsDeepLinksAndBrowserHistory(t *testing.T) {
	page := fetchSPA(t)
	for _, want := range []string{
		`const requested=new URLSearchParams(location.search).get('view');`,
		`return Object.prototype.hasOwnProperty.call(viewLoaders,requested)?requested:'home';`,
		`const changedView=currentView!==view;`,
		`fleet:'Fleet view opened.'`,
		`engine:'Engine view opened.'`,
		`if(changedView)document.getElementById('view-announcement').textContent=viewAnnouncements[view];`,
		`if(changedView)document.getElementById('main-content').focus({preventScroll:true});`,
		`if(currentView==='home')input.focus();`,
		`history.pushState({view:view},'',nextURL);`,
		`window.addEventListener('popstate',function(){switchView(viewFromLocation(),{history:false})});`,
		`if(event.button!==0||event.metaKey||event.ctrlKey||event.shiftKey||event.altKey)return;`,
		`event.preventDefault();switchView(link.dataset.view);`,
		`switchView(currentView,{history:false});`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("dashboard navigation is missing deep-link/history behavior %q", want)
		}
	}
	if strings.Contains(page, `onclick="switchView('fleet')`) {
		t.Fatal("sidebar navigation intercepts modified browser link actions inline")
	}
	if !strings.Contains(page, `location.replace('/?view=%s')`) {
		t.Fatal("legacy view entry points do not land on the selected SPA view")
	}
}

func TestEngineSelectionCompletionDoesNotWriteIntoUnrelatedView(t *testing.T) {
	page := fetchSPA(t)
	selectionAt := strings.Index(page, "function selectEngine(kind,variant,button){")
	if selectionAt < 0 {
		t.Fatal("engine selection handler is missing")
	}
	catchAt := strings.Index(page[selectionAt:], ".catch(function(e){")
	finallyAt := strings.Index(page[selectionAt:], ".finally(function(){")
	if catchAt < 0 || finallyAt <= catchAt {
		t.Fatal("engine selection response handlers are missing or out of order")
	}
	success := page[selectionAt+strings.Index(page[selectionAt:], ".then(function(snapshot){") : selectionAt+catchAt]
	failure := page[selectionAt+catchAt : selectionAt+finallyAt]
	for name, handler := range map[string]string{"success": success, "failure": failure} {
		engineBranch := strings.Index(handler, "if(currentView==='engine'){\n")
		homeBranch := strings.Index(handler, "}else if(currentView==='home'){\n   viewHome();\n  }")
		if engineBranch < 0 || homeBranch < engineBranch {
			t.Errorf("%s completion does not fence Engine effects and reconcile Home policy", name)
			continue
		}
		if strings.Contains(handler[homeBranch+1:], "out(") || strings.Contains(handler[homeBranch+1:], "appendEnginePromptAction()") {
			t.Errorf("%s completion writes Engine feedback into a non-Engine view", name)
		}
	}
	engineSuccessEnd := strings.Index(success, "}else if(currentView==='home')")
	if engineSuccessEnd < 0 || !strings.Contains(success[:engineSuccessEnd], "appendEnginePromptAction();") {
		t.Fatal("successful route selection no longer offers the Engine follow-through in the Engine view")
	}
}

func TestEngineSelectionOffersExplicitPromptFollowThrough(t *testing.T) {
	page := fetchSPA(t)
	for _, want := range []string{
		"Live availability is checked when a session opens.",
		"function appendEnginePromptAction(){",
		"button.textContent='Ask a question with this route'",
		"button.addEventListener('click',function(){switchView('home');input.focus()})",
		"row.appendChild(button);T.appendChild(row);button.focus();T.scrollTop=T.scrollHeight;",
		"appendEnginePromptAction();",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("engine selection follow-through is missing %q", want)
		}
	}
}

func TestEngineSelectionIsSingleFlightAndRequiresCanonicalReadback(t *testing.T) {
	page := fetchSPA(t)
	for _, want := range []string{
		"let selectionPending=false",
		"if(selectionPending)return;",
		"selectionPending=true;syncSubmitControl();",
		"document.querySelectorAll('.engine-select').forEach(function(control){control.disabled=true});",
		"button.setAttribute('aria-busy','true')",
		"snapshot.preferred!==kind||snapshot.preferred_variant!==variant",
		"Engine selection rejected: '+e.message",
		"if(currentView==='engine'){",
		".finally(function(){selectionPending=false;syncSubmitControl()});",
		"function syncSubmitControl(){termSubmit.disabled=!input.value.trim()||asking||selectionPending||!promptRouteReady}",
		"if(selectionPending){out('Wait for the dashboard route change to finish before submitting.','t-dim');return}",
		"selectionPending=false;syncSubmitControl();\n  if(currentView==='engine'){",
		"body:JSON.stringify({preferred:kind,preferred_variant:variant})",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("engine selection feedback contract is missing %q", want)
		}
	}

	guardAt := strings.Index(page, "if(selectionPending)return;")
	startAt := strings.Index(page, "selectionPending=true;")
	selectionAt := strings.Index(page, "function selectEngine(kind,variant,button){")
	if selectionAt < 0 {
		t.Fatal("engine selection handler is missing")
	}
	requestOffset := strings.Index(page[selectionAt:], "fetch('/api/engine/select'")
	requestAt := -1
	if requestOffset >= 0 {
		requestAt = selectionAt + requestOffset
	}
	if guardAt < 0 || startAt < 0 || requestAt < 0 || !(guardAt < startAt && startAt < requestAt) {
		t.Fatal("engine selection must claim its single-flight state before reading or mutating canonical policy")
	}
	settleBeforeRefresh := "selectionPending=false;syncSubmitControl();\n  if(currentView==='engine'){"
	if got := strings.Count(page[selectionAt:], settleBeforeRefresh); got != 2 {
		t.Fatalf("selection success/failure refreshes occur before pending state clears %d times, want 2", got)
	}
	selectEndOffset := strings.Index(page[selectionAt:], "\n}\n\nfunction viewSNE")
	if selectEndOffset < 0 {
		t.Fatal("engine selection handler boundary is missing")
	}
	if strings.Contains(page[selectionAt:selectionAt+selectEndOffset], "fetch('/api/engine')") {
		t.Fatal("route selection must not use a stale client-side read/modify/write policy update")
	}
}

func TestSidebarNavigationIsLabelFirst(t *testing.T) {
	page := fetchSPA(t)

	for _, label := range []string{"Home", "Fleet", "Scan", "Ghosts", "Guard", "Notifications", "Code Graph", "Vault", "Engine", "SNE", "Recovery", "Ra"} {
		if !strings.Contains(page, `<span class="nav-label">`+label+`</span>`) {
			t.Errorf("sidebar is missing the readable %q label", label)
		}
	}
	if strings.Contains(page, `class="nav-glyph"`) {
		t.Error("sidebar still relies on decorative glyph spans instead of readable labels")
	}
	for _, want := range []string{
		"out('Code Graph','t-gold')",
		"graph:'horus',horus:'horus'",
		"raw.startsWith('graph ')",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("Code Graph navigation/command contract missing %q", want)
		}
	}
	for _, want := range []string{
		`<nav class="sidebar-nav" aria-label="Primary navigation">`,
		`<a class="skip-link" href="#main-content">Skip to main content</a>`,
		`<main class="main" id="main-content" tabindex="-1">`,
		`.skip-link:focus{top:12px`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("dashboard keyboard/screen-reader landmark missing %q", want)
		}
	}
}
