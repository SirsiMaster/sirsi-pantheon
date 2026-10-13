const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');
const source=fs.readFileSync(__dirname+'/ui/app.js','utf8');
const scope={state:{},esc:s=>String(s??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]))};
vm.createContext(scope);
vm.runInContext(source.slice(source.indexOf("var fleetSelection="),source.indexOf('function vHost(')),scope);
const fleet={summary:{total:4,in_flight:3,done:1,active:1,blocked:1,stalled:1,pct_done:25,lanes_working:0,lanes_total:2},lanes:[{agent:'a',state:'IDLE_WITH_WORK',open:3,inbox:1,blocked:1},{agent:'b',state:'COMPLETE',open:0,inbox:0,blocked:0}],tasks:['done','active','blocked','stalled'].map((bucket,i)=>({id:'task-'+i,agent:'a',subject:'Task '+i,bucket,status:bucket,blocked_by:bucket==='blocked'?'dependency':''}))};
scope.state.fleet=fleet;
test('every top metric is a keyboard-accessible filter and stalled is separately selectable',()=>{
 const html=scope.vFleet();
 assert.equal((html.match(/class="card kpi fleet-metric"/g)||[]).length,4);
 for(const key of ['done','active','blocked','lanes','stalled']) assert.match(html,new RegExp('data-fleet="'+key+'"'));
 assert.match(html,/1 idle with work/);
 assert.match(html,/<th class="num">Open tasks<\/th>/);
});
test('blocked and stalled reveal exact records, not a truncated lane summary',()=>{
 for(const key of ['done','active','blocked','stalled']) {
  const html=scope.fleetTaskRows(fleet,key);
  assert.equal((html.match(/data-task-id=/g)||[]).length,1);
  assert.match(html,/1 matching tasks/);
 }
 assert.match(scope.fleetTaskRows(fleet,'blocked'),/dependency/);
 assert.equal((scope.fleetTaskRows(fleet,'all').match(/data-task-id=/g)||[]).length,4);
});
test('missing and inconsistent detail payloads fail visibly, and subjects are escaped',()=>{
 assert.match(scope.fleetTaskRows({...fleet,tasks:undefined},'blocked'),/unavailable/);
 assert.match(scope.fleetTaskRows({...fleet,tasks:[]},'blocked'),/Snapshot mismatch/);
 const f={...fleet,tasks:fleet.tasks.map(t=>({...t,subject:'<img onerror="bad">'}))};
 assert.doesNotMatch(scope.fleetTaskRows(f,'all'),/<img/);
});
test('numeric headers and cells share alignment',()=>{
 const css=fs.readFileSync(__dirname+'/ui/app.css','utf8');
 assert.match(css,/\.table th\.num,\.table td\.num\{text-align:right\}/);
});
test('lane values and state are actionable and agent drilldown remains scoped',()=>{
 assert.match(scope.vFleet(),/data-fleet-state="IDLE_WITH_WORK"/);
 assert.match(scope.vFleet(),/data-fleet="open" data-fleet-agent="a"/);
 assert.match(scope.vFleet(),/data-queue="a"/);
 scope.fleetAgent='b';assert.match(scope.fleetTaskRows(fleet,'open'),/0 matching tasks/);
 scope.fleetAgent='a';assert.match(scope.fleetTaskRows(fleet,'open'),/3 matching tasks/);
 scope.fleetAgent='*';
});

test('availability is current consumer truth, separately rendered and escaped',()=>{
 assert.match(scope.fleetAvailability({availability:'WATCH_ONLY',availability_detail:'<script>'}),/watch only/);
 assert.doesNotMatch(scope.fleetAvailability({availability:'LIVE',availability_detail:'<script>'}),/<script>/);
 assert.match(scope.fleetAvailability({recipient_type:'human'}),/Human recipient/);
 assert.match(scope.fleetAvailability({}),/unknown/);
 assert.match(scope.vFleet(),/Current consumer availability not supplied/);
});
