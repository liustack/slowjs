// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
//
// Inputs and outputs of the Math functions, for comparing two engines on what
// JavaScript sees. It only uses integer arithmetic, the exact operations of
// IEEE 754 and the functions under test, so every engine runs it the same way.
// runFn(name) calls Math[name] (or the operator or conversion the name stands
// for) on 100000 inputs and returns two lines: "in NAME 100000 DATA" with the
// bits of the arguments and "out NAME 100000 DATA" with the bits of the
// results, 16 hex digits each. A NaN result is written as the one NaN of
// JavaScript, so its sign and payload are not compared (a typed array can show
// them, but they are not something the specification fixes).
//
// This tests the 33 operations below as the engine reaches them. Math.round
// is the engine's own code and not libc's round, and clamp and pause test
// lrint and modf only for the conversions and the whole number test they are
// used for. The functions of libc are called directly by libmath (see math.sh).
"use strict";

const N = 100000;
const buf = new ArrayBuffer(8);
const U = new Uint32Array(buf);
const F = new Float64Array(buf);
F[0] = 1;
if (U[1] !== 0x3ff00000) throw new Error("little-endian machine expected");

const H = [];
for (let i = 0; i < 256; i++) H.push((i + 256).toString(16).slice(1));
function hex8(u) { return H[u >>> 24] + H[(u >>> 16) & 255] + H[(u >>> 8) & 255] + H[u & 255]; }
function hex(x) { F[0] = x; return hex8(U[1]) + hex8(U[0]); }
function bits(hi, lo) { U[1] = hi; U[0] = lo; return F[0]; }
function ulps(x, n) {
  F[0] = x;
  let lo = U[0] + n, hi = U[1];
  if (lo > 4294967295) { lo -= 4294967296; hi = (hi + 1) >>> 0; }
  else if (lo < 0) { lo += 4294967296; hi = (hi - 1) >>> 0; }
  U[0] = lo; U[1] = hi;
  return F[0];
}

// xoshiro128**, seeded from a name with splitmix32.
function rotl(x, k) { return (x << k) | (x >>> (32 - k)); }
function makeRng(name, salt) {
  let h = 0x811c9dc5 ^ salt;
  for (let i = 0; i < name.length; i++) h = Math.imul(h ^ name.charCodeAt(i), 0x01000193);
  let s = h | 0;
  function splitmix() {
    s = (s + 0x9e3779b9) | 0;
    let z = s;
    z = Math.imul(z ^ (z >>> 16), 0x85ebca6b);
    z = Math.imul(z ^ (z >>> 13), 0xc2b2ae35);
    return (z ^ (z >>> 16)) | 0;
  }
  let a = splitmix(), b = splitmix(), c = splitmix(), d = splitmix();
  return function () {
    const r = Math.imul(rotl(Math.imul(b, 5), 7), 9);
    const t = b << 9;
    c ^= a; d ^= b; b ^= c; a ^= d; c ^= t; d = rotl(d, 11);
    return r >>> 0;
  };
}

const MIN = Number.MIN_VALUE, MAX = Number.MAX_VALUE;
const EDGE = [
  0, -0, Infinity, -Infinity, NaN, 1, -1, 0.5, -0.5, 2, -2, 3, -3, 10, 100, 0.1, 0.3,
  MIN, -MIN, bits(0x000fffff, 0xffffffff), bits(0x00100000, 0), -bits(0x00100000, 0), MAX, -MAX,
  1e-300, -1e-300, 1e300, -1e300, 1e-10, 1e22, 2 ** 53, 2 ** 53 + 2, 2 ** 63, 2 ** 64, 2 ** -1022, 2 ** 1023,
  1 + 2 ** -52, 1 - 2 ** -53, 0.9999999999999999, 1.0000000000000002,
  Math.PI, -Math.PI, Math.PI / 2, -Math.PI / 2, Math.PI / 4, 3 * Math.PI / 2, 2 * Math.PI, 1e6 * Math.PI,
  20, 21, 36, 37, 38, 709, 709.782712893384, 709.7827128933841, 710, -708, -745.1332191019411, -745.2, -746,
  22, 710.4758600739439, 1023, 1024, -1074, -1075, 255, 256, 255.5, 254.5, 0.4999999999999999, -0.4999999999999999,
];

function sample(i, r) {
  switch (i % 6) {
    case 0: return bits(r(), r());
    case 1: return bits((r() & 0x80000000) | ((1023 + (r() % 81) - 40) << 20) | (r() & 0xfffff), r());
    case 2: return (r() / 4294967296) * 40 - 20;
    case 3: return (r() / 4294967296) * 2 - 1;
    case 4: return ulps(((r() % 2001) - 1000) * (Math.PI / 2), (r() % 9) - 4);
    default: {
      const m = r() % 4;
      if (m === 0) return (r() % 201) - 100;
      if (m === 1) return ((r() % 201) - 100) / 2;
      if (m === 2) return ulps(2 ** ((r() % 140) - 70) * ((r() & 1) ? 1 : -1), (r() % 5) - 2);
      return ((r() % 600) - 100) + ((r() % 8) / 8);
    }
  }
}

const cl = new Uint8ClampedArray(1);
const unary = {
  sin: Math.sin, cos: Math.cos, tan: Math.tan, asin: Math.asin, acos: Math.acos, atan: Math.atan,
  sinh: Math.sinh, cosh: Math.cosh, tanh: Math.tanh, asinh: Math.asinh, acosh: Math.acosh, atanh: Math.atanh,
  exp: Math.exp, expm1: Math.expm1, log: Math.log, log10: Math.log10, log1p: Math.log1p, log2: Math.log2,
  cbrt: Math.cbrt, floor: Math.floor, ceil: Math.ceil, trunc: Math.trunc, round: Math.round,
  sqrt: Math.sqrt, abs: Math.abs,
  // lrint, when a number is stored in a Uint8ClampedArray
  clamp: (x) => { cl[0] = x; return cl[0]; },
  // modf, when Atomics.pause tests that a number is whole
  pause: (x) => { try { Atomics.pause(x); return 0; } catch (e) { return 1; } },
};
const binary = {
  atan2: Math.atan2, pow: Math.pow, hypot: Math.hypot, min: Math.min, max: Math.max,
  // fmod
  mod: (x, y) => x % y,
};

function runFn(name) {
  const f1 = unary[name], f2 = binary[name];
  if (!f1 && !f2) throw new Error("no function " + name);
  const rx = makeRng(name, 1), ry = makeRng(name, 2);
  const E = EDGE.length;
  const ins = [], outs = [];
  for (let i = 0; i < N; i++) {
    let x, y, r;
    if (f1) {
      // the edge values first, then the generator
      x = i < E ? EDGE[i] : sample(i, rx);
      ins.push(hex(x));
      r = f1(x);
    } else {
      // every pair of edge values, then the generator
      if (i < E * E) {
        x = EDGE[(i / E) | 0];
        y = EDGE[i % E];
      } else {
        x = sample(i, rx);
        y = sample(i + 3, ry);
      }
      ins.push(hex(x) + hex(y));
      r = f2(x, y);
    }
    if (r !== r) r = NaN;
    outs.push(hex(r));
  }
  return "in " + name + " " + N + " " + ins.join("") + "\nout " + name + " " + N + " " + outs.join("");
}
