// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// The return() of an iterator helper (map, filter, take, drop, flatMap) closes
// the iterator under it by calling its return method once, and closes the
// inner iterator of a flatMap first. The next method of an inner iterator is
// read once, when it is made (upstream QuickJS abf571a). The file is what
// Node 24 prints.
const out = []
function run(name, f) {
  let r
  try { r = f() } catch (e) { r = 'throws ' + e.name }
  out.push(name + ': ' + String(r))
}
function source(log, n = 3, opts = {}) {
  let i = 0
  const it = {
    get next() {
      log.push('get next')
      return function () {
        log.push('next')
        if (opts.nextThrows && i === opts.nextThrows) throw new RangeError('next')
        return i < n ? { done: false, value: i++ } : { done: true, value: undefined }
      }
    },
    get return() {
      log.push('get return')
      if (opts.noReturn) return undefined
      return function () {
        log.push('return')
        if (opts.returnThrows) throw new RangeError('return')
        return opts.returnPrimitive ? 1 : {}
      }
    },
    __proto__: Iterator.prototype,
  }
  return it
}
const makers = {
  map: it => it.map(x => x * 2),
  filter: it => it.filter(x => x % 2 === 0),
  take: it => it.take(2),
  drop: it => it.drop(1),
  flatMap: it => it.flatMap(x => [x, x]),
}
for (const [name, mk] of Object.entries(makers)) {
  run(name + ' return before start', () => { const log = []; const h = mk(source(log)); const r = h.return(); return JSON.stringify(r) + ' ' + log.join() })
  run(name + ' return twice', () => { const log = []; const h = mk(source(log)); h.return(); const r = h.return(); return JSON.stringify(r) + ' ' + log.join() })
  run(name + ' return after one', () => { const log = []; const h = mk(source(log)); const a = h.next(); const r = h.return(); return JSON.stringify(a) + JSON.stringify(r) + ' ' + log.join() })
  run(name + ' return when done', () => { const log = []; const h = mk(source(log, 1)); h.toArray(); const r = h.return(); return JSON.stringify(r) + ' ' + log.join() })
  run(name + ' next after return', () => { const log = []; const h = mk(source(log)); h.return(); const r = h.next(); return JSON.stringify(r) + ' ' + log.join() })
  run(name + ' no return method', () => { const log = []; const h = mk(source(log, 3, { noReturn: true })); const r = h.return(); return JSON.stringify(r) + ' ' + log.join() })
  run(name + ' return throws', () => { const log = []; const h = mk(source(log, 3, { returnThrows: true })); h.next(); h.return(); return log.join() })
  run(name + ' return result is a primitive', () => { const log = []; const h = mk(source(log, 3, { returnPrimitive: true })); h.next(); h.return(); return log.join() })
  run(name + ' next throws', () => { const log = []; const h = mk(source(log, 5, { nextThrows: 2 })); h.next(); h.next(); h.next(); return log.join() })
  run(name + ' return after next threw', () => { const log = []; const h = mk(source(log, 5, { nextThrows: 1 })); try { h.next(); h.next() } catch (e) { log.push('caught') } const r = h.return(); return JSON.stringify(r) + ' ' + log.join() })
  run(name + ' return while running', () => {
    const log = []
    const src = source(log)
    let h
    const f = x => { try { h.return() } catch (e) { log.push(e.name) } return x }
    h = name === 'flatMap' ? src.flatMap(x => { try { h.return() } catch (e) { log.push(e.name) } return [x] }) : name === 'map' ? src.map(f) : name === 'filter' ? src.filter(x => { f(x); return true }) : mk(src)
    try { h.next() } catch (e) { log.push('outer ' + e.name) }
    return log.join()
  })
}
function inner(log, tag, n = 2, opts = {}) {
  let i = 0
  return {
    get next() {
      log.push('get next ' + tag)
      return function () { log.push('next ' + tag); return i < n ? { done: false, value: tag + i++ } : { done: true, value: undefined } }
    },
    get return() {
      log.push('get return ' + tag)
      return function () { log.push('return ' + tag); if (opts.throws) throw new RangeError('inner'); return {} }
    },
    [Symbol.iterator]() { log.push('iterator ' + tag); return this },
  }
}
run('flatMap reads next of an inner iterator once', () => {
  const log = []
  const h = source(log, 2).flatMap(x => inner(log, 'i' + x, 3))
  const a = h.toArray()
  return a.join() + ' ' + log.join()
})
run('flatMap return while inside an inner iterator', () => {
  const log = []
  const h = source(log, 3).flatMap(x => inner(log, 'i' + x, 3))
  h.next(); h.next()
  const r = h.return()
  return JSON.stringify(r) + ' ' + log.join()
})
run('flatMap return, inner return throws', () => {
  const log = []
  const h = source(log, 3).flatMap(x => inner(log, 'i' + x, 3, { throws: true }))
  h.next()
  try { h.return() } catch (e) { log.push('caught ' + e.name) }
  return log.join()
})
run('flatMap return, both throw', () => {
  const log = []
  const h = source(log, 3, { returnThrows: true }).flatMap(x => inner(log, 'i' + x, 3, { throws: true }))
  h.next()
  try { h.return() } catch (e) { log.push('caught ' + e.name + ' ' + e.message) }
  return log.join()
})
run('flatMap inner done moves on', () => {
  const log = []
  const h = source(log, 2).flatMap(x => inner(log, 'i' + x, 1))
  const a = [h.next().value, h.next().value, h.next().done]
  return a.join() + ' ' + log.join()
})
run('flatMap mapper throws', () => {
  const log = []
  const h = source(log, 3).flatMap(x => { throw new RangeError('m') })
  try { h.next() } catch (e) { log.push('caught ' + e.name) }
  return log.join()
})
run('flatMap mapper returns a non-iterable', () => {
  const log = []
  const h = source(log, 3).flatMap(x => 5)
  try { h.next() } catch (e) { log.push('caught ' + e.name) }
  return log.join()
})
run('flatMap inner is a string', () => { const log = []; const a = source(log, 2).flatMap(x => 'ab').toArray(); return a.join() })
run('flatMap inner next getter throws', () => {
  const log = []
  const h = source(log, 3).flatMap(x => ({ get next() { throw new RangeError('g') }, [Symbol.iterator]() { return this } }))
  try { h.next() } catch (e) { log.push('caught ' + e.name) }
  return log.join()
})
run('take return after the limit', () => {
  const log = []
  const h = source(log, 5).take(1)
  const a = [h.next().value, h.next().done]
  return a.join() + ' ' + log.join()
})
run('drop return', () => {
  const log = []
  const h = source(log, 5).drop(2)
  h.next()
  h.return()
  return log.join()
})
out.join('\n')
