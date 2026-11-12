// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// rvdiff: new.target in a generator, an async function and an async
// generator, which are never constructed: it is undefined in each, before
// and after a yield or an await, and an arrow function inside reads the
// same. A constructor next to them reads its own. The functions run
// several times, so that their later calls run in the register loop.
// jsrun: -result
(function () {
var out = [];
function p(x) { out.push(String(x)); }
function* gen() { var a = new.target; yield typeof a; yield typeof new.target; }
async function af() { var a = new.target; await 0; return typeof a + "/" + typeof new.target; }
async function* ag() { yield typeof new.target; await 0; yield (() => typeof new.target)(); }
function* genArrow() { var f = () => new.target; yield typeof f(); yield typeof f(); }
function Ctor() { this.t = new.target === Ctor; }
for (var i = 0; i < 3; i++) {
  p([...gen()].join());
  p([...genArrow()].join());
  af().then(p);
  (async function () { var r = []; for await (var v of ag()) r.push(v); p(r.join()); })();
  p(new Ctor().t);
  p(String(Ctor.call({})));
}
Promise.resolve().then(function () { return 0; }).then(function () {
  rvExpect(gen, af, ag, genArrow, Ctor);
  globalThis.result = out.join(";");
});
})();
