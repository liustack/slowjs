// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// A call that hands over the last reference to the function it calls: the
// function object (and with a Function made at run time, its bytecode and
// register code) is freed when the frame lets go of its function slot,
// while the return path may still read the function. Run under ASan: a
// use after free shows there, not in the printed values. Each shape is
// called three times, so that the first call converts the function and the
// later ones run it in the register loop, and it returns normally, throws,
// and returns through finally.
(function () {
  "use strict";
  var L = [];
  function show(v) {
    if (typeof v === "object" && v !== null) return "{" + Object.keys(v).join(",") + "}";
    return typeof v + ":" + String(v);
  }
  function run(name, f) {
    for (var i = 0; i < 3; i++) {
      try {
        L.push(name + " " + i + " = " + show(f(i)));
      } catch (e) {
        L.push(name + " " + i + " ! " + (e instanceof Error ? e.name + ": " + e.message : show(e)));
      }
    }
  }

  // the bodies: return, throw, return through finally, a value made after
  // the work (so the frame does more than one op)
  var bodies = {
    ret: "return 7;",
    retArgs: "var s = 0; for (var i = 0; i < arguments.length; i++) s += typeof arguments[i] === 'number' ? arguments[i] : 1; return s;",
    thr: "throw new RangeError('lastref');",
    fin: "try { return 1; } finally { var z = [1, 2]; z.push(3); }",
    finThrow: "try { throw new TypeError('in try'); } finally { return 'from finally'; }",
    loop: "var a = 0; for (var i = 0; i < 20; i++) a += i; return a;",
  };
  var modes = { strict: '"use strict"; ', sloppy: "" };

  for (var m in modes) {
    for (var b in bodies) {
      (function (m, b) {
        var src = modes[m] + bodies[b];
        // a Function made at run time: the caller's constants do not keep
        // its bytecode alive; f() converts it, box.pop() hands over the
        // only reference
        run("fn-box " + m + " " + b, function () {
          var f = Function(src);
          try { f(); } catch (e) {}
          var box = [f];
          f = null;
          return box.pop()({});
        });
        run("fn-box-args " + m + " " + b, function () {
          var f = Function("a", "b", "c", src);
          try { f(1, 2, 3); } catch (e) {}
          var box = [f];
          f = null;
          return box.pop()(1, 2, 3, 4, 5);
        });
        run("fn-box-noargs " + m + " " + b, function () {
          var f = Function("a", "b", src);
          try { f(1, 2); } catch (e) {}
          var box = [f];
          f = null;
          return box.pop()();
        });
        // the only reference held by a property deleted before the call
        run("fn-prop " + m + " " + b, function () {
          var o = { f: Function(src) };
          try { o.f(); } catch (e) {}
          var g = o.f;
          delete o.f;
          return [g].pop().call(o, 1);
        });
        // a method that deletes itself from its object while it runs
        run("fn-delete-self " + m + " " + b, function () {
          var o = {};
          var g = Function("o", "delete o.m; " + src);
          try { g(o); } catch (e) {}
          o.m = g;
          g = null;
          return o.m(o);
        });
        // a function that overwrites the only variable holding itself
        run("fn-self-overwrite " + m + " " + b, function () {
          globalThis.__lastref = Function("globalThis.__lastref = null; " + src);
          var keep = globalThis.__lastref;
          try { keep(); } catch (e) {}
          globalThis.__lastref = keep;
          keep = null;
          return globalThis.__lastref();
        });
      })(m, b);
    }
  }

  // closures: made and called once, the bytecode shared by all of them, so
  // the first converts it and the later ones run in the register loop
  run("iife", function (i) { return (function () { return i + 1; })(); });
  run("iife-throw", function (i) { return (function () { throw new Error("iife " + i); })(); });
  run("iife-finally", function (i) { return (function () { try { return i; } finally { i++; } })(); });
  run("iife-args", function (i) { return (function (a, b) { return a + b + arguments.length; })(i, 2, 3, 4); });
  run("box-pop", function (i) { return [function () { return i * 2; }].pop()(); });
  run("box-pop-throw", function (i) { return [function () { throw new TypeError("pop " + i); }].pop()(); });
  run("box-pop-finally", function (i) { return [function () { try { throw 1; } catch (e) { return e + i; } finally { i = 0; } }].pop()(); });
  run("arrow-once", function (i) { return ((x) => x + i)(1); });
  run("method-delete-self", function (i) {
    var o = { n: i };
    o.m = function () { delete o.m; return this.n + 1; };
    return o.m();
  });
  run("method-delete-self-throw", function (i) {
    var o = {};
    o.m = function () { delete o.m; throw new RangeError("m " + i); };
    return o.m();
  });
  run("method-delete-self-finally", function (i) {
    var o = {};
    o.m = function () { delete o.m; try { return i; } finally { o.k = 1; } };
    return o.m() + Object.keys(o).length;
  });
  run("var-overwrite-self", function (i) {
    var g = function () { g = null; return i + 3; };
    return g();
  });
  run("var-overwrite-self-throw", function (i) {
    var g = function () { g = 0; throw new Error("g " + i); };
    return g();
  });
  run("global-overwrite-self", function (i) {
    globalThis.__lastref2 = function () { globalThis.__lastref2 = undefined; return i; };
    return globalThis.__lastref2();
  });
  run("call-apply", function (i) {
    return [function (a) { return this.v + a; }].pop().apply({ v: i }, [5]);
  });
  run("new-once", function (i) {
    return new ([function (x) { this.x = x; }].pop())(i);
  });
  // generators and async functions, called once, the only reference in a box
  run("gen-once", function (i) {
    var it = [function* () { yield i; return i + 1; }].pop()();
    return it.next().value + it.next().value;
  });
  run("gen-fn-box", function () {
    var f = Function("return (function* () { yield 1; yield 2; })")();
    var box = [f];
    f = null;
    var it = box.pop()();
    return it.next().value + it.next().value;
  });
  run("async-once", function (i) {
    var p = [async function () { return i; }].pop()();
    p.then(function (v) { L.push("async-once then " + v); });
    return typeof p;
  });
  run("async-throw-once", function (i) {
    var p = [async function () { throw new Error("a " + i); }].pop()();
    p.catch(function (e) { L.push("async-throw-once catch " + e.message); });
    return typeof p;
  });
  // the jobs of the async shapes write to L after the run: return a
  // function, which the host calls after the jobs
  return function () { return L.join("\n"); };
})()
