// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// Strings converted to numbers, and the keys of a typed array that are
// canonical numeric strings (the string a number converts to): such a key
// is an element, valid or not, and never an ordinary property, while any
// other key is one. The strings are also long ropes made by concatenation.
const keys = ['0', '1', '-0', '0.5', '1.0', '01', '+1', ' 1', '1 ', '-1', '1e3', '1000',
  '1e21', '1e+21', '0.000001', '1e-7', '0.0000001', 'Infinity', '-Infinity', 'NaN',
  '0x10', '0b1', '0o7', '4294967295', '4294967296', '9007199254740993', '1.5e300', '', 'a', '-', '-a']
const out = []
for (const k of keys) {
  const ta = new Uint8Array(4)
  const set = Reflect.set(ta, k, 7)
  const has = k in ta
  const own = Object.getOwnPropertyNames(ta).includes(k)
  const def = (() => { try { Object.defineProperty(ta, k, { value: 1, configurable: true, writable: true, enumerable: true }); return 'ok' } catch (e) { return e.name } })()
  out.push(JSON.stringify(k) + ': ' + Number(k) + ' ' + set + ' ' + has + ' ' + own + ' ' + def + ' ' + String(ta[k]))
}
const pad = ' '.repeat(3000), digits = '1'.repeat(400)
const ropes = [pad + '42' + pad, pad + '0x1F' + pad, '0b' + '1'.repeat(40) + pad, digits + '.5' + pad,
  pad + '-Infinity', pad + '4' + pad + '2', '\n\t' + pad + '7e-3 ﻿']
for (const r of ropes) out.push(r.length + ': ' + Number(r) + ' ' + (+r) + ' ' + Math.abs(r))
out.join('\n')
