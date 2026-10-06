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
var VIEWS=[['overview','Overview','Is the router healthy, and what needs attention'],['lanes','Lanes','Can each lane work right now'],['queue','Queue','Open items by recipient'],['releases','Releases','What each release added'],['failures','Known failures','Registered once, fixed with a guard'],['fleet','Fleet','Work in flight across every lane'],['host','Host','Memory, swap and this Mac']];
var state={view:'overview',data:null,fleet:null,stats:null,err:null,at:null,lane:{filter:'ALL',q:'',sort:'agent',dir:1},timer:null};

function get(u){return fetch(u,{cache:'no-store',signal:AbortSignal.timeout(15000)}).then(function(r){return r.json().then(function(j){if(!r.ok)throw new Error(j.error||('HTTP '+r.status));return j})})}
function load(){
 if(state.loading)return state.loading;
 setLive('Refreshing…','stale');
 var router=get('/api/router').then(function(d){state.data=d;state.err=null;state.at=new Date()}).catch(function(e){state.err=e.message}).then(function(){render();setLive(state.err?'Stale / unavailable · '+state.err:'Received '+state.at.toLocaleTimeString(),' '+(state.err?'err':''))});
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
function go(v){state.view=v;location.hash=v;render();if(v==='fleet'&&!state.fleet)loadFleet();$('#main').focus({preventScroll:true})}

function skeleton(){return '<p class="sub loading">Loading the local snapshot… Other views remain available.</p>'+ '<div class="grid g4"><div class="skel"></div><div class="skel"></div><div class="skel"></div><div class="skel"></div></div>'}
function chip(v){return '<span class="chip v-'+esc(v)+'">'+esc(v.replace('_',' ').toLowerCase().replace(/^./,function(c){return c.toUpperCase()}))+'</span>'}
var VCOL={LIVE:'var(--ok)',WAKEABLE:'var(--ok)',HELD:'var(--warn)',AUTH_REQUIRED:'var(--danger)',UNREACHABLE:'var(--danger)',WATCH_ONLY:'var(--info)',UNSTAFFED:'var(--dim)'};
var ORDER=['LIVE','WAKEABLE','HELD','WATCH_ONLY','AUTH_REQUIRED','UNREACHABLE','UNSTAFFED'];

function stackBar(counts){
 var tot=ORDER.reduce(function(s,k){return s+(counts[k]||0)},0)||1;
 var bar=ORDER.filter(function(k){return counts[k]}).map(function(k){return '<span style="flex:'+counts[k]+';--c:'+VCOL[k]+'" title="'+k+' '+counts[k]+'"></span>'}).join('');
 var leg=ORDER.filter(function(k){return counts[k]}).map(function(k){return '<span><i style="--c:'+VCOL[k]+'"></i>'+esc(k.replace('_',' ').toLowerCase())+' '+counts[k]+'</span>'}).join('');
 return '<div class="bar" role="img" aria-label="Lane verdicts">'+bar+'</div><div class="legend">'+leg+'</div>';
}
function ring(pct,color){var c=2*Math.PI*40,d=Math.max(0,Math.min(100,pct))/100*c;return '<svg class="ring" viewBox="0 0 100 100" style="--c:'+color+'" role="img" aria-label="'+Math.round(pct)+' percent"><circle class="bg" cx="50" cy="50" r="40"/><circle class="fg" cx="50" cy="50" r="40" stroke-dasharray="'+d+' '+c+'" transform="rotate(-90 50 50)"/></svg>'}

function nextStep(n){return n&&n.kind==='command-copy'&&n.command?'<button class="btn" data-copy="'+esc(n.command)+'">'+esc(n.label||'Copy command')+'</button>':''}
function vOverview(d){
 var att=(d.attention||[]).slice().sort(function(a,b){return ['critical','warn','info'].indexOf(a.severity)-['critical','warn','info'].indexOf(b.severity)});
 var h='<section class="attention"><div class="eyebrow">Operator attention</div><h2 class="hero-title">'+(att.length?att.length+' reported conditions':'No attention items reported')+'</h2><p class="sub">Snapshot '+esc(d.generated_at||'time unknown')+'</p><div class="attn">'+att.map(function(a){return '<article class="row sev-'+esc(a.severity)+'"><div><span class="chip">'+esc(a.severity)+'</span><h3>'+esc(a.title)+'</h3><p>'+esc(a.detail)+'</p>'+(a.action?'<p class="sub">'+esc(a.action)+'</p>':'')+nextStep(a.next)+(a.agent?'<button class="btn" data-inspect="'+esc(a.agent)+'">Inspect lane</button>':'')+'</div></article>'}).join('')+'</div></section>';
 var counts=(d.lanes||{}).counts||{},q=d.queue||[],c=d.consumers||{};
 h+='<div class="status-strip">'+posture('Live',counts.LIVE??'Unknown','reported live lanes',true)+posture('Wakeable',counts.WAKEABLE??'Unknown','ready to receive work',true)+posture('Queue',q.reduce(function(s,x){return s+x.open},0),'open items',true)+posture('Consumers',(c.running??'Unknown')+' / '+(c.max??'Unknown'),'occupied slots',true)+'</div>';
 h+='<section class="card provenance"><h2>Snapshot provenance</h2><p>Installed '+esc(d.version)+' · Registry '+esc(d.registry&&d.registry.pinned?'pinned':'unconfirmed')+'</p><p class="sub">'+esc(d.registry&&d.registry.commit||'Commit unknown')+' · Producer '+esc(d.built_ms??'unknown')+' ms</p></section>';return h;
}
function posture(t,v,s,ok){return '<div class="card kpi"><h2>'+esc(t)+'</h2><div style="font-size:20px;font-weight:650;letter-spacing:-.02em;color:'+(ok?'var(--ink)':'var(--warn)')+'">'+esc(v)+'</div><div class="s">'+esc(s)+'</div></div>'}
function rel(t){if(!t)return '—';var m=Math.round((Date.now()-new Date(t).getTime())/60000);return m<1?'just now':m<60?m+' min ago':m<1440?Math.round(m/60)+' h ago':Math.round(m/1440)+' d ago'}

function vLanes(d){
 var L=state.lane,list=(d.lanes.list||[]).filter(function(l){return (L.filter==='ALL'||l.verdict===L.filter)&&(!L.q||(l.agent+' '+l.detail).toLowerCase().indexOf(L.q)>-1)});
 list.sort(function(a,b){var x=a[L.sort],y=b[L.sort];return (typeof x==='number'?x-y:String(x).localeCompare(String(y)))*L.dir});
 var counts=d.lanes.counts,keys=['ALL'].concat(ORDER.filter(function(k){return counts[k]}));
 var h='<div class="toolbar"><div class="seg" role="group" aria-label="Filter">'+keys.map(function(k){return '<button type="button" data-f="'+k+'" aria-pressed="'+(L.filter===k)+'">'+(k==='ALL'?'All '+(d.lanes.list||[]).length:k.replace('_',' ').toLowerCase()+' '+counts[k])+'</button>'}).join('')+'</div><input class="search" id="lq" type="search" placeholder="Search lanes" aria-label="Search lanes" value="'+esc(L.q)+'"></div>';
 h+='<table class="table"><thead><tr><th><button class="sort" data-s="agent">Lane ↕</button></th><th><button class="sort" data-s="verdict">Status ↕</button></th><th class="num"><button class="sort" data-s="open">Open ↕</button></th><th>Detail</th></tr></thead><tbody>'+
  (list.length?list.map(function(l){return '<tr><td class="mono"><button class="lane-select" data-inspect="'+esc(l.agent)+'">'+esc(l.agent)+'</button>'+'</td><td>'+chip(l.verdict)+'</td><td class="num">'+(l.open??'Unknown')+'</td><td class="dim">'+esc(l.detail)+'</td></tr>'}).join(''):'<tr><td colspan="4" class="empty">No lanes match.</td></tr>')+'</tbody></table>';
 return h;
}
function bindLanes(){
 document.querySelectorAll('[data-f]').forEach(function(b){b.addEventListener('click',function(){state.lane.filter=b.dataset.f;render()})});
 document.querySelectorAll('[data-s]').forEach(function(t){t.parentElement.setAttribute('aria-sort',state.lane.sort===t.dataset.s?(state.lane.dir===1?'ascending':'descending'):'none');t.addEventListener('click',function(){var L=state.lane;if(L.sort===t.dataset.s)L.dir*=-1;else{L.sort=t.dataset.s;L.dir=1}render()})});
 var q=$('#lq');if(q)q.addEventListener('input',function(){state.lane.q=q.value.toLowerCase();var p=q.selectionStart;render();var n=$('#lq');n.focus();n.setSelectionRange(p,p)});
}
function vQueue(d){
 var q=(d.queue||[]).slice().sort(function(a,b){return b.open-a.open}),max=Math.max.apply(null,q.map(function(x){return x.open}).concat([1]));
 if(!q.length)return '<div class="card empty">The queue is empty.</div>';
 return '<div class="card"><h2>Open items by recipient</h2>'+q.map(function(x){return '<div class="hbar"><span class="mono">'+esc(x.agent)+'</span><div class="t"><span style="width:'+(x.open/max*100)+'%"></span></div><span class="n">'+x.open+'</span></div>'}).join('')+'</div>';
}
function vReleases(d){return '<div class="card">'+(d.releases||[]).map(function(x,i){return '<details class="release" data-release="'+i+'"><summary><strong>'+esc(x.version)+'</strong><span class="sub">'+esc(x.date||'Date not supplied')+' · '+(x.items||[]).length+' entries</span><p>'+esc((x.items||[])[0]||'No entries supplied')+'</p></summary><ul>'+(x.items||[]).map(function(t){return '<li>'+esc(t)+'</li>'}).join('')+'</ul></details>'}).join('')+'</div>'}
function vFailures(d){
 var k=d.known_failures||[];
 return '<p class="sub" style="margin-bottom:16px">A recurring failure is registered once with its signature and cause. The fix records the release it shipped in and a regression test that must exist. The wake loop recognizes the failure next time instead of waiting for a person.</p><table class="table"><thead><tr><th>Failure</th><th>Status</th><th>Fixed in</th><th>Regression guard</th></tr></thead><tbody>'+k.map(function(x){return '<tr><td><button class="lane-select" data-failure="'+esc(x.id)+'">'+esc(x.id)+'</button><div class="dim">'+esc(x.title)+'</div></td><td><span class="chip st-'+esc(x.status)+'">'+esc(x.status)+'</span></td><td class="mono">'+esc(x.fixed_in||'—')+'</td><td class="mono">'+esc(x.guard||'—')+'</td></tr>'}).join('')+'</tbody></table>';
}
function vFleet(){
 if(state.fleetErr&&!state.fleet)return '<div class="card err-card"><h2>Fleet unavailable</h2>'+esc(state.fleetErr)+'</div>';
 var f=state.fleet;if(!f)return skeleton();
 var s=f.summary||{};
 var h='<div class="grid g4">'+
 '<div class="card kpi"><h2>Completed</h2><div class="v">'+s.pct_done+'%</div><div class="l">'+s.done+' of '+s.total+' tasks</div></div>'+
 '<div class="card kpi"><h2>In progress</h2><div class="v">'+s.active+'</div><div class="l">'+s.assigned+' assigned, not started</div></div>'+
 '<div class="card kpi"><h2>Blocked</h2><div class="v">'+s.blocked+'</div><div class="l">'+s.stalled+' more stalled</div></div>'+
 '<div class="card kpi"><h2>Lanes</h2><div class="v">'+s.lanes_working+' <span style="font-size:20px;color:var(--dim)">of '+s.lanes_total+'</span></div><div class="l">working · '+s.idle_lanes+' idle with work</div></div></div>';
 h+='<h2 class="sec">Lanes</h2><table class="table"><thead><tr><th>Lane</th><th>State</th><th class="num">Open</th><th class="num">Inbox</th><th class="num">Blocked</th><th>Last touched</th></tr></thead><tbody>'+(f.lanes||[]).map(function(l){return '<tr><td class="mono"><button class="lane-select" data-inspect="'+esc(l.agent)+'">'+esc(l.agent)+'</button>'+'</td><td><span class="chip" style="--c:'+(l.state==='WORKING'?'var(--ok)':l.state==='UNROUTABLE'?'var(--danger)':'var(--warn)')+'">'+esc(l.state.replace(/_/g,' ').toLowerCase())+'</span></td><td class="num">'+l.open+'</td><td class="num">'+l.inbox+'</td><td class="num">'+l.blocked+'</td><td class="dim">'+esc(l.touched_ago||'—')+'</td></tr>'}).join('')+'</tbody></table>';
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
function showInspector(title,body){
 var dlg=document.createElement('dialog');dlg.className='inspector';dlg.setAttribute('aria-label',title);
 dlg.innerHTML='<div class="inspector-head"><h2>'+esc(title)+'</h2><button class="btn" data-close>Close</button></div>'+body;
 var previous=document.activeElement;document.body.appendChild(dlg);dlg.querySelector('[data-close]').onclick=function(){dlg.close()};dlg.addEventListener('close',function(){dlg.remove();if(previous&&previous.isConnected)previous.focus()});dlg.showModal();
}
function failureInspector(id){
 var failure=((state.data||{}).known_failures||[]).find(function(x){return x.id===id});if(!failure)return;
 showInspector('Failure details','<h3>'+esc(failure.id)+'</h3><p>'+esc(failure.title)+'</p><dl>'+[['Status',failure.status],['Fixed in',failure.fixed_in],['Regression guard',failure.guard],['Signature',failure.signature],['Cause',failure.cause],['Resolution',failure.resolution]].map(function(x){return '<dt>'+esc(x[0])+'</dt><dd>'+esc(x[1]??'Not supplied by this snapshot')+'</dd>'}).join('')+'</dl>');
}
function inspector(agent){
 var l=((state.data||{}).lanes||{}).list||[],lane=l.find(function(x){return x.agent===agent});
 showInspector('Lane inspector','<h3>'+esc(agent)+'</h3>'+(lane?chip(lane.verdict)+'<p>'+esc(lane.detail)+'</p><dl>'+[['Open items',lane.open],['Observed',lane.observed_at],['Worker thread',lane.worker_thread_id],['Last report',lane.last_report_at],['Report',lane.last_report_summary]].map(function(x){return '<dt>'+esc(x[0])+'</dt><dd>'+esc(x[1]??'Unknown / not supplied')+'</dd>'}).join('')+'</dl>'+nextStep(lane.next):'<p>No lane projection supplied for this recipient.</p>'));
}
document.addEventListener('click',function(e){var b=e.target.closest('[data-inspect],[data-copy],[data-failure]');if(!b)return;if(b.dataset.failure)failureInspector(b.dataset.failure);else if(b.dataset.inspect)inspector(b.dataset.inspect);else if(!navigator.clipboard){$('#announcement').textContent='Clipboard unavailable. Command: '+b.dataset.copy}else navigator.clipboard.writeText(b.dataset.copy).then(function(){$('#announcement').textContent='Command copied.'}).catch(function(){$('#announcement').textContent='Clipboard unavailable. Command: '+b.dataset.copy})});
document.addEventListener('keydown',function(e){if(e.target.closest('input,textarea,select,dialog')||e.metaKey||e.ctrlKey||e.altKey)return;if(e.key==='r'){e.preventDefault();load()}if(e.key==='/'){e.preventDefault();go('lanes');$('#lq').focus()}});
$('#refresh').addEventListener('click',function(){load()});
window.addEventListener('hashchange',function(){var h=location.hash.slice(1);if(h&&h!==state.view&&VIEWS.some(function(v){return v[0]===h}))go(h)});
var h0=location.hash.slice(1);if(VIEWS.some(function(v){return v[0]===h0}))state.view=h0;
render();load().then(function(){if(state.view==='fleet'&&!state.fleet)loadFleet()});
state.timer=setInterval(function(){if(!document.hidden)load()},30000);
})();
