// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.
// Array.from with a constructor of the program: the index the mapping
// function gets and the length set at the end, for an array-like and for an
// iterable, on a constructor whose object keeps nothing (a proxy that
// accepts every define) and on one that keeps the elements. The file is what
// Node 24 prints.
const out = []
function run(name, f) {
  let r
  try { r = f() } catch (e) { r = 'throws ' + e.name }
  out.push(name + ': ' + JSON.stringify(r))
}
function Sink() {
  const log = []
  return new Proxy({ log }, {
    defineProperty(t, k, d) { if (k !== 'log') log.push(String(k)); return true },
    set(t, k, v) { log.push(k + '=' + v); t[k] = v; return true },
  })
}
function Keep() {}
const idx = (v, i) => typeof i + ':' + i

for (const n of [0, 1, 3]) {
  run('sink array-like ' + n, () => { const r = Array.from.call(Sink, { length: n }, idx); return [r.length, r.log] })
  run('sink iterable ' + n, () => {
    const r = Array.from.call(Sink, Array.from({ length: n }, (_, i) => i), idx)
    return [r.length, r.log]
  })
  run('keep array-like ' + n, () => {
    const r = Array.from.call(Keep, { length: n, 0: 'a', 1: 'b', 2: 'c' }, idx)
    return [r.length, Array.prototype.slice.call(r)]
  })
  run('keep iterable ' + n, () => {
    const r = Array.from.call(Keep, 'abc'.slice(0, n), (v, i) => v + i)
    return [r.length, Array.prototype.slice.call(r)]
  })
}
run('length set through a setter of the object', () => {
  const r = Array.from.call(function () { return { set length(v) { this.l = v } } }, { length: 0 })
  return r.l
})
out.join('\n')
