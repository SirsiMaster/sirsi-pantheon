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
function taskPage(b){
 var p=valid(b)&&b.tasks;
 if(!p||p.state!=='observed'||!Array.isArray(p.rows)||p.rows.length>64||p.limit!==64||!Object.prototype.hasOwnProperty.call(p,'next_offset')||!Number.isSafeInteger(p.total)||p.total<0||p.total!==b.work.open||!Number.isSafeInteger(p.offset)||p.offset<0||p.offset%64||!(/^[0-9a-f]{64}$/).test(p.snapshot)||p.offset+p.rows.length>p.total)return null;
 if(p.next_offset==null){if(p.offset+p.rows.length!==p.total)return null}
 else if(!Number.isSafeInteger(p.next_offset)||p.next_offset!==p.offset+p.rows.length||p.next_offset>=p.total||p.rows.length!==64)return null;
 if(p.total>0&&!p.rows.length||p.offset>0&&p.offset>=p.total)return null;
 var seen=new Set();
 if(!p.rows.every(function(t){var key=t&&t.agent+'\0'+t.id;if(!t||typeof t.agent!=='string'||!t.agent||typeof t.id!=='string'||!t.id||!['subject','status','liveness','role','responsible','stage','test_state','updated'].every(function(k){return typeof t[k]==='string'})||typeof t.blocker_recorded!=='boolean'||typeof t.text_limited!=='boolean'||!t.provenance||t.provenance.source!=='canonical-task-registry'||!Array.isArray(t.provenance.public_links)||t.provenance.public_links.length>8||!t.provenance.public_links.every(publicLink)||!['created','commissioned_at','commissioned_by'].every(function(k){return typeof t.provenance[k]==='string'})||!['charter_recorded','outline_recorded','result_recorded'].every(function(k){return typeof t.provenance[k]==='boolean'})||!Number.isSafeInteger(t.provenance.withheld_links)||t.provenance.withheld_links<0||seen.has(key))return false;seen.add(key);return true}))return null;
 return p;
}
function taskList(b,e){
 var p=taskPage(b),h='<h2>Open ledger tasks</h2>';
 if(!p)return h+'<p role="status">Task rows unavailable — not an empty task list. Refresh Fabric to retry.</p>';
 h+='<p>Showing '+(p.rows.length?p.offset+1:0)+'–'+(p.offset+p.rows.length)+' of '+p.total+' task rows. Page bound to this canonical observation.</p>';
 h+='<button class="btn" data-fabric-task-page="first">Refresh task list from first page</button>';
 if(p.next_offset!=null)h+=' <button class="btn" data-fabric-task-page="next">Next task page</button>';
 if(!p.rows.length)return h+'<p>No open ledger tasks in this observation. Inbox messages are a separate stream.</p>';
 return h+'<table class="table responsive-table"><thead><tr><th>Task</th><th>Agent</th><th>Status</th></tr></thead><tbody>'+p.rows.map(function(t){
  return '<tr><td data-label="Task"><button class="lane-select" data-fabric-task-agent="'+e(t.agent)+'" data-fabric-task-id="'+e(t.id)+'">'+e(t.subject||'Subject not supplied')+'</button><div class="sub">'+e(t.id)+'</div></td><td data-label="Agent">'+e(t.agent)+'</td><td data-label="Status" class="'+roleClass(t.role)+'">'+e(t.status)+' · '+e(t.liveness)+'</td></tr>';
 }).join('')+'</tbody></table>';
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
 element.querySelectorAll('[data-fabric-task-page]').forEach(function(button){button.addEventListener('click',function(){var p=taskPage(b);if(button.dataset.fabricTaskPage==='first')page(null);else if(p&&p.next_offset!=null)page({offset:p.next_offset,snapshot:p.snapshot})})});
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
 if(!valid(b))return '<p role="status">Fabric unavailable — no task or message counts observed.</p>';
 var w=b.work;
 return '<section aria-label="Canonical fabric tasks and messages"><div class="grid g4">'+
 '<div class="card kpi '+headlineRole(b,'completed')+'"><h2>Completed / in flight</h2><div class="v">'+w.done+' / '+w.open+'</div><div class="l">Tasks · '+w.total+' total</div></div>'+
 '<div class="card kpi '+headlineRole(b,'progress')+'"><h2>In progress / assigned</h2><div class="v">'+w.in_progress+' / '+w.assigned_not_started+'</div><div class="l">Tasks</div></div>'+
 '<div class="card kpi '+headlineRole(b,'stalled')+'"><h2>Stalled / blocked</h2><div class="v">'+w.stalled+' / '+w.blocked+'</div><div class="l">Tasks</div></div>'+
 '<div class="card kpi '+headlineRole(b,'messages')+'"><h2>Messages pending</h2><div class="v">'+b.messages.open+'</div><div class="l">Inbox messages · '+b.messages.stranded+' stranded</div></div></div>'+
 '<p class="sub">Fabric observation '+e(b.generated_at)+' · '+e(b.build)+'</p></section>';
}
function render(b,e){
 if(!valid(b))return headlines(b,e);
 var h=headlines(b,e),v=b.health;
 function fact(value,unit){return value==null?'Unavailable':e(value)+(unit||'')}
 h+='<h2>Needs attention</h2>'+(!b.runtime_available?'<p role="status">Runtime classification unavailable.</p>':'')+
 (v.issues.length?'<ul>'+v.issues.map(function(x){return '<li>'+e(x)+'</li>'}).join('')+'</ul>':'<p>No issue entries supplied; absence is not a host-health verdict.</p>');
 h+='<h2>Lanes — supervised runtime</h2><table class="table"><thead><tr><th>Agent</th><th>State</th><th>Open tasks</th></tr></thead><tbody>'+b.lanes.map(function(l){return '<tr><td><button class="lane-select" data-fabric-inspect="'+e(l.agent)+'">'+e(l.agent)+'</button></td><td class="'+roleClass(l.role)+'">'+e(l.state_label)+'</td><td>'+l.open+'</td></tr>'}).join('')+'</tbody></table>';
 h+=taskList(b,e);
 h+='<h2>Host health — separate from work</h2><p>RAM availability complement: '+fact(v.ram_pct,'%')+' · Swap: '+fact(v.swap,' bytes')+' · Capacity: '+fact(v.total_ram,' bytes')+' · CPU cores: '+fact(v.cpu_cores)+'</p><p>Repository dirty: '+(v.git_dirty==null?'Unavailable':e(String(v.git_dirty)))+' · Engine: '+e(v.engine)+' · '+e(v.engine_state)+' (session observation, not readiness proof)</p>';
 h+='<h2>Observed task transitions</h2>';
 if(!b.activity_seeded)h+='<p>Baseline only — no prior transition observation. Empty is not evidence of inactivity.</p>';
 else if(!b.activity.length)h+='<p>No status transitions observed since this producer baseline.</p>';
 else h+='<ul>'+b.activity.map(function(a){return '<li>'+e(a.at)+' · '+e(a.agent)+' · '+e(a.task_id)+' · '+e(a.from)+' → '+e(a.to)+'</li>'}).join('')+'</ul>';
 return h;
}
var api={valid:valid,headlines:headlines,render:render,freshness:freshness,receipt:receipt,laneDetail:laneDetail,bindLaneInspectors:bindLaneInspectors,taskPage:taskPage,taskDetail:taskDetail,bindTaskInspectors:bindTaskInspectors,roleClass:roleClass};
if(typeof module!=='undefined'&&module.exports)module.exports=api;else root.PantheonFabric=api;
})(typeof window!=='undefined'?window:globalThis);
