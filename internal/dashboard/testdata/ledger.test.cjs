'use strict';
// Pure rendering of the shared ledger language (ui/ledger.js) and its CSS contract.
const {test}=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs');
const L=require('../ui/ledger.js');
const css=fs.readFileSync(__dirname+'/../ui/app.css','utf8');
const NOW=Date.parse('2026-11-06T12:00:00Z');
const T=(id,status,subject,phase,extra)=>Object.assign({task_id:id,status,subject,phase,blocked_by:'',updated:'',responsible_party:'self'},extra);
function ledger(tasks,items){return {data:{generated_at:'2026-11-06T12:00:00Z',agent:'lane-a',items:items||[],tasks},at:NOW-1000,err:null}}
const pillWords=html=>Array.from(html.matchAll(/<span class="pill r-[a-z0-9-]+">([^<]+)<\/span>/g)).map(m=>m[1]);

test('subject text never overrides the authoritative task status',()=>{
 assert.equal(L.view(T('a','pending','DONE — shipped','p')),'pending');
 assert.equal(L.view(T('a','blocked','Item RESOLVED upstream','p')),'blocked');
 assert.equal(L.view(T('a','blocked','NOT DONE — still needs review','p')),'blocked');
 assert.equal(L.view(T('a','blocked','DONE only after tests pass','p')),'blocked');
 assert.equal(L.view(T('a','done','DONE — shipped','p')),'done');
 assert.equal(L.view(T('a','pending','x'.repeat(80)+' DONE','p')),'pending','the marker must sit inside the first 80 characters');
 assert.equal(L.view(T('a','pending','UNDONE work','p')),'pending','word boundary: UNDONE is not DONE');
 assert.equal(L.view(T('a','','no status','p')),'unknown');
 const html=L.laneLedger({agent:'lane-a',ledger:ledger([T('t1','pending','WITHDRAWN: old plan','Phase A')]),now:NOW});
 assert.deepEqual(pillWords(html).filter(w=>w==='pending'),['pending']);
});

test('tasks group by phase in first-seen order with a box row, done/total, and rows sorted done, in-progress, pending, blocked',()=>{
 const tasks=[T('p1','blocked','blocked one','Beta'),T('p2','done','finished','Alpha'),T('p3','pending','waiting','Beta'),T('p4','in-progress','running','Beta'),T('p5','pending','SUPERSEDED plan','Beta'),T('p6','done','done two','Beta'),T('p7','pending','no phase task','')];
 const html=L.laneLedger({agent:'lane-a',ledger:ledger(tasks),now:NOW});
 const order=Array.from(html.matchAll(/<span class="name">([^<]+)<\/span>/g)).map(m=>m[1]);
 assert.deepEqual(order,['Beta','Alpha','No phase'],'first-seen order, not alphabetical');
 const beta=html.slice(html.indexOf('<span class="name">Beta'),html.indexOf('<span class="name">Alpha'));
 assert.match(html,/<h2 class="ph has-block"><span class="boxes"[^>]*>(<i[^>]*><\/i>){13}<\/span><span class="xy">1\/5<\/span><span class="name">Beta<\/span><span class="tally r-danger">1 blocked<\/span><\/h2>/);
 assert.deepEqual(pillWords(beta),['done','in-progress','pending','pending','blocked'],'stored status only: a SUPERSEDED subject stays pending');
 assert.match(html,/<h2 class="ph"><span class="boxes"[^>]*>(<i class="r-ok on"><\/i>){13}<\/span><span class="xy">1\/1<\/span><span class="name">Alpha<\/span><\/h2>/,'a finished phase fills every box');
});

