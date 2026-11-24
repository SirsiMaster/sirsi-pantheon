const {test}=require('node:test');
const assert=require('node:assert/strict');
const api=require('./ui/board-signoff.js');
const create=()=>api.attempt('lane','exact-task','Original scope passed; retained evidence is reviewed.','owner@sirsi.ai');
test('validates and snapshots one exact verified intent before requests',()=>{
 for(const args of [['../lane','task','evidence','owner@sirsi.ai'],['lane','task/other','evidence','owner@sirsi.ai'],['lane','task',' ','owner@sirsi.ai'],['lane','task','x'.repeat(4001),'owner@sirsi.ai'],['lane','task','evidence','foreign@example.com']])assert.throws(()=>api.attempt(...args));
 const attempt=create();assert.ok(Object.isFrozen(attempt.plan));
 assert.equal(attempt.plan.task_id,'exact-task');assert.equal(attempt.plan.expected_owner,'board:owner@sirsi.ai');
});
test('requires current owner and sends only one exact verified completion',async()=>{
 const calls=[],attempt=create();let reads=0;
 assert.equal(await attempt.run(async()=>{reads++;return 'owner@sirsi.ai'},async(v,b)=>{calls.push([v,b]);return true}),'completed');
 assert.equal(reads,2);assert.equal(calls.length,1);assert.equal(calls[0][0],'sign-off-verified');
 assert.deepEqual(calls[0][1],attempt.plan);assert.equal(calls[0][1].note,'Original scope passed; retained evidence is reviewed.');
 assert.equal(await attempt.run(async()=>{throw Error('must not read')},async()=>{throw Error('must not repeat')}),'already-started');
});
test('changed or unavailable owner before dispatch makes no mutation',async()=>{
 for(const [owner,expected] of [[async()=> 'other@sirsi.ai','refused'],[async()=>{throw Error('offline')},'unavailable']]){
  let writes=0;const result=await create().run(owner,async()=>{writes++;return true});
  assert.equal(result,expected);assert.equal(writes,0);
 }
});
test('double submit while owner verification is held cannot dispatch twice',async()=>{
 let release,reads=0,writes=0;const attempt=create(),held=new Promise(r=>release=r);
 const first=attempt.run(async()=>{reads++;if(reads===1)return held;return 'owner@sirsi.ai'},async()=>{writes++;return true});
 assert.equal(await attempt.run(async()=> 'owner@sirsi.ai',async()=>{writes++;return true}),'already-started');
 release('owner@sirsi.ai');assert.equal(await first,'completed');assert.equal(writes,1);
});
test('definite server refusal is not completion',async()=>{
 let writes=0;assert.equal(await create().run(async()=> 'owner@sirsi.ai',async()=>{writes++;return false}),'refused');assert.equal(writes,1);
});
test('lost acknowledgement is unknown and never automatically retried',async()=>{
 let writes=0;const attempt=create();
 assert.equal(await attempt.run(async()=> 'owner@sirsi.ai',async()=>{writes++;throw Error('applied then response lost')}),'unknown');
 assert.equal(await attempt.run(async()=> 'owner@sirsi.ai',async()=>{writes++;return true}),'already-started');assert.equal(writes,1);
});
test('owner changes after write: no stale completion publication',async()=>{
 let reads=0,writes=0;const result=await create().run(async()=> ++reads===1?'owner@sirsi.ai':'other@sirsi.ai',async()=>{writes++;return true});
 assert.equal(result,'unknown');assert.equal(writes,1);
});
