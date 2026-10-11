const {test} = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const source = fs.readFileSync(__dirname+'/ui/app.js','utf8');
const scope = {state:{recipient:'*'},esc:s=>String(s??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c])),chip:s=>s,nextStep:()=>''};
vm.createContext(scope);
vm.runInContext(source.slice(source.indexOf('function vOverview('),source.indexOf('function posture(')),scope);
vm.runInContext(source.slice(source.indexOf('function vQueue('),source.indexOf('function itemInspector(')),scope);
const queue = Array.from({length:10},(_,i)=>({agent:'recipient-'+i,open:1}));
const snapshot = {queue,items:queue.map((q,i)=>({id:'item-'+i,to:q.agent,from:'sender',title:'Request '+i,opened:'2026-10-10'})),lanes:{counts:{},list:[{agent:'empty-lane',open:0}]},attention:[]};
test('overview includes all recipients, including unregistered ones, and no empty lanes',()=>{
 const html=scope.vOverview(snapshot);
 assert.match(html,/10 open items/);
 assert.equal((html.match(/data-label="Open"/g)||[]).length,10);
 assert.match(html,/data-queue="recipient-9"/);
 assert.doesNotMatch(html,/empty-lane/);
});
test('queue shows all records and recipient filter agrees with its count',()=>{
 assert.equal((scope.vQueue(snapshot).match(/data-item=/g)||[]).length,10);
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
