// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.
// Array.prototype.sort calls the comparator for two identical values too, as
// the specification says (upstream QuickJS 1cc7446). The file is what Node 24
// prints.
const out = []
const o = {}
let calls = 0
const same = [o, o, o].sort((a, b) => { calls++; return 0 })
out.push('objects: ' + calls + ' ' + (same[0] === o))
calls = 0
;[1, 1].sort(() => { calls++; return 0 })
out.push('numbers: ' + calls)
calls = 0
;['a', 'a', 'a'].sort(() => { calls++; return 0 })
out.push('strings: ' + calls)
const seen = []
;[5, 5].sort((a, b) => { seen.push(a === b); return a - b })
out.push('arguments: ' + seen.join())
out.push('stable: ' + [3, 1, 3, 2, 1].sort(() => 0).join())
out.push('sorted: ' + [3, 1, 3, 2, 1].sort((a, b) => a - b).join())
out.join('\n')
