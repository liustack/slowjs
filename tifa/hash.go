// Copyright (c) 2026 The liustack/tifajs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package tifa

import "github.com/liustack/tifajs"

// The hash of @emotion/hash (MurmurHash2 of the low bytes of the UTF-16 code units of a string,
// in base 36), which emotion takes of every style it serializes. MUI's styles are long, and the
// JavaScript loop over their code units takes about 30% of the render of an MUI page in QuickJS.
// The tifa plugin makes murmur2 ask __tifa_murmur2__ first, see js/vite/plugin.js of tifa. The
// hash is the engine's, written in C ([tifajs.VM.NewMurmur2Function]), which reads the characters
// where the engine keeps them.

// murmur2Hook is __tifa_murmur2__: the hash of a string, or undefined when the argument is not a
// string or a page replaced String.prototype.charCodeAt or Number.prototype.toString, which
// murmur2 calls. murmur2 then hashes in JavaScript.
const murmur2Hook = `(hash) => {
  const S = String.prototype, N = Number.prototype, charCodeAt = S.charCodeAt, toString = N.toString
  Object.defineProperty(globalThis, '__tifa_murmur2__', {
    value: (s) => ('string' === typeof s && S.charCodeAt === charCodeAt && N.toString === toString ? hash(s) : void 0),
  })
}`

func registerHash(vm *tifajs.VM) error {
	define, err := vm.EvalValue(murmur2Hook, tifajs.EvalGlobal)
	if err != nil {
		return err
	}
	defer define.Free()
	h, err := vm.NewMurmur2Function()
	if err != nil {
		return err
	}
	defer h.Free()
	r, err := define.CallValue(tifajs.UndefinedValue, h)
	if err != nil {
		return err
	}
	r.Free()
	return nil
}
