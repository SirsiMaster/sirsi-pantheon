const {test}=require('node:test');
const assert=require('node:assert/strict');
const api=require('./ui/board-retire.js');
const plan=()=>api.parse('lane','one\ntwo','Owner administrative retirement; not completed work.');
test('validates complete input before any request',()=>{
 for(const ids of ['', 'one one', '../private', Array.from({length:501},(_,i)=>'t'+i).join('\n')])assert.throws(()=>api.parse('lane',ids,'reason'));
 assert.throws(()=>api.parse('bad/lane','one','reason'));
 assert.throws(()=>api.parse('lane','one',' '));
 assert.deepEqual(plan().ids,['one','two']);
});
test('only cancels exact identities, never reopens or completes',async()=>{
 const calls=[],reports=[];
 await api.run(plan(),async(v,b)=>{calls.push([v,b]);return true},(id,s)=>reports.push([id,s]),()=>false);
 assert.deepEqual(calls.map(x=>[x[0],x[1].task_id]),[['cancel-task','one'],['cancel-task','two']]);
 assert.equal(calls[0][1].note,plan().note);
 assert.equal(reports.length,2);assert.ok(reports.every(x=>x[1]==='canceled (not done)'));
});
test('unconfirmed refusal stops subsequent requests',async()=>{
 const calls=[];
 await api.run(plan(),async(v,b)=>{calls.push([v,b.task_id]);return b.task_id!=='one'},()=>{},()=>false);
 assert.deepEqual(calls,[['cancel-task','one']]);
});
test('unknown response stops without later writes',async()=>{
 const calls=[],reports=[];
 await api.run(plan(),async(v)=>{calls.push(v);throw Error('lost acknowledgement')},(id,s)=>reports.push(s),()=>false);
 assert.deepEqual(calls,['cancel-task']);assert.match(reports[0],/^unknown:/);
});
test('unknown cancellation response is never called success or replayed',async()=>{
 const calls=[],reports=[];
 await api.run(plan(),async(v)=>{calls.push(v);if(v==='cancel-task')throw Error('lost acknowledgement');return true},(id,s)=>reports.push(s),()=>false);
 assert.deepEqual(calls,['cancel-task']);assert.match(reports[0],/^unknown:/);
});
test('stop after current request prevents another request',async()=>{
 let stop=false;const calls=[];
 await api.run(plan(),async(v)=>{calls.push(v);stop=true;return true},()=>{},()=>stop);
 assert.deepEqual(calls,['cancel-task']);
});
