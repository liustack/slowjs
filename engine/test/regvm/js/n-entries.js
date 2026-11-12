// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// rvdiff: the entries into the register loop. Each target is called once
// only, so that it runs in the register loop at its first call (converted
// before it runs), from the loop, from C and from the old loop: a plain
// call, a callback of a builtin, a getter and a setter, a bound function,
// Function.prototype.call and apply, Reflect.apply, a proxy's apply trap and
// the function it calls, a job of a promise, a comparator of sort, a reviver
// of JSON.parse, a replacer of String.prototype.replace, a function of
// another realm called from this one and calling back, and its errors.
(function () { "use strict";
var out = [];
function once(a, b) { return "once " + a + b; }
function cb(x, i) { return x * 10 + i; }
function getter() { return "got " + this.v; }
function setter(x) { this.w = x * 2; }
function bound(a, b) { return this.k + a + b; }
function viaCall(a) { return "call " + this.k + a; }
function viaApply(a, b) { return "apply " + this.k + a + b; }
function viaReflect(a) { return "reflect " + a; }
function trap(t, self, args) { return "trap " + t(args[0]); }
function target(x) { return "target " + x; }
function job(v) { out.push("job " + v); }
function cmp(a, b) { return b - a; }
function reviver(k, v) { return typeof v === "number" ? v + 1 : v; }
function replacer(m, i) { return "<" + m + i + ">"; }
function back(x) { return "back " + x; }

out.push(once(1, 2));
out.push([1, 2, 3].map(cb).join());
var o = { v: 5 };
Object.defineProperty(o, "g", { get: getter, set: setter });
out.push(o.g);
o.g = 4;
out.push("set " + o.w);
out.push(bound.bind({ k: "B" }, 1)(2));
out.push(viaCall.call({ k: "C" }, 1));
out.push(viaApply.apply({ k: "A" }, [1, 2]));
out.push(Reflect.apply(viaReflect, null, ["r"]));
out.push(new Proxy(target, { apply: trap })("p"));
Promise.resolve("j").then(job);
out.push([3, 1, 2].sort(cmp).join());
out.push(JSON.stringify(JSON.parse("[1,{\"a\":2}]", reviver)));
out.push("abc".replace(/b/, replacer));

// another realm: its functions run in the loop with their own realm
var other = newRealm('"use strict"; ({' +
  ' twice: function (x) { return x * 2; },' +
  ' call: function (f, x) { return f(x) + "!"; },' +
  ' arr: function () { return []; },' +
  ' bad: function (o) { return o.x.y; } })');
function cross(n) {
  var s = 0;
  for (var i = 0; i < n; i++)
    s += other.twice(i);
  return s;
}
out.push("cross " + cross(10) + " " + other.call(back, 7));
out.push("realm " + (Object.getPrototypeOf(other.arr()) === Array.prototype) + " " +
         Array.isArray(other.arr()));
function crossThrow() {
  try {
    other.bad({});
    return "no";
  } catch (e) {
    return e.name + " " + (e instanceof TypeError);
  }
}
out.push(crossThrow());

rvExpect(once, cb, getter, setter, bound, viaCall, viaApply, viaReflect, trap, target,
         cmp, reviver, replacer, back, cross, crossThrow, other.twice, other.call, other.bad);
return function () { rvExpect(job); return out.join("\n"); };
})()
