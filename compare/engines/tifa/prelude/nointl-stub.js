// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// The Intl-free check: no Intl, and the locale methods that reach it throw.
(function () {
  delete globalThis.Intl;
  function stub(obj, name, label) {
    Object.defineProperty(obj, name, {
      value: function () { throw new Error("nointl: " + label + " called"); },
      writable: true, configurable: true, enumerable: false,
    });
  }
  stub(String.prototype, "localeCompare", "String.prototype.localeCompare");
  stub(Number.prototype, "toLocaleString", "Number.prototype.toLocaleString");
  stub(Date.prototype, "toLocaleString", "Date.prototype.toLocaleString");
  stub(Date.prototype, "toLocaleDateString", "Date.prototype.toLocaleDateString");
  stub(Date.prototype, "toLocaleTimeString", "Date.prototype.toLocaleTimeString");
  if (typeof BigInt === "function") stub(BigInt.prototype, "toLocaleString", "BigInt.prototype.toLocaleString");
})();
