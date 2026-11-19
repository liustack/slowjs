// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

// Only for Hermes: what the bundles read at load and Hermes V1 does not have yet. Resizable ArrayBuffer (ES2024)
// is absent, so its two getters and the one of `resizable` are given as a fixed-length buffer has them.
(function () {
  var AB = ArrayBuffer.prototype;
  if (!Object.getOwnPropertyDescriptor(AB, "resizable")) Object.defineProperty(AB, "resizable", { get: function () { return false; }, configurable: true });
  if (!Object.getOwnPropertyDescriptor(AB, "maxByteLength")) Object.defineProperty(AB, "maxByteLength", { get: function () { return this.byteLength; }, configurable: true });
})();
// RegExp flag getters that are missing (hasIndices, unicodeSets), answered from the flags text.
(function () {
  var R = RegExp.prototype, f = { hasIndices: "d", global: "g", ignoreCase: "i", multiline: "m", dotAll: "s", unicode: "u", unicodeSets: "v", sticky: "y" };
  Object.keys(f).forEach(function (k) {
    if (!Object.getOwnPropertyDescriptor(R, k)) Object.defineProperty(R, k, { get: function () { return String(this.flags).indexOf(f[k]) >= 0; }, configurable: true });
  });
})();
// SharedArrayBuffer is absent: a stand-in with the getter the bundle captures. No page makes one.
if (typeof SharedArrayBuffer === "undefined") {
  globalThis.SharedArrayBuffer = function SharedArrayBuffer() { throw new TypeError("SharedArrayBuffer is not supported"); };
  Object.defineProperty(SharedArrayBuffer.prototype, "byteLength", { get: function () { return 0; }, configurable: true });
}
// Float16Array (ES2026) is absent in the release build: a stand-in the bundle can list among the typed arrays. No page makes one.
if (typeof Float16Array === "undefined") {
  globalThis.Float16Array = function Float16Array() { throw new TypeError("Float16Array is not supported"); };
}
// Iterator.concat (ES2026) is absent in the release build: the bundle calls it once at load, to collect a prototype. Same code as node-run.cjs.
if (typeof Iterator !== "undefined" && !Iterator.concat) {
  Iterator.concat = function () { var its = arguments; return Iterator.from((function* () { for (var i = 0; i < its.length; i++) yield* its[i]; })()); };
}
