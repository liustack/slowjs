// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package slowjs // import "github.com/liustack/slowjs"

import "testing"

// An await whose PromiseResolve throws an error a script can catch throws it
// at the await (Await: ? PromiseResolve): a catch around it sees it, a
// finally runs and the function goes on. The async function was left
// suspended at the await for good, its promise rejected, and neither ran.
// The error comes from a getter of constructor of the promise awaited, and
// from the call depth refusing the calls of that getter. Each want but that of
// the call depth is what Node 24 gives, whose depth is the bytes of its stack:
// the engine counts levels, so that want holds on every run
// (engine/test/compare/await_resolve_throws.js has more cases).
func TestAwaitPromiseResolveThrows(t *testing.T) {
	const setup = `
globalThis.out = [];
globalThis.poisoned = (v, msg) => {
  const p = Promise.resolve(v);
  Object.defineProperty(p, 'constructor', { get() { throw new Error(msg); } });
  return p;
};
globalThis.settle = (pr, log) => pr.then(v => log.push('resolved ' + v), e => log.push('rejected ' + e.message))
  .then(() => { out.push(log.join(' | ')); });
`
	cases := []struct {
		name, js, want string
	}{
		{"top", `
const log = [];
settle((async () => {
  try { await poisoned(1, 'boom'); log.push('not here'); } catch (e) { log.push('caught ' + e.message); } finally { log.push('finally'); }
  return 'done';
})(), log);`, "caught boom | finally | resolved done"},
		{"nested await", `
const log = [];
settle((async () => {
  const inner = async () => {
    await null;
    try { return await poisoned(2, 'nested'); } catch (e) { log.push('inner caught ' + e.message); return 'inner'; } finally { log.push('inner finally'); }
  };
  const v = await inner();
  try { await (async () => { await poisoned(3, 'deeper'); })(); } catch (e) { log.push('outer caught ' + e.message); }
  return v;
})(), log);`, "inner caught nested | inner finally | outer caught deeper | resolved inner"},
		{"class method arrow", `
const log = [];
class Box {
  constructor() { this.n = 0; }
  method() {
    const step = async (p) => {
      try { this.n += await p; } catch (e) { log.push('arrow caught ' + e.message); this.n += 10; } finally { log.push('arrow finally'); }
      return this.n;
    };
    return step(poisoned(1, 'in arrow'));
  }
}
settle(new Box().method(), log);`, "arrow caught in arrow | arrow finally | resolved 10"},
		{"no catch", `
const log = [];
settle((async () => {
  try { await poisoned(1, 'uncaught'); } finally { log.push('finally'); }
  log.push('not here');
})(), log);`, "finally | rejected uncaught"},
		// Near the call depth the body starts but the calls of its getter,
		// bound twice, are refused. The body records into properties, as a
		// call there could be refused too, and each level of the unwinding
		// starts a run.
		{"call depth", `
const p = Promise.resolve(1);
Object.defineProperty(p, 'constructor', { get: (function () { return Promise; }).bind(null).bind(null) });
const runs = [];
async function body(rec) {
  rec.started = true;
  try { await p; rec.at = 'resumed'; } catch (e) { rec.at = 'caught ' + e.name; } finally { rec.fin = 'finally'; }
  return 'ok';
}
function down() {
  try { down(); } catch (e) {}
  const rec = { started: false, at: 'suspended', fin: 'no finally' };
  let pr;
  try { pr = body(rec); } catch (e) { return; }
  runs.push({ rec, pr });
}
down();
Promise.all(runs.map(r => r.pr.then(v => v, e => 'rejected ' + e.name)
  .then(end => r.rec.started ? [r.rec.at, r.rec.fin, end].join(' ') : 'not started'))).then(outs => {
  const seen = new Set(outs);
  const other = [...seen].filter(k => k !== 'not started' && k !== 'resumed finally ok' && k !== 'caught RangeError finally ok');
  out.push('refused at the await: ' + (seen.has('caught RangeError finally ok') ? 'some' : 'none') +
    ', other outcomes: ' + (other.length ? other.join('; ') : 'none'));
});`, "refused at the await: some, other outcomes: none"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := newTestVM(t)
			if _, err := m.Eval(setup, EvalGlobal); err != nil {
				t.Fatal(err)
			}
			if _, err := m.Eval(c.js, EvalGlobal); err != nil {
				t.Fatal(err)
			}
			if _, err := m.ExecutePendingJobs(); err != nil {
				t.Fatal(err)
			}
			wantEval(t, m, `out.join('\n')`, c.want)
		})
	}
}