test('box rows fill done then settled in proportion, never round a nonzero count to no box, and say their numbers',()=>{
 const on=(h,r)=>(h.match(new RegExp('<i class="r-'+r+' on">','g'))||[]).length;
 const b=L.boxes({done:1,settled:2},275);
 assert.equal(on(b,'ok'),1,'1 of 275 still shows one box');assert.equal(on(b,'settled'),1);
 assert.equal((b.match(/<i><\/i>/g)||[]).length,11);
 assert.match(b,/aria-label="1 done, 2 settled of 275"/);
 const half=L.boxes({done:2,settled:0},4);assert.equal(on(half,'ok'),7);
 assert.equal((L.boxes({},0).match(/<i><\/i>/g)||[]).length,13,'an empty phase is all outlines, not NaN');
 const top=L.laneLedger({agent:'lane-a',ledger:ledger([T('a','done','x','P'),T('b','pending','UNDONE','P'),T('c','pending','DONE x','P'),T('d','blocked','y','P')]),now:NOW});
 assert.match(top,/<div class="n r-ok">1\/4<\/div><div class="l">Complete<\/div>/,'Complete counts stored done only');
 assert.match(top,/<b>25% done<\/b><span>1 done · 2 active · 1 blocked · 0 router items<\/span>/);
});

test('blocked_by shows as waits on, and cards carry every word with their counts',()=>{
 const html=L.laneLedger({agent:'lane-a',ledger:ledger([T('b1','blocked','needs input','P',{blocked_by:'owner decision'}),T('d1','done','ok','P')],[{id:'i1',title:'Review please',from:'ra',type:'request',opened:'2026-11-06T11:00:00Z',age_seconds:720,blocked:false}]),now:NOW});
 assert.match(html,/waits on: owner decision/);
 for(const w of ['Tasks','Done','Running','Pending','Blocked','Inbox'])assert.match(html,new RegExp('<div class="l">'+w+'</div>'));
 assert.match(html,/from ra · 12 min old/);
 assert.match(html,/<span class="pill r-info">request<\/span>/);
});

test('bars carry words and numbers for assistive tech and tooltips',()=>{
 const b=L.bar([{word:'done',n:2,role:'ok'},{word:'blocked',n:1,role:'danger'},{word:'pending',n:0,role:'info'}]);
 assert.match(b,/aria-label="done 2, blocked 1"/);assert.match(b,/title="done 2"/);assert.doesNotMatch(b,/pending 0/);
 assert.match(L.bar([]),/aria-label="no data"/);
});

