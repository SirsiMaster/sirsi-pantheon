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

function get(u){return fetch(u,{cache:'no-store'}).then(function(r){return r.json().then(function(j){if(!r.ok)throw new Error(j.error||('HTTP '+r.status));return j})})}
function load(){
 setLive('Updating…','');
 return Promise.all([get('/api/router').catch(function(e){state.err=e.message;return null}),get('/api/stats').catch(function(){return null})]).then(function(a){
  if(a[0]){state.data=a[0];state.err=null;state.at=new Date()}
  state.stats=a[1];
  render();
  if(state.err)setLive('Router unavailable: '+state.err,'err');else setLive('Updated '+state.at.toLocaleTimeString([], {hour:'2-digit',minute:'2-digit',second:'2-digit'}),'');
 });
}
function loadFleet(){return get('/api/fleet').then(function(f){state.fleet=f;state.fleetErr=null}).catch(function(e){state.fleetErr=e.message}).then(render)}
function setLive(t,cls){$('#live-t').textContent=t;$('#live .dot').className='dot'+(cls?' '+cls:'')}

function nav(){
 var n=(state.data&&state.data.attention||[]).filter(function(a){return a.severity==='critical'}).length;
 $('#nav').innerHTML='<div class="nav-label">Router</div>'+VIEWS.map(function(v){
  return '<button type="button" data-v="'+v[0]+'"'+(state.view===v[0]?' aria-current="page"':'')+'>'+ICON[v[0]]+'<span>'+v[1]+'</span>'+(v[0]==='overview'&&n?'<span class="count" aria-label="'+n+' critical">'+n+'</span>':'')+'</button>'}).join('');
 $('#nav').querySelectorAll('button').forEach(function(b){b.addEventListener('click',function(){go(b.dataset.v)})});
}
function go(v){state.view=v;location.hash=v;render();if(v==='fleet'&&!state.fleet)loadFleet();$('#main').focus({preventScroll:true})}

function skeleton(){return '<div class="grid g4"><div class="skel"></div><div class="skel"></div><div class="skel"></div><div class="skel"></div></div>'}
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

