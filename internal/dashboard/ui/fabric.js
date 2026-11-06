/* Pure rendering of the canonical FabricBoard. No per-surface aggregation. */
(function(root){
'use strict';
function valid(b){
 if(!b||!b.work||!b.messages||!b.health||!Array.isArray(b.health.issues)||!Array.isArray(b.lanes)||!Array.isArray(b.activity)||typeof b.generated_at!=='string'||!Number.isFinite(Date.parse(b.generated_at)))return false;
 if(!['done','open','in_progress','assigned_not_started','stalled','blocked','total'].every(function(k){return Number.isSafeInteger(b.work[k])&&b.work[k]>=0}))return false;
 if(typeof b.build!=='string'||!Number.isFinite(b.messages.oldest_age_seconds)||b.messages.oldest_age_seconds<0)return false;
 if(![b.health.ram_pct,b.health.swap,b.health.total_ram,b.health.cpu_cores].every(function(n){return n==null||Number.isFinite(n)&&n>=0}))return false;
 if(b.health.ram_pct!=null&&b.health.ram_pct>100)return false;
 if(b.health.git_dirty!=null&&typeof b.health.git_dirty!=='boolean')return false;
 if(typeof b.health.engine!=='string'||typeof b.health.engine_state!=='string')return false;
 return ['open','stranded'].every(function(k){return Number.isSafeInteger(b.messages[k])&&b.messages[k]>=0})&&typeof b.activity_seeded==='boolean'&&typeof b.runtime_available==='boolean';
}
// Age belongs to this observation, independently of router receipt success.
function freshness(b,error,now){
 if(!valid(b))return {state:'unavailable',text:'Fabric unavailable — no observation received.'};
 var age=now-Date.parse(b.generated_at),stamp=' · observed '+b.generated_at;
 if(age<0)return {state:'unknown',text:'Fabric time unknown — future observation'+stamp+(error?' · refresh unavailable: '+error:'')};
 var seconds=Math.floor(age/1000);
 if(error||age>30000)return {state:'retained',text:'Retained Fabric observation · '+seconds+'s old'+stamp+(error?' · refresh unavailable: '+error:' · refresh to retry')};
 return {state:'current',text:'Current Fabric observation · '+seconds+'s old'+stamp};
}
function receipt(b,error,now,e){var f=freshness(b,error,now);return '<p role="status" class="fabric-receipt" data-fabric-state="'+f.state+'">'+e(f.text)+'</p>'}
function roleClass(role){return 'fabric-role-'+(['ok','gold','warn','ink2','dim'].includes(role)?role:'dim')}
function headlineRole(b,key){return roleClass(b.presentation&&b.presentation.headlines&&b.presentation.headlines[key])}
function publicLink(l){try{var u=new URL(l.url);return ['canon','repo','pr','owner-instruction','evidence'].includes(l.kind)&&u.protocol==='https:'&&u.host==='github.com'&&!u.username&&!u.password&&!u.search&&!u.hash}catch(_){return false}}
var taskFilters={'':'Open ledger tasks',open:'Open ledger tasks',done:'Completed tasks',in_progress:'In-progress tasks',assigned_not_started:'Assigned tasks — not started',stalled:'Stalled tasks',blocked:'Blocked tasks'};
function taskCount(b,filter){return Object.prototype.hasOwnProperty.call(taskFilters,filter)?b.work[filter||'open']:null}
function matchesFilter(t,filter){if(filter==='done')return t.status==='done';if(t.status==='done')return false;switch(filter){case '':case 'open':return true;case 'blocked':return t.status==='blocked';case 'in_progress':return t.status==='in-progress'&&t.liveness!=='stalled';case 'stalled':return t.status!=='blocked'&&t.liveness==='stalled';case 'assigned_not_started':return t.status!=='blocked'&&t.status!=='in-progress';default:return false}}
function taskPage(b){
 var p=valid(b)&&b.tasks;
 if(!p||p.filter!=null&&typeof p.filter!=='string'||p.state!=='observed'||!Array.isArray(p.rows)||p.rows.length>64||p.limit!==64||!Object.prototype.hasOwnProperty.call(p,'next_offset')||!Number.isSafeInteger(p.total)||p.total<0||p.total!==taskCount(b,p.filter||'')||!Number.isSafeInteger(p.offset)||p.offset<0||p.offset%64||!(/^[0-9a-f]{64}$/).test(p.snapshot)||p.offset+p.rows.length>p.total)return null;
 if(p.next_offset==null){if(p.offset+p.rows.length!==p.total)return null}
 else if(!Number.isSafeInteger(p.next_offset)||p.next_offset!==p.offset+p.rows.length||p.next_offset>=p.total||p.rows.length!==64)return null;
 if(p.total>0&&!p.rows.length||p.offset>0&&p.offset>=p.total)return null;
 var seen=new Set();
 if(!p.rows.every(function(t){var key=t&&t.agent+'\0'+t.id;if(!t||typeof t.agent!=='string'||!t.agent||typeof t.id!=='string'||!t.id||!['subject','status','liveness','role','responsible','stage','test_state','updated'].every(function(k){return typeof t[k]==='string'})||typeof t.blocker_recorded!=='boolean'||typeof t.text_limited!=='boolean'||!t.provenance||t.provenance.source!=='canonical-task-registry'||!Array.isArray(t.provenance.public_links)||t.provenance.public_links.length>8||!t.provenance.public_links.every(publicLink)||!['created','commissioned_at','commissioned_by'].every(function(k){return typeof t.provenance[k]==='string'})||!['charter_recorded','outline_recorded','result_recorded'].every(function(k){return typeof t.provenance[k]==='boolean'})||!Number.isSafeInteger(t.provenance.withheld_links)||t.provenance.withheld_links<0||seen.has(key))return false;seen.add(key);return true}))return null;
 if(!p.rows.every(function(t){return matchesFilter(t,p.filter||'')}))return null;
 return p;
}
function taskList(b,e,now,showRecent){
 var p=taskPage(b),h='<h2 id="fabric-task-heading" tabindex="-1">'+(p?taskFilters[p.filter||'']:'Ledger tasks')+'</h2>';
 if(!p)return h+'<p role="status">Task rows unavailable — not an empty task list. Refresh Fabric to retry.</p>';
 h+='<p>Showing '+(p.rows.length?p.offset+1:0)+'–'+(p.offset+p.rows.length)+' of '+p.total+' task rows. Page bound to this canonical observation.</p>';
 h+='<button class="btn" data-fabric-task-page="first">Refresh task list from first page</button>';
 if(p.filter&&p.filter!=='open')h+=' <button class="btn" data-fabric-filter="open">Show all open tasks</button>';
 if(p.next_offset!=null)h+=' <button class="btn" data-fabric-task-page="next">Next task page</button>';
 if(!p.rows.length)return h+'<p>'+(p.filter&&p.filter!=='open'?'No tasks match this selection in this observation.':'No open ledger tasks in this observation.')+' Inbox messages are a separate stream.</p>';
 var groups=new Map(),observationAge=now-Date.parse(b.generated_at);
 p.rows.forEach(function(t){if(!groups.has(t.agent))groups.set(t.agent,[]);groups.get(t.agent).push(t)});
 h+='<p class="fabric-page-note">Grouped by lane on this page only; counts do not represent the entire lane.</p>';
 groups.forEach(function(rows,agent){
  h+='<section class="fabric-ledger-group" aria-label="Tasks for '+e(agent)+' on this page"><h3>'+e(agent)+' <span>'+rows.length+' on this page</span></h3><table class="rows"><tbody>'+rows.map(function(t){
   // A recent transition is shown only from a current, error-free observation and only while it is
   // genuinely recent; it is derived from the producer's own activity feed, never inferred client-side.
   var event=showRecent&&b.activity_seeded&&Number.isFinite(observationAge)&&observationAge>=0&&observationAge<=30000&&b.activity.find(function(a){var age=now-Date.parse(a.at);return a.agent===t.agent&&a.task_id===t.id&&Number.isFinite(age)&&age>=0&&age<=30000});
   var change=event?'<span class="sub-line fabric-recent-change" data-fabric-at="'+e(event.at)+'" data-fabric-observed="'+e(b.generated_at)+'">Observed transition: '+e(event.from)+' → '+e(event.to)+' · '+e(event.at)+'</span>':'';
   return '<tr'+(event?' class="changed fabric-recent-row"':'')+'><td class="s"><span class="pill fabric-status '+roleClass(t.role)+'">'+e(t.status)+'</span></td><td class="id"><span class="idv" title="'+e(t.id)+'">'+e(t.id)+'</span></td><td class="t"><button class="lane-select" data-fabric-task-agent="'+e(t.agent)+'" data-fabric-task-id="'+e(t.id)+'">'+e(t.subject||'Subject not supplied')+'</button><span class="sub-line rid">'+e(t.id)+'</span><span class="sub-line dim">liveness '+e(t.liveness)+'</span>'+change+'</td></tr>';
  }).join('')+'</tbody></table></section>';
 });
 return h;
}
function taskDetail(b,agent,id,e){
 var p=taskPage(b),t=p&&p.rows.find(function(row){return row.agent===agent&&row.id===id});
 if(!t)return '<p>Task not supplied by this exact Fabric page. Refresh the task list to inspect another observation.</p>';
 var lane=b.lanes.find(function(row){return row.agent===agent}),r=lane&&lane.registered||{},o=t.provenance;
 function flag(v){return v===true?'Registered':v===false?'Not registered':'Unknown / not supplied'}
 var values=[['Task ID',t.id],['Agent',t.agent],['Status',t.status],['Task liveness',t.liveness],['Responsible party',t.responsible||'Not supplied'],['Stage',t.stage],['Test state',t.test_state],['Blocker',t.blocker_task_id|| (t.blocker_recorded?'Dependency recorded; private detail withheld':'No blocker entry supplied')],['Updated',t.updated||'Not supplied'],['Source',o.source],['Created',o.created||'Not supplied'],['Commissioned',o.commissioned_at||'Not supplied'],['Commissioned by',o.commissioned_by||'Not supplied'],['Charter',o.charter_recorded?'Retained by task authority':'Not supplied'],['Outline',o.outline_recorded?'Retained by task authority':'Not supplied'],['Result reference',o.result_recorded?'Retained by task authority':'Not supplied'],['Router registration',flag(r.router)],['Thread registration',flag(r.thread)],['Ledger registration',flag(r.ledger)]];
 var h='<p>Canonical ledger task · captured observation '+e(b.generated_at)+' · '+e(b.build)+'</p><h3>'+e(t.subject||'Subject not supplied')+'</h3><dl>'+values.map(function(x){return '<dt>'+e(x[0])+'</dt><dd'+(x[0]==='Status'?' class="'+roleClass(t.role)+'"':'')+'>'+e(x[1])+'</dd>'}).join('')+'</dl>';
 if(t.text_limited)h+='<p>Display text bounded; full source text remains in the authenticated task registry.</p>';
 h+='<h4>Provenance links · not fetched or independently verified</h4><ul>'+o.public_links.map(function(l){return '<li>'+e(l.kind)+': <a rel="noopener noreferrer" href="'+e(l.url)+'">'+e(l.url)+'</a></li>'}).join('')+'</ul>';
 return h+'<p>'+e(o.withheld_links||0)+' references withheld. Raw lease, charter, outline, phase and journal contents are not included; local/service reference URLs are withheld.</p>';
}
function bindTaskInspectors(element,b,inspect,page){
 element.querySelectorAll('[data-fabric-task-id]').forEach(function(button){button.addEventListener('click',function(){inspect(b,button.dataset.fabricTaskAgent,button.dataset.fabricTaskId)})});
 element.querySelectorAll('[data-fabric-task-page]').forEach(function(button){button.addEventListener('click',function(){var p=taskPage(b);if(button.dataset.fabricTaskPage==='first')page(p&&p.filter?{filter:p.filter}:null);else if(p&&p.next_offset!=null){var next={offset:p.next_offset,snapshot:p.snapshot};if(p.filter)next.filter=p.filter;page(next)}})});
}
function bindHeadlineControls(element,b,select,queue){
 if(!valid(b))return;
 element.querySelectorAll('[data-fabric-filter]').forEach(function(button){button.addEventListener('click',function(){var filter=button.dataset.fabricFilter;if(Object.prototype.hasOwnProperty.call(taskFilters,filter))select(filter)})});
 element.querySelectorAll('[data-fabric-messages]').forEach(function(button){button.addEventListener('click',queue)});
}
function laneDetail(b,agent,e){
 var lane=valid(b)&&b.lanes.find(function(l){return l.agent===agent});
 if(!lane)return '<p>No lane supplied by this Fabric observation.</p>';
 var r=lane.registered||{};
 function flag(v){return v===true?'Registered':v===false?'Not registered':'Unknown / not supplied'}
 return '<p>Canonical Fabric · ledger tasks / supervised runtime</p><p>Observation '+e(b.generated_at)+' · '+e(b.build)+'</p><h3>'+e(lane.agent)+'</h3><dl>'+[
 ['Runtime state',lane.state_label],['Canonical state',lane.state],['Open tasks',lane.open],['Active tasks',lane.active],['Stalled tasks',lane.stalled],['Blocked tasks',lane.blocked],['Last task touch',lane.last_touch||'Not supplied'],['Router registration',flag(r.router)],['Thread registration',flag(r.thread)],['Ledger registration',flag(r.ledger)]
 ].map(function(x){return '<dt>'+e(x[0])+'</dt><dd'+(x[0]==='Runtime state'?' class="'+roleClass(lane.role)+'"':'')+'>'+e(x[1])+'</dd>'}).join('')+'</dl><p>Router inbox messages are a separate observation; not substituted for this task lane.</p>';
}
function bindLaneInspectors(element,b,inspect){
 element.querySelectorAll('[data-fabric-inspect]').forEach(function(button){
  var agent=button.dataset.fabricInspect;
  button.addEventListener('click',function(){inspect(b,agent)});
 });
}
function headlines(b,e){
 if(!valid(b))return '<p role="status" class="banner b-warn">Fabric unavailable — no task or message counts observed.</p>';
 var w=b.work;
 function metric(filter,value,label){return '<button type="button" class="fabric-metric" data-fabric-filter="'+filter+'" aria-label="Show '+value+' '+label+'">'+value+'</button>'}
 function card(role,cls,n,label,sub){return '<div class="card count '+headlineRole(b,role)+'"><div class="n '+cls+'">'+n+'</div><div class="l">'+label+'</div>'+(sub?'<div class="s">'+sub+'</div>':'')+'</div>'}
 // Stalled counts include pending tasks that are also "not started", so the stack uses the
 // stalled-while-in-progress remainder: open minus the other disjoint buckets. Words and numbers are in the aria-label.
 var stalledRunning=Math.max(0,w.open-w.blocked-w.in_progress-w.assigned_not_started);
 var segs=[['Done',w.done,'r-ok'],['In progress',w.in_progress,'r-warn'],['Not started',w.assigned_not_started,'r-info'],['Stalled in progress',stalledRunning,'r-warn'],['Blocked',w.blocked,'r-danger']].filter(function(s){return s[1]>0});
 var total=segs.reduce(function(n,s){return n+s[1]},0);
 var bar='<div class="bar" role="img" aria-label="Task distribution: '+(segs.length?segs.map(function(s){return s[0]+' '+s[1]}).join(', '):'no tasks')+'">'+segs.map(function(s){return '<span class="'+s[2]+'" style="width:'+(100*s[1]/total).toFixed(2)+'%" title="'+s[0]+' '+s[1]+'"></span>'}).join('')+'</div>';
 return '<section aria-label="Canonical fabric tasks and messages"><div class="cards">'+
 card('progress','r-ink',w.total,'Tasks')+
 card('completed','r-ok','<button type="button" class="fabric-metric" data-fabric-filter="done" aria-label="Show '+w.done+' completed tasks">'+w.done+'</button>','Done')+
 card('progress','r-warn',metric('in_progress',w.in_progress,'in-progress tasks'),'In progress')+
 card('progress','r-info',metric('assigned_not_started',w.assigned_not_started,'assigned tasks not started'),'Not started')+
 card('stalled','r-warn',metric('stalled',w.stalled,'stalled tasks'),'Stalled')+
 card('stalled','r-danger',metric('blocked',w.blocked,'blocked tasks'),'Blocked')+
 card('messages','r-info','<button type="button" class="fabric-metric" data-fabric-messages aria-label="Show '+b.messages.open+' pending inbox messages">'+b.messages.open+'</button>','Messages pending','Inbox messages · '+b.messages.stranded+' stranded')+
 '</div>'+bar+'<p class="sub">'+metric('open',w.open,'open tasks')+' open tasks · stalled may overlap not started · Fabric observation '+e(b.generated_at)+' · '+e(b.build)+'</p></section>';
}
function render(b,e,now,showRecent){
 if(!valid(b))return headlines(b,e);
 if(!Number.isFinite(now))now=Date.now();
 if(showRecent===undefined)showRecent=true;
 var h=headlines(b,e),v=b.health;
 function fact(value,unit){return value==null?'Unavailable':e(value)+(unit||'')}
 h+='<h2>Needs attention</h2>'+(!b.runtime_available?'<p role="status">Runtime classification unavailable.</p>':'')+
 (v.issues.length?'<ul>'+v.issues.map(function(x){return '<li>'+e(x)+'</li>'}).join('')+'</ul>':'<p>No issue entries supplied; absence is not a host-health verdict.</p>');
 h+='<h2>Lanes — supervised runtime</h2><table class="rows fabric-lane-table"><tbody>'+b.lanes.map(function(l){return '<tr><td class="s"><button class="lane-select pill-btn" data-fabric-control="state" data-fabric-inspect="'+e(l.agent)+'"><span class="'+roleClass(l.role)+'">'+e(l.state_label)+'</span></button></td><td class="id"><span class="idv" title="'+e(l.agent)+'"><button class="lane-select" data-fabric-control="agent" data-fabric-inspect="'+e(l.agent)+'">'+e(l.agent)+'</button></span></td><td class="t"><button class="lane-select" data-fabric-control="open" data-fabric-inspect="'+e(l.agent)+'" aria-label="Inspect '+e(l.agent)+' with '+l.open+' open tasks">'+l.open+' open tasks</button><span class="sub-line rid">'+e(l.agent)+'</span></td></tr>'}).join('')+'</tbody></table>';
 h+=taskList(b,e,now,showRecent);
 h+='<h2>Host health — separate from work</h2><p>RAM availability complement: '+fact(v.ram_pct,'%')+' · Swap: '+fact(v.swap,' bytes')+' · Capacity: '+fact(v.total_ram,' bytes')+' · CPU cores: '+fact(v.cpu_cores)+'</p><p>Repository dirty: '+(v.git_dirty==null?'Unavailable':e(String(v.git_dirty)))+' · Engine: '+e(v.engine)+' · '+e(v.engine_state)+' (session observation, not readiness proof)</p>';
 h+='<h2>Observed task transitions</h2>';
 if(!b.activity_seeded)h+='<p>Baseline only — no prior transition observation. Empty is not evidence of inactivity.</p>';
 else if(!b.activity.length)h+='<p>No status transitions observed since this producer baseline.</p>';
 else h+='<ul>'+b.activity.map(function(a){return '<li>'+e(a.at)+' · '+e(a.agent)+' · '+e(a.task_id)+' · '+e(a.from)+' → '+e(a.to)+'</li>'}).join('')+'</ul>';
 return h;
}
var api={valid:valid,headlines:headlines,render:render,freshness:freshness,receipt:receipt,laneDetail:laneDetail,bindLaneInspectors:bindLaneInspectors,taskPage:taskPage,taskDetail:taskDetail,bindTaskInspectors:bindTaskInspectors,bindHeadlineControls:bindHeadlineControls,roleClass:roleClass};
if(typeof module!=='undefined'&&module.exports)module.exports=api;else root.PantheonFabric=api;
})(typeof window!=='undefined'?window:globalThis);
