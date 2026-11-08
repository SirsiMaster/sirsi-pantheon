'use strict';
// The real app.js (and real fabric.js / ledger.js) run against an inert DOM and a scripted fetch.
// Covers the #lane/<agent> route, its data loads, and the honest states of the ledger and jobs slots.
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),vm=require('node:vm');
const fabric=require('../ui/fabric.js'),ledger=require('../ui/ledger.js');
const source=fs.readFileSync(__dirname+'/../ui/app.js','utf8');
const at=Date.parse('2026-11-06T12:00:00Z');
const snapshot={version:'v',generated_at:'g',lanes:{counts:{},list:[{agent:'lane-a',verdict:'LIVE',detail:'ok',open:0}]},queue:[],items:[],attention:[]};
const ledgerBody={generated_at:'2026-11-06T12:00:00Z',agent:'lane-a',items:[{id:'i1',title:'Review',from:'ra',type:'request',opened:'2026-11-06T11:50:00Z',age_seconds:600,blocked:false}],
 tasks:[{task_id:'T1',subject:'Build it',status:'in-progress',phase:'Phase 1',blocked_by:'',updated:'',responsible_party:'self'},{task_id:'T2',subject:'DONE: old',status:'pending',phase:'Phase 1',blocked_by:'',updated:'',responsible_party:'self'}]};
function host(hash,replies){
 let clock=at,tick,elements={},requests=[],listeners={},winListeners={};
 class Element{constructor(){this.dataset={};this.classList={add(){},remove(){},contains(){return false}};this.hidden=false;this.textContent='';this.innerHTML=''}
  querySelectorAll(){return[]} querySelector(){return new Element()} addEventListener(k,f){this.handlers=this.handlers||{};this.handlers[k]=f}
  setAttribute(){} focus(){this.focused=true} replaceChildren(){}}
 const doc={hidden:false,activeElement:null,body:{classList:{add(){},remove(){}}},
  querySelector(s){return s==='.inspector'||s==='.inspector.persistent'?null:elements[s]||(elements[s]=new Element())},
  querySelectorAll(){return[]},getElementById(id){return elements['#'+id]||(elements['#'+id]=new Element())},createElement(){return new Element()},
  addEventListener(k,f){listeners[k]=f}};
 const loc={hash};
 const context={document:doc,location:loc,navigator:{},PantheonFabric:fabric,PantheonLedger:ledger,
  Date:class extends Date{constructor(...a){super(...(a.length?a:[clock]))}static now(){return clock}},
  AbortSignal:{timeout(){}},setInterval(f){tick=f;return 1},
  fetch:async url=>{requests.push(url);const key=Object.keys(replies).find(k=>url.startsWith(k));let v=key?replies[key]:{};if(typeof v==='function')v=await v(url);
   const bad=v instanceof Error;return {ok:!bad,status:bad?v.status||503:200,json:async()=>bad?{error:v.message}:v}},
  addEventListener(k,f){winListeners[k]=f},scrollTo(){},scrollY:0,matchMedia(){return{matches:false}}};
 context.window=context;vm.createContext(context);vm.runInContext(source,context);
 return {elements,requests,loc,
  async settle(){for(let i=0;i<30;i++)await Promise.resolve()},
  advance(ms){clock+=ms;tick()},hashchange(h){loc.hash='#'+h;winListeners.hashchange()},
  get html(){return elements['#view'].innerHTML}};
}
const base=()=>({'/api/router/v1/snapshot':snapshot,'/api/fabric':new Error('no fabric'),'/api/stats':{},'/api/macs':{}});
const jobsNone=Object.assign(new Error('no source connected'),{status:404});

