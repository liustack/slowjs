// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// A closure holds the value of a captured variable that no code can change
// once it is captured (capture_by_value, engine/quickjs.c): constants,
// parameters, a let set before the closure is made, the binding of each
// round of a for with let and of a for of, a variable two and three
// functions down, a function declaration, a variable captured in its dead
// zone (the closure throws when it reads it), objects whose properties
// change, more than 64 captured variables, closures made in a generator and
// an async function that go on later, closures kept after their function
// threw, cycles through a captured value that a collection frees, and next
// to them variables written after the capture, which keep their reference.
(function () {
  "use strict";
  var out = [];
  function rec(label, f) {
    var r;
    try {
      r = f();
    } catch (e) {
      r = "throws " + e.name;
    }
    out.push(label + ": " + r);
  }

  rec("const", function () {
    const k = 7, s = "str" + k, o = { a: 1 };
    const f = () => k + s.length + o.a;
    var t = 0;
    for (var i = 0; i < 1000; i++)
      t += f();
    return t;
  });
  rec("let before", function () {
    let x = 1;
    x = x + 1;
    const f = () => x * 10;
    return f() + f();
  });
  rec("let after", function () {
    let x = 1;
    const f = () => x;
    x = 2;
    return f();
  });
  rec("params", function () {
    function g(a, b, c) {
      return () => [a, b, c, arguments.length].join(",");
    }
    return g(1, "two")() + " " + g(undefined, null, 3n)();
  });
  rec("for let", function () {
    const fs = [];
    for (let i = 0; i < 5; i++)
      fs.push(() => i);
    return fs.map(f => f()).join(",");
  });
  rec("for let written", function () {
    const fs = [];
    for (let i = 0; i < 6; i++) {
      fs.push(() => i);
      i++;
    }
    return fs.map(f => f()).join(",");
  });
  rec("for of", function () {
    const fs = [];
    for (const v of ["a", 2, null, 4.5])
      fs.push(() => v);
    return fs.map(f => String(f())).join(",");
  });
  rec("two down", function () {
    const x = "deep";
    const f = () => () => () => x;
    return f()()() + f()()().length;
  });
  rec("two down written", function () {
    let x = 1;
    const f = () => () => x++;
    f()();
    return x + " " + f()();
  });
  rec("declarations", function () {
    function a(n) { return n ? b(n - 1) : "a"; }
    function b(n) { return n ? a(n - 1) : "b"; }
    return a(3) + a(4) + [a, b].map(h => typeof h).join("");
  });
  rec("dead zone", function () {
    function g(v) {
      switch (v) {
      case 0:
        let x = 1;
        return () => x;
      case 1:
        return () => x;
      }
    }
    var r = g(0)() + " ";
    try {
      g(1)();
    } catch (e) {
      r += e.name;
    }
    return r;
  });
  rec("object", function () {
    const o = { n: 0 }, a = [];
    const f = () => o.n + a.length;
    o.n = 5;
    a.push(1, 2);
    return f() + " " + (f() === 7);
  });
  rec("many", function () {
    const v0 = 0, v1 = 1, v2 = 2, v3 = 3, v4 = 4, v5 = 5, v6 = 6, v7 = 7, v8 = 8, v9 = 9;
    const w0 = 10, w1 = 11, w2 = 12, w3 = 13, w4 = 14, w5 = 15, w6 = 16, w7 = 17, w8 = 18, w9 = 19;
    const x0 = 20, x1 = 21, x2 = 22, x3 = 23, x4 = 24, x5 = 25, x6 = 26, x7 = 27, x8 = 28, x9 = 29;
    const y0 = 30, y1 = 31, y2 = 32, y3 = 33, y4 = 34, y5 = 35, y6 = 36, y7 = 37, y8 = 38, y9 = 39;
    const z0 = 40, z1 = 41, z2 = 42, z3 = 43, z4 = 44, z5 = 45, z6 = 46, z7 = 47, z8 = 48, z9 = 49;
    const u0 = 50, u1 = 51, u2 = 52, u3 = 53, u4 = 54, u5 = 55, u6 = 56, u7 = 57, u8 = 58, u9 = 59;
    let q0 = 60, q1 = 61, q2 = 62, q3 = 63, q4 = 64, q5 = 65, q6 = 66, q7 = 67, q8 = 68, q9 = 69;
    const f = () => v0 + v1 + v2 + v3 + v4 + v5 + v6 + v7 + v8 + v9 +
      w0 + w1 + w2 + w3 + w4 + w5 + w6 + w7 + w8 + w9 +
      x0 + x1 + x2 + x3 + x4 + x5 + x6 + x7 + x8 + x9 +
      y0 + y1 + y2 + y3 + y4 + y5 + y6 + y7 + y8 + y9 +
      z0 + z1 + z2 + z3 + z4 + z5 + z6 + z7 + z8 + z9 +
      u0 + u1 + u2 + u3 + u4 + u5 + u6 + u7 + u8 + u9 +
      q0 + q1 + q2 + q3 + q4 + q5 + q6 + q7 + q8 + q9;
    q5 = 0;
    q9 = 1000;
    return f();
  });
  rec("generator", function () {
    function* g(a) {
      const k = a * 2;
      const f = () => k + a;
      let w = 1;
      const h = () => w;
      yield f();
      w = 5;
      yield f() + h();
      return () => f() + h();
    }
    const it = g(3);
    const r = [it.next().value, it.next().value];
    const last = it.next().value;
    r.push(last());
    const left = g(10);
    left.next();
    return r.join(",");
  });
  rec("async", function () {
    var seen = [];
    async function a(n) {
      const k = n + 1;
      let w = 0;
      const f = () => k + w;
      seen.push(f());
      await null;
      w = 100;
      seen.push(f());
      return () => k;
    }
    a(1).then(f => out.push("async then: " + f() + " " + seen.join(",")));
    a(2);
    return "started " + seen.join(",");
  });
  rec("after throw", function () {
    var kept = [];
    function g(n) {
      const v = "v" + n;
      kept.push(() => v);
      if (n % 2)
        throw new RangeError("odd " + n);
      return n;
    }
    var r = [];
    for (var i = 0; i < 4; i++) {
      try {
        r.push(g(i));
      } catch (e) {
        r.push(e.name);
      }
    }
    return r.join(",") + " " + kept.map(f => f()).join(",");
  });
  rec("cycles", function () {
    var made = 0;
    for (var i = 0; i < 200; i++) {
      const self = { i: i };
      const f = () => self;
      self.f = f;
      made += f().i === i ? 1 : 0;
    }
    return made;
  });
  rec("class", function () {
    const base = 40;
    class A {
      v = base + 1;
      static s = base + 2;
      m() { return base + 3; }
    }
    const a = new A();
    return [a.v, A.s, a.m()].join(",");
  });
  rec("recursion", function () {
    function g(n) {
      const k = n;
      const f = () => k;
      return n ? f() + g(n - 1) : f();
    }
    return g(50);
  });
  if (typeof gc === "function")
    gc();
  // called after the jobs ran: the async rounds have pushed their lines
  return function () {
    return out.join("\n");
  };
})();
