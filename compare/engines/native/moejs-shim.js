// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Only for moejs: what the tifa bundles read at load and moejs v0.1.1 lacks.
// FinalizationRegistry: a registry whose callback is never called is a valid one (no callback is
// promised), so register and unregister do nothing.
if (typeof FinalizationRegistry === "undefined") {
  globalThis.FinalizationRegistry = class FinalizationRegistry {
    constructor(callback) {
      if (typeof callback !== "function") throw new TypeError("FinalizationRegistry: callback must be a function");
    }
    register(target) {
      if (target === null || (typeof target !== "object" && typeof target !== "function")) throw new TypeError("FinalizationRegistry.register: invalid target");
    }
    unregister() { return false; }
  };
}
