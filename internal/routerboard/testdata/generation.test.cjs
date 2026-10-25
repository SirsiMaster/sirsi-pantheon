'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),vm=require('node:vm');
const html=fs.readFileSync(__dirname+'/../ui/index.html','utf8');
const scripts=Array.from(html.matchAll(/<script>([\s\S]*?)<\/script>/g),m=>m[1]).filter(s=>s.includes('function render(d)')&&s.includes('new EventSource('));
assert.equal(scripts.length,1,'exact actual legacy renderer script required, not the file-URL redirect');
const script=scripts[0];
function wire(){
 const path=process.env.PANTHEON_TEST_FABRIC_GENERATION_WIRE_OUTPUT;
 assert.ok(path,'Main must supply the actual mixed Go generation export; absence is not qualification');
 return JSON.parse(fs.readFileSync(path,'utf8'));
}
function host(){
 const elements=new Map(),messages=[],network=[];let source;
 class Element { constructor(){this.style={};this.textContent='';this.innerHTML='';this.classList={add(){},remove(){},toggle(){}};this.offsetWidth=0} }
 const context={document:{getElementById(id){if(!elements.has(id))elements.set(id,new Element());return elements.get(id)}},
  localStorage:{getItem(){return null},setItem(){}},location:{reload(){assert.fail('unexpected reload')}},
  fetch(url){network.push(url);throw Error('forbidden live mutation/network')},
  EventSource:class {constructor(url){assert.equal(url,'/api/router/v1/stream');source=this}addEventListener(){}},
  Date,console,setTimeout,clearTimeout};
 context.window=context;context.parent={postMessage(message){messages.push(message)}};
 vm.createContext(context);vm.runInContext(script,context);
 return {elements,messages,network,render(payload){source.onmessage({data:JSON.stringify(payload)});assert.doesNotMatch(elements.get('err').textContent,/render error:/)},value(id){return elements.get(id).textContent}};
}
test('actual legacy SSE renderer consumes the real Go generation and retains full burndown',()=>{
 const w=wire(),h=host();h.render(w.legacy);
 assert.equal(h.messages[0].schema,'router-board.v1');assert.equal(h.value('c1-v'),'1 / 5');assert.equal(h.value('c2-v'),'1 / 2');assert.equal(h.value('c3-v'),'1 / 1 / 1');
 assert.match(h.elements.get('phaseburndown').innerHTML,/Build/);assert.match(h.elements.get('phaseburndown').innerHTML,/Inert step/);assert.match(h.elements.get('opentasks').innerHTML,/pending/);
 assert.match(h.elements.get('apps').innerHTML,/0 of 3 lanes actively working/);assert.match(h.elements.get('apps').innerHTML,/IDLE with work/);assert.match(h.elements.get('apps').innerHTML,/UNROUTABLE/);assert.doesNotMatch(h.elements.get('registration').innerHTML,/horus/);
 assert.equal(w.legacy.generation,w.horus.fabric_generation);assert.deepEqual(w.legacy.fabric,w.fabric);assert.deepEqual(w.horus.fabric,w.fabric);
 assert.equal(w.ledger.active_tasks,5);assert.equal(w.legacy.ledger.active_tasks,5);assert.equal(w.legacy.ledger.blocked_tasks,1);
 assert.equal(h.elements.get('c1-v').style.color,w.fabric.presentation.palette.ok.dark);assert.deepEqual(h.network,[]);
});
test('legacy task and heartbeat detail cannot independently recount canonical headlines',()=>{
 const w=wire(),h=host();for(const task of w.legacy.tasks){task.status='done';task.liveness='active'}for(const lane of w.legacy.fleet){lane.activity='active now';lane.idle_with_work=false}
 h.render(w.legacy);assert.equal(h.value('c1-v'),'1 / 5');assert.equal(h.value('c2-v'),'1 / 2');assert.equal(h.value('c3-v'),'1 / 1 / 1');assert.match(h.elements.get('apps').innerHTML,/0 of 3 lanes actively working/);assert.deepEqual(h.network,[]);
});
test('missing canonical generation stays unavailable while legacy detail remains reachable',()=>{
 const w=wire(),h=host();h.render(w.legacy);delete w.legacy.fabric;h.render(w.legacy);
 for(const id of ['c1-v','c2-v','c3-v']){assert.equal(h.value(id),'Unavailable');assert.equal(h.elements.get(id).style.color,'')}
 assert.match(h.elements.get('phaseburndown').innerHTML,/Build/);assert.match(h.value('c1-s'),/legacy detail is separate/);assert.deepEqual(h.network,[]);
});
test('missing semantic palette is neutral and never blocks the retained legacy workflow',()=>{
 const w=wire(),h=host();delete w.legacy.fabric.presentation;h.render(w.legacy);
 assert.equal(h.value('c1-v'),'1 / 5');assert.equal(h.elements.get('c1-v').style.color,'');assert.match(h.elements.get('phaseburndown').innerHTML,/Inert step/);assert.deepEqual(h.network,[]);
});
