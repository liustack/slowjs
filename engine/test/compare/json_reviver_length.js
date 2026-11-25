// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.
// JSON.parse with a reviver reads the length of an array with ToLength, as
// LengthOfArrayLike does: a length of -1 is 0, so the loop does not run 2**32 - 1
// times. The file is what Node 24 prints.
const out = []
let calls = 0
const r = JSON.parse('[[1], [2]]', function (k, v) {
  calls++
  if (this.length === 2 && k === '0') {
    this[1] = new Proxy([], {
      get(t, p) { return p === 'length' ? -1 : undefined },
    })
  }
  return v
})
out.push('reviver calls: ' + calls)
out.push('result length: ' + r.length)
out.join('\n')