function vOverview(d){
 var lanes=d.lanes.list||[],ok=(d.lanes.counts.LIVE||0)+(d.lanes.counts.WAKEABLE||0);
 var q=d.queue||[],qt=q.reduce(function(s,x){return s+x.open},0);
 var kf=d.known_failures||[],kr=kf.filter(function(k){return k.status==='resolved'}).length;
 var s=d.swap,sw=s?Math.round(s.used_mib/Math.max(1,s.total_mib)*100):0,swc=s&&s.verdict==='pressure'?'var(--danger)':s&&s.verdict==='active-paging'?'var(--warn)':'var(--ok)';
 var att=d.attention||[];
 var h='<div class="grid g4">'+
 '<div class="card kpi"><h2>Lanes working</h2><div class="v">'+ok+' <span style="font-size:20px;color:var(--dim)">of '+lanes.length+'</span></div><div class="l">can take work now</div><div class="s">'+stackBar(d.lanes.counts)+'</div></div>'+
 '<div class="card kpi"><h2>Open queue</h2><div class="v">'+qt+'</div><div class="l">items across '+q.length+' lanes</div><div class="s">'+esc(q[0]?q[0].agent+' holds the most ('+q[0].open+')':'empty')+'</div></div>'+
 '<div class="card kpi with-ring">'+ring(d.consumers.max?d.consumers.running/d.consumers.max*100:0,'var(--emerald)')+'<div><h2>Consumers</h2><div class="v">'+d.consumers.running+'<span style="font-size:20px;color:var(--dim)"> / '+d.consumers.max+'</span></div><div class="l">headless sessions running</div></div></div>'+
 '<div class="card kpi with-ring">'+(s?ring(sw,swc):ring(0,'var(--dim)'))+'<div><h2>Swap</h2><div class="v" style="font-size:24px">'+(s?esc(s.verdict.replace('-',' ')):'no receipt')+'</div><div class="l">'+(s?Math.round(s.used_mib)+' of '+Math.round(s.total_mib)+' MiB · '+s.free_pct+'% free':'run sirsi swap-hygiene')+'</div></div></div>'+
 '</div>';
 h+='<h2 class="sec">Needs attention</h2>';
 if(!att.length)h+='<div class="ok-banner">'+ICON.check+'<div><b>Nothing needs attention.</b><div class="sub">Every lane that has work can reach a worker, the registry is pinned, and no failure is unresolved.</div></div></div>';
 else h+='<div class="attn">'+att.map(function(a){return '<div class="row sev-'+esc(a.severity)+'">'+(a.severity==='info'?ICON.info:ICON.warn)+'<div><b>'+esc(a.title)+'</b><span>'+esc(a.detail)+'</span>'+(a.action?'<em>→ '+esc(a.action)+'</em>':'')+'</div></div>'}).join('')+'</div>';
 h+='<h2 class="sec">Posture</h2><div class="grid g4">'+
 posture('Registry',d.registry.pinned?'Pinned to origin/main':'Reading a working tree',d.registry.pinned?(d.registry.commit||'').slice(0,12)+' · fetched '+rel(d.registry.fetched_at):'any branch can change identity',d.registry.pinned)+
 posture('Known failures',kr+' of '+kf.length+' resolved','each with a fix release and a guard test',kr===kf.length)+
 posture('Version',d.version,'installed on this Mac',true)+
 posture('Releases','See what shipped','latest: '+esc(((d.releases||[]).filter(function(r){return r.date})[0]||{}).version||'—'),true)+'</div>';
 return h;
}
function posture(t,v,s,ok){return '<div class="card kpi"><h2>'+esc(t)+'</h2><div style="font-size:20px;font-weight:650;letter-spacing:-.02em;color:'+(ok?'var(--ink)':'var(--warn)')+'">'+esc(v)+'</div><div class="s">'+esc(s)+'</div></div>'}
function rel(t){if(!t)return '—';var m=Math.round((Date.now()-new Date(t).getTime())/60000);return m<1?'just now':m<60?m+' min ago':m<1440?Math.round(m/60)+' h ago':Math.round(m/1440)+' d ago'}

