/* Horus dashboard — reads /api/router, /api/fleet, /api/stats. No framework, no build step. */
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
 host:'<svg viewBox="0 0 24 24"><rect x="3" y="4" width="18" height="12" rx="2"/><path d="M8 20h8M12 16v4"/></svg>',
 warn:'<svg class="ico" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M12 3l9 16H3L12 3z"/><path d="M12 10v4"/><circle cx="12" cy="17" r=".6"/></svg>',
 info:'<svg class="ico" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"><circle cx="12" cy="12" r="9"/><path d="M12 11v6"/><circle cx="12" cy="7.5" r=".6"/></svg>',
 check:'<svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="var(--ok)" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="9"/><path d="M8 12.5l2.7 2.7L16 9.5"/></svg>'
};
function plural(n,one,many){return n+' '+(n===1?one:many)}
var VIEWS=[['overview','Work fabric','One canonical router. Every lane, one accountable history.'],['lanes','Lanes','Can each lane work right now'],['queue','Queue','Open items by recipient'],['releases','Releases','What each release added'],['failures','Known failures','Registered once, fixed with a guard'],['fleet','Fleet','Work in flight across every lane'],['host','Host','Memory, swap and this Mac']];
var state={view:'overview',data:null,fleet:null,stats:null,err:null,at:null,recipient:'*',lane:{filter:'ALL',q:'',sort:'agent',dir:1},timer:null};

function get(u){return fetch(u,{cache:'no-store',signal:AbortSignal.timeout(15000)}).then(function(r){return r.json().then(function(j){if(!r.ok)throw new Error(j.error||('HTTP '+r.status));return j})})}
function load(){
 if(state.loading)return state.loading;
 setLive('Refreshing…','stale');
 var router=get('/api/router/v1/snapshot').then(function(d){state.data=d;state.err=null;state.at=new Date()}).catch(function(e){state.err=e.message}).then(function(){render();setLive(state.err?'Stale / unavailable · '+state.err:'Received '+state.at.toLocaleTimeString(),' '+(state.err?'err':''))});
 var host=get('/api/stats').then(function(d){state.stats=d;state.hostErr=null}).catch(function(e){state.hostErr=e.message}).then(function(){if(state.view==='host')render()});
 var fleet=loadFleet();
 state.loading=Promise.allSettled([router,host,fleet]).finally(function(){state.loading=null});return state.loading;
}
function loadFleet(){return get('/api/fleet').then(function(f){state.fleet=f;state.fleetErr=null}).catch(function(e){state.fleetErr=e.message}).then(function(){if(state.view==='fleet')render()})}
function setLive(t,cls){$('#live-t').textContent=t;$('#live .dot').className='dot'+(cls?' '+cls:'')}

function nav(){
 var n=(state.data&&state.data.attention||[]).filter(function(a){return a.severity==='critical'}).length;
 $('#nav').innerHTML='<div class="nav-label">Router</div>'+VIEWS.map(function(v){
  return '<button type="button" data-v="'+v[0]+'"'+(state.view===v[0]?' aria-current="page"':'')+'>'+ICON[v[0]]+'<span>'+v[1]+'</span>'+(v[0]==='overview'&&n?'<span class="count" aria-label="'+n+' critical">'+n+'</span>':'')+'</button>'}).join('');
 $('#nav').querySelectorAll('button').forEach(function(b){b.addEventListener('click',function(){go(b.dataset.v)})});
}
function go(v){var dlg=$('.inspector');if(dlg&&dlg.open)dlg.close();state.view=v;location.hash=v;render();if(v==='fleet'&&!state.fleet)loadFleet();$('#main').focus({preventScroll:true})}

function skeleton(){return '<p class="sub loading">Loading the local snapshot… Other views remain available.</p>'+ '<div class="grid g4"><div class="skel"></div><div class="skel"></div><div class="skel"></div><div class="skel"></div></div>'}
function chip(v){return '<span class="chip v-'+esc(v)+'">'+esc(v.replace('_',' ').toLowerCase().replace(/^./,function(c){return c.toUpperCase()}))+'</span>'}
var VCOL={LIVE:'var(--ok)',WAKEABLE:'var(--wakeable)',HELD:'var(--warn)',AUTH_REQUIRED:'var(--danger)',UNREACHABLE:'var(--danger)',WATCH_ONLY:'var(--info)',UNSTAFFED:'var(--dim)'};
var ORDER=['LIVE','WAKEABLE','HELD','WATCH_ONLY','AUTH_REQUIRED','UNREACHABLE','UNSTAFFED'];

