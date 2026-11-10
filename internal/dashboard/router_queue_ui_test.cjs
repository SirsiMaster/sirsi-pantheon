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