test('#lane/<agent> loads that lane ledger and renders the CompassPoint view',async()=>{
 const h=host('#lane/lane-a',{...base(),'/api/router/v1/ledger':ledgerBody,'/api/router/v1/jobs':jobsNone});await h.settle();
 assert.ok(h.requests.includes('/api/router/v1/ledger?agent=lane-a'));assert.ok(h.requests.includes('/api/router/v1/jobs?agent=lane-a'));
 assert.equal(h.elements['#title'].textContent,'lane-a');assert.equal(h.elements['#subtitle'].textContent,'Lane ledger');
 assert.match(h.elements['#nav'].innerHTML,/data-v="lanes" aria-current="page"/,'a lane ledger highlights Lanes in the nav');
 assert.doesNotMatch(h.elements['#nav'].innerHTML,/data-v="overview" aria-current/);
 assert.match(h.html,/<h2 class="ph[^"]*"><span class="boxes"[^>]*>[\s\S]*?<span class="name">Phase 1<\/span>/);
 assert.match(h.html,/router still stores: pending/,'settled derivation runs in the live app');
 assert.match(h.html,/<h2>Router inbox<span class="tally">1 open<\/span><\/h2>/);
 assert.match(h.html,/Cloud jobs: no source connected for this lane/);
 assert.match(h.elements['#live-t'].textContent,/^live · updated .* ET · refreshes every 10 s$/);
});

test('an unknown lane (404) and a failed read (503) never look like an empty ledger',async()=>{
 const nf=host('#lane/ghost',{...base(),'/api/router/v1/ledger':Object.assign(new Error('unknown agent'),{status:404}),'/api/router/v1/jobs':jobsNone});await nf.settle();
 assert.match(nf.html,/Unknown lane\./);assert.doesNotMatch(nf.html,/Empty: nothing waiting/);
 const down=host('#lane/lane-a',{...base(),'/api/router/v1/ledger':new Error('ledger producer unavailable: store read failed'),'/api/router/v1/jobs':jobsNone});await down.settle();
 assert.match(down.html,/Ledger unavailable\./);assert.doesNotMatch(down.html,/Empty: nothing waiting/);
});

test('a refresh failure keeps the last good ledger on screen and says so; a long silence goes stale',async()=>{
 const replies={...base(),'/api/router/v1/ledger':ledgerBody,'/api/router/v1/jobs':jobsNone};
 const h=host('#lane/lane-a',replies);await h.settle();
 replies['/api/router/v1/ledger']=new Error('HTTP 503');
 h.advance(11000);await h.settle();
 assert.match(h.html,/Refresh failed\./);assert.match(h.html,/Showing last good ledger/);assert.match(h.html,/Build it/);
 const hang={...base(),'/api/router/v1/ledger':ledgerBody,'/api/router/v1/jobs':jobsNone};
 const quiet=host('#lane/lane-a',hang);await quiet.settle();
 assert.doesNotMatch(quiet.html,/Stale\./);
 hang['/api/router/v1/ledger']=()=>new Promise(()=>{}); // later reads never answer
 quiet.advance(11000);quiet.advance(11000);await quiet.settle();
 assert.match(quiet.html,/<b>Stale\./);assert.doesNotMatch(quiet.html,/Refresh failed/);assert.match(quiet.html,/Build it/);
});

test('the lane route follows the hash and the ledger refreshes every 10 s',async()=>{
 const h=host('#overview',{...base(),'/api/router/v1/ledger':ledgerBody,'/api/router/v1/jobs':jobsNone});await h.settle();
 assert.ok(!h.requests.some(u=>u.startsWith('/api/router/v1/ledger')),'no ledger read until a lane is opened');
 h.hashchange('lane/lane-a');await h.settle();
 assert.equal(h.requests.filter(u=>u==='/api/router/v1/ledger?agent=lane-a').length,1);
 h.advance(11000);await h.settle();
 assert.ok(h.requests.filter(u=>u==='/api/router/v1/ledger?agent=lane-a').length>=2,'10 s poll re-reads the open lane');
 h.hashchange('queue');await h.settle();assert.equal(h.elements['#title'].textContent,'Inbox');
});

test('hash with an encoded agent id decodes it',async()=>{
 const h=host('#lane/lane%20x',{...base(),'/api/router/v1/ledger':ledgerBody,'/api/router/v1/jobs':jobsNone});await h.settle();
 assert.ok(h.requests.includes('/api/router/v1/ledger?agent=lane%20x'));assert.equal(h.elements['#title'].textContent,'lane x');
});

test('hosted board Host view says swap hygiene is not available there, as a muted line rather than an error',async()=>{
 const h=host('#host',{...base(),'/api/macs':{macs:[{name:'m5',url:'https://m5.example'}]}});await h.settle();
 assert.match(h.html,/<p class="muted">Swap hygiene: not available on the hosted board/);
 assert.doesNotMatch(h.html,/err-card|Run <span class="mono">sirsi swap-hygiene/);
});
