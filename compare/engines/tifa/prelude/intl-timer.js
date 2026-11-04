// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// Times the work done inside Intl and the locale methods, from the outermost
// call only, with the host's monotonic clock (__jsbench_now). Load it after
// the Intl it should time.
(function () {
  var now = globalThis.__jsbench_now;
  if (typeof now !== "function") throw new Error("intl-timer needs __jsbench_now");
  var depth = 0, total = 0, calls = 0;
  function timed(f) {
    return function () {
      if (depth > 0) return f.apply(this, arguments);
      depth = 1; calls++;
      var t = now();
      try { return f.apply(this, arguments); } finally { total += now() - t; depth = 0; }
    };
  }
  function timedNew(C) {
    var p = new Proxy(C, {
      construct: function (t, a, nt) {
        if (depth > 0) return Reflect.construct(t, a, nt);
        depth = 1; calls++;
        var s = now();
        try { return Reflect.construct(t, a, nt === p ? t : nt); } finally { total += now() - s; depth = 0; }
      },
      apply: function (t, self, a) {
        if (depth > 0) return Reflect.apply(t, self, a);
        depth = 1; calls++;
        var s = now();
        try { return Reflect.apply(t, self, a); } finally { total += now() - s; depth = 0; }
      },
    });
    return p;
  }
  var bound = new WeakMap();
  function wrapProto(proto) {
    Object.getOwnPropertyNames(proto).forEach(function (k) {
      if (k === "constructor") return;
      var d = Object.getOwnPropertyDescriptor(proto, k);
      if (typeof d.value === "function") {
        d.value = timed(d.value);
        Object.defineProperty(proto, k, d);
      } else if (typeof d.get === "function") {
        var g = d.get;
        d.get = function () {
          var v = timed(g).call(this);
          if (typeof v !== "function") return v;
          var w = bound.get(v);
          if (!w) { w = timed(v); bound.set(v, w); }
          return w;
        };
        Object.defineProperty(proto, k, d);
      }
    });
  }
  Object.getOwnPropertyNames(Intl).forEach(function (k) {
    var d = Object.getOwnPropertyDescriptor(Intl, k);
    var C = d.value;
    if (typeof C !== "function") return;
    if (C.prototype) wrapProto(C.prototype);
    Object.getOwnPropertyNames(C).forEach(function (s) {
      var sd = Object.getOwnPropertyDescriptor(C, s);
      if (typeof sd.value === "function" && sd.writable) { sd.value = timed(sd.value); Object.defineProperty(C, s, sd); }
    });
    d.value = C.prototype ? timedNew(C) : timed(C);
    Object.defineProperty(Intl, k, d);
  });
  [[String.prototype, "localeCompare"], [Number.prototype, "toLocaleString"], [Date.prototype, "toLocaleString"],
   [Date.prototype, "toLocaleDateString"], [Date.prototype, "toLocaleTimeString"]].forEach(function (x) {
    var d = Object.getOwnPropertyDescriptor(x[0], x[1]);
    d.value = timed(d.value);
    Object.defineProperty(x[0], x[1], d);
  });
  globalThis.__intlTime = function () { var r = JSON.stringify({ ms: total, calls: calls }); total = 0; calls = 0; return r; };
})();
