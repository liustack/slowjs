// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// Records which Intl members and locale methods a render reaches, with the
// first caller. Everything still goes to the real Intl.
(function () {
  var hits = {};
  function where() {
    // Skip where, hit and the trap or wrapper, keep the next frames.
    var s = String(new Error().stack || "").split("\n").map(function (l) { return l.trim(); }).filter(function (l) { return l.indexOf("at ") === 0; });
    return s.slice(3, 7).join(" | ").slice(0, 400);
  }
  function hit(name) {
    var h = hits[name];
    if (!h) h = hits[name] = { n: 0, at: where() };
    h.n++;
  }
  globalThis.__intlHits = function () { var h = hits; hits = {}; return JSON.stringify(h); };
  if (typeof Intl === "object" && Intl) {
    var real = Intl;
    var p = new Proxy(real, { get: function (t, k, r) { if (typeof k === "string") hit("Intl." + k); return Reflect.get(t, k, r); } });
    Object.defineProperty(globalThis, "Intl", { value: p, writable: true, configurable: true, enumerable: false });
  }
  function wrap(obj, name, label) {
    var f = obj[name];
    if (typeof f !== "function") return;
    Object.defineProperty(obj, name, {
      value: function () { hit(label); return f.apply(this, arguments); },
      writable: true, configurable: true, enumerable: false,
    });
  }
  wrap(String.prototype, "localeCompare", "String.prototype.localeCompare");
  wrap(Number.prototype, "toLocaleString", "Number.prototype.toLocaleString");
  wrap(Date.prototype, "toLocaleString", "Date.prototype.toLocaleString");
  wrap(Date.prototype, "toLocaleDateString", "Date.prototype.toLocaleDateString");
  wrap(Date.prototype, "toLocaleTimeString", "Date.prototype.toLocaleTimeString");
  wrap(Array.prototype, "toLocaleString", "Array.prototype.toLocaleString");
  if (typeof BigInt === "function") wrap(BigInt.prototype, "toLocaleString", "BigInt.prototype.toLocaleString");
})();