test('stale, failed, unknown and loading ledgers are distinct from an empty one',()=>{
 const empty=L.laneLedger({agent:'a',ledger:ledger([],[]),now:NOW});
 assert.match(empty,/Empty: nothing waiting\./);assert.match(empty,/No tasks are registered/);assert.doesNotMatch(empty,/class="banner/);
 const unknown=L.laneLedger({agent:'a',ledger:{notFound:true},now:NOW});
 assert.match(unknown,/Unknown lane\./);assert.match(unknown,/not an empty ledger/);assert.doesNotMatch(unknown,/Empty: nothing waiting/);
 const down=L.laneLedger({agent:'a',ledger:{err:'ledger producer unavailable'},now:NOW});
 assert.match(down,/Ledger unavailable\./);assert.match(down,/read failure, not an empty ledger/);assert.doesNotMatch(down,/Empty: nothing waiting/);
 const retained=L.laneLedger({agent:'a',ledger:Object.assign(ledger([T('t','pending','x','P')]),{err:'HTTP 503'}),now:NOW});
 assert.match(retained,/Refresh failed\./);assert.match(retained,/Showing last good ledger from/);assert.match(retained,/HTTP 503/);assert.match(retained,/<span class="pill r-info">pending<\/span>/,'last good rows stay visible');
 const stale=L.laneLedger({agent:'a',ledger:Object.assign(ledger([T('t','pending','x','P')]),{at:NOW-30000}),now:NOW});
 assert.match(stale,/class="banner b-warn" role="status"><b>Stale\./);assert.doesNotMatch(stale,/Refresh failed/);
 assert.match(L.laneLedger({agent:'a',ledger:{},now:NOW}),/Loading the ledger/);
 assert.equal(new Set([unknown,down,retained,stale,empty]).size,5);
});

test('cloud jobs: honest empty state, read failure, and a populated source',()=>{
 const base=ledger([T('t','pending','x','P')]);
 assert.match(L.laneLedger({agent:'a',ledger:base,jobs:{state:'none'},now:NOW}),/Cloud jobs: no source connected for this lane/);
 assert.match(L.laneLedger({agent:'a',ledger:base,jobs:{state:'error',err:'HTTP 502'},now:NOW}),/Cloud jobs: read failed · HTTP 502 \(not an empty list\)/);
 assert.match(L.laneLedger({agent:'a',ledger:base,jobs:{state:'loading'},now:NOW}),/Cloud jobs: loading/);
 const html=L.laneLedger({agent:'a',ledger:base,jobs:{state:'ok',data:[{stream:'s1',purpose:'Build X',activity:'reported',last_push:'2026-11-06T11:53:00Z',verdict:'not verified'},{stream:'s2',purpose:'Build Y',activity:'building'}]},now:NOW});
 assert.deepEqual(pillWords(html.slice(html.indexOf('Cloud jobs'),html.indexOf('<h2 class="ph'))),['building','reported'],'building sorts before reported');
 assert.match(html,/last push 7 min ago/);assert.match(html,/builder verified: not verified/);
});

test('header status distinguishes live, stale and failed refresh',()=>{
 assert.match(L.status({at:NOW-3000,err:null,now:NOW}).text,/^live · updated .* ET · refreshes every 10 s$/);
 assert.equal(L.status({at:NOW-3000,err:null,now:NOW}).cls,'');
 const stale=L.status({at:NOW-45000,err:null,now:NOW});assert.match(stale.text,/^stale · last update 45 s ago/);assert.equal(stale.cls,'stale');
 const failed=L.status({at:NOW-45000,err:'HTTP 503',now:NOW});assert.match(failed.text,/^showing last good data from .* ET · HTTP 503$/);assert.equal(failed.cls,'err');
 assert.equal(L.status({at:0,err:'boom',now:NOW}).cls,'err');
});

test('overview: attention words, waiting list, and lanes merged from Fabric and the snapshot',()=>{
 const snap={attention:[{id:'x',severity:'critical',title:'Wake lane down',detail:'No heartbeat',agent:'lane-a',next:{kind:'command-copy',label:'Restart',command:'sirsi router wake-install'}}],
  queue:[{agent:'lane-a',open:3},{agent:'lane-z',open:1},{agent:'idle',open:0}],
  lanes:{counts:{},list:[{agent:'lane-a',verdict:'HELD',detail:'held by owner',open:3},{agent:'idle',verdict:'LIVE',detail:'ok',open:0},{agent:'snap-only',verdict:'WAKEABLE',detail:'sleeps',open:0}]}};
 const fabric={lanes:[{agent:'lane-a',state_label:'WORKING',role:'ok',open:5,active:2,stalled:1,blocked:1,last_touch:'2026-11-06T11:00:00Z'},{agent:'fabric-only',state_label:'IDLE with work',role:'warn',open:2,active:0,stalled:0,blocked:0}]};
 const html=L.overview(snap,fabric,{now:NOW,changed:()=>false});
 assert.match(html,/<span class="pill r-danger">critical<\/span>/);assert.match(html,/Restart <code>sirsi router wake-install<\/code>/);
 const waiting=html.slice(html.indexOf('Where work is waiting'),html.indexOf('<h2>Lanes'));
 assert.match(waiting,/4 open items/);assert.match(waiting,/data-queue="lane-z"/);assert.doesNotMatch(waiting,/idle/);
 assert.match(waiting,/href="#lane\/lane-a"/);
 const lanes=html.slice(html.indexOf('<h2>Lanes'));
 assert.match(lanes,/<span class="pill r-ok">WORKING<\/span>/);assert.match(lanes,/<span class="pill r-warn">IDLE with work<\/span>/);
 assert.match(lanes,/aria-label="active 2, stalled 1, blocked 1, open 1"/);
 assert.match(lanes,/active 2 · stalled 1 · blocked 1 · open 5/);
 assert.match(lanes,/last touch 1 h ago · runtime held/);assert.match(lanes,/held by owner/,'verdict detail surfaces as a sub-line');
 assert.match(lanes,/snap-only/);assert.match(lanes,/<span class="pill r-dim">unknown<\/span>/,'a lane the Fabric does not know says unknown');
 assert.match(lanes,/href="#lane\/fabric-only"/);
});

test('a changed row is marked, and nothing escapes into markup',()=>{
 const html=L.laneLedger({agent:'a',ledger:ledger([T('t1','pending','<script>x</script>','<b>P</b>')]),now:NOW,changed:(scope,key)=>scope==='lane:a'&&key==='t1'});
 assert.match(html,/<tr class="changed">/);assert.doesNotMatch(html,/<script>|<b>P/);assert.match(html,/&lt;script&gt;/);
});

test('narrow layout: id column is hidden under 640px and the id folds into the text cell',()=>{
 const html=L.laneLedger({agent:'a',ledger:ledger([T('task-77','pending','x','P')]),now:NOW});
 assert.match(html,/<td class="id"><span class="idv" title="task-77">task-77<\/span><\/td><td class="t">x<span class="sub-line rid" title="task-77">task-77<\/span>/,'id is in the id column once (with a title); the text-cell copy is CSS-hidden on wide screens');
 assert.match(css,/\.sub-line\.rid\{display:none/);
 assert.match(css,/\.idv\{display:block;overflow:hidden;text-overflow:ellipsis;white-space:nowrap\}/,'wide id is one truncated line');
 assert.match(css,/\.rows td\.id\{width:210px/,'fixed id column so text columns align');
 assert.doesNotMatch(css,/\.fabric-metric\{[^}]*underline/,'count numbers carry no underline');
 // Totals are neutral; only state cards take state colors.
 const F=require('../ui/fabric.js'),b={work:{done:1,open:2,in_progress:1,assigned_not_started:1,stalled:0,blocked:0,total:3},messages:{open:0,stranded:0,oldest_age_seconds:0},lanes:[],health:{ram_pct:null,swap:null,total_ram:null,cpu_cores:null,git_dirty:null,engine:'u',engine_state:'u',issues:[]},activity:[],activity_seeded:false,runtime_available:true,generated_at:'2026-11-06T12:00:00Z',build:'b',presentation:{headlines:{progress:'gold'}}};
 assert.match(F.headlines(b,L.esc),/<div class="n r-ink">3<\/div><div class="l">Tasks/);
 const narrow=css.slice(css.indexOf('@media(max-width:640px)'));
 assert.match(narrow,/\.rows td\.id\{display:none\}/);assert.match(narrow,/\.sub-line\.rid\{display:block\}/);
 assert.match(css,/repeat\(auto-fit,minmax\(130px,1fr\)\)/,'count cards wrap');
 assert.match(css,/@media\(prefers-reduced-motion:reduce\)\{\*\{animation:none!important/,'the change flash obeys reduced motion');
 assert.match(css,/tr\.changed td\{animation:flash 3s/);
});

test('inbox groups every open item by recipient with counts',()=>{
 const items=[{id:'1',to:'b',from:'x',title:'one',opened:'2026-11-06T11:00:00Z',type:'task'},{id:'2',to:'a',from:'x',title:'two',opened:'2026-11-06T11:30:00Z'},{id:'3',to:'b',from:'y',title:'three',opened:'2026-11-06T10:00:00Z',blocked_by:'item-0'}];
 const html=L.inbox(items,{now:NOW,changed:()=>false});
 assert.ok(html.indexOf('<h2>b')<html.indexOf('<h2>a'),'bigger group first');
 assert.match(html,/<h2>b<span class="tally">2 open<\/span>/);assert.match(html,/from x · 1 h old/);assert.match(html,/waits on: item-0/);
 assert.equal((html.match(/data-item=/g)||[]).length,3);
});
