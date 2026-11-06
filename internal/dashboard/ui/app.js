/* Horus dashboard — reads /api/router, /api/fabric, /api/stats. No framework, no build step.
   Views render through PantheonLedger (ledger.js) and PantheonFabric (fabric.js). */
(function(){
'use strict';
var $=function(s,r){return (r||document).querySelector(s)};
var esc=function(s){return String(s==null?'':s).replace(/[&<>"']/g,function(c){return {'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]})};
var ICON={
 overview:'<svg viewBox="0 0 24 24"><rect x="3" y="3" width="7" height="9" rx="2"/><rect x="14" y="3" width="7" height="5" rx="2"/><rect x="14" y="12" width="7" height="9" rx="2"/><rect x="3" y="16" width="7" height="5" rx="2"/></svg>',
 lanes:'<svg viewBox="0 0 24 24"><path d="M4 6h16M4 12h16M4 18h16"/><circle cx="8" cy="6" r="1.4"/><circle cx="15" cy="12" r="1.4"/><circle cx="10" cy="18" r="1.4"/></svg>',
 queue:'<svg viewBox="0 0 24 24"><path d="M4 7l8-4 8 4-8 4-8-4z"/><path d="M4 12l8 4 8-4"/><path d="M4 17l8 4 8-4"/></svg>',
 releases:'<svg viewBox="0 0 24 24"><path d="M12 3v12"/><path d="M7 10l5 5 5-5"/><path d="M5 20h14"/></svg>',
 failures:'<svg viewBox="0 0 24 24"><path d="M12 3l9 16H3L12 3z"/><path d="M12 10v4"/><circle cx="12" cy="17" r=".6"/></svg>',
 fleet:'<svg viewBox="0 0 24 24"><circle cx="6" cy="7" r="2.5"/><circle cx="18" cy="7" r="2.5"/><circle cx="12" cy="17" r="2.5"/><path d="M8 8.5l3 6M16 8.5l-3 6M8.5 7h7"/></svg>',
 host:'<svg viewBox="0 0 24 24"><rect x="3" y="4" width="18" height="12" rx="2"/><path d="M8 20h8M12 16v4"/></svg>'
};
function plural(n,one,many){return n+' '+(n===1?one:many)}
var VIEWS=[['overview','Overview','The whole organization at a glance'],['lanes','Lanes','Can each lane work right now'],['queue','Inbox','Open items grouped by recipient'],['releases','Releases','What each release added'],['failures','Known failures','Registered once, fixed with a guard'],['fleet','Fleet','Work in flight across every lane'],['host','Host','Memory, swap and this Mac']];
var POLL=10000;
var state={view:'overview',laneAgent:null,data:null,fleet:null,stats:null,err:null,at:null,recipient:'*',lane:{filter:'ALL',q:'',sort:'agent',dir:1},timer:null,ledgers:{},jobs:{},flash:{},gen:0,dataAt:0};
VIEWS.splice(VIEWS.length-1,0,['mercury','Mercury','Observed interconnect activity — read only']);
ICON.mercury='<svg viewBox="0 0 24 24"><path d="M3 8h18l-4-4M21 16H3l4 4"/></svg>';
var stopMercury=null;

function get(u){return fetch(u,{cache:'no-store',signal:AbortSignal.timeout(15000)}).then(function(r){return r.json().then(function(j){if(!r.ok){var e=new Error(j.error||('HTTP '+r.status));e.status=r.status;throw e}return j})})}
function arrived(){state.gen++;state.dataAt=Date.now()}
// A row that changed since the previous poll flashes for 3 s. The answer is fixed per data generation, so
// re-rendering for any other reason cannot make an unchanged row flash or lose a flash early.
function changed(scope,key,sig){
 var s=state.flash[scope]||(state.flash[scope]={gen:-1,prev:{},cur:{},primed:false});
 if(s.gen!==state.gen){s.prev=s.cur;s.primed=s.gen!==-1;s.cur={};s.gen=state.gen}
 if(sig===undefined)sig=key;
 s.cur[key]=sig;
 return s.primed&&Date.now()-state.dataAt<3000&&(!Object.prototype.hasOwnProperty.call(s.prev,key)||s.prev[key]!==sig);
}
function load(){
 if(state.loading)return state.loading;
 if(!state.at&&!state.err)setLive('Loading…','stale');
 // The snapshot already binds one canonical Fabric generation. Reuse it only
 // for the default board; an explicit filter/cursor remains its own read.
 var useBoundFabric=!state.fabricTaskPage,generation=null;
 if(useBoundFabric){generation=(state.fabricReadGeneration||0)+1;state.fabricReadGeneration=generation}
 function currentDefault(){return useBoundFabric&&generation===state.fabricReadGeneration&&!state.fabricTaskPage}
 function receiveBoundFabric(d){
  if(!currentDefault())return;
  var f=d&&d.fabric,p=PantheonFabric.taskPage(f);
  if(PantheonFabric.valid(f)&&(!f.tasks||p&&p.offset===0&&(!p.filter||p.filter==='open'))){state.fleet=f;state.fleetErr=null;return}
  // Older/malformed snapshot producers must not erase the independent Fleet
  // recovery path or substitute a filtered/continued page for the default.
  return loadFleet(null);
 }
 var router=get('/api/router/v1/snapshot').then(function(d){state.data=d;state.err=null;state.at=new Date();arrived();return receiveBoundFabric(d)}).catch(function(e){state.err=e.message;if(currentDefault())return loadFleet(null)}).then(function(){render();paintLive()});
 var host=get('/api/stats').then(function(d){state.stats=d;state.hostErr=null}).catch(function(e){state.hostErr=e.message}).then(function(){if(state.view==='host')render()});
 var fleet=useBoundFabric?null:loadFleet();
 var lane=state.view==='lane'&&state.laneAgent?Promise.all([loadLane(state.laneAgent),loadJobs(state.laneAgent)]):null;
 state.loading=Promise.allSettled([router,host,fleet,lane]).finally(function(){state.loading=null});return state.loading;
}
function loadFleet(taskPage,focusTasks){var selected=taskPage===undefined?state.fabricTaskPage:taskPage,url='/api/fabric',query=[],generation=(state.fabricReadGeneration||0)+1;state.fabricReadGeneration=generation;state.fabricTaskPage=selected||null;if(selected&&selected.filter)query.push('tasks_filter='+encodeURIComponent(selected.filter));if(selected&&selected.offset){query.push('tasks_offset='+encodeURIComponent(selected.offset));query.push('tasks_snapshot='+encodeURIComponent(selected.snapshot))}if(query.length)url+='?'+query.join('&');return get(url).then(function(f){if(generation!==state.fabricReadGeneration)return;if(!PantheonFabric.valid(f)||selected&&selected.filter&&(!PantheonFabric.taskPage(f)||f.tasks.filter!==selected.filter))throw new Error('Invalid fabric projection or task selection');state.fleet=f;state.fleetErr=null;arrived()}).catch(function(e){if(generation===state.fabricReadGeneration)state.fleetErr=e.message}).then(function(){if(generation===state.fabricReadGeneration&&(state.view==='fleet'||state.view==='overview'||state.view==='host')){render();if(focusTasks&&!state.fleetErr&&state.view==='fleet'){var heading=document.getElementById('fabric-task-heading');if(heading){heading.focus({preventScroll:true});if(heading.scrollIntoView)heading.scrollIntoView({block:'start'})}}}})}
// One lane's ledger. 404 means the router does not know the lane; any other failure keeps the last good
// ledger on screen and says so. Neither is ever drawn as an empty ledger.
function loadLane(agent){
 var L=state.ledgers[agent]||(state.ledgers[agent]={});
 if(L.busy)return L.busy;
 L.busy=get('/api/router/v1/ledger?agent='+encodeURIComponent(agent)).then(function(d){L.data=d;L.err=null;L.notFound=false;L.at=Date.now();L.staleShown=false;arrived()},function(e){if(e.status===404){L.notFound=true;L.err=null;L.data=null}else L.err=e.message}).then(function(){L.busy=null;if(state.view==='lane'&&state.laneAgent===agent)render()});
 return L.busy;
}
// The cloud-jobs slot. There is no producer yet: the server answers 404 "no source connected" and the
// panel says exactly that. Any other failure is a read error, shown as one.
function loadJobs(agent){
 var J=state.jobs[agent]||(state.jobs[agent]={state:'loading'});
 return get('/api/router/v1/jobs?agent='+encodeURIComponent(agent)).then(function(d){J.state='ok';J.data=Array.isArray(d)?d:[];J.err=null},function(e){if(e.status===404&&/no source connected/.test(e.message)){J.state='none';J.err=null}else{J.state='error';J.err=e.message}}).then(function(){if(state.view==='lane'&&state.laneAgent===agent)render()});
}
function setLive(t,cls){$('#live-t').textContent=t;$('#live .dot').className='dot'+(cls?' '+cls:'')}
function paintLive(){var s=PantheonLedger.status({at:state.at?state.at.getTime():0,err:state.err,now:Date.now()});setLive(s.text,s.cls)}

function route(h){
 if(h.indexOf('lane/')===0){var a;try{a=decodeURIComponent(h.slice(5))}catch(_){return null}return a?{view:'lane',agent:a}:null}
 return VIEWS.some(function(v){return v[0]===h})?{view:h,agent:null}:null;
}
function nav(){
 var n=(state.data&&state.data.attention||[]).filter(function(a){return a.severity==='critical'}).length,cur=state.view==='lane'?'lanes':state.view;
 $('#nav').innerHTML='<div class="nav-label">Router</div>'+VIEWS.map(function(v){
  return '<button type="button" data-v="'+v[0]+'"'+(cur===v[0]?' aria-current="page"':'')+'>'+ICON[v[0]]+'<span>'+v[1]+'</span>'+(v[0]==='overview'&&n?'<span class="count" aria-label="'+n+' critical">'+n+'</span>':'')+'</button>'}).join('');
 $('#nav').querySelectorAll('button').forEach(function(b){b.addEventListener('click',function(){go(b.dataset.v)})});
}
function go(v){
 var dlg=$('.inspector');if(dlg&&dlg.open)dlg.close();
 var r=route(v)||{view:v,agent:null};state.view=r.view;state.laneAgent=r.agent;location.hash=v;
 if(v==='fleet'&&Number.isFinite(nextPoll))nextPoll=Math.min(nextPoll,Date.now()+POLL);
 render();
 if(v==='fleet'&&!state.fleet&&!state.loading)loadFleet();
 if(r.view==='lane'){loadLane(r.agent);loadJobs(r.agent)}
 $('#main').focus({preventScroll:true});
}

function skeleton(){return '<p class="sub loading">Loading the local snapshot… Other views remain available.</p>'+ '<div class="grid g4"><div class="skel"></div><div class="skel"></div><div class="skel"></div><div class="skel"></div></div>'}
function chip(v){return '<span class="chip v-'+esc(v)+'">'+esc(v.replace('_',' ').toLowerCase().replace(/^./,function(c){return c.toUpperCase()}))+'</span>'}
var ORDER=['LIVE','WAKEABLE','HELD','WATCH_ONLY','AUTH_REQUIRED','UNREACHABLE','UNSTAFFED'];

function vOverview(d){
 var counts=(d.lanes||{}).counts||{};
 return PantheonLedger.overview(d,PantheonFabric.valid(state.fleet)?state.fleet:null,{now:Date.now(),changed:changed})+
  '<footer class="fabric-provenance"><span>Service reports '+esc(counts.LIVE??'unknown')+' live · '+esc(counts.WAKEABLE??'unknown')+' wakeable · '+esc(counts.HELD??'unknown')+' held lanes</span><span>Received '+esc(d.generated_at||'time unavailable')+'</span><span>'+esc(d.version||'Build unavailable')+' · '+esc(d.built_ms??'unknown')+' ms collection</span><span>Read-only · no competing local store</span></footer>';
}
function rel(t){if(!t)return '—';var m=Math.round((Date.now()-new Date(t).getTime())/60000);return m<1?'just now':m<60?m+' min ago':m<1440?Math.round(m/60)+' h ago':Math.round(m/1440)+' d ago'}

function vLanes(d){
 var L=state.lane,list=(d.lanes.list||[]).filter(function(l){return (L.filter==='ALL'||l.verdict===L.filter)&&(!L.q||(l.agent+' '+l.detail).toLowerCase().indexOf(L.q)>-1)});
 list.sort(function(a,b){var x=a[L.sort],y=b[L.sort];return (typeof x==='number'?x-y:String(x).localeCompare(String(y)))*L.dir});
 var counts=d.lanes.counts,keys=['ALL'].concat(ORDER.filter(function(k){return counts[k]}));
 var h='<div class="toolbar"><div class="seg" role="group" aria-label="Filter">'+keys.map(function(k){return '<button type="button" data-f="'+k+'" aria-pressed="'+(L.filter===k)+'">'+(k==='ALL'?'All '+(d.lanes.list||[]).length:k.replace('_',' ').toLowerCase()+' '+counts[k])+'</button>'}).join('')+'</div><input class="search" id="lq" type="search" placeholder="Search lanes" aria-label="Search lanes" value="'+esc(L.q)+'"></div>';
 h+='<table class="table responsive-table"><thead><tr><th><button class="sort" data-s="agent">Lane ↕</button></th><th><button class="sort" data-s="verdict">Status ↕</button></th><th class="num"><button class="sort" data-s="open">Open ↕</button></th><th>Detail</th></tr></thead><tbody>'+
  (list.length?list.map(function(l){return '<tr><td data-label="Lane" class="mono"><button class="lane-select" data-inspect="'+esc(l.agent)+'">'+esc(l.agent)+'</button>'+'</td><td data-label="Status">'+chip(l.verdict)+'</td><td data-label="Open items" class="num">'+(l.open??'Unknown')+'</td><td data-label="Detail" class="dim">'+esc(l.detail)+'</td></tr>'}).join(''):'<tr><td colspan="4" class="empty">No lanes match.</td></tr>')+'</tbody></table>';
 return h;
}
function bindLanes(){
 document.querySelectorAll('[data-f]').forEach(function(b){b.addEventListener('click',function(){state.lane.filter=b.dataset.f;render()})});
 document.querySelectorAll('[data-s]').forEach(function(t){t.parentElement.setAttribute('aria-sort',state.lane.sort===t.dataset.s?(state.lane.dir===1?'ascending':'descending'):'none');t.addEventListener('click',function(){var L=state.lane;if(L.sort===t.dataset.s)L.dir*=-1;else{L.sort=t.dataset.s;L.dir=1}render()})});
 var q=$('#lq');if(q)q.addEventListener('input',function(){state.lane.q=q.value.toLowerCase();var p=q.selectionStart;render();var n=$('#lq');n.focus();n.setSelectionRange(p,p)});
}
function vQueue(d){
 var q=d.queue||[],items=d.items,all=q.reduce(function(n,x){return n+x.open},0);
 var h='<div class="section-heading"><h2>Open messages</h2><button class="btn" data-queue="*">All recipients · '+all+'</button></div><div class="toolbar">'+q.map(function(x){return '<button class="btn" data-queue="'+esc(x.agent)+'" aria-pressed="'+(state.recipient===x.agent)+'">'+esc(x.agent)+' · '+x.open+'</button>'}).join('')+'</div>';
 if(!Array.isArray(items))return h+'<p class="empty">This producer does not provide message details. Refresh after the dashboard upgrade; counts are not a message list.</p>';
 if(items.length!==all)return h+'<p role="alert" class="banner b-danger">Snapshot mismatch: '+all+' counted items but '+items.length+' records. Refresh before relying on this queue.</p>';
 var rows=items.filter(function(x){return state.recipient==='*'||x.to===state.recipient});
 h+='<p class="sub">Showing '+rows.length+' of '+all+' open items'+(state.recipient==='*'?'':' for '+esc(state.recipient))+'. Oldest first. Received '+esc(d.generated_at)+'.</p>';
 return h+(rows.length?PantheonLedger.inbox(rows,{now:Date.now(),changed:changed}):'<p class="empty">No open items for this selection.</p>');
}
function itemInspector(id){var x=(state.data.items||[]).find(function(x){return x.id===id});if(!x)return;showInspector(x.title||x.id,'<p class="mono">'+esc(x.id)+'</p><p>'+esc(x.from)+' → '+esc(x.to)+'</p><p class="sub">Opened '+esc(x.opened)+' · '+esc(x.type||'Request')+'</p><h3>Request</h3><pre class="message-body">'+esc(x.instructions||'No request body supplied.')+'</pre>',true)}
function vLane(){
 var a=state.laneAgent;
 return PantheonLedger.laneLedger({agent:a,ledger:state.ledgers[a]||{},jobs:state.jobs[a],now:Date.now(),changed:changed});
}
function vReleases(d){return '<div class="card">'+(d.releases||[]).map(function(x,i){return '<details class="release" data-release="'+i+'"><summary><strong>'+esc(x.version)+'</strong><span class="sub">'+esc(x.date||'Date not supplied')+' · '+plural((x.items||[]).length,'entry','entries')+'</span><p>'+esc((x.items||[])[0]||'No changes recorded for this release.')+'</p></summary><ul>'+(x.items||[]).map(function(t){return '<li>'+esc(t)+'</li>'}).join('')+'</ul></details>'}).join('')+'</div>'}
function vFailures(d){
 var k=d.known_failures||[];
 return '<p class="sub" style="margin-bottom:16px">A recurring failure is registered once with its signature and cause. The fix records the release it shipped in and a regression test that must exist. The wake loop recognizes the failure next time instead of waiting for a person.</p><table class="table responsive-table"><thead><tr><th>Failure</th><th>Status</th><th>Fixed in</th><th>Regression guard</th></tr></thead><tbody>'+k.map(function(x){return '<tr><td data-label="Failure"><button class="lane-select" data-failure="'+esc(x.id)+'">'+esc(x.id)+'</button><div class="dim">'+esc(x.title)+'</div></td><td data-label="Status"><span class="chip st-'+esc(x.status)+'">'+esc(x.status)+'</span></td><td data-label="Fixed in" class="mono">'+esc(x.fixed_in||'—')+'</td><td data-label="Regression guard" class="mono">'+esc(x.guard||'—')+'</td></tr>'}).join('')+'</tbody></table>';
}
function fabricReceipt(){return PantheonFabric.receipt(state.fleet,state.fleetErr,Date.now(),esc)}
function fabricHeadlines(){return fabricReceipt()+PantheonFabric.headlines(state.fleet,esc)}
function bindFabricHeadlines(el){PantheonFabric.bindHeadlineControls(el,state.fleet,function(filter){go('fleet');loadFleet({filter:filter},true)},function(){state.recipient='*';go('queue')})}
function bindFabricLanes(el){PantheonFabric.bindLaneInspectors(el,state.fleet,fabricInspector);PantheonFabric.bindTaskInspectors(el,state.fleet,fabricTaskInspector,function(page){loadFleet(page,true)});bindFabricHeadlines(el)}
function ageFabric(){var now=Date.now();document.querySelectorAll('.fabric-receipt').forEach(function(node){var f=PantheonFabric.freshness(state.fleet,state.fleetErr,now);node.textContent=f.text;node.dataset.fabricState=f.state});document.querySelectorAll('.fabric-recent-change').forEach(function(node){var eventAge=now-Date.parse(node.dataset.fabricAt),observationAge=now-Date.parse(node.dataset.fabricObserved);if(state.fleetErr||!Number.isFinite(eventAge)||eventAge<0||eventAge>30000||!Number.isFinite(observationAge)||observationAge<0||observationAge>30000){var row=node.closest('tr');if(row)row.classList.remove('fabric-recent-row','changed');node.remove()}})}
function vFleet(){
 if(state.fleetErr&&!state.fleet)return PantheonLedger.banner('danger','Fabric unavailable.',state.fleetErr,true);
 return fabricReceipt()+PantheonFabric.render(state.fleet,esc,Date.now(),!state.fleetErr);
}
function memoryDiagnostics(){
 var m=state.memory,err=state.memoryErr,busy=state.memoryBusy;
 var body=err?'<p class="stale-banner banner b-warn" role="status">Memory diagnostics unavailable · '+esc(err)+'. Try Inspect memory again.</p>':'';
 if(m){
  body+='<p class="sub">'+(err?'Previous read · ':'Read at ')+esc(state.memoryAt)+'</p>';
  body+=m.top.length?'<table class="table responsive-table"><thead><tr><th>Process</th><th>PID</th><th>Memory</th></tr></thead><tbody>'+m.top.map(function(p){return '<tr><td data-label="Process">'+esc(p.name)+'</td><td data-label="PID" class="mono">'+esc(p.pid)+'</td><td data-label="Memory">'+(Number.isFinite(p.rss_bytes)?esc((p.rss_bytes/1048576).toFixed(1))+' MiB':'—')+'</td></tr>'}).join('')+'</tbody></table>':'<p class="sub">No ranked process data was returned. This does not establish that no processes are running.</p>';
 }else if(!err){body='<p class="sub">Inspect the largest sampled memory users. This read runs only when requested.</p>'}
 return '<section class="card" aria-labelledby="memory-users-title"><h2 id="memory-users-title">Memory users</h2><button type="button" class="btn" data-memory="inspect"'+(busy?' disabled':'')+'>'+(busy?'Reading memory…':'Inspect memory')+'</button><div aria-live="polite">'+body+'</div></section>';
}
async function inspectMemory(){
 if(state.memoryBusy)return;
 state.memoryBusy=true;state.memoryErr=null;render();
 try{var read=await get('/api/vitals');if(!read||!Array.isArray(read.top))throw new Error('ranked data was not returned');state.memory=read;state.memoryAt=new Date().toLocaleTimeString()}
 catch(e){state.memoryErr=e.message}
 finally{state.memoryBusy=false;if(state.view==='host')render()}
}
function vHost(d){
 var s=d.swap,h=state.fleet&&state.fleet.health;
 var ram=h&&h.ram_pct!=null?h.ram_pct:null;
 // The hosted board has no workstation behind it: say so plainly instead of showing a missing receipt as a fault.
 var hosted=!!(state.macs&&state.macs.macs);
 return fabricReceipt()+'<div class="grid g2">'+
 '<div class="card"><h2>Host facts — canonical Fabric observation</h2><p>RAM availability complement: '+(ram==null?'Unavailable':esc(ram)+'%')+'</p><p>Capacity: '+(h&&h.total_ram!=null?esc(h.total_ram)+' bytes':'Unavailable')+' · CPU cores: '+(h&&h.cpu_cores!=null?esc(h.cpu_cores):'Unavailable')+'</p><p>Swap: '+(h&&h.swap!=null?esc(h.swap)+' bytes':'Unavailable')+' · Engine session: '+esc(h&&h.engine||'unknown')+' · '+esc(h&&h.engine_state||'unavailable')+' (not readiness proof)</p><p>Fabric observed '+esc(state.fleet&&state.fleet.generated_at||'Unavailable')+'</p></div>'+
 '<div class="card"><h2>Swap hygiene — retained separate receipt</h2>'+(s?'<div class="big">'+esc(s.verdict.replace('-',' '))+'</div><p class="sub">'+Math.round(s.used_mib)+' of '+Math.round(s.total_mib)+' MiB allocated · '+s.free_pct+'% free · paging delta '+(s.delta_pages<0?'not measurable yet':s.delta_pages+' pages')+'</p><p class="gap">Correctness-only diagnosis: <b>'+(s.correctness_only_ok==null?'unknown':s.correctness_only_ok?'eligible':'not eligible')+'</b><br>Release-timing qualification: <b>'+(s.release_timing_ok==null?'unknown':s.release_timing_ok?'eligible':'not eligible')+'</b></p>'+(s.restart_proposed?'<p class="gap r-danger"><b>A coordinated restart is proposed</b> for the owner and workload owners. Nothing was restarted.</p>':'')+'<p class="sub gap">Sampled '+rel(s.at)+'. Allocation is not paging: the verdict comes from swap movement between samples.</p>':hosted?'<p class="muted">Swap hygiene: not available on the hosted board. Open a Mac console for its receipt.</p>':'<p class="sub">No receipt yet. Run <span class="mono">sirsi swap-hygiene</span>.</p>')+'</div></div>'+(hosted?'<p class="muted">Process memory diagnostics: not available on the hosted board.</p>':memoryDiagnostics());
}
var RENDER={overview:vOverview,lanes:vLanes,queue:vQueue,releases:vReleases,failures:vFailures,host:vHost};

// Focus survives every re-render: an element is found again by id, else by its tag and data-* signature.
function sig(x){var d=x.dataset||{};return x.tagName+'|'+Object.keys(d).map(function(k){return k+'='+d[k]}).join('&')+'|'+(x.getAttribute&&x.getAttribute('href')||'')}
function focusKey(a,root){
 if(!a||a===document.body||typeof root.contains!=='function'||!root.contains(a))return null;
 if(a.id)return {id:a.id,start:a.selectionStart,end:a.selectionEnd};
 return {sig:sig(a)};
}
function restoreFocus(root,s){
 if(!s)return;
 var t=s.id?document.getElementById(s.id):Array.from(root.querySelectorAll('button,a,summary,input')).find(function(x){return sig(x)===s.sig});
 if(t){t.focus({preventScroll:true});if(s.start!=null&&t.setSelectionRange)t.setSelectionRange(s.start,s.end);return}
 var fb=(state.view==='fleet'&&document.getElementById('fabric-task-heading'))||document.getElementById('main');
 if(fb)fb.focus({preventScroll:true});
}
function draw(el){
 if(state.view==='lane'){el.innerHTML=vLane();return}
 if(state.view==='fleet'){el.innerHTML=(state.fleetErr?'<p class="stale-banner banner b-warn">Fleet refresh failed · '+esc(state.fleetErr)+'</p>':'')+vFleet();bindFabricLanes(el);return}
 if(state.view==='host'){el.innerHTML=macPicker()+vHost(state.data||{});return}
 if(!state.data){el.innerHTML=(state.view==='overview'?fabricHeadlines():'')+(state.err?PantheonLedger.banner('danger','Router unavailable.',state.err,true):skeleton());if(state.view==='overview')bindFabricHeadlines(el);return}
 var opened=Array.from(el.querySelectorAll('details[open]')).map(function(x){return x.dataset.release});el.innerHTML=(state.err?'<p class="stale-banner banner b-warn">Retained snapshot · '+esc(state.data.generated_at||'time unknown')+' · '+esc(state.err)+'</p>':'')+(state.view==='overview'?fabricHeadlines():'')+RENDER[state.view](state.data);el.querySelectorAll('details').forEach(function(x){x.open=opened.includes(x.dataset.release)});
 if(state.view==='lanes')bindLanes();
 if(state.view==='overview')bindFabricHeadlines(el);
}
function render(){
 var el=$('#view'),saved=focusKey(document.activeElement,el),scroll=window.scrollY;
 nav();
 var v=VIEWS.filter(function(x){return x[0]===state.view})[0]||VIEWS[0];
 $('#title').textContent=state.view==='lane'?state.laneAgent:v[1];$('#subtitle').textContent=state.view==='lane'?'Lane ledger':v[2];
 $('#foot-ver').textContent=state.data?'Pantheon '+state.data.version:'';
 $('#live').hidden=state.view==='mercury'; // Mercury owns its own typed read-state label, not the router receipt's lamp.
 if(state.view==='mercury'){if(!stopMercury){el.replaceChildren();stopMercury=window.mountMercury(el)}return}
 if(stopMercury){stopMercury();stopMercury=null}
 draw(el);
 restoreFocus(el,saved);
 window.scrollTo(0,scroll);
}
function showInspector(title,body,persistent){
 var old=$('.inspector');if(old){old.close();old.remove()}

 var dlg=document.createElement('dialog');dlg.className='inspector';dlg.setAttribute('aria-label',title);
 dlg.innerHTML='<div class="inspector-head"><h2>'+esc(title)+'</h2><button class="btn" data-close>Close</button></div>'+body;
 var previous=document.activeElement;var previousAgent=previous&&previous.dataset&&previous.dataset.inspect;var previousFabricAgent=previous&&previous.dataset&&previous.dataset.fabricInspect;var previousFabricControl=previous&&previous.dataset&&previous.dataset.fabricControl;var previousTask=previous&&previous.dataset&&previous.dataset.fabricTaskId;var previousTaskAgent=previous&&previous.dataset&&previous.dataset.fabricTaskAgent;document.body.appendChild(dlg);if(persistent&&window.matchMedia('(min-width:1200px)').matches){dlg.classList.add('persistent');document.body.classList.add('has-inspector')}
dlg.addEventListener('keydown',function(e){if(e.key==='Escape'){e.preventDefault();dlg.close()}});dlg.querySelector('[data-close]').onclick=function(){dlg.close()};dlg.addEventListener('close',function(){dlg.remove();document.body.classList.remove('has-inspector');if(previous&&previous.isConnected)previous.focus();else if(previousAgent||previousFabricAgent||previousTask){var target=Array.from(document.querySelectorAll('[data-inspect],[data-fabric-inspect],[data-fabric-task-id]')).find(function(x){return previousTask?x.dataset.fabricTaskId===previousTask&&x.dataset.fabricTaskAgent===previousTaskAgent:previousFabricAgent?x.dataset.fabricInspect===previousFabricAgent&&x.dataset.fabricControl===previousFabricControl:x.dataset.inspect===previousAgent});if(target)target.focus();else $('#main').focus()}});if(dlg.classList.contains('persistent'))dlg.show();else dlg.showModal();dlg.querySelector('[data-close]').focus();
}
function fabricInspector(board,agent){showInspector('Fabric lane inspector',PantheonFabric.laneDetail(board,agent,esc),true)}
function fabricTaskInspector(board,agent,id){showInspector('Canonical task inspector',PantheonFabric.taskDetail(board,agent,id,esc),true)}
function failureInspector(id){
 var failure=((state.data||{}).known_failures||[]).find(function(x){return x.id===id});if(!failure)return;
 showInspector('Failure details','<h3>'+esc(failure.id)+'</h3><p>'+esc(failure.title)+'</p><dl>'+[['Status',failure.status],['Fixed in',failure.fixed_in],['Regression guard',failure.guard],['Signature',failure.signature],['Cause',failure.cause],['Resolution',failure.resolution]].map(function(x){return '<dt>'+esc(x[0])+'</dt><dd>'+esc(x[1]??'Not supplied by this snapshot')+'</dd>'}).join('')+'</dl>');
}
function nextStep(n){return n&&n.kind==='command-copy'&&n.command?'<button class="btn" data-copy="'+esc(n.command)+'">'+esc(n.label||'Copy command')+'</button>':''}
function inspector(agent){
 var l=((state.data||{}).lanes||{}).list||[],lane=l.find(function(x){return x.agent===agent});
 showInspector('Lane inspector','<p class="sub">Snapshot '+esc(state.data&&state.data.generated_at||'time unknown')+' · Reopen to see the latest received snapshot.</p><h3>'+esc(agent)+'</h3>'+(lane?chip(lane.verdict)+'<p>'+esc(lane.detail)+'</p><dl>'+[['Open items',lane.open],['Observed',lane.observed_at],['Worker thread',lane.worker_thread_id],['Last report',lane.last_report_at],['Report',lane.last_report_summary]].map(function(x){return '<dt>'+esc(x[0])+'</dt><dd>'+esc(x[1]??'Unknown / not supplied')+'</dd>'}).join('')+'</dl>'+nextStep(lane.next):'<p>No lane projection supplied for this recipient.</p>'),true);
}
document.addEventListener('click',function(e){var b=e.target.closest('[data-memory],[data-queue],[data-item],[data-inspect],[data-copy],[data-failure],[data-view]');if(!b)return;if(b.dataset.memory){inspectMemory()}else if(b.dataset.queue){state.recipient=b.dataset.queue;go('queue')}else if(b.dataset.item)itemInspector(b.dataset.item);else if(b.dataset.view)go(b.dataset.view);else if(b.dataset.failure)failureInspector(b.dataset.failure);else if(b.dataset.inspect)inspector(b.dataset.inspect);else if(!navigator.clipboard){$('#announcement').textContent='Clipboard unavailable. Command: '+b.dataset.copy}else navigator.clipboard.writeText(b.dataset.copy).then(function(){$('#announcement').textContent='Command copied.'}).catch(function(){$('#announcement').textContent='Clipboard unavailable. Command: '+b.dataset.copy})});
document.addEventListener('keydown',function(e){var dlg=$('.inspector.persistent');if(e.key==='Escape'&&dlg&&dlg.open&&!e.target.closest('input,textarea,select,[contenteditable]')){e.preventDefault();dlg.close();return}if(e.target.closest('input,textarea,select,dialog')||e.metaKey||e.ctrlKey||e.altKey)return;if(e.key==='r'){e.preventDefault();if(stopMercury)stopMercury.refresh();else load()}if(e.key==='/'){e.preventDefault();go('lanes');$('#lq').focus()}});
$('#refresh').addEventListener('click',function(){if(stopMercury)stopMercury.refresh();else load()});
window.addEventListener('pagehide',function(){if(stopMercury){stopMercury();stopMercury=null}});
window.addEventListener('pageshow',function(){ageFabric();paintLive();if(state.view==='mercury'&&!stopMercury)render()});
document.addEventListener('visibilitychange',function(){if(!document.hidden){ageFabric();paintLive()}});
window.addEventListener('hashchange',function(){var h=location.hash.slice(1),r=route(h);if(r&&(r.view!==state.view||r.agent!==state.laneAgent))go(h)});
// The hosted board answers /api/macs; a Mac's own console does not. Host details
// and controls live on each Mac's console, opened straight from this device over
// the Sirsi tailnet, where the Mac itself checks who you are. The app installs
// with an offline fallback that finds a live console if the board goes down.
function macPicker(){
 var m=state.macs;if(!m||!m.macs||!m.macs.length)return '';
 return '<div class="card"><h2>Mac consoles</h2><p class="sub">Controls run on each Mac, opened directly from this device (it must be on the Sirsi tailnet).</p><p>'+m.macs.map(function(x){return '<a class="btn" target="_blank" rel="noopener" href="'+esc(x.url)+'/">'+esc(x.name)+'</a>'}).join(' ')+'</p></div>';
}
fetch('/api/macs',{cache:'no-store'}).then(function(r){return r.ok?r.json():null}).then(function(d){
 if(!d)return;state.macs=d;if(state.view==='host')render();
 if('serviceWorker' in navigator&&location.protocol==='https:')navigator.serviceWorker.register('/sw.js').catch(function(){});
}).catch(function(){});
var r0=route(location.hash.slice(1));if(r0){state.view=r0.view;state.laneAgent=r0.agent}
render();load().then(function(){if(state.view==='fleet'&&!state.fleet)loadFleet()});
if(state.view==='lane'){loadLane(state.laneAgent);loadJobs(state.laneAgent)}
var nextPoll=Date.now()+POLL;
state.timer=setInterval(function(){if(document.hidden)return;ageFabric();paintLive();
 var L=state.view==='lane'&&state.ledgers[state.laneAgent];
 // A ledger that has gone stale must say so even if no new data arrives to trigger a render.
 if(L&&L.at&&!L.staleShown&&Date.now()-L.at>PantheonLedger.STALE_MS){L.staleShown=true;render()}
 if(Date.now()>=nextPoll){nextPoll=Date.now()+POLL;load()}},5000);
})();
