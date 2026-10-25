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
 let clock=at,dialog=null,tick,reply={fabric:board(),router:router(),...initial},elements={},listeners={};
 class Element {
  constructor(){this.dataset={};this.handlers={};this.classList={add(){},remove(){},contains(){return false}};this.isConnected=true;this.hidden=false;this._html=''}
  set innerHTML(s){this._html=s;this.buttons=[];for(const m of s.matchAll(/data-fabric-inspect="([^"]+)"/g)){const b=new Element();b.dataset.fabricInspect=m[1];this.buttons.push(b)}this.receipt= s.includes('fabric-receipt') ? new Element():null}
  get innerHTML(){return this._html}
  querySelectorAll(s){return s==='[data-fabric-inspect]'?this.buttons||[]:[]}
  querySelector(){return new Element()}
  addEventListener(k,f){this.handlers[k]=f}
  setAttribute(k,v){this[k]=v}
  focus(){} showModal(){this.open=true} show(){this.open=true} close(){this.open=false} remove(){if(dialog===this)dialog=null}
 }
 const doc={hidden:false,activeElement:null,
  querySelector(s){if(s==='.inspector'||s==='.inspector.persistent')return dialog;return elements[s]||(elements[s]=new Element())},
  querySelectorAll(s){return s==='.fabric-receipt'&&elements['#view']?.receipt?[elements['#view'].receipt]:[]},
  createElement(){return new Element()},
  addEventListener(k,f){listeners[k]=f},
  body:{appendChild(d){dialog=d},classList:{add(){},remove(){}}}
 };
 const context={document:doc,location:{hash:'#'+view},navigator:{},PantheonFabric:fabric,
  Date:class extends Date {constructor(...args){super(...(args.length?args:[clock]))}static now(){return clock}},
  AbortSignal:{timeout(){}},setInterval(f){tick=f;return 1},
  fetch:async url=>{const value=url==='/api/fabric'?reply.fabric:url==='/api/router/v1/snapshot'?reply.router:{};return {ok:!(value instanceof Error),status:503,json:async()=>value instanceof Error?{error:value.message}:value}},
  addEventListener(){},scrollTo(){},scrollY:0,matchMedia(){return {matches:false}}
 };
 context.window=context;vm.createContext(context);vm.runInContext(source,context);
 async function settle(){for(let i=0;i<20;i++)await Promise.resolve()}
 return {elements,doc,reply,settle,get dialog(){return dialog},advance(ms){clock+=ms;tick()},refresh(){elements['#refresh'].handlers.click()},navigate(v){const handler=listeners.click;handler({target:{closest(){return {dataset:{view:v}}}}})},
  clickFabric(){const button=elements['#view'].buttons[0];assert.ok(button,'actual Fabric lane button');button.handlers.click()},
  clickRouter(){listeners.click({target:{closest(){return {dataset:{inspect:'own-lane'}}}}})},
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
