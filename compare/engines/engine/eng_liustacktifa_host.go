// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build eng_liustacktifa

package engine

// The Go functions tifa gives each VM of its pool, copied from
// internal/ssr/hash.go and internal/ssr/random.go of github.com/liustack/tifa
// (MIT) so that this module does not import tifa. When tifa changes them,
// copy the change.

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"math"
	"regexp"
	"strconv"

	"github.com/liustack/slowjs"
)

// hookCalls counts how often a bundle called each Go function, so that a
// result shows whether the bundle uses them at all.
var hookCalls struct{ murmur2, randomBytes int }

// murmur2Builtin hashes a string argument and returns undefined for anything else.
var murmur2Builtin = slowjs.NewBuiltin("murmur2", 1, func(a slowjs.Args) (slowjs.Value, error) {
	s, ok := a.WTF8(0)
	if !ok {
		return slowjs.Value{}, nil
	}
	hookCalls.murmur2++
	return a.VM().NewString(murmur2(s))
})

// murmur2Hook is __tifa_murmur2__: the hash of a string, or undefined when
// the argument is not a string or a page replaced String.prototype.charCodeAt
// or Number.prototype.toString, which murmur2 calls.
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

// murmur2 is murmur2 of @emotion/hash 0.9 of the string s, which is WTF-8:
// UTF-8 with lone surrogates as the three bytes of their code points, as the
// engine gives it. The JavaScript computes with doubles that stay exact
// integers and cuts them to 32 bits where its bit operators do, so this
// follows each step with int64 and the same cuts.
func murmur2(s string) string {
	var h int64
	var k [4]uint32
	n := 0
	mix := func() {
		w := int64(int32(k[0] | k[1]<<8 | k[2]<<16 | k[3]<<24))
		w = imul(w)
		w = int64(int32(uint32(w)) ^ int32(uint32(w)>>24))
		h = int64(int32(uint32(imul(w))) ^ int32(uint32(imul(h))))
	}
	unit := func(u uint32) {
		k[n] = u & 0xff
		n++
		if n == 4 {
			mix()
			n = 0
		}
	}
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c < 0x80:
			unit(uint32(c))
			i++
		case c < 0xe0:
			unit(uint32(c&0x1f)<<6 | uint32(s[i+1]&0x3f))
			i += 2
		case c < 0xf0:
			unit(uint32(c&0x0f)<<12 | uint32(s[i+1]&0x3f)<<6 | uint32(s[i+2]&0x3f))
			i += 3
		default:
			r := uint32(c&0x07)<<18 | uint32(s[i+1]&0x3f)<<12 | uint32(s[i+2]&0x3f)<<6 | uint32(s[i+3]&0x3f)
			r -= 0x10000
			unit(0xd800 + r>>10)
			unit(0xdc00 + r&0x3ff)
			i += 4
		}
	}
	switch n {
	case 3:
		h = int64(int32(uint32(h)) ^ int32(k[2]<<16))
		fallthrough
	case 2:
		h = int64(int32(uint32(h)) ^ int32(k[1]<<8))
		fallthrough
	case 1:
		h = int64(int32(uint32(h)) ^ int32(k[0]))
		h = imul(h)
	}
	h = int64(int32(uint32(h)) ^ int32(uint32(h)>>13))
	h = imul(h)
	return strconv.FormatUint(uint64(uint32(int32(uint32(h))^int32(uint32(h)>>15))), 36)
}

// imul is (x & 0xffff) * 0x5bd1e995 + ((x >>> 16) * 0xe995 << 16) for x an
// exact integer: the product of the low half stays an exact double, the shift
// cuts the other to 32 bits.
func imul(x int64) int64 {
	u := uint32(x)
	return int64(u&0xffff)*0x5bd1e995 + int64(int32(uint32(uint64(u>>16)*0xe995)<<16))
}

// registerRandom reads the OS entropy source on each call, as tifa does.
func registerRandom(vm *slowjs.VM) error {
	return vm.RegisterHostFunc("__tifa_random_bytes__", func(args []any) (any, error) {
		if len(args) != 1 {
			return nil, errors.New("random bytes requires a length")
		}
		n, ok := args[0].(float64)
		if i, integral := args[0].(int); integral {
			n, ok = float64(i), true
		}
		if !ok || n < 0 || n > 65536 || n != math.Trunc(n) {
			return nil, errors.New("random byte length must be an integer from 0 to 65536")
		}
		hookCalls.randomBytes++
		buf := make([]byte, int(n))
		if _, err := rand.Read(buf); err != nil {
			return nil, err
		}
		return base64.StdEncoding.EncodeToString(buf), nil
	})
}

// emotionHashFunc matches murmur2 of @emotion/hash in a minified bundle: the
// function header and the start of its loop over four characters.
var emotionHashFunc = regexp.MustCompile(`function ([\w$]+)\(([\w$]+)\)\{(for\(var [\w$]+=0,[\w$]+,[\w$]+=0,[\w$]+=[\w$]+\.length;[\w$]+>=4;\+\+[\w$]+,[\w$]+-=4\)[\w$]+=[\w$]+\.charCodeAt\([\w$]+\)&255)`)

// hashInGo is what the tifa plugin does to murmur2 (hashInGo in js/vite/plugin.js):
// the function asks __tifa_murmur2__ first and goes on to hash in JavaScript
// when that gives undefined. The bundles pinned in tifa/bundles were built
// before the plugin did so, so the adapter makes the same one-line change in
// the text of a bundle that has @emotion/hash. It reports how many functions
// it changed, which is one for MUI and none for the other bundles.
func hashInGo(src string) (string, int) {
	n := 0
	out := emotionHashFunc.ReplaceAllStringFunc(src, func(m string) string {
		n++
		sub := emotionHashFunc.FindStringSubmatch(m)
		return "function " + sub[1] + "(" + sub[2] + "){var tifaHash=\"function\"===typeof __tifa_murmur2__?__tifa_murmur2__(" + sub[2] + "):void 0;if(void 0!==tifaHash)return tifaHash;" + sub[3]
	})
	return out, n
}
