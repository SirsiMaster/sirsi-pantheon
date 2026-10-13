const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');
const source=fs.readFileSync(__dirname+'/ui/app.js','utf8');
test('overview does not request hidden fleet/host data and refresh is single-flight',async()=>{
 const requested=[];let release;
 const response=new Promise(r=>release=r);
 const scope={state:{view:'overview'},Promise,Date,AbortSignal:{timeout:ms=>{assert.equal(ms,60000);return {}}},fetch:u=>{requested.push(u);return response},setLive:()=>{},render:()=>{}};
 vm.createContext(scope);
 vm.runInContext(source.slice(source.indexOf('function get('),source.indexOf('function setLive(')),scope);
 const first=scope.load();assert.equal(scope.load(),first);
 assert.deepEqual(requested,['/api/router/v1/snapshot']);
 release({ok:true,json:()=>Promise.resolve({})});await first;
 assert.equal(scope.state.loading,null);
});
test('fleet navigation and timer share the same outstanding fleet read',async()=>{
 let release,calls=0;const scope={state:{view:'fleet'},get:()=>{calls++;return new Promise(r=>release=r)},render:()=>{}};
 vm.createContext(scope);
 vm.runInContext(source.slice(source.indexOf('function loadFleet('),source.indexOf('function setLive(')),scope);
 const first=scope.loadFleet();assert.equal(scope.loadFleet(),first);assert.equal(calls,1);
 release({tasks:[]});await first;assert.equal(scope.state.fleetLoading,null);
});
