'use strict';
// Built-in Node fixtures only. Reads the actual embedded script; no browser,
// external packages, endpoint, service, model or producer control is invoked.
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const test = require('node:test');
const assert = require('node:assert/strict');
const source = fs.readFileSync(path.join(__dirname, '../internal/dashboard/mercury.go'), 'utf8');
const marker = 'const mercuryJS = `';
assert.equal(source.split(marker).length, 2);
const script = source.split(marker)[1].split('`')[0];

function consumer(extra = {}) {
  const context = {window: {}, ...extra};
  vm.runInNewContext(script, context, {timeout: 1000});
  return context;
}
function mixed() {
  return {state: 'partial', snapshot: {generated_at: 100, source: {host: 'a'},
    nodes: ['a', 'b'].map(id => ({id, state: 'live', observed_at: 100, age_s: 0, freshness_s: 6})),
    links: [{id: 'ab', state: 'up', directions: [
      {from: 'a', to: 'b', observed_at: 93, wire_gbs: 0.1}, {from: 'b', to: 'a', observed_at: 100, wire_gbs: 0.1}]}]},
    scene: {cables: [{id: 'ab', state: 'up', directions: [
      {from: 'a', to: 'b', rate: '— MB/s'}, {from: 'b', to: 'a', rate: '100.0 MB/s'}]}]}};
}
test('typed degraded states stay distinct; only expired current envelopes become stale', () => {
  assert.equal(script.includes('innerHTML'), false, 'producer data stays in text/DOM attributes, never executable HTML');
  const p = consumer().window.mercuryReadEligibility;
  for (const state of ['offline', 'invalid', 'permission_required', 'empty', 'unconfigured']) {
    assert.equal(p.state({state}, 100, 0), state.toUpperCase());
  }
  assert.equal(p.state(mixed(), 100, 0), 'PARTIAL');
  assert.equal(p.state(mixed(), 100, 7), 'STALE');
});
test('partial individuals survive, unknown aggregate and elapsed/future samples do not', () => {
  const p = consumer().window.mercuryReadEligibility, read = mixed(), cable = read.scene.cables[0];
  assert.equal(p.aggregate(read, 100, 0), false);
  assert.equal(p.direction(read, cable, cable.directions[0], 100, 0), false);
  assert.equal(p.direction(read, cable, cable.directions[1], 100, 0), true);
  assert.equal(p.direction(read, cable, cable.directions[1], 100, 7), false);
  read.snapshot.nodes[1].observed_at = 103;
  assert.equal(p.direction(read, cable, cable.directions[1], 100, 0), false);
});
test('completion bound includes transit and does not trust a behind display clock', () => {
  const p = consumer().window.mercuryReadEligibility, read = {read_at: '1970-01-01T00:01:40Z'};
  assert.equal(p.completion(read, 98, 2), 102.001);
  assert.equal(p.completion(read, 105, 2), 105);
  assert.equal(p.completion({}, 100, 0), null);
  assert.equal(p.completion(read, 100, -1), null);
});
test('actual Go mixed read is consumed without replacing unknown totals', {skip: !process.env.PANTHEON_TEST_MERCURY_MIXED_JSON}, () => {
  const data = fs.readFileSync(process.env.PANTHEON_TEST_MERCURY_MIXED_JSON);
  assert.ok(data.length <= 1024 * 1024);
  const read = JSON.parse(data), p = consumer().window.mercuryReadEligibility;
  const at = read.snapshot.generated_at;
  assert.equal(read.state, 'partial'); assert.equal(read.total_gbs, null); assert.equal(read.incoming_gbs, null);
  assert.equal(read.headline.wire.number, '—'); assert.equal(p.aggregate(read, at, 0), false);
  assert.equal(read.headline.compressed.number, '—'); assert.equal(read.headline.gain.number, '—');
  for (const node of read.snapshot.nodes.filter(n => n.state === 'live')) assert.equal(p.node(read, node.id, at, 0), true);
  const cable = read.scene.cables.find(c => c.directions.some(f => f.rate === '— MB/s') && c.directions.some(f => f.rate !== '— MB/s'));
  assert.ok(cable); assert.equal(cable.rate.number, '—');
  for (const flow of cable.directions) assert.equal(p.direction(read, cable, flow, at, 0), flow.rate !== '— MB/s');
});
test('unmount cancels only the owned fetch and prevents late rendering or refresh replay', async () => {
  class Element {
    constructor() { this.children = []; this.textContent = ''; }
    append(...items) { this.children.push(...items); }
    replaceChildren() { this.children = []; }
    setAttribute() {}
    querySelector() { return null; }
  }
  const timeouts = new Map(), intervals = new Map(); let next = 0, resolve, calls = 0, signal;
  const context = consumer({document: {createElement: () => new Element()}, performance: {now: () => 0}, Date,
    AbortController, setTimeout: f => { const id = ++next; timeouts.set(id, f); return id; }, clearTimeout: id => timeouts.delete(id),
    setInterval: f => { const id = ++next; intervals.set(id, f); return id; }, clearInterval: id => intervals.delete(id),
    fetch: (_url, options) => { calls++; signal = options.signal; return new Promise(r => { resolve = r; }); }});
  const host = new Element(), stop = context.window.mountMercury(host);
  assert.equal(calls, 1); assert.equal(signal.aborted, false);
  resolve({ok: true, json: async () => ({state: 'offline', message: 'first retained read'})});
  await new Promise(done => setImmediate(done));
  for (const tick of intervals.values()) tick(); assert.equal(calls, 1, 'local clock never fetches');
  stop.refresh(); // The next owned read is pending while the clock redraws retained evidence.
  assert.equal(calls, 2); assert.equal(signal.aborted, false);
  for (const tick of intervals.values()) tick(); assert.equal(calls, 2, 'retained-read clock never fetches');
  const before = host.children[0].children;
  stop(); assert.equal(signal.aborted, true); assert.equal(intervals.size, 0);
  resolve({ok: true, json: async () => ({state: 'offline', message: 'late'})});
  await new Promise(done => setImmediate(done));
  assert.equal(host.children[0].children, before); assert.equal(timeouts.size, 0);
  stop.refresh(); assert.equal(calls, 2);
});
