// Run by TestConsoleProbeJS: node internal/dashboard/ui/consoleprobe.test.js
'use strict';
const assert = require('node:assert');
require('./consoleprobe.js');
const { live, probe, pick } = globalThis.consoleProbe;

const res = (status, extra) => Object.assign({ status, type: 'cors', redirected: false }, extra);
assert.strictEqual(live(res(200), 'ok\n'), true, '200 ok is live');
for (const s of [403, 404, 500, 502]) assert.strictEqual(live(res(s), 'ok'), false, `${s} is not live`);
assert.strictEqual(live(res(0, { type: 'opaque' }), ''), false, 'opaque is unknown, not live');
assert.strictEqual(live(res(200, { redirected: true }), 'ok'), false, 'redirected is not live');
assert.strictEqual(live(res(200), '<html>sign in</html>'), false, 'a login page is not live');

const macs = [{ name: 'm1', url: 'https://m1.t.ts.net' }, { name: 'm5', url: 'https://m5.t.ts.net' }];
assert.strictEqual(pick(macs, [false, true]).name, 'm5', 'first bad, second healthy picks the second');
assert.strictEqual(pick(macs, [false, false]), null, 'none healthy picks none');

const answers = {
  'https://m1.t.ts.net/healthz': () => Promise.reject(new TypeError('redirect')),
  'https://m5.t.ts.net/healthz': () => Promise.resolve(Object.assign(res(200), { text: () => Promise.resolve('ok\n') })),
};
const fake = (url, opts) => {
  assert.strictEqual(opts.mode, 'cors');
  assert.strictEqual(opts.redirect, 'error');
  return answers[url]();
};
Promise.all(macs.map(m => probe(fake, m.url + '/'))).then(oks => {
  assert.deepStrictEqual(oks, [false, true]);
  console.log('consoleprobe ok');
}).catch(e => { console.error(e); process.exit(1); });
