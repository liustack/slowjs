// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// The async context of promise jobs (JS_SetAsyncContext): the cases of
// asynccontext_test.go, each logging into a log of its own, all queued at
// once, so their jobs run interleaved. get, set and run are what
// AsyncLocalStorage's getStore, enterWith and run do.
var get = getAsyncContext, set = setAsyncContext;
function run(value, fn) {
  var outer = get();
  set(value);
  try {
    return fn();
  } finally {
    set(outer);
  }
}
var logs = {};
function test(name, fn) {
  var log = logs[name] = [];
  set(undefined);
  try {
    fn(log);
  } catch (e) {
    log.push('threw ' + e);
  }
  set(undefined);
}
test('two async functions', function (log) {
  var f = async (tag) => { await null; log.push(tag + get().n); await new Promise((r) => Promise.resolve().then(r)); log.push(tag + get().n); };
  run({ n: 1 }, () => f('a'));
  run({ n: 2 }, () => f('b'));
  log.push('top' + String(get()));
});
test('then not resolve', function (log) {
  var resolve;
  var p = new Promise((r) => { resolve = r; });
  run({ n: 'then' }, () => p.then(() => log.push(get().n)));
  run({ n: 'resolve' }, () => resolve());
  set({ n: 'later' });
  Promise.resolve().then(() => log.push(get().n));
  set(undefined);
});
test('outside any context', function (log) {
  var p = Promise.resolve();
  p.then(() => log.push(String(get())));
  set({ n: 'set after then' });
  p.then(() => log.push(get().n));
  set(undefined);
});
test('throwing handler', function (log) {
  run({ n: 'inner' }, () => Promise.resolve().then(() => { throw new Error('x'); })).catch(() => log.push(String(get())));
  run({ n: 'run' }, () => Promise.reject(1).catch(() => log.push(get().n)));
  Promise.resolve().then(() => Promise.resolve()).then(() => log.push(String(get())));
});
test('set inside a job', function (log) {
  run({ n: 'job' }, () => Promise.resolve().then(() => { set({ n: 'inside' }); Promise.resolve().then(() => log.push(get().n)); }));
  Promise.resolve().then(() => Promise.resolve().then(() => log.push(String(get()))));
});
test('thenable', function (log) {
  var thenable = { then(r) { log.push('then' + String(get()?.n)); r(1); } };
  run({ n: 'resolve' }, () => Promise.resolve(thenable).then(() => log.push('after' + String(get()?.n))));
  run({ n: 'await' }, async () => { await thenable; log.push('await' + String(get()?.n)); });
  log.push('top' + String(get()?.n));
});
test('all, finally, generators', function (log) {
  async function* g() { yield 1; await null; yield get().n; }
  run({ n: 'all' }, () => Promise.all([Promise.resolve(), 1]).then(() => log.push(get().n)));
  run({ n: 'finally' }, () => Promise.resolve().finally(() => log.push(get().n)));
  run({ n: 'gen' }, async () => { for await (const v of g()) log.push(String(v)); });
});
test('an object or none', function (log) {
  set(1);
  log.push(String(get()));
  set({ n: 'object' });
  log.push(get().n);
  set(null);
  log.push(String(get()));
});
test('handler keeps this and arguments', function (log) {
  run({ n: 'args' }, () => Promise.resolve(7).then(function (v) { log.push(typeof this + ' ' + v + ' ' + arguments.length + ' ' + get().n); }));
});
// Left for the end of the run: a reaction that holds a context and never
// runs, and a context still set. Freeing the runtime frees both.
globalThis.pending = new Promise(() => {});
run({ big: new Array(1e4).fill(1) }, () => pending.then(() => {}));
(function () {
  set({ n: 'left' });
  return Object.keys(logs).map((k) => k + ': ' + logs[k].join(' ')).join('\n') + '\nat the end: ' + get().n;
});
