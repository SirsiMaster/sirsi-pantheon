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
function laneDetail(b,agent,e){
 var lane=valid(b)&&b.lanes.find(function(l){return l.agent===agent});
 if(!lane)return '<p>No lane supplied by this Fabric observation.</p>';
 var r=lane.registered||{};
 function flag(v){return v===true?'Registered':v===false?'Not registered':'Unknown / not supplied'}
 return '<p>Canonical Fabric · ledger tasks / supervised runtime</p><p>Observation '+e(b.generated_at)+' · '+e(b.build)+'</p><h3>'+e(lane.agent)+'</h3><dl>'+[
 ['Runtime state',lane.state_label],['Canonical state',lane.state],['Open tasks',lane.open],['Active tasks',lane.active],['Stalled tasks',lane.stalled],['Blocked tasks',lane.blocked],['Last task touch',lane.last_touch||'Not supplied'],['Router registration',flag(r.router)],['Thread registration',flag(r.thread)],['Ledger registration',flag(r.ledger)]
 ].map(function(x){return '<dt>'+e(x[0])+'</dt><dd>'+e(x[1])+'</dd>'}).join('')+'</dl><p>Router inbox messages are a separate observation; not substituted for this task lane.</p>';
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
 '<div class="card kpi"><h2>Completed / in flight</h2><div class="v">'+w.done+' / '+w.open+'</div><div class="l">Tasks · '+w.total+' total</div></div>'+
 '<div class="card kpi"><h2>In progress / assigned</h2><div class="v">'+w.in_progress+' / '+w.assigned_not_started+'</div><div class="l">Tasks</div></div>'+
 '<div class="card kpi"><h2>Stalled / blocked</h2><div class="v">'+w.stalled+' / '+w.blocked+'</div><div class="l">Tasks</div></div>'+
 '<div class="card kpi"><h2>Messages pending</h2><div class="v">'+b.messages.open+'</div><div class="l">Inbox messages · '+b.messages.stranded+' stranded</div></div></div>'+
 '<p class="sub">Fabric observation '+e(b.generated_at)+' · '+e(b.build)+'</p></section>';
}
function render(b,e){
 if(!valid(b))return headlines(b,e);
 var h=headlines(b,e),v=b.health;
 function fact(value,unit){return value==null?'Unavailable':e(value)+(unit||'')}
 h+='<h2>Needs attention</h2>'+(!b.runtime_available?'<p role="status">Runtime classification unavailable.</p>':'')+
 (v.issues.length?'<ul>'+v.issues.map(function(x){return '<li>'+e(x)+'</li>'}).join('')+'</ul>':'<p>No issue entries supplied; absence is not a host-health verdict.</p>');
 h+='<h2>Lanes — supervised runtime</h2><table class="table"><thead><tr><th>Agent</th><th>State</th><th>Open tasks</th></tr></thead><tbody>'+b.lanes.map(function(l){return '<tr><td><button class="lane-select" data-fabric-inspect="'+e(l.agent)+'">'+e(l.agent)+'</button></td><td>'+e(l.state_label)+'</td><td>'+l.open+'</td></tr>'}).join('')+'</tbody></table>';
 h+='<h2>Host health — separate from work</h2><p>RAM availability complement: '+fact(v.ram_pct,'%')+' · Swap: '+fact(v.swap,' bytes')+' · Capacity: '+fact(v.total_ram,' bytes')+' · CPU cores: '+fact(v.cpu_cores)+'</p><p>Repository dirty: '+(v.git_dirty==null?'Unavailable':e(String(v.git_dirty)))+' · Engine: '+e(v.engine)+' · '+e(v.engine_state)+' (session observation, not readiness proof)</p>';
 h+='<h2>Observed task transitions</h2>';
 if(!b.activity_seeded)h+='<p>Baseline only — no prior transition observation. Empty is not evidence of inactivity.</p>';
 else if(!b.activity.length)h+='<p>No status transitions observed since this producer baseline.</p>';
 else h+='<ul>'+b.activity.map(function(a){return '<li>'+e(a.at)+' · '+e(a.agent)+' · '+e(a.task_id)+' · '+e(a.from)+' → '+e(a.to)+'</li>'}).join('')+'</ul>';
 return h;
}
var api={valid:valid,headlines:headlines,render:render,freshness:freshness,receipt:receipt,laneDetail:laneDetail,bindLaneInspectors:bindLaneInspectors};
if(typeof module!=='undefined'&&module.exports)module.exports=api;else root.PantheonFabric=api;
})(typeof window!=='undefined'?window:globalThis);
