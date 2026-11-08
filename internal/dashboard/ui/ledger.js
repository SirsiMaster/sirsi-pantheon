/* Pure rendering for the shared ledger language (CompassPoint / Apollo Rail ledgers).
   No DOM, no fetch, no clock: every function takes its data and `now`. Colors are CSS
   classes (r-<role>) bound to brand tokens in app.css; a state is always a WORD in a pill. */
(function(root){
'use strict';
var ORDER=['done','in-progress','pending','blocked'];
var ROLE={done:'ok',settled:'settled','in-progress':'warn',pending:'info',blocked:'danger',unknown:'dim'};
var SEV={critical:'danger',warn:'warn',info:'info'};
var VERDICT={LIVE:'ok',WAKEABLE:'ok',HELD:'warn',WATCH_ONLY:'info',AUTH_REQUIRED:'danger',UNREACHABLE:'danger',UNSTAFFED:'dim'};
var PRODUCER_ROLES=['ok','gold','warn','ink2','dim'];
var JOB_ORDER=['building','qualifying','waiting','reported','withdrawn'];
var JOB_ROLE={building:'warn',qualifying:'warn',waiting:'info',reported:'ok',withdrawn:'dim'};
var INTERVAL_MS=10000,STALE_MS=2*INTERVAL_MS;
function has(o,k){return Object.prototype.hasOwnProperty.call(o,k)}
function esc(s){return String(s==null?'':s).replace(/[&<>"']/g,function(c){return {'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]})}
function role(v){return has(ROLE,v)?ROLE[v]:'dim'}
function pill(word,r){return '<span class="pill r-'+esc(r||'dim')+'">'+esc(word||'unknown')+'</span>'}
function producerRole(r){return PRODUCER_ROLES.indexOf(r)>-1?r:'dim'}
function view(t){return t&&t.status?String(t.status):'unknown'}
function count(tasks){return (tasks||[]).reduce(function(a,t){var v=view(t);a[v]=(a[v]||0)+1;return a},{})}
function sum(segs){return segs.reduce(function(n,s){return n+s.n},0)}
// Stacked bar. Every segment carries its word (tooltip) and the whole bar carries the numbers (aria-label).
function bar(segs,cls){
 var live=segs.filter(function(s){return s.n>0}),total=sum(live);
 var label=live.length?live.map(function(s){return s.word+' '+s.n}).join(', '):'no data';
 return '<div class="bar'+(cls?' '+cls:'')+'" role="img" aria-label="'+esc(label)+'">'+live.map(function(s){return '<span class="r-'+esc(s.role)+'" style="width:'+(100*s.n/total).toFixed(2)+'%" title="'+esc(s.word+' '+s.n)+'"></span>'}).join('')+'</div>';
}
// Box row (the owner's x/y format): done boxes fill first, then settled, the rest stay empty outlines.
function boxes(c,total,n){
 n=n||13;total=total||0;
 var d=c.done||0,s=c.settled||0,fd=total?Math.round(n*d/total):0,fs=total?Math.round(n*(d+s)/total)-fd:0;
 if(d&&!fd)fd=1;if(s&&fs<1)fs=1;
 var out='';for(var i=0;i<n;i++)out+=i<fd?'<i class="r-ok on"></i>':i<fd+fs?'<i class="r-settled on"></i>':'<i></i>';
 return '<span class="boxes" role="img" aria-label="'+esc(d+' done, '+s+' settled of '+total)+'">'+out+'</span>';
}
function phaseHead(name,pc,total){
 var b=pc.blocked||0;
 return '<h2 class="ph'+(b?' has-block':'')+'">'+boxes(pc,total)+'<span class="xy">'+(pc.done||0)+'/'+total+'</span><span class="name">'+esc(name)+'</span>'+
  (pc.settled?'<span class="tally">'+pc.settled+' settled</span>':'')+(b?'<span class="tally r-danger">'+b+' blocked</span>':'')+'</h2>';
}
function viewSegs(c){return ORDER.map(function(k){return {word:k,n:c[k]||0,role:role(k)}}).concat(Object.keys(c).filter(function(k){return ORDER.indexOf(k)<0}).map(function(k){return {word:k,n:c[k],role:'dim'}}))}
function cards(list){return '<div class="cards">'+list.map(function(c){return '<div class="card count"><div class="n'+(c[2]?' r-'+esc(c[2]):'')+'">'+esc(c[1])+'</div><div class="l">'+esc(c[0])+'</div></div>'}).join('')+'</div>'}
function heading(text,o){o=o||{};return '<h2>'+esc(text)+(o.segs?bar(o.segs,'mini'):'')+(o.total!=null?'<span class="tally">'+o.done+'/'+o.total+'</span>':'')+(o.note?'<span class="tally">'+esc(o.note)+'</span>':'')+(o.extra||'')+'</h2>'}
function ageText(sec){
 if(!Number.isFinite(sec)||sec<0)return 'age unknown';
 var m=Math.round(sec/60);
 return m<1?'under a minute old':m<60?m+' min old':m<1440?Math.round(m/60)+' h old':Math.round(m/1440)+' d old';
}
function ago(iso,now){
 var t=Date.parse(iso);if(!iso||!Number.isFinite(t))return 'unknown';
 var m=Math.round((now-t)/60000);
 return m<1?'just now':m<60?m+' min ago':m<1440?Math.round(m/60)+' h ago':Math.round(m/1440)+' d ago';
}
function clock(ms){return new Date(ms).toLocaleTimeString('en-US',{timeZone:'America/New_York'})+' ET'}
function banner(r,title,detail,alert){return '<div class="banner b-'+r+'" role="'+(alert?'alert':'status')+'"><b>'+esc(title)+'</b> '+esc(detail)+'</div>'}
function muted(t){return '<p class="muted">'+esc(t)+'</p>'}
// One row of the shared table: pill, mono id, text with muted sub-lines. Narrow screens hide the id
// column and show .rid (the id) as the first muted line instead (CSS only).
function row(o){
 var subs=(o.subs||[]).map(function(s){return '<span class="sub-line'+(s.c?' '+s.c:'')+'">'+s.t+'</span>'}).join('');
 return '<tr'+(o.changed?' class="changed"':'')+'><td class="s">'+o.pill+'</td><td class="id"><span class="idv" title="'+o.rid+'">'+o.id+'</span></td><td class="t">'+o.text+'<span class="sub-line rid" title="'+o.rid+'">'+o.rid+'</span>'+subs+'</td></tr>';
}
function table(rows,cls){return '<table class="rows'+(cls?' '+cls:'')+'"><tbody>'+rows.join('')+'</tbody></table>'}
function laneLink(agent){return '<a class="lane-link" data-lane="'+esc(agent)+'" href="#lane/'+encodeURIComponent(agent)+'">'+esc(agent)+'</a>'}

// Header status line: healthy, stale (no fresh data for more than two refresh intervals) and failed
// refresh are three different statements; none of them is ever rendered as an empty page.
function status(o){
 if(!o.at){return o.err?{text:'Router unavailable · '+o.err,cls:'err'}:{text:'Loading…',cls:'stale'}}
 if(o.err)return {text:'showing last good data from '+clock(o.at)+' · '+o.err,cls:'err'};
 var age=o.now-o.at;
 if(age>STALE_MS)return {text:'stale · last update '+Math.round(age/1000)+' s ago ('+clock(o.at)+')',cls:'stale'};
 return {text:'live · updated '+clock(o.at)+' · refreshes every '+INTERVAL_MS/1000+' s',cls:''};
}

function inboxRows(items,o){
 return items.map(function(i){
  var title=o.inspect?'<button class="link" data-item="'+esc(i.id)+'">'+esc(i.title||i.id)+'</button>':esc(i.title||i.id);
  var age=i.age_seconds!=null?ageText(i.age_seconds):i.opened?ageText((o.now-Date.parse(i.opened))/1000):'age unknown';
  var subs=[{t:'from '+esc(i.from||'unknown')+(i.to&&o.showTo?' → '+esc(i.to):'')+' · '+esc(age),c:'dim'}];
  var dep=i.blocked_by;
  if(dep)subs.push({t:'waits on: '+esc(dep),c:'wait'});
  else if(i.blocked)subs.push({t:'blocked by an open dependency',c:'wait'});
  if(o.inspect)subs.push({t:i.acked_at?'acknowledged':'awaiting acknowledgment',c:'dim'});
  return row({pill:pill(i.type||'item','info'),id:esc(i.id),rid:esc(i.id),text:title,subs:subs,changed:o.changed&&o.changed(o.scope,i.id,[i.title,i.blocked_by,i.blocked,i.acked_at].join('|'))});
 });
}

function waiting(snap){
 var q=snap.queue||[],lanes=(snap.lanes||{}).list||[];
 return q.filter(function(x){return x.open>0}).map(function(x){
  var l=lanes.find(function(l){return l.agent===x.agent});
  return Object.assign({},l||{agent:x.agent,verdict:'UNSTAFFED',detail:'Recipient is outside the lane registry.'},{open:x.open});
 }).sort(function(a,b){return b.open-a.open||a.agent.localeCompare(b.agent)});
}
function nextLine(n){
 if(!n)return null;
 var t=esc(n.label||'')+(n.kind==='command-copy'&&n.command?' <code>'+esc(n.command)+'</code> <button class="link" data-copy="'+esc(n.command)+'">Copy command</button>':'');
 return t?{t:t,c:''}:null;
}
// Overview body: needs attention, where work is waiting, lanes. The count cards come from fabric.js.
function overview(snap,fabric,o){
 o=o||{};
 var att=snap.attention||[],ranked=waiting(snap),open=ranked.reduce(function(n,x){return n+x.open},0),h='';
 h+=heading('Needs attention',{note:att.length+' open'});
 h+=att.length?table(att.map(function(a){
  var subs=[{t:esc(a.detail),c:'dim'}],nx=nextLine(a.next_step||a.next);
  if(nx)subs.push(nx);
  if(a.agent)subs.push({t:'<button class="link" data-inspect="'+esc(a.agent)+'">Inspect lane</button>',c:''});
  return row({pill:pill(a.severity,SEV[a.severity]||'dim'),id:esc(a.id||''),rid:esc(a.id||''),text:esc(a.title),subs:subs});
 })):muted('Nothing needs attention in this snapshot.');
 h+=heading('Where work is waiting',{note:open+' open items',extra:'<button class="link" data-queue="*">View all '+open+' items</button>'});
 h+=ranked.length?table(ranked.map(function(l){
  return row({pill:pill(String(l.verdict||'unknown').replace('_',' ').toLowerCase(),VERDICT[l.verdict]||'dim'),id:laneLink(l.agent),rid:esc(l.agent),
   text:'<button class="link" data-queue="'+esc(l.agent)+'" aria-label="Read '+l.open+' open items for '+esc(l.agent)+'">'+l.open+' open</button>',
   subs:[{t:esc(l.detail),c:'dim'},{t:'<button class="link" data-inspect="'+esc(l.agent)+'">Inspect lane</button>',c:''}]});
 })):muted('No open items in this snapshot.');
 h+=heading('Lanes',{note:laneRows(snap,fabric).length+' lanes'});
 h+=lanes(snap,fabric,o);
 return h;
}
function laneRows(snap,fabric){
 var by=new Map(),out=[];
 ((fabric&&fabric.lanes)||[]).forEach(function(l){var e={agent:l.agent,f:l};by.set(l.agent,e);out.push(e)});
 (((snap.lanes||{}).list)||[]).forEach(function(l){var e=by.get(l.agent);if(e)e.s=l;else{e={agent:l.agent,s:l};by.set(l.agent,e);out.push(e)}});
 return out;
}
function lanes(snap,fabric,o){
 var list=laneRows(snap,fabric);
 if(!list.length)return muted(fabric?'No lanes in this snapshot.':'No lanes in this snapshot; the Fabric observation is not available.');
 return table(list.map(function(e){
  var f=e.f,s=e.s,subs=[];
  var p=f?pill(f.state_label||'unknown',producerRole(f.role)):pill('unknown','dim');
  var line='';
  if(f){
   var rest=Math.max(0,f.open-f.active-f.stalled-f.blocked);
   var segs=[{word:'active',n:f.active,role:'warn'},{word:'stalled',n:f.stalled,role:'warn'},{word:'blocked',n:f.blocked,role:'danger'},{word:'open',n:rest,role:'info'}];
   line=bar(segs,'mini')+'<span class="nums">active '+f.active+' · stalled '+f.stalled+' · blocked '+f.blocked+' · open '+f.open+'</span>';
   subs.push({t:'last touch '+esc(f.last_touch?ago(f.last_touch,o.now):'unknown')+' · runtime '+esc(s?String(s.verdict).replace('_',' ').toLowerCase():'unknown'),c:'dim'});
  }else{
   line='<span class="nums dim">no task counts: this lane is not in the Fabric observation</span>';
   subs.push({t:'runtime '+esc(s?String(s.verdict).replace('_',' ').toLowerCase():'unknown'),c:'dim'});
  }
  if(s&&s.detail)subs.push({t:esc(s.detail),c:'dim'});
  var sig=[f&&f.state_label,f&&f.open,f&&f.active,f&&f.stalled,f&&f.blocked,f&&f.last_touch,s&&s.verdict,s&&s.detail].join('|');
  return row({pill:p,id:laneLink(e.agent),rid:esc(e.agent),text:line,subs:subs,changed:o.changed&&o.changed('lanes',e.agent,sig)});
 }),'lane-rows');
}

function jobsPanel(j,now){
 var h='<h2>Cloud jobs'+(j&&j.state==='ok'?'<span class="tally">'+j.data.length+' reported</span>':'')+'</h2>';
 if(!j||j.state==='loading')return h+muted('Cloud jobs: loading…');
 if(j.state==='none')return h+muted('Cloud jobs: no source connected for this lane');
 if(j.state==='error')return h+'<p class="muted r-danger">Cloud jobs: read failed · '+esc(j.err)+' (not an empty list)</p>';
 var list=j.data.slice().sort(function(a,b){return JOB_ORDER.indexOf(a.activity)-JOB_ORDER.indexOf(b.activity)});
 if(!list.length)return h+muted('Cloud jobs: the source reports none for this lane.');
 return h+table(list.map(function(x){
  var subs=[];
  if(x.last_push)subs.push({t:'last push '+esc(ago(x.last_push,now)),c:'dim'});
  if(x.headline)subs.push({t:'report: '+esc(x.headline),c:'dim'});
  if(x.verdict)subs.push({t:'builder verified: '+esc(x.verdict),c:/not|fail|partial|withdrawn/i.test(x.verdict)?'wait':'dim'});
  return row({pill:pill(x.activity||'unknown',JOB_ROLE[x.activity]||'dim'),id:esc(x.stream),rid:esc(x.stream),text:esc(x.purpose||x.stream),subs:subs});
 }));
}

// The lane ledger: o = {agent, ledger:{data,err,at,notFound}, jobs, now, changed}.
function laneLedger(o){
 var L=o.ledger||{},now=o.now;
 if(L.notFound)return banner('danger','Unknown lane.',o.agent+' is not known to the router. This is not an empty ledger.',true);
 if(!L.data){
  if(L.err)return banner('danger','Ledger unavailable.','Could not read the ledger for '+o.agent+': '+L.err+'. This is a read failure, not an empty ledger.',true);
  return muted('Loading the ledger for '+o.agent+'…');
 }
 var d=L.data,tasks=d.tasks||[],items=d.items||[],c=count(tasks),h='';
 if(L.err)h+=banner('danger','Refresh failed.','Showing last good ledger from '+clock(L.at)+' · '+L.err,true);
 else if(now-L.at>STALE_MS)h+=banner('warn','Stale.','This ledger is '+Math.round((now-L.at)/1000)+' s old (last read '+clock(L.at)+').',false);
 h+=cards([['Complete',(c.done||0)+'/'+tasks.length,'ok'],['Tasks',tasks.length,''],['Done',c.done||0,'ok'],['Running',c['in-progress']||0,'warn'],['Pending',c.pending||0,'info'],['Blocked',c.blocked||0,'danger'],['Inbox',items.length,items.length?'danger':'ok']]);
 h+=bar(viewSegs(c));
 h+='<div class="overall">'+boxes(c,tasks.length,20)+'<b>'+(tasks.length?Math.round(100*(c.done||0)/tasks.length):0)+'% done</b><span>'+
  (c.done||0)+' done · '+((c.pending||0)+(c['in-progress']||0))+' active · '+(c.blocked||0)+' blocked · '+items.length+' router items</span></div>';
 h+=heading('Router inbox',{note:items.length+' open'});
 h+=items.length?table(inboxRows(items,{now:now,changed:o.changed,scope:'lane-inbox:'+o.agent,inspect:false})):muted('Empty: nothing waiting.');
 h+=jobsPanel(o.jobs,now);
 if(!tasks.length)return h+heading('Tasks')+muted('No tasks are registered for this lane.');
 var groups=new Map();
 tasks.forEach(function(t){var p=t.phase||'No phase';if(!groups.has(p))groups.set(p,[]);groups.get(p).push(t)});
 groups.forEach(function(ts,phase){
  var pc=count(ts);
  var sorted=ts.map(function(t,i){return {t:t,i:i}}).sort(function(a,b){
   var x=ORDER.indexOf(view(a.t)),y=ORDER.indexOf(view(b.t));
   return (x<0?ORDER.length:x)-(y<0?ORDER.length:y)||a.i-b.i;
  }).map(function(x){return x.t});
  h+=phaseHead(phase,pc,ts.length);
  h+=table(sorted.map(function(t){
   var v=view(t),subs=[];
   if(t.blocked_by)subs.push({t:'waits on: '+esc(t.blocked_by),c:'wait'});
   var sig=[t.status,t.subject,t.phase,t.blocked_by].join('|');
   return row({pill:pill(v,role(v)),id:esc(t.task_id),rid:esc(t.task_id),text:esc(t.subject),subs:subs,changed:o.changed&&o.changed('lane:'+o.agent,t.task_id,sig)});
  }));
 });
 return h;
}

// Inbox: every open item grouped by recipient. Counts come from the same items, never a second read.
function inbox(items,o){
 var groups=new Map();
 items.forEach(function(i){var k=i.to||'unknown';if(!groups.has(k))groups.set(k,[]);groups.get(k).push(i)});
 return Array.from(groups.entries()).sort(function(a,b){return b[1].length-a[1].length||a[0].localeCompare(b[0])}).map(function(g){
  return heading(g[0],{note:g[1].length+' open',extra:'<span class="actions">'+laneLink(g[0])+' <button class="link" data-inspect="'+esc(g[0])+'">Inspect</button></span>'})+
   table(inboxRows(g[1],{now:o.now,changed:o.changed,scope:'queue',inspect:true,showTo:false}));
 }).join('');
}

var api={ORDER:ORDER,STALE_MS:STALE_MS,INTERVAL_MS:INTERVAL_MS,esc:esc,view:view,count:count,bar:bar,boxes:boxes,phaseHead:phaseHead,cards:cards,pill:pill,heading:heading,ageText:ageText,ago:ago,clock:clock,status:status,
 overview:overview,lanes:lanes,laneLedger:laneLedger,inbox:inbox,jobsPanel:jobsPanel,banner:banner,muted:muted,waiting:waiting,laneLink:laneLink,role:role};
if(typeof module!=='undefined'&&module.exports)module.exports=api;else root.PantheonLedger=api;
})(typeof window!=='undefined'?window:globalThis);
