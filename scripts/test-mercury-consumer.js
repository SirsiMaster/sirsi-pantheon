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
// The production renderer uses appendChild at mount and append/replaceChildren
// during draw. Model their node ownership and return value, without evaluating
// producer HTML, mounting a browser, or changing any renderer assertion.
class Element {
  constructor(tag = '') {
    this.tag = tag; this.children = []; this.textContent = ''; this.className = ''; this.attributes = {}; this.parentNode = null;
  }
  appendChild(child) {
    if (child.parentNode) {
      const siblings = child.parentNode.children, index = siblings.indexOf(child);
      if (index >= 0) siblings.splice(index, 1);
    }
    this.children.push(child); child.parentNode = this;
    return child;
  }
  append(...items) { items.forEach(item => this.appendChild(item)); }
  replaceChildren(...items) {
    this.children.forEach(child => { child.parentNode = null; });
    this.children = []; this.append(...items);
  }
  setAttribute(key, value) { this.attributes[key] = String(value); if (key === 'class') this.className = String(value); }
  querySelector(selector) {
    return this.descendants().find(child => selector.startsWith('.')
      ? child.className.split(' ').includes(selector.slice(1)) : child.tag === selector) || null;
  }
  descendants() { return this.children.flatMap(child => [child, ...child.descendants()]); }
  text() { return this.textContent + this.children.map(child => child.text()).join(' '); }
}

