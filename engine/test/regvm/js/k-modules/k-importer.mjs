// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// The module of k-import-files.mjs that imports: its import() names a file
// next to it, which the folder of the importing module would not have.
export const meta = import.meta;
export function inner(spec) {
  return [import(spec), import.meta];
}
export function outer(spec) {
  var r = inner(spec);
  return [r[0], r[1] === meta];
}
