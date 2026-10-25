const {test} = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const source = fs.readFileSync(__dirname+'/ui/app.js','utf8');

function fixture(get) {
 const scope = {state:{view:'host'}, get, render(){}, ring:()=>'', rel:()=>'',
  esc:s=>String(s??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]))};
 vm.createContext(scope);
 vm.runInContext(source.slice(source.indexOf('function memoryDiagnostics('),source.indexOf('var RENDER=')),scope);
 return scope;
}

test('actual Host diagnostic is explicit, ranked, escaped and usable without router data',async()=>{
 const calls=[];
 const scope=fixture(async path=>{calls.push(path);return {top:[{name:'<fixture-worker>',pid:123,rss_bytes:2097152}]}});
 assert.match(scope.vHost({}),/Inspect memory/);
 assert.deepEqual(calls,[]); // Rendering/ordinary refresh never samples processes.
 let click;
 scope.document={addEventListener(name,fn){assert.equal(name,'click');click=fn}};
 vm.runInContext(source.slice(source.indexOf("document.addEventListener('click'"),source.indexOf("document.addEventListener('keydown'")),scope);
 const actual=scope.inspectMemory;
 let read;
 scope.inspectMemory=()=>{read=actual();return read};
 click({target:{closest(selector){assert.match(selector,/data-memory/);return {dataset:{memory:'inspect'}}}}});
 await read;
 assert.deepEqual(calls,['/api/vitals']);
 const html=scope.vHost({});
 assert.match(html,/&lt;fixture-worker&gt;/);
 assert.match(html,/2.0 MiB/);
 assert.match(html,/123/);
 assert.match(html,/Read at /);
 assert.doesNotMatch(html,/<fixture-worker>/);
 // The production root renders Host before the router-unavailable return.
 assert.ok(source.indexOf("if(state.view==='host'){el.innerHTML=vHost(state.data||{})") < source.indexOf('if(!state.data){el.innerHTML='));
});

test('empty ranked snapshot is truthful and keeps the real Inspect action',async()=>{
 const scope=fixture(async()=>({top:[]}));
 await scope.inspectMemory();
 const html=scope.memoryDiagnostics();
 assert.match(html,/No ranked process data was returned/);
 assert.match(html,/does not establish that no processes are running/);
 assert.match(html,/data-memory="inspect"/);
 assert.doesNotMatch(html,/<tbody>/);
 assert.equal(scope.state.memoryBusy,false);
});

test('failed diagnostic retains last read as previous and retry uses a fresh explicit request',async()=>{
 let calls=0;
 const scope=fixture(async()=>{calls++;if(calls===2)throw new Error('inert unavailable');return {top:[{name:'fixture-'+calls,pid:1,rss_bytes:1}]}});
 await scope.inspectMemory();
 await scope.inspectMemory();
 let html=scope.memoryDiagnostics();
 assert.match(html,/Memory diagnostics unavailable/);
 assert.match(html,/Previous read/);
 assert.match(html,/fixture-1/);
 assert.match(html,/Try Inspect memory again/);
 assert.equal(scope.state.memoryBusy,false);
 await scope.inspectMemory();
 html=scope.memoryDiagnostics();
 assert.match(html,/fixture-3/);
 assert.doesNotMatch(html,/Previous read|unavailable/);
});

test('null or missing ranking is unavailable rather than a fabricated empty diagnostic',async()=>{
 for(const read of [{top:null},{}]){
  const scope=fixture(async()=>read);
  await scope.inspectMemory();
  assert.match(scope.memoryDiagnostics(),/Memory diagnostics unavailable/);
  assert.doesNotMatch(scope.memoryDiagnostics(),/No ranked process data was returned/);
 }
});

test('concurrent clicks submit only one diagnostic read',async()=>{
 let release,calls=0;
 const scope=fixture(()=>{calls++;return new Promise(resolve=>{release=resolve})});
 const pending=scope.inspectMemory();
 assert.match(scope.memoryDiagnostics(),/Reading memory…/);
 assert.match(scope.memoryDiagnostics(),/disabled/);
 await scope.inspectMemory();
 assert.equal(calls,1);
 release({top:[]});
 await pending;
 assert.equal(scope.state.memoryBusy,false);
});
