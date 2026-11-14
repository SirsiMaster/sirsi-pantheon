const {test} = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const source = fs.readFileSync(__dirname+'/ui/app.js','utf8');
// Pinned slices of the real app.js: the overview/queue renderers run against the real ledger renderer.
// The 2026-11-06 redesign moved their row markup into ledger.js (shared pill/row language), so the
// harness now supplies PantheonLedger and the change-flash hook instead of the old chip/nextStep stubs.
const scope = {state:{recipient:'*'},PantheonFabric:require('./ui/fabric.js'),PantheonLedger:require('./ui/ledger.js'),changed:()=>false,esc:s=>String(s??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c])),chip:s=>s,nextStep:()=>''};
vm.createContext(scope);
vm.runInContext(source.slice(source.indexOf('function vOverview('),source.indexOf('function vLanes(')),scope);
vm.runInContext(source.slice(source.indexOf('function vQueue('),source.indexOf('function itemInspector(')),scope);
const queue = Array.from({length:10},(_,i)=>({agent:'recipient-'+i,open:1}));
const snapshot = {queue,items:queue.map((q,i)=>({id:'item-'+i,to:q.agent,from:'sender',title:'Request '+i,opened:'2026-10-10'})),lanes:{counts:{},list:[{agent:'empty-lane',verdict:'LIVE',detail:'idle',open:0}]},attention:[]};
test('overview waiting list includes all recipients, including unregistered ones, and no empty lanes',()=>{
 const html=scope.vOverview(snapshot);
 assert.match(html,/Where work is waiting/);
 assert.match(html,/10 open items/);
 const waiting=html.slice(html.indexOf('Where work is waiting'),html.indexOf('<h2>Lanes'));
 assert.equal((waiting.match(/data-queue="recipient-\d"/g)||[]).length,10);
 assert.match(waiting,/data-queue="recipient-9"/);
 assert.doesNotMatch(waiting,/empty-lane/);
 // The lane registry still lists the idle lane, in the Lanes section.
 assert.match(html.slice(html.indexOf('<h2>Lanes')),/empty-lane/);
});
test('queue shows all records grouped by recipient and the recipient filter agrees with its count',()=>{
 const all=scope.vQueue(snapshot);
 assert.equal((all.match(/data-item=/g)||[]).length,10);
 assert.equal((all.match(/<h2>recipient-\d/g)||[]).length,10,'one group heading per recipient');
 scope.state.recipient='recipient-9';
 const html=scope.vQueue(snapshot);
 assert.match(html,/Showing 1 of 10/);
 assert.equal((html.match(/data-item=/g)||[]).length,1);
 scope.state.recipient='*';
});
test('missing details and inconsistent counts are disclosed, never silently hidden',()=>{
 assert.match(scope.vQueue({...snapshot,items:undefined}),/does not provide message details/);
 assert.match(scope.vQueue({...snapshot,items:[]}),/Snapshot mismatch/);
 assert.match(scope.vQueue({queue:[],items:[]}),/No open items/);
});
test('request titles cannot inject markup',()=>{
 const html=scope.vQueue({queue:[{agent:'x',open:1}],items:[{id:'x',to:'x',title:'<img onerror="bad">'}]});
 assert.doesNotMatch(html,/<img/);
 assert.match(html,/&lt;img/);
});

// The actual inspector/release functions, not replacement renderers. As above,
// only in-memory VM evaluation with an inert inspector sink; no app startup.
let inspected;
scope.showInspector=(title,body,persistent)=>{inspected={title,body,persistent}};
scope.plural=(n,one,many)=>n+' '+(n===1?one:many);
vm.runInContext(source.slice(source.indexOf('function itemInspector('),source.indexOf('function vLane(')),scope);
vm.runInContext(source.slice(source.indexOf('function vReleases('),source.indexOf('function vFailures(')),scope);

test('hosted request withholding is explicit; local body and empty rendering stay exact',()=>{
 const item={id:'item',from:'sender',to:'worker',title:'Request',type:'review',opened:'today'};
 for(const instructions of [undefined,'PRIVATE-CANARY <script>']){
  const hosted={...item,request_body_withheld:true,instructions};
  const before=JSON.stringify(hosted);
  scope.state.data={items:[hosted]};scope.itemInspector('item');
  assert.match(inspected.body,/Request body withheld on the hosted board\./);
  assert.doesNotMatch(inspected.body,/No request body supplied|PRIVATE-CANARY|<script>/);
  assert.equal(JSON.stringify(hosted),before);
 }
 for(const [instructions,body] of [[undefined,'No request body supplied.'],['Local <body>','Local &lt;body&gt;']]){
  const local={...item,instructions};
  scope.state.data={items:[local]};scope.itemInspector('item');
  assert.equal(inspected.body,'<p class="mono">item</p><p>sender → worker</p><p class="sub">Opened today · review</p><h3>Request</h3><pre class="message-body">'+body+'</pre>');
 }
});

test('hosted release withholding never becomes zero entries or no changes; local rendering stays exact',()=>{
 for(const items of [undefined,[],['PRIVATE-CANARY <script>']]){
  const release={version:'v1',date:'today',release_bullets_withheld:true,items};
  const before=JSON.stringify(release),html=scope.vReleases({releases:[release]});
  assert.match(html,/Release bullets withheld on the hosted board\./);
  assert.doesNotMatch(html,/0 entries|No changes|PRIVATE-CANARY|<script>/);
  assert.equal(JSON.stringify(release),before);
 }
 assert.equal(scope.vReleases({releases:[{version:'v1',date:'today',items:[]}]}),'<div class="card"><details class="release" data-release="0"><summary><strong>v1</strong><span class="sub">today · 0 entries</span><p>No changes recorded for this release.</p></summary><ul></ul></details></div>');
 assert.equal(scope.vReleases({releases:[{version:'v1',date:'today',items:['Local <change>']}]}),'<div class="card"><details class="release" data-release="0"><summary><strong>v1</strong><span class="sub">today · 1 entry</span><p>Local &lt;change&gt;</p></summary><ul><li>Local &lt;change&gt;</li></ul></details></div>');
});

test('hosted blocker presence does not claim an unresolved dependency; local indicators stay unchanged',()=>{
 const item={id:'item',to:'worker',from:'sender',title:'Request'};
 const hosted={...item,blocker_recorded:true},before=JSON.stringify(hosted);
 const html=scope.PantheonLedger.inbox([hosted],{now:Date.now()});
 assert.match(html,/Blocker recorded · details withheld; dependency status not evaluated\./);
 assert.doesNotMatch(html,/waits on:|blocked by an open dependency/);
 assert.equal(JSON.stringify(hosted),before);
 for(const blocker_recorded of [false,undefined]){
  assert.doesNotMatch(scope.PantheonLedger.inbox([{...item,blocker_recorded}],{now:Date.now()}),/Blocker recorded|blocked by an open dependency/);
 }
 assert.match(scope.PantheonLedger.inbox([{...item,blocked_by:'local blocker'}],{now:Date.now()}),/waits on: local blocker/);
 assert.match(scope.PantheonLedger.inbox([{...item,blocked:true}],{now:Date.now()}),/blocked by an open dependency/);
});
