// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

// A bundle for engine/test/render-test.sh: each render leaves pairs of objects
// that hold each other, which only a collection frees.
globalThis.startRender = function () {
  for (let i = 0; i < 400; i++) {
    const a = { n: i }, b = { a }
    a.b = b
  }
  globalThis.renderResult = 'cycles\n'
}