function vLanes(d){
 var L=state.lane,list=(d.lanes.list||[]).filter(function(l){return (L.filter==='ALL'||l.verdict===L.filter)&&(!L.q||(l.agent+' '+l.detail).toLowerCase().indexOf(L.q)>-1)});
 list.sort(function(a,b){var x=a[L.sort],y=b[L.sort];return (typeof x==='number'?x-y:String(x).localeCompare(String(y)))*L.dir});
 var counts=d.lanes.counts,keys=['ALL'].concat(ORDER.filter(function(k){return counts[k]}));
 var h='<div class="toolbar"><div class="seg" role="group" aria-label="Filter">'+keys.map(function(k){return '<button type="button" data-f="'+k+'" aria-pressed="'+(L.filter===k)+'">'+(k==='ALL'?'All '+(d.lanes.list||[]).length:k.replace('_',' ').toLowerCase()+' '+counts[k])+'</button>'}).join('')+'</div><input class="search" id="lq" type="search" placeholder="Search lanes" aria-label="Search lanes" value="'+esc(L.q)+'"></div>';
 h+='<table class="table"><thead><tr><th data-s="agent">Lane</th><th data-s="verdict">Status</th><th class="num" data-s="open">Open</th><th>Detail</th></tr></thead><tbody>'+
  (list.length?list.map(function(l){return '<tr><td class="mono">'+esc(l.agent)+'</td><td>'+chip(l.verdict)+'</td><td class="num">'+(l.open||'—')+'</td><td class="dim">'+esc(l.detail)+'</td></tr>'}).join(''):'<tr><td colspan="4" class="empty">No lanes match.</td></tr>')+'</tbody></table>';
 return h;
}
function bindLanes(){
 document.querySelectorAll('[data-f]').forEach(function(b){b.addEventListener('click',function(){state.lane.filter=b.dataset.f;render()})});
 document.querySelectorAll('th[data-s]').forEach(function(t){t.addEventListener('click',function(){var L=state.lane;if(L.sort===t.dataset.s)L.dir*=-1;else{L.sort=t.dataset.s;L.dir=1}render()})});
 var q=$('#lq');if(q)q.addEventListener('input',function(){state.lane.q=q.value.toLowerCase();var p=q.selectionStart;render();var n=$('#lq');n.focus();n.setSelectionRange(p,p)});
}
function vQueue(d){
 var q=d.queue||[],max=Math.max.apply(null,q.map(function(x){return x.open}).concat([1]));
 if(!q.length)return '<div class="card empty">The queue is empty.</div>';
 return '<div class="card"><h2>Open items by recipient</h2>'+q.map(function(x){return '<div class="hbar"><span class="mono">'+esc(x.agent)+'</span><div class="t"><span style="width:'+(x.open/max*100)+'%"></span></div><span class="n">'+x.open+'</span></div>'}).join('')+'</div>';
}
function vReleases(d){
 var r=d.releases||[];if(!r.length)return '<div class="card empty">No changelog available.</div>';
 return '<div class="card">'+r.map(function(x){return '<div class="release"><div><h3>'+esc(x.version)+'</h3><small>'+(x.date?esc(x.date):'not yet released')+'</small></div><ul>'+((x.items||[]).map(function(i){var m=i.split(' — ');return '<li>'+(m.length>1?'<b>'+esc(m[0])+'</b> — '+esc(m.slice(1).join(' — ')):esc(i))+'</li>'}).join('')||'<li>No entries.</li>')+'</ul></div>'}).join('')+'</div>';
}
function vFailures(d){
 var k=d.known_failures||[];
 return '<p class="sub" style="margin-bottom:16px">A recurring failure is registered once with its signature and cause. The fix records the release it shipped in and a regression test that must exist. The wake loop recognizes the failure next time instead of waiting for a person.</p><table class="table"><thead><tr><th>Failure</th><th>Status</th><th>Fixed in</th><th>Regression guard</th></tr></thead><tbody>'+k.map(function(x){return '<tr><td><b>'+esc(x.id)+'</b><div class="dim">'+esc(x.title)+'</div></td><td><span class="chip st-'+esc(x.status)+'">'+esc(x.status)+'</span></td><td class="mono">'+esc(x.fixed_in||'—')+'</td><td class="mono">'+esc(x.guard||'—')+'</td></tr>'}).join('')+'</tbody></table>';
}
function vFleet(){
 if(state.fleetErr)return '<div class="card err-card"><h2>Fleet unavailable</h2>'+esc(state.fleetErr)+'</div>';
 var f=state.fleet;if(!f)return skeleton();
 var s=f.summary||{};
 var h='<div class="grid g4">'+
 '<div class="card kpi"><h2>Completed</h2><div class="v">'+s.pct_done+'%</div><div class="l">'+s.done+' of '+s.total+' tasks</div></div>'+
 '<div class="card kpi"><h2>In progress</h2><div class="v">'+s.active+'</div><div class="l">'+s.assigned+' assigned, not started</div></div>'+
 '<div class="card kpi"><h2>Blocked</h2><div class="v">'+s.blocked+'</div><div class="l">'+s.stalled+' more stalled</div></div>'+
 '<div class="card kpi"><h2>Lanes</h2><div class="v">'+s.lanes_working+' <span style="font-size:20px;color:var(--dim)">of '+s.lanes_total+'</span></div><div class="l">working · '+s.idle_lanes+' idle with work</div></div></div>';
 h+='<h2 class="sec">Lanes</h2><table class="table"><thead><tr><th>Lane</th><th>State</th><th class="num">Open</th><th class="num">Inbox</th><th class="num">Blocked</th><th>Last touched</th></tr></thead><tbody>'+(f.lanes||[]).map(function(l){return '<tr><td class="mono">'+esc(l.agent)+'</td><td><span class="chip" style="--c:'+(l.state==='WORKING'?'var(--ok)':l.state==='UNROUTABLE'?'var(--danger)':'var(--warn)')+'">'+esc(l.state.replace(/_/g,' ').toLowerCase())+'</span></td><td class="num">'+l.open+'</td><td class="num">'+l.inbox+'</td><td class="num">'+l.blocked+'</td><td class="dim">'+esc(l.touched_ago||'—')+'</td></tr>'}).join('')+'</tbody></table>';
 return h;
}
function vHost(d){
 var st=state.stats||{},s=d.swap;
 var ram=st.ram_percent!=null?Math.round(st.ram_percent):null;
 return '<div class="grid g2">'+
 '<div class="card kpi with-ring">'+ring(ram||0,ram>85?'var(--danger)':ram>70?'var(--warn)':'var(--emerald)')+'<div><h2>Memory</h2><div class="v">'+(ram==null?'—':ram+'%')+'</div><div class="l">'+(st.used_ram?Math.round(st.used_ram/1073741824*10)/10+' GB used of '+Math.round(st.total_ram/1073741824)+' GB':'')+'</div></div></div>'+
 '<div class="card"><h2>Swap hygiene</h2>'+(s?'<div style="font-size:24px;font-weight:700;letter-spacing:-.02em">'+esc(s.verdict.replace('-',' '))+'</div><p class="sub">'+Math.round(s.used_mib)+' of '+Math.round(s.total_mib)+' MiB allocated · '+s.free_pct+'% free · paging delta '+(s.delta_pages<0?'not measurable yet':s.delta_pages+' pages')+'</p><p style="margin-top:12px">Correctness-only diagnosis: <b>'+(s.correctness_only_ok?'eligible':'not eligible')+'</b><br>Release-timing qualification: <b>'+(s.release_timing_ok?'eligible':'not eligible')+'</b></p>'+(s.restart_proposed?'<p style="color:var(--danger);margin-top:12px"><b>A coordinated restart is proposed</b> for the owner and workload owners. Nothing was restarted.</p>':'')+'<p class="sub" style="margin-top:12px">Sampled '+rel(s.at)+'. Allocation is not paging: the verdict comes from swap movement between samples.</p>':'<p class="sub">No receipt yet. Run <span class="mono">sirsi swap-hygiene</span>.</p>')+'</div></div>';
}
var RENDER={overview:vOverview,lanes:vLanes,queue:vQueue,releases:vReleases,failures:vFailures,host:vHost};
function render(){
 nav();
 var v=VIEWS.filter(function(x){return x[0]===state.view})[0]||VIEWS[0];
 $('#title').textContent=v[1];$('#subtitle').textContent=v[2];
 $('#foot-ver').textContent=state.data?'Pantheon '+state.data.version:'';
 var el=$('#view');
 if(state.view==='fleet'){el.innerHTML=vFleet();return}
 if(!state.data){el.innerHTML=state.err?'<div class="card err-card"><h2>Router unavailable</h2>'+esc(state.err)+'</div>':skeleton();return}
 el.innerHTML=RENDER[state.view](state.data);
 if(state.view==='lanes')bindLanes();
}
$('#refresh').addEventListener('click',function(){load();if(state.view==='fleet')loadFleet()});
window.addEventListener('hashchange',function(){var h=location.hash.slice(1);if(h&&h!==state.view&&VIEWS.some(function(v){return v[0]===h}))go(h)});
var h0=location.hash.slice(1);if(VIEWS.some(function(v){return v[0]===h0}))state.view=h0;
render();load().then(function(){if(state.view==='fleet'&&!state.fleet)loadFleet()});
state.timer=setInterval(function(){if(!document.hidden)load()},30000);
})();
