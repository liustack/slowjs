// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package tifa

import (
	"encoding/binary"
	"strconv"
	"unsafe"

	"github.com/liustack/slowjs"
)

// The hash of @emotion/hash (MurmurHash2 of the low bytes of the UTF-16 code units of a string,
// in base 36), which emotion takes of every style it serializes. MUI's styles are long, and the
// JavaScript loop over their code units takes about 30% of the render of an MUI page in QuickJS.
// The tifa plugin makes murmur2 ask __tifa_murmur2__ first, see js/vite/plugin.js of tifa.
// Moved here from internal/ssr/hash.go of tifa.

// murmur2Builtin hashes a string argument and returns undefined for anything else. It reads the
// characters where the engine keeps them, and only a rope, a string kept as the pieces it was
// joined from, as WTF-8.
var murmur2Builtin = slowjs.NewBuiltin("murmur2", 1, func(a slowjs.Args) (slowjs.Value, error) {
	if b, ok := a.Latin1(0); ok {
		return a.VM().NewString(murmur2Bytes(b))
	}
	if u, ok := a.UTF16(0); ok {
		return a.VM().NewString(murmur2Units(u))
	}
	s, ok := a.WTF8(0)
	if !ok {
		return slowjs.Value{}, nil
	}
	return a.VM().NewString(murmur2(s))
})

// murmur2Hook is __tifa_murmur2__: the hash of a string, or undefined when the argument is not a
// string or a page replaced String.prototype.charCodeAt or Number.prototype.toString, which
// murmur2 calls. murmur2 then hashes in JavaScript.
const murmur2Hook = `(hash) => {
  const S = String.prototype, N = Number.prototype, charCodeAt = S.charCodeAt, toString = N.toString
  Object.defineProperty(globalThis, '__tifa_murmur2__', {
    value: (s) => ('string' === typeof s && S.charCodeAt === charCodeAt && N.toString === toString ? hash(s) : void 0),
  })
}`

func registerHash(vm *slowjs.VM) error {
	define, err := vm.EvalValue(murmur2Hook, slowjs.EvalGlobal)
	if err != nil {
		return err
	}
	defer define.Free()
	b, err := vm.NewBuiltinFunction(murmur2Builtin)
	if err != nil {
		return err
	}
	defer b.Free()
	r, err := define.CallValue(slowjs.UndefinedValue, b)
	if err != nil {
		return err
	}
	r.Free()
	return nil
}

// murmur2 is murmur2 of @emotion/hash 0.9 of the string s, which is WTF-8: UTF-8 with lone
// surrogates as the three bytes of their code points, as the engine gives it. It hashes the low
// byte of each UTF-16 code unit of s.
func murmur2(s string) string {
	b := unsafe.Slice(unsafe.StringData(s), len(s))
	i := 0
	for i < len(b) && b[i] < 0x80 {
		i++
	}
	if i == len(b) {
		return murmur2Bytes(b) // ASCII: each byte is its code unit
	}
	low := make([]byte, i, len(b))
	copy(low, b[:i])
	for i < len(b) {
		c := b[i]
		switch {
		case c < 0x80:
			low = append(low, c)
			i++
		case c < 0xe0:
			low = append(low, c<<6|b[i+1]&0x3f)
			i += 2
		case c < 0xf0:
			low = append(low, b[i+1]<<6|b[i+2]&0x3f)
			i += 3
		default:
			r := (uint32(c&0x07)<<18 | uint32(b[i+1]&0x3f)<<12 | uint32(b[i+2]&0x3f)<<6 | uint32(b[i+3]&0x3f)) - 0x10000
			low = append(low, byte(0xd800+r>>10), byte(0xdc00+r&0x3ff))
			i += 4
		}
	}
	return murmur2Bytes(low)
}

// murmur2Units is murmur2 of the string of the UTF-16 code units u.
func murmur2Units(u []uint16) string {
	low := make([]byte, len(u))
	for i, c := range u {
		low[i] = byte(c)
	}
	return murmur2Bytes(low)
}

// murmur2Bytes is murmur2 of the string whose code units have the low bytes b. The JavaScript
// multiplies in two halves that stay exact doubles, and its bit operators then cut each product to
// 32 bits, so each step is a product of uint32.
func murmur2Bytes(b []byte) string {
	const m = 0x5bd1e995
	var h uint32
	for ; len(b) >= 4; b = b[4:] {
		k := binary.LittleEndian.Uint32(b) * m
		k ^= k >> 24
		h = k*m ^ h*m
	}
	switch len(b) {
	case 3:
		h ^= uint32(b[2]) << 16
		fallthrough
	case 2:
		h ^= uint32(b[1]) << 8
		fallthrough
	case 1:
		h ^= uint32(b[0])
		h *= m
	}
	h ^= h >> 13
	h *= m
	return strconv.FormatUint(uint64(h^h>>15), 36)
}
