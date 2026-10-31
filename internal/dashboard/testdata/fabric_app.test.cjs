'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),vm=require('node:vm');
const fabric=require('../ui/fabric.js');
const source=fs.readFileSync(__dirname+'/../ui/app.js','utf8');
const at=Date.parse('2026-10-25T12:00:00Z');
function board(){return {work:{done:0,open:3,in_progress:0,assigned_not_started:3,stalled:0,blocked:0,total:3},messages:{open:7,stranded:0,oldest_age_seconds:0},lanes:[{agent:'own-lane',state:'IDLE_WITH_WORK',state_label:'IDLE with work',open:3,active:0,stalled:0,blocked:0,registered:{router:true,thread:false,ledger:true},last_touch:'task-touch'}],health:{ram_pct:null,swap:null,total_ram:null,cpu_cores:null,git_dirty:null,engine:'unknown',engine_state:'unavailable',issues:[]},activity:[],activity_seeded:false,runtime_available:true,generated_at:new Date(at).toISOString(),build:'fabric-build'}}
function router(){return {version:'router-build',generated_at:'router-other-time',lanes:{counts:{},list:[{agent:'own-lane',verdict:'HELD',open:99,detail:'router-only-condition'}]},queue:[],attention:[]}}
// Run the actual app IIFE with an inert DOM boundary and controlled fetch/clock.
// No replacement renderer, loader, click handler or inspector implementation.
function host(view='fleet',initial={}){
 let clock=at,dialog=null,tick,reply={fabric:board(),router:router(),...initial},elements={},listeners={},requests=[];
 class Element {
  constructor(){this.dataset={};this.handlers={};this.classList={add(){},remove(){},contains(){return false}};this.isConnected=true;this.hidden=false;this._html=''}
  set innerHTML(s){this._html=s;this.buttons=[];this.taskButtons=[];this.pageButtons=[];this.filterButtons=[];this.messageButtons=[];for(const m of s.matchAll(/data-fabric-filter="([^"]+)"/g)){const b=new Element();b.dataset.fabricFilter=m[1];this.filterButtons.push(b)}if(s.includes('data-fabric-messages'))this.messageButtons.push(new Element());for(const m of s.matchAll(/data-fabric-inspect="([^"]+)"/g)){const b=new Element();b.dataset.fabricInspect=m[1];this.buttons.push(b)}for(const m of s.matchAll(/data-fabric-task-agent="([^"]+)" data-fabric-task-id="([^"]+)"/g)){const b=new Element();b.dataset.fabricTaskAgent=m[1];b.dataset.fabricTaskId=m[2];this.taskButtons.push(b)}for(const m of s.matchAll(/data-fabric-task-page="([^"]+)"/g)){const b=new Element();b.dataset.fabricTaskPage=m[1];this.pageButtons.push(b)}this.receipt= s.includes('fabric-receipt') ? new Element():null}
  get innerHTML(){return this._html}
  querySelectorAll(s){return s==='[data-fabric-filter]'?this.filterButtons||[]:s==='[data-fabric-messages]'?this.messageButtons||[]:s==='[data-fabric-inspect]'?this.buttons||[]:s==='[data-fabric-task-id]'?this.taskButtons||[]:s==='[data-fabric-task-page]'?this.pageButtons||[]:[]}
  querySelector(){return new Element()}
  addEventListener(k,f){this.handlers[k]=f}
  setAttribute(k,v){this[k]=v}
  focus(){this.focused=true} scrollIntoView(){this.scrolled=true} showModal(){this.open=true} show(){this.open=true} close(){this.open=false} remove(){if(dialog===this)dialog=null}
 }
 const doc={hidden:false,activeElement:null,
  querySelector(s){if(s==='.inspector'||s==='.inspector.persistent')return dialog;return elements[s]||(elements[s]=new Element())},
  querySelectorAll(s){return s==='.fabric-receipt'&&elements['#view']?.receipt?[elements['#view'].receipt]:[]},
  getElementById(id){return elements['#'+id]||(elements['#'+id]=new Element())},
  createElement(){return new Element()},
  addEventListener(k,f){listeners[k]=f},
  body:{appendChild(d){dialog=d},classList:{add(){},remove(){}}}
 };
 const context={document:doc,location:{hash:'#'+view},navigator:{},PantheonFabric:fabric,
  Date:class extends Date {constructor(...args){super(...(args.length?args:[clock]))}static now(){return clock}},
  AbortSignal:{timeout(){}},setInterval(f){tick=f;return 1},
  fetch:async url=>{requests.push(url);let value=url.startsWith('/api/fabric')?reply.fabric:url==='/api/router/v1/snapshot'?reply.router:{};if(typeof value==='function')value=await value(url);return {ok:!(value instanceof Error),status:value instanceof Error?value.status||503:200,json:async()=>value instanceof Error?{error:value.message}:value}},
  addEventListener(){},scrollTo(){},scrollY:0,matchMedia(){return {matches:false}}
 };
 context.window=context;vm.createContext(context);vm.runInContext(source,context);
 async function settle(){for(let i=0;i<20;i++)await Promise.resolve()}
 return {elements,doc,reply,requests,settle,get dialog(){return dialog},advance(ms){clock+=ms;tick()},refresh(){elements['#refresh'].handlers.click()},navigate(v){const handler=listeners.click;handler({target:{closest(){return {dataset:{view:v}}}}})},
  clickFabric(){const button=elements['#view'].buttons[0];assert.ok(button,'actual Fabric lane button');button.handlers.click()},
  clickRouter(){listeners.click({target:{closest(){return {dataset:{inspect:'own-lane'}}}}})},
  clickTask(){const b=elements['#view'].taskButtons[0];assert.ok(b,'actual full-board task button');b.handlers.click()},
  taskPage(value){const b=elements['#view'].pageButtons.find(x=>x.dataset.fabricTaskPage===value);assert.ok(b,'actual task page control');b.handlers.click()},
  metric(value){const b=elements['#view'].filterButtons.find(x=>x.dataset.fabricFilter===value);assert.ok(b,'actual metric button');b.handlers.click()},
  messages(){elements['#view'].messageButtons[0].handlers.click()},
  age(){clock+=31000;listeners.visibilitychange()}
 };
}
test('actual app keeps Fabric receipt independent of router read success and failure',async()=>{
 const h=host('overview');await h.settle();assert.match(h.elements['#view'].innerHTML,/Current Fabric observation/);
 h.reply.fabric=new Error('fabric-read-refused');h.refresh();await h.settle();
 assert.match(h.elements['#view'].innerHTML,/Retained Fabric observation/);assert.match(h.elements['#live-t'].textContent,/Router received/);
 h.reply.fabric=board();h.reply.router=new Error('router-read-refused');h.refresh();await h.settle();
 assert.match(h.elements['#view'].innerHTML,/Current Fabric observation/);assert.match(h.elements['#live-t'].textContent,/Router stale/);
 h.navigate('host');assert.match(h.elements['#view'].innerHTML,/Current Fabric observation/);
});
function taskBoard(offset=0){
 const b=board();b.work.open=65;b.work.total=65;b.work.assigned_not_started=65;
 b.tasks={state:'observed',offset,limit:64,total:65,snapshot:'a'.repeat(64),next_offset:offset===0?64:null,
  rows:Array.from({length:offset===0?64:1},(_,i)=>({agent:'own-lane',id:'task-'+(offset+i),subject:'Actual task '+(offset+i),status:'pending',liveness:'unknown',role:'gold',responsible:'owner',stage:'build',test_state:'untested',updated:'',blocker_recorded:false,text_limited:false,provenance:{source:'canonical-task-registry',created:'',commissioned_at:'',commissioned_by:'lead',charter_recorded:true,outline_recorded:false,result_recorded:false,public_links:[],withheld_links:1}}))};return b;
}
test('overview metric invokes actual selected endpoint and every blocked page remains accessible',async()=>{
 const h=host('overview',{fabric:taskBoard()});await h.settle();
 h.reply.fabric=url=>{const b=taskBoard(url.includes('tasks_offset=64')?64:0);b.tasks.filter='blocked';b.tasks.rows.forEach(row=>row.status='blocked');b.work.blocked=65;b.work.assigned_not_started=0;return b};
 h.metric('blocked');await h.settle();assert.equal(h.requests.at(-1),'/api/fabric?tasks_filter=blocked');assert.match(h.elements['#view'].innerHTML,/Blocked tasks/);assert.equal(h.elements['#fabric-task-heading'].focused,true);assert.equal(h.elements['#fabric-task-heading'].scrolled,true);
 h.taskPage('next');await h.settle();assert.equal(h.requests.at(-1),'/api/fabric?tasks_filter=blocked&tasks_offset=64&tasks_snapshot='+'a'.repeat(64));assert.match(h.elements['#view'].innerHTML,/Actual task 64/);
 h.taskPage('first');await h.settle();assert.equal(h.requests.at(-1),'/api/fabric?tasks_filter=blocked');assert.match(h.elements['#view'].innerHTML,/Actual task 0/);
});
test('wrong selection and cursor errors retain labeled last-good tasks rather than replacing them',async()=>{
 const h=host('fleet',{fabric:taskBoard()});await h.settle();h.metric('stalled');await h.settle();
 assert.match(h.elements['#view'].innerHTML,/Invalid fabric projection or task selection/);assert.match(h.elements['#view'].innerHTML,/Retained Fabric observation/);assert.match(h.elements['#view'].innerHTML,/Open ledger tasks/);
});
test('actual full-board task click page conflict and explicit refresh remain bound to canonical rows',async()=>{
 const h=host('fleet',{fabric:taskBoard()});await h.settle();h.clickTask();
 assert.match(h.dialog.innerHTML,/Actual task 0/);assert.match(h.dialog.innerHTML,/canonical-task-registry/);assert.match(h.dialog.innerHTML,/<dt>Thread registration<\/dt><dd>Not registered/);
 const captured=h.dialog.innerHTML;h.reply.fabric=taskBoard(64);h.taskPage('next');await h.settle();
 assert.ok(h.requests.includes('/api/fabric?tasks_offset=64&tasks_snapshot='+'a'.repeat(64)));assert.match(h.elements['#view'].innerHTML,/Actual task 64/);assert.equal(h.dialog.innerHTML,captured);
 h.clickTask();assert.match(h.dialog.innerHTML,/Actual task 64/);
 h.reply.fabric=Object.assign(new Error('tasks changed; refresh list'),{status:409});h.refresh();await h.settle();assert.match(h.elements['#view'].innerHTML,/Retained Fabric observation/);assert.match(h.elements['#view'].innerHTML,/Actual task 64/);
 h.reply.fabric=taskBoard();h.taskPage('first');await h.settle();assert.match(h.elements['#view'].innerHTML,/Actual task 0/);assert.equal(h.requests.at(-1),'/api/fabric');
});
test('older task-page response cannot overwrite explicit first-page refresh',async()=>{
 const h=host('fleet',{fabric:taskBoard()});await h.settle();let finish;
 h.reply.fabric=url=>url.includes('?')?new Promise(resolve=>{finish=resolve}):taskBoard();
 h.taskPage('next');await h.settle();assert.equal(typeof finish,'function');
 h.taskPage('first');await h.settle();assert.match(h.elements['#view'].innerHTML,/Actual task 0/);
 finish(taskBoard(64));await h.settle();assert.match(h.elements['#view'].innerHTML,/Actual task 0/);assert.doesNotMatch(h.elements['#view'].innerHTML,/Actual task 64/);
});
test('actual app ages retained Fabric without response and discloses future time and known empty',async()=>{
 const b=board();b.work={done:0,open:0,in_progress:0,assigned_not_started:0,stalled:0,blocked:0,total:0};b.lanes=[];const h=host('fleet',{fabric:b});await h.settle();
 assert.match(h.elements['#view'].innerHTML,/Current Fabric observation/);assert.match(h.elements['#view'].innerHTML,/0 \/ 0/);
 h.age();assert.equal(h.elements['#view'].receipt.dataset.fabricState,'retained');assert.match(h.elements['#view'].receipt.textContent,/31s old/);
 const future=board();future.generated_at=new Date(at+120000).toISOString();h.reply.fabric=future;h.refresh();await h.settle();
 assert.match(h.elements['#view'].innerHTML,/Fabric time unknown — future observation/);
 h.navigate('host');assert.match(h.elements['#view'].innerHTML,/Fabric time unknown/);
 const timed=host();await timed.settle();timed.advance(29000);assert.equal(timed.elements['#view'].receipt.dataset.fabricState,'current');
 timed.advance(2001);assert.equal(timed.elements['#view'].receipt.dataset.fabricState,'retained','clock ages before any new response');await timed.settle();
});
test('actual Fabric button binds clicked board even with router absent or contradictory',async()=>{
 const h=host('fleet',{router:new Error('absent')});await h.settle();h.clickFabric();
 assert.match(h.dialog.innerHTML,/Canonical Fabric · ledger tasks/);assert.match(h.dialog.innerHTML,/fabric-build/);assert.match(h.dialog.innerHTML,/IDLE with work/);
 assert.match(h.dialog.innerHTML,/<dt>Open tasks<\/dt><dd>3<\/dd>/);assert.match(h.dialog.innerHTML,/<dt>Thread registration<\/dt><dd>Not registered/);assert.match(h.dialog.innerHTML,/task-touch/);
 const retained=h.dialog.innerHTML;
 h.reply.router=router();h.reply.fabric={...board(),generated_at:new Date(at+1000).toISOString()};h.refresh();await h.settle();
 assert.equal(h.dialog.innerHTML,retained,'open detail retains clicked observation');
 h.clickFabric();assert.doesNotMatch(h.dialog.innerHTML,/router-only-condition|<dd>99<\/dd>|HELD/);
 assert.match(h.dialog.innerHTML,/2026-10-25T12:00:01/);
 h.clickRouter();assert.match(h.dialog.innerHTML,/router-only-condition/);assert.match(h.dialog.innerHTML,/99/);assert.doesNotMatch(h.dialog.innerHTML,/fabric-build/);
});

function snapshotWithFabric(f){return {...router(),fabric:f,fabric_generation:'fixture-bound-generation'}}
function blockedTaskBoard(offset=0){
 const b=taskBoard(offset);b.tasks.filter='blocked';b.tasks.rows.forEach(row=>row.status='blocked');b.work.blocked=65;b.work.assigned_not_started=0;return b;
}
test('default refresh consumes the bound snapshot Fabric without a duplicate endpoint read',async()=>{
 const h=host('overview',{router:snapshotWithFabric(taskBoard()),fabric:new Error('duplicate-default-read')});
 await h.settle();h.navigate('fleet');await h.settle();
 assert.equal(h.requests.filter(url=>url.startsWith('/api/fabric')).length,0,'default Fabric must come from RouterSnapshot.Fabric');
 assert.equal(h.requests.filter(url=>url==='/api/router/v1/snapshot').length,1);
 assert.match(h.elements['#view'].innerHTML,/Actual task 0/);
 h.age();assert.equal(h.elements['#view'].receipt.dataset.fabricState,'retained');
 h.refresh();await h.settle();
 assert.equal(h.requests.filter(url=>url==='/api/router/v1/snapshot').length,2,'reuse is per response, never a TTL read suppression');
 assert.equal(h.requests.filter(url=>url.startsWith('/api/fabric')).length,0);
 assert.match(h.elements['#view'].innerHTML,/Retained Fabric observation/,'receiving an old observation again must not renew its freshness');
 assert.match(h.elements['#view'].innerHTML,/2026-10-25T12:00:00/);
});
test('Fleet entry during a pending snapshot waits for its bound default board',async()=>{
 let finish;const h=host('overview',{router:()=>new Promise(resolve=>{finish=resolve})});
 await h.settle();h.navigate('fleet');await h.settle();
 assert.equal(h.requests.filter(url=>url.startsWith('/api/fabric')).length,0);
 finish(snapshotWithFabric(taskBoard()));await h.settle();
 assert.match(h.elements['#view'].innerHTML,/Actual task 0/);
 assert.equal(h.requests.filter(url=>url.startsWith('/api/fabric')).length,0);
});
test('missing invalid or nondefault embedded Fabric preserves independent Fleet fallback',async()=>{
 const invalid={...board(),work:{...board().work,open:-1}};
 for(const embedded of [undefined,invalid,taskBoard(64),blockedTaskBoard()]){
  const h=host('fleet',{router:snapshotWithFabric(embedded),fabric:taskBoard()});await h.settle();
  assert.equal(h.requests.filter(url=>url==='/api/fabric').length,1);
  assert.match(h.elements['#view'].innerHTML,/Actual task 0/);
  assert.doesNotMatch(h.elements['#view'].innerHTML,/Actual task 64|Blocked tasks/);
 }
 const unavailable=host('fleet',{router:new Error('router-not-available'),fabric:taskBoard()});await unavailable.settle();
 assert.match(unavailable.elements['#view'].innerHTML,/Actual task 0/);
 assert.match(unavailable.elements['#live-t'].textContent,/Router stale/);
 assert.equal(unavailable.requests.filter(url=>url==='/api/fabric').length,1);
 unavailable.reply.fabric=new Error('fabric-not-available');unavailable.refresh();await unavailable.settle();
 assert.match(unavailable.elements['#view'].innerHTML,/Retained Fabric observation/);
 assert.match(unavailable.elements['#view'].innerHTML,/Actual task 0/);
});
test('late bound snapshot cannot replace a newer explicit filtered selection',async()=>{
 const h=host('overview',{router:snapshotWithFabric(taskBoard())});await h.settle();
 let finish;h.reply.router=()=>new Promise(resolve=>{finish=resolve});
 h.refresh();await h.settle();
 h.reply.fabric=url=>blockedTaskBoard(url.includes('tasks_offset=64')?64:0);
 h.metric('blocked');await h.settle();
 assert.match(h.elements['#view'].innerHTML,/Blocked tasks/);
 finish(snapshotWithFabric(taskBoard()));await h.settle();
 assert.match(h.elements['#view'].innerHTML,/Blocked tasks/,'delayed default snapshot cannot publish across a newer selected-read generation');
 assert.equal(h.requests.filter(url=>url==='/api/fabric').length,0,'late snapshot cannot launch a default fallback either');
 h.taskPage('next');await h.settle();
 assert.equal(h.requests.at(-1),'/api/fabric?tasks_filter=blocked&tasks_offset=64&tasks_snapshot='+'a'.repeat(64));
 assert.match(h.elements['#view'].innerHTML,/Actual task 64/);
});
test('filtered continuation refresh stays independent even when snapshot has a valid default board',async()=>{
 const h=host('overview',{router:snapshotWithFabric(taskBoard())});await h.settle();
 h.reply.fabric=url=>blockedTaskBoard(url.includes('tasks_offset=64')?64:0);
 h.metric('blocked');await h.settle();h.taskPage('next');await h.settle();
 const before=h.requests.length;h.refresh();await h.settle();
 const reads=h.requests.slice(before).filter(url=>url.startsWith('/api/fabric'));
 assert.deepEqual(reads,['/api/fabric?tasks_filter=blocked&tasks_offset=64&tasks_snapshot='+'a'.repeat(64)]);
 assert.match(h.elements['#view'].innerHTML,/Blocked tasks/);
 assert.match(h.elements['#view'].innerHTML,/Actual task 64/);
 assert.doesNotMatch(h.elements['#view'].innerHTML,/Actual task 0</);
});