function completeRead() {
  return {state: 'ok', message: 'originally complete', read_at: '1970-01-01T00:01:40Z',
    snapshot: {generated_at: 100, generation: 1, source: {host: 'a', build: 'inert'},
      nodes: ['a', 'b'].map(id => ({id, name: id, state: 'live', observed_at: 100, age_s: 0, freshness_s: 6,
        gpu: {work_pct: null, calibration: {status: 'unknown', source: 'fixture'}}, cpu: {busy_pct: 10}, decode: {}, seal: {}})),
      links: [{id: 'ab', state: 'up', ends: [{node: 'a', iface: 'en1'}, {node: 'b', iface: 'en2'}], directions: [
        {from: 'a', to: 'b', observed_at: 95, wire_gbs: 0.1}, {from: 'b', to: 'a', observed_at: 100, wire_gbs: 0.1}]}]},
    headline: {wire: {number: '0.2', unit: 'GB/s', note: ''}, compressed: {number: '0.1', unit: 'GB/s', note: ''},
      gain: {number: '2.00', unit: '×', note: ''}, context: 'CURRENT ACTIVE CONTEXT', run_status: 'running',
      lamps: [{label: 'TCP', on: true}, {label: 'RDMA', on: false}], cables: 1, node_order: ['a', 'b']},
    scene: {width: 1000, height: 400, cards: [], cables: [{id: 'ab', state: 'up', iface: 'en1', letter: 'A',
      x1: 10, y1: 10, x2: 100, y2: 10, lane: 100, rate: {number: '0.2', unit: 'GB/s'}, directions: [
        {from: 'a', to: 'b', rate: '100.0 MB/s', speed: 10, forward: true, color: 'out'},
        {from: 'b', to: 'a', rate: '100.0 MB/s', speed: 10, forward: false, color: 'in'}]}]}};
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

test('embedded renderer expires all aggregate activity while preserving eligible directions', async () => {
  for (const state of ['direction-expired', 'partial', 'envelope-expired']) {
    let mono = 0, fetches = 0, next = 0;
    const timeouts = new Map(), intervals = new Map(), read = completeRead();
    if (state === 'partial') read.state = 'partial';
    class Clock extends Date { static now() { return 100000; } }
    const context = consumer({document: {createElement: tag => new Element(tag), createElementNS: (_, tag) => new Element(tag), activeElement: null},
      performance: {now: () => mono}, Date: Clock, AbortController,
      setTimeout: f => { const id = ++next; timeouts.set(id, f); return id; }, clearTimeout: id => timeouts.delete(id),
      setInterval: f => { const id = ++next; intervals.set(id, f); return id; }, clearInterval: id => intervals.delete(id),
      fetch: async () => { fetches++; return {ok: true, json: async () => read}; }});
    const host = new Element(), stop = context.window.mountMercury(host), shell = host.children[0];
    try {
      await new Promise(done => setImmediate(done));
      if (state !== 'partial') {
        assert.match(shell.querySelector('.mc-lamps').text(), /● TCP/);
        assert.match(shell.querySelector('.mc-lamps').text(), /1 CABLES/);
        assert.equal(shell.querySelector('.mc-hctx').text(), 'CURRENT ACTIVE CONTEXT');
        assert.equal(shell.descendants().filter(e => e.className === 'mc-pulse').length, 24);
      }
      mono = state === 'direction-expired' ? 2000 : state === 'envelope-expired' ? 7000 : 0;
      for (const tick of intervals.values()) tick();
      const lamps = shell.querySelector('.mc-lamps');
      assert.equal(lamps.attributes['aria-label'], 'Current activity unavailable', state);
      assert.doesNotMatch(lamps.text(), /●|1 CABLES/, state);
      assert.match(lamps.text(), /ACTIVE CABLES UNKNOWN/, state);
      assert.notEqual(shell.querySelector('.mc-hctx').text(), 'CURRENT ACTIVE CONTEXT', state);
      assert.match(shell.querySelector('.mc-hctx').text(), /unavailable|no longer current/, state);
      assert.deepEqual(shell.descendants().filter(e => e.className.startsWith('mc-num')).map(e => e.textContent), ['—', '—', '—'], state);
      assert.equal(shell.descendants().filter(e => e.className === 'mc-pulse').length,
        state === 'direction-expired' ? 12 : state === 'partial' ? 24 : 0, state);
      assert.equal(fetches, 1, 'local aging must not start another read');
    } finally { stop(); }
    assert.equal(timeouts.size, 0); assert.equal(intervals.size, 0);
  }
});

test('embedded renderer never carries active lamps into fresh idle null or missing lamp vectors', async () => {
  // projectHeadline's fresh zero-wire, non-running branch returns unknown cells,
  // Idle context, zero active cables and its nil Lamps slice as JSON null.
  // Missing is a separate unavailable observation, not evidence of "off".
  for (const vector of ['null', 'missing', 'empty']) {
    const active = completeRead(), idle = completeRead();
    idle.message = 'fresh idle projection';
    idle.snapshot.run = {configured: false, phase: 'idle'};
    idle.snapshot.links[0].directions.forEach(direction => { direction.wire_gbs = 0; direction.observed_at = 100; });
    idle.scene.cables[0].directions.forEach(direction => { direction.rate = '0.0 MB/s'; direction.speed = 0; });
    idle.scene.cables[0].rate = {number: '0.0', unit: 'MB/s'};
    for (const key of ['wire', 'compressed', 'gain']) idle.headline[key] = {number: '—', unit: '', note: ''};
    idle.headline.context = 'Idle · nothing moving on 1 Thunderbolt links';
    idle.headline.run_status = '';
    idle.headline.cables = 0;
    if (vector === 'missing') delete idle.headline.lamps;
    else idle.headline.lamps = vector === 'null' ? null : [];
    let mono = 0, fetches = 0, next = 0;
    const timeouts = new Map(), intervals = new Map();
    class Clock extends Date { static now() { return 100000; } }
    const context = consumer({document: {createElement: tag => new Element(tag), createElementNS: (_, tag) => new Element(tag), activeElement: null},
      performance: {now: () => mono}, Date: Clock, AbortController,
      setTimeout: f => { const id = ++next; timeouts.set(id, f); return id; }, clearTimeout: id => timeouts.delete(id),
      setInterval: f => { const id = ++next; intervals.set(id, f); return id; }, clearInterval: id => intervals.delete(id),
      fetch: async () => { fetches++; return {ok: true, json: async () => fetches === 1 ? active : idle}; }});
    const host = new Element(), stop = context.window.mountMercury(host), shell = host.children[0];
    try {
      await new Promise(done => setImmediate(done));
      assert.match(shell.querySelector('.mc-lamps').text(), /● TCP/, 'positive active precondition');
      assert.equal(shell.querySelector('.mc-hctx').text(), 'CURRENT ACTIVE CONTEXT');
      stop.refresh();
      await new Promise(done => setImmediate(done));
      assert.equal(fetches, 2, 'exactly the requested two responses');
      const checkIdle = () => {
        const lamps = shell.querySelector('.mc-lamps');
        assert.doesNotMatch(lamps.text(), /●|TCP|RDMA|1 CABLES/, vector);
        assert.match(lamps.text(), /0 CABLES/, vector);
        assert.equal(lamps.attributes['aria-label'], vector === 'missing' ? 'Current activity unavailable' : 'Current activity', vector);
        assert.equal(lamps.text().includes('ACTIVITY LAMPS UNKNOWN'), vector === 'missing', vector);
        assert.equal(shell.querySelector('.mc-hctx').text(), idle.headline.context, vector);
        assert.deepEqual(shell.descendants().filter(e => e.className.startsWith('mc-num')).map(e => e.textContent), ['—', '—', '—'], vector);
        assert.equal(shell.descendants().filter(e => e.className === 'mc-pulse').length, 0, vector);
        assert.equal(shell.querySelector('.mc-run-status'), null, vector);
      };
      checkIdle();
      mono = 2000;
      for (const tick of intervals.values()) tick();
      checkIdle();
      assert.equal(fetches, 2, 'clock redraw must neither fetch nor revive prior activity');
    } finally { stop(); }
    assert.equal(timeouts.size, 0); assert.equal(intervals.size, 0);
  }
});