function stackBar(counts){
 var tot=ORDER.reduce(function(s,k){return s+(counts[k]||0)},0)||1;
 var bar=ORDER.filter(function(k){return counts[k]}).map(function(k){return '<span style="flex:'+counts[k]+';--c:'+VCOL[k]+'" title="'+k+' '+counts[k]+'"></span>'}).join('');
 var leg=ORDER.map(function(k){return '<span><i style="--c:'+VCOL[k]+'"></i>'+esc(k.replace('_',' ').toLowerCase())+' '+(counts[k]??'Unknown')+'</span>'}).join('');
 return '<div class="bar" role="img" aria-label="Lane verdicts">'+bar+'</div><div class="legend">'+leg+'</div>';
}
function ring(pct,color){var c=2*Math.PI*40,d=Math.max(0,Math.min(100,pct))/100*c;return '<svg class="ring" viewBox="0 0 100 100" style="--c:'+color+'" role="img" aria-label="'+Math.round(pct)+' percent"><circle class="bg" cx="50" cy="50" r="40"/><circle class="fg" cx="50" cy="50" r="40" stroke-dasharray="'+d+' '+c+'" transform="rotate(-90 50 50)"/></svg>'}

function nextStep(n){return n&&n.kind==='command-copy'&&n.command?'<button class="btn" data-copy="'+esc(n.command)+'">'+esc(n.label||'Copy command')+'</button>':''}
function vOverview(d){
 var counts=(d.lanes||{}).counts||{},q=d.queue||[],lanes=(d.lanes||{}).list||[],att=d.attention||[];
 var open=q.reduce(function(n,x){return n+x.open},0);
 var ranked=q.filter(function(x){return x.open>0}).map(function(x){var l=lanes.find(function(l){return l.agent===x.agent});return Object.assign({},l||{agent:x.agent,verdict:'UNSTAFFED',detail:'Recipient is outside the lane registry.'},{open:x.open})}).sort(function(a,b){return b.open-a.open||a.agent.localeCompare(b.agent)});
 var h='<section class="fabric-summary"><h2>'+esc(counts.WAKEABLE??'Unknown')+' lanes ready.<br><button class="queue-total" data-queue="*">'+esc(open)+' open items <span aria-hidden="true">↗</span></button></h2><p>Across '+ranked.length+' recipients. Every waiting recipient is listed below; select its count to read the messages. The service reports '+esc(counts.LIVE??'unknown')+' live lanes and '+esc(counts.HELD??'unknown')+' held lanes.</p><button class="btn primary" data-view="lanes">Explore all '+lanes.length+' lanes</button></section>';
 h+='<div class="workbench"><section class="work-list"><div class="section-heading"><h2>Where work is waiting</h2><button class="btn" data-queue="*">View all '+open+' items</button></div><table class="table responsive-table"><thead><tr><th>Lane</th><th>Availability</th><th class="num">Open</th><th>Recorded condition</th></tr></thead><tbody>'+ranked.map(function(l){return '<tr><td data-label="Lane"><button class="lane-select" data-inspect="'+esc(l.agent)+'">'+esc(l.agent)+'</button></td><td data-label="Availability">'+chip(l.verdict)+'</td><td class="num" data-label="Open"><button class="lane-select" data-queue="'+esc(l.agent)+'" aria-label="Read '+l.open+' open items for '+esc(l.agent)+'">'+l.open+'</button></td><td class="dim" data-label="Condition">'+esc(l.detail)+'</td></tr>'}).join('')+'</tbody></table>'+(ranked.length?'':'<p class="empty">No open items in this snapshot.</p>')+'</section>';
 h+='<aside class="fabric-notes"><div class="section-heading"><h2>Needs attention</h2><span>'+att.length+'</span></div>'+(att.length?att.slice(0,4).map(function(a){return '<article><span class="chip sev-'+esc(a.severity)+'">'+esc(a.severity)+'</span><h3>'+esc(a.title)+'</h3><p>'+esc(a.detail)+'</p>'+(a.agent?'<button class="btn" data-inspect="'+esc(a.agent)+'">Inspect lane</button>':'')+nextStep(a.next_step||a.next)+'</article>'}).join(''):'<p class="sub">No attention conditions in this snapshot.</p>')+'</aside></div>';
 h+='<footer class="fabric-provenance"><span>Received '+esc(d.generated_at||'time unavailable')+'</span><span>'+esc(d.version||'Build unavailable')+' · '+esc(d.built_ms??'unknown')+' ms collection</span><span>Read-only · no competing local store</span></footer>';return h;
}
function posture(t,v,s,ok){return '<div class="card kpi"><h2>'+esc(t)+'</h2><div style="font-size:20px;font-weight:650;letter-spacing:-.02em;color:'+(ok?'var(--ink)':'var(--warn)')+'">'+esc(v)+'</div><div class="s">'+esc(s)+'</div></div>'}
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
 if(items.length!==all)return h+'<p role="alert">Snapshot mismatch: '+all+' counted items but '+items.length+' records. Refresh before relying on this queue.</p>';
 var rows=items.filter(function(x){return state.recipient==='*'||x.to===state.recipient});
 h+='<p class="sub">Showing '+rows.length+' of '+all+' open items'+(state.recipient==='*'?'':' for '+esc(state.recipient))+'. Oldest first. Received '+esc(d.generated_at)+'.</p>';
 return h+(rows.length?'<table class="table responsive-table"><thead><tr><th>Request</th><th>From → recipient</th><th>Opened</th><th>State</th></tr></thead><tbody>'+rows.map(function(x){return '<tr><td data-label="Request"><button class="lane-select" data-item="'+esc(x.id)+'">'+esc(x.title||x.id)+'</button><div class="dim mono">'+esc(x.id)+'</div></td><td data-label="Routing">'+esc(x.from)+' → '+esc(x.to)+'</td><td data-label="Opened">'+esc(x.opened||'Unknown')+'</td><td data-label="State">'+(x.blocked_by?'Blocked by '+esc(x.blocked_by):x.acked_at?'Acknowledged':'Awaiting acknowledgment')+'</td></tr>'}).join('')+'</tbody></table>':'<p class="empty">No open items for this selection.</p>');
}
var drafts={},ownerSession=null;
function itemInspector(id){
 var x=(state.data.items||[]).find(function(x){return x.id===id});if(!x)return;
 var draft=drafts[id]||{action:'reply',message:'',recipient:''};
 var recipients=(state.data.recipients||[]).filter(function(a){return a!==x.to&&a!=='user'});
 showInspector(x.title||x.id,'<p class="mono">'+esc(x.id)+'</p><p>'+esc(x.from)+' → '+esc(x.to)+'</p><p class="sub">Opened '+esc(x.opened)+' · '+esc(x.type||'Request')+'</p>'+
 '<form class="owner-composer" data-owner-form data-owner-item="'+esc(x.id)+'" data-revision="'+esc(x.revision)+'"><h3>Act as owner</h3><label for="owner-action">Action</label><select id="owner-action" name="action"><option value="reply">Reply — keep request open</option><option value="assign">Assign — move this request</option><option value="close">Close — record a disposition</option></select><div data-assignment hidden><label for="owner-recipient">Assign to</label><select id="owner-recipient" name="recipient"><option value="">Choose a lane</option>'+recipients.map(function(a){return '<option value="'+esc(a)+'">'+esc(a)+'</option>'}).join('')+'</select></div><label for="owner-message">Response / assignment note / closing disposition</label><textarea id="owner-message" name="message" maxlength="32768" rows="5" required placeholder="Write your response or explain the disposition…">'+esc(draft.message)+'</textarea><p class="sub" data-action-help></p><p class="action-error" data-action-status role="status" aria-live="polite"></p><button type="submit" class="btn primary" data-owner-submit>Send reply</button></form><details class="request-source" open><summary>Original request</summary><pre class="message-body">'+esc(x.instructions||'No request body supplied.')+'</pre></details>',true);
 var f=$('[data-owner-form]');f.elements.action.value=draft.action;f.elements.recipient.value=draft.recipient;
 function mode(){var a=f.elements.action.value;f.querySelector('[data-assignment]').hidden=a!=='assign';f.elements.recipient.required=a==='assign';f.querySelector('[data-owner-submit]').textContent=a==='reply'?'Send reply':a==='assign'?'Assign request':'Close item';f.querySelector('[data-action-help]').textContent=a==='reply'?'Creates a fresh owner response in '+x.from+'’s inbox. This request stays open.':a==='assign'?'Moves the same item and retains its history. A claimed worker item cannot be reassigned.':'Closes this item with your recorded disposition. It does not certify a build, review or release.';}
 mode();f.addEventListener('input',function(){drafts[id]={action:f.elements.action.value,message:f.elements.message.value,recipient:f.elements.recipient.value}});f.elements.action.addEventListener('change',mode);
 f.addEventListener('submit',async function(e){
  e.preventDefault();if(f.dataset.busy==='true'||!f.reportValidity())return;
  f.dataset.busy='true';Array.from(f.elements).forEach(function(el){el.disabled=true});
  var status=f.querySelector('[data-action-status]');status.textContent='Submitting to the canonical router…';
  var req={action:f.elements.action.value,item_id:id,expected:f.dataset.revision,recipient:f.elements.recipient.value,message:f.elements.message.value};
  try{
   ownerSession=ownerSession||await get('/api/router/v1/owner-session');
   var response=await fetch('/api/router/v1/action',{method:'POST',headers:{'Content-Type':'application/json','X-Sirsi-Owner-Token':ownerSession.token},body:JSON.stringify(req),signal:AbortSignal.timeout(30000)});
   var result=await response.json();if(!response.ok){if(response.status===403)ownerSession=null;throw new Error(result.error||('HTTP '+response.status))}
   delete drafts[id];
   var message=result.action==='reply'?'Reply delivered to '+result.recipient+' · '+result.response_id:result.action==='assign'?'Assigned to '+result.recipient+'; original item history retained.':'Item closed with your disposition.';
   if(result.action==='reply'){f.elements.message.value='';status.className='action-success';status.textContent=message;await load()}
   else{var dlg=$('.inspector');if(dlg)dlg.close();await load();$('#announcement').textContent=message;var notice=document.createElement('p');notice.className='action-success operation-notice';notice.setAttribute('role','status');notice.textContent=message;$('#view').prepend(notice)}
  }catch(err){status.className='action-error';status.textContent=err.name==='TimeoutError'?'Result not confirmed. Your draft is preserved. Refresh the queue and check router records before retrying.':err.message;}
  finally{f.dataset.busy='false';Array.from(f.elements).forEach(function(el){el.disabled=false});}
 });
}
function vReleases(d){return '<div class="card">'+(d.releases||[]).map(function(x,i){return '<details class="release" data-release="'+i+'"><summary><strong>'+esc(x.version)+'</strong><span class="sub">'+esc(x.date||'Date not supplied')+' · '+plural((x.items||[]).length,'entry','entries')+'</span><p>'+esc((x.items||[])[0]||'No changes recorded for this release.')+'</p></summary><ul>'+(x.items||[]).map(function(t){return '<li>'+esc(t)+'</li>'}).join('')+'</ul></details>'}).join('')+'</div>'}
function vFailures(d){
 var k=d.known_failures||[];
 return '<p class="sub" style="margin-bottom:16px">A recurring failure is registered once with its signature and cause. The fix records the release it shipped in and a regression test that must exist. The wake loop recognizes the failure next time instead of waiting for a person.</p><table class="table responsive-table"><thead><tr><th>Failure</th><th>Status</th><th>Fixed in</th><th>Regression guard</th></tr></thead><tbody>'+k.map(function(x){return '<tr><td data-label="Failure"><button class="lane-select" data-failure="'+esc(x.id)+'">'+esc(x.id)+'</button><div class="dim">'+esc(x.title)+'</div></td><td data-label="Status"><span class="chip st-'+esc(x.status)+'">'+esc(x.status)+'</span></td><td data-label="Fixed in" class="mono">'+esc(x.fixed_in||'—')+'</td><td data-label="Regression guard" class="mono">'+esc(x.guard||'—')+'</td></tr>'}).join('')+'</tbody></table>';
}
var fleetSelection='lanes';
function fleetMetric(key,title,value,detail){return '<button type="button" class="card kpi fleet-metric" data-fleet="'+key+'" aria-pressed="'+(fleetSelection===key)+'"><span class="metric-title">'+title+'</span><span class="v">'+value+'</span><span class="l">'+detail+'</span><span class="metric-link">View '+title.toLowerCase()+' →</span></button>'}
function fleetTaskRows(f,key){
 if(!Array.isArray(f.tasks))return '<p role="alert">Task details are unavailable from this dashboard version. Counts alone are not a task list.</p>';
 var tasks=f.tasks.filter(function(t){return key==='all'||t.bucket===key});
 var expected=key==='all'?f.summary.total:key==='done'?f.summary.done:f.summary[key];
 if(tasks.length!==expected)return '<p role="alert">Snapshot mismatch: '+esc(expected)+' counted tasks but '+tasks.length+' records. Refresh before relying on this list.</p>';
 return '<p class="sub">'+tasks.length+' matching tasks · '+esc(f.generated_at)+'</p>'+(tasks.length?'<table class="table responsive-table fleet-tasks"><thead><tr><th>Task</th><th>Lane</th><th>Status</th><th>Blocked by</th><th>Updated</th></tr></thead><tbody>'+tasks.map(function(t){return '<tr><td data-label="Task"><button class="lane-select" data-task="'+esc(t.agent)+'" data-task-id="'+esc(t.id)+'">'+esc(t.subject||t.id)+'</button><div class="dim">'+esc(t.id)+'</div></td><td data-label="Lane">'+esc(t.agent)+'</td><td data-label="Status">'+esc(t.status)+'</td><td data-label="Blocked by">'+esc(t.blocked_by||'—')+'</td><td data-label="Updated">'+esc(t.updated||'—')+'</td></tr>'}).join('')+'</tbody></table>':'<p class="empty">No tasks match this selection.</p>');
}
function taskInspector(agent,id){var t=((state.fleet||{}).tasks||[]).find(function(t){return t.agent===agent&&t.id===id});if(!t)return;showInspector(t.subject||t.id,'<dl>'+[['Task',t.id],['Lane',t.agent],['Status',t.status],['Blocked by',t.blocked_by||'No dependency recorded'],['Updated',t.updated||'Unknown']].map(function(x){return '<dt>'+esc(x[0])+'</dt><dd>'+esc(x[1])+'</dd>'}).join('')+'</dl>',true)}
function vFleet(){
 if(state.fleetErr&&!state.fleet)return '<div class="card err-card"><h2>Fleet unavailable</h2>'+esc(state.fleetErr)+'</div>';
 var f=state.fleet;if(!f)return skeleton();
 var s=f.summary||{};
 var h='<div class="grid g4">'+
 fleetMetric('done','Completed',s.pct_done+'%',s.done+' of '+s.total+' tasks')+
 fleetMetric('active','In progress',s.active,s.assigned+' assigned, not started')+
 fleetMetric('blocked','Blocked',s.blocked,'Tasks with recorded blockers')+
 fleetMetric('lanes','Lanes',s.lanes_working+' of '+s.lanes_total,'working · '+(f.lanes||[]).filter(function(l){return l.state==='IDLE_WITH_WORK'}).length+' idle with work')+'</div>';
 h+='<div class="toolbar fleet-filters" role="group" aria-label="Fleet lists">'+[['lanes','Lanes'],['all','All tasks'],['done','Completed'],['active','In progress'],['blocked','Blocked'],['stalled','Stalled / not started']].map(function(x){return '<button type="button" class="btn" data-fleet="'+x[0]+'" aria-pressed="'+(fleetSelection===x[0])+'">'+x[1]+(x[0]==='stalled'?' · '+s.stalled:'')+'</button>'}).join('')+'</div>';
 if(fleetSelection!=='lanes')return h+fleetTaskRows(f,fleetSelection);
 h+='<h2 class="sec">Lanes</h2><table class="table responsive-table fleet-lanes"><thead><tr><th>Lane</th><th>State</th><th class="num">Open tasks</th><th class="num">Inbox</th><th class="num">Blocked</th><th>Last touched</th></tr></thead><tbody>'+(f.lanes||[]).map(function(l){return '<tr><td data-label="Lane" class="mono"><button class="lane-select" data-inspect="'+esc(l.agent)+'">'+esc(l.agent)+'</button>'+'</td><td data-label="State"><span class="chip" style="--c:'+(l.state==='WORKING'?'var(--ok)':l.state==='UNROUTABLE'?'var(--danger)':'var(--warn)')+'">'+esc(l.state.replace(/_/g,' ').toLowerCase())+'</span></td><td data-label="Open tasks" class="num">'+l.open+'</td><td data-label="Inbox" class="num">'+l.inbox+'</td><td data-label="Blocked" class="num">'+l.blocked+'</td><td data-label="Last ledger update" class="dim">'+esc(l.touched_ago||'—')+'</td></tr>'}).join('')+'</tbody></table>';
 return h;
}
function vHost(d){
 var st=state.stats||{},s=d.swap;
 var ram=st.ram_percent!=null?Math.round(st.ram_percent):null;
 return (state.hostErr?'<p class="stale-banner">Host refresh failed · '+esc(state.hostErr)+'</p>':'')+'<div class="grid g2">'+
 '<div class="card kpi with-ring">'+ring(ram||0,ram>85?'var(--danger)':ram>70?'var(--warn)':'var(--emerald)')+'<div><h2>Memory</h2><div class="v">'+(ram==null?'—':ram+'%')+'</div><div class="l">'+(st.used_ram?Math.round(st.used_ram/1073741824*10)/10+' GB used of '+Math.round(st.total_ram/1073741824)+' GB':'')+'</div></div></div>'+
 '<div class="card"><h2>Swap hygiene</h2>'+(s?'<div style="font-size:24px;font-weight:700;letter-spacing:-.02em">'+esc(s.verdict.replace('-',' '))+'</div><p class="sub">'+Math.round(s.used_mib)+' of '+Math.round(s.total_mib)+' MiB allocated · '+s.free_pct+'% free · paging delta '+(s.delta_pages<0?'not measurable yet':s.delta_pages+' pages')+'</p><p style="margin-top:12px">Correctness-only diagnosis: <b>'+(s.correctness_only_ok==null?'unknown':s.correctness_only_ok?'eligible':'not eligible')+'</b><br>Release-timing qualification: <b>'+(s.release_timing_ok==null?'unknown':s.release_timing_ok?'eligible':'not eligible')+'</b></p>'+(s.restart_proposed?'<p style="color:var(--danger);margin-top:12px"><b>A coordinated restart is proposed</b> for the owner and workload owners. Nothing was restarted.</p>':'')+'<p class="sub" style="margin-top:12px">Sampled '+rel(s.at)+'. Allocation is not paging: the verdict comes from swap movement between samples.</p>':'<p class="sub">No receipt yet. Run <span class="mono">sirsi swap-hygiene</span>.</p>')+'</div></div>';
}
var RENDER={overview:vOverview,lanes:vLanes,queue:vQueue,releases:vReleases,failures:vFailures,host:vHost};
function render(){
 var active=document.activeElement, key=active&&active.id, start=active&&active.selectionStart, end=active&&active.selectionEnd;
 var scroll=window.scrollY;
 nav();
 var v=VIEWS.filter(function(x){return x[0]===state.view})[0]||VIEWS[0];
 $('#title').textContent=v[1];$('#subtitle').textContent=v[2];
 $('#foot-ver').textContent=state.data?'Pantheon '+state.data.version:'';
 var el=$('#view');
 if(state.view==='fleet'){el.innerHTML=(state.fleetErr?'<p class="stale-banner">Fleet refresh failed · '+esc(state.fleetErr)+'</p>':'')+vFleet();return}
 if(!state.data){el.innerHTML=state.err?'<div class="card err-card"><h2>Router unavailable</h2>'+esc(state.err)+'</div>':skeleton();return}
 var opened=Array.from(el.querySelectorAll('details[open]')).map(function(x){return x.dataset.release});el.innerHTML=(state.err?'<p class="stale-banner">Retained snapshot · '+esc(state.data.generated_at||'time unknown')+' · '+esc(state.err)+'</p>':'')+RENDER[state.view](state.data);el.querySelectorAll('details').forEach(function(x){x.open=opened.includes(x.dataset.release)});
 if(state.view==='lanes')bindLanes();
 if(key){var replacement=document.getElementById(key);if(replacement){replacement.focus({preventScroll:true});if(start!=null&&replacement.setSelectionRange)replacement.setSelectionRange(start,end)}}
 window.scrollTo(0,scroll);
}
function showInspector(title,body,persistent){
 var old=$('.inspector');if(old){old.close();old.remove()}

 var dlg=document.createElement('dialog');dlg.className='inspector';dlg.setAttribute('aria-label',title);
 dlg.innerHTML='<div class="inspector-head"><h2>'+esc(title)+'</h2><button class="btn" data-close>Close</button></div>'+body;
 var previous=document.activeElement;var previousAgent=previous&&previous.dataset&&previous.dataset.inspect;document.body.appendChild(dlg);if(persistent&&window.matchMedia('(min-width:1200px)').matches){dlg.classList.add('persistent');document.body.classList.add('has-inspector')}
dlg.addEventListener('keydown',function(e){if(e.key==='Escape'){e.preventDefault();dlg.close()}});dlg.querySelector('[data-close]').onclick=function(){dlg.close()};dlg.addEventListener('close',function(){dlg.remove();document.body.classList.remove('has-inspector');if(previous&&previous.isConnected)previous.focus();else if(previousAgent){var target=Array.from(document.querySelectorAll('[data-inspect]')).find(function(x){return x.dataset.inspect===previousAgent});if(target)target.focus();else $('#main').focus()}});if(dlg.classList.contains('persistent'))dlg.show();else dlg.showModal();dlg.querySelector('[data-close]').focus();
}
function failureInspector(id){
 var failure=((state.data||{}).known_failures||[]).find(function(x){return x.id===id});if(!failure)return;
 showInspector('Failure details','<h3>'+esc(failure.id)+'</h3><p>'+esc(failure.title)+'</p><dl>'+[['Status',failure.status],['Fixed in',failure.fixed_in],['Regression guard',failure.guard],['Signature',failure.signature],['Cause',failure.cause],['Resolution',failure.resolution]].map(function(x){return '<dt>'+esc(x[0])+'</dt><dd>'+esc(x[1]??'Not supplied by this snapshot')+'</dd>'}).join('')+'</dl>');
}
function inspector(agent){
 var l=((state.data||{}).lanes||{}).list||[],lane=l.find(function(x){return x.agent===agent});
 showInspector('Lane inspector','<p class="sub">Snapshot '+esc(state.data&&state.data.generated_at||'time unknown')+' · Reopen to see the latest received snapshot.</p><h3>'+esc(agent)+'</h3>'+(lane?chip(lane.verdict)+'<p>'+esc(lane.detail)+'</p><dl>'+[['Open items',lane.open],['Observed',lane.observed_at],['Worker thread',lane.worker_thread_id],['Last report',lane.last_report_at],['Report',lane.last_report_summary]].map(function(x){return '<dt>'+esc(x[0])+'</dt><dd>'+esc(x[1]??'Unknown / not supplied')+'</dd>'}).join('')+'</dl>'+nextStep(lane.next):'<p>No lane projection supplied for this recipient.</p>'),true);
}
document.addEventListener('click',function(e){var b=e.target.closest('[data-fleet],[data-task],[data-queue],[data-item],[data-inspect],[data-copy],[data-failure],[data-view]');if(!b)return;if(b.dataset.fleet){fleetSelection=b.dataset.fleet;render();var selected=$('.fleet-filters [aria-pressed="true"]');if(selected)selected.focus({preventScroll:true})}else if(b.dataset.task)taskInspector(b.dataset.task,b.dataset.taskId);else if(b.dataset.queue){state.recipient=b.dataset.queue;go('queue')}else if(b.dataset.item)itemInspector(b.dataset.item);else if(b.dataset.view)go(b.dataset.view);else if(b.dataset.failure)failureInspector(b.dataset.failure);else if(b.dataset.inspect)inspector(b.dataset.inspect);else if(!navigator.clipboard){$('#announcement').textContent='Clipboard unavailable. Command: '+b.dataset.copy}else navigator.clipboard.writeText(b.dataset.copy).then(function(){$('#announcement').textContent='Command copied.'}).catch(function(){$('#announcement').textContent='Clipboard unavailable. Command: '+b.dataset.copy})});
document.addEventListener('keydown',function(e){var dlg=$('.inspector.persistent');if(e.key==='Escape'&&dlg&&dlg.open&&!e.target.closest('input,textarea,select,[contenteditable]')){e.preventDefault();dlg.close();return}if(e.target.closest('input,textarea,select,dialog')||e.metaKey||e.ctrlKey||e.altKey)return;if(e.key==='r'){e.preventDefault();load()}if(e.key==='/'){e.preventDefault();go('lanes');$('#lq').focus()}});
$('#refresh').addEventListener('click',function(){load()});
window.addEventListener('hashchange',function(){var h=location.hash.slice(1);if(h&&h!==state.view&&VIEWS.some(function(v){return v[0]===h}))go(h)});
var h0=location.hash.slice(1);if(VIEWS.some(function(v){return v[0]===h0}))state.view=h0;
render();load().then(function(){if(state.view==='fleet'&&!state.fleet)loadFleet()});
state.timer=setInterval(function(){if(!document.hidden)load()},30000);
})();
