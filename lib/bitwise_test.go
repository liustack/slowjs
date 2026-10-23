// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package libquickjs // import "github.com/liustack/slowjs/lib"

import (
	"math"
	"math/rand/v2"
	"testing"
	"unsafe"

	"modernc.org/libc"
)

// testdata/bitwise.js runs the six bitwise operators on numbers of every
// range and on the other kinds of values. testdata/bitwise.txt is the
// transcript of the engine before fixBitwise (Node gives the same), so it
// must stay as it is.
func TestBitwiseSemantics(t *testing.T) {
	matchTranscript(t, "bitwise")
}

// bitwiseNumbers gives what the specification gives for two numbers, on
// random floats of every exponent and on the edges of each range, and
// leaves every other pair alone. The reference does not go through the slow
// paths, which call bitwiseNumbers first: it converts each operand with
// specToInt32 and checks that the engine's JS_ToInt32Free agrees, then does
// the operation in Go.
func TestBitwiseNumbers(t *testing.T) {
	tls := libc.NewTLS()
	defer tls.Close()
	rt := XJS_NewRuntime(tls)
	defer XJS_FreeRuntime(tls, rt)
	ctx := XJS_NewContext(tls, rt)
	defer XJS_FreeContext(tls, ctx)

	num := func(d float64) TJSValue {
		var v TJSValue
		if d == math.Trunc(d) && d >= math.MinInt32 && d <= math.MaxInt32 && !(d == 0 && math.Signbit(d)) && rand.IntN(2) == 0 {
			*(*uint64)(unsafe.Pointer(&v.Fu)) = uint64(uint32(int32(d)))
			v.Ftag = EJS_TAG_INT
			return v
		}
		*(*float64)(unsafe.Pointer(&v.Fu)) = d
		v.Ftag = EJS_TAG_FLOAT64
		return v
	}
	edges := []float64{0, math.Copysign(0, -1), 0.5, -0.5, 1, -1, 31, 32, 33, math.MaxInt32, math.MinInt32, 1 << 31, -(1 << 31) - 1,
		1 << 32, 1<<32 + 5, 1<<32 - 0.5, 1 << 52, 1<<53 + 2, 1 << 63, 1<<84 + 1<<33, 1 << 85, 1 << 86, 1e300, math.MaxFloat64,
		math.SmallestNonzeroFloat64, math.NaN(), math.Inf(1), math.Inf(-1)}
	var cases [][2]float64
	for _, x := range edges {
		for _, y := range edges {
			cases = append(cases, [2]float64{x, y}, [2]float64{-x, y})
		}
	}
	r := rand.New(rand.NewPCG(1, 2))
	for range 20000 {
		cases = append(cases, [2]float64{math.Float64frombits(r.Uint64()), math.Float64frombits(r.Uint64())},
			[2]float64{math.Ldexp(r.Float64(), r.IntN(100)), float64(r.IntN(70)) - 3})
	}
	stack := tls.Alloc(2 * int(unsafe.Sizeof(TJSValue{})))
	defer tls.Free(2 * int(unsafe.Sizeof(TJSValue{})))
	sp := stack + 2*unsafe.Sizeof(TJSValue{})
	slot := func(i uintptr) *TJSValue { return (*TJSValue)(unsafe.Pointer(stack + i*unsafe.Sizeof(TJSValue{}))) }
	for _, c := range cases {
		for _, op := range []int32{EOP_shl, EOP_sar, EOP_and, EOP_or, EOP_xor, EOP_shr} {
			a, b := num(c[0]), num(c[1])
			*slot(0), *slot(1) = a, b
			if !bitwiseNumbers(sp, op) {
				t.Fatalf("%v %d %v: bitwiseNumbers refuses two numbers", c[0], op, c[1])
			}
			got := *slot(0)
			if want := bitwiseWant(t, tls, ctx, a, b, op); got != want {
				t.Fatalf("%v %d %v: %+v, want %+v", c[0], op, c[1], got, want)
			}
		}
	}

	for _, js := range []string{"'1'", "true", "null", "undefined", "1n", "({})", "Symbol()"} {
		v := eval0(t, tls, ctx, js)
		for _, pair := range [][2]TJSValue{{v, num(1.5)}, {num(2.5), v}} {
			*slot(0), *slot(1) = pair[0], pair[1]
			if bitwiseNumbers(sp, EOP_or) || *slot(0) != pair[0] || *slot(1) != pair[1] {
				t.Errorf("%s: bitwiseNumbers took a value that is not a number", js)
			}
		}
		XFreeValue(tls, ctx, v)
	}
}

// bitwiseWant is the result of the bitwise operator op of the numbers a and
// b by the specification, as the engine stores it: an int, or a float for a
// result of >>> past 2^31 - 1.
func bitwiseWant(t *testing.T, tls *libc.TLS, ctx uintptr, a, b TJSValue, op int32) TJSValue {
	t.Helper()
	conv := func(v TJSValue) int32 {
		var d float64
		if v.Ftag == EJS_TAG_INT {
			d = float64(int32(*(*uint64)(unsafe.Pointer(&v.Fu))))
		} else {
			d = *(*float64)(unsafe.Pointer(&v.Fu))
		}
		want := specToInt32(d)
		p := tls.Alloc(4)
		defer tls.Free(4)
		if _JS_ToInt32Free(tls, ctx, p, v) != 0 {
			t.Fatalf("JS_ToInt32Free(%v) failed", d)
		}
		if got := *(*int32)(unsafe.Pointer(p)); got != want {
			t.Fatalf("JS_ToInt32Free(%v) = %d, the specification gives %d", d, got, want)
		}
		return want
	}
	x, y := conv(a), conv(b)
	n := uint32(y) % 32
	var v TJSValue
	var r int32
	switch op {
	case EOP_shl:
		r = int32(uint32(x) << n)
	case EOP_sar:
		r = x >> n
	case EOP_and:
		r = x & y
	case EOP_or:
		r = x | y
	case EOP_xor:
		r = x ^ y
	case EOP_shr:
		u := uint32(x) >> n
		if u > math.MaxInt32 {
			*(*float64)(unsafe.Pointer(&v.Fu)) = float64(u)
			v.Ftag = EJS_TAG_FLOAT64
			return v
		}
		r = int32(u)
	default:
		t.Fatalf("op %d", op)
	}
	*(*uint64)(unsafe.Pointer(&v.Fu)) = uint64(uint32(r))
	v.Ftag = EJS_TAG_INT
	return v
}

// specToInt32 is ToInt32 of ECMA-262 (7.1.6): truncate, take the value
// modulo 2^32, and map the upper half to negative numbers. NaN and the
// infinities give 0.
func specToInt32(d float64) int32 {
	if math.IsNaN(d) || math.IsInf(d, 0) {
		return 0
	}
	m := math.Mod(math.Trunc(d), 1<<32)
	if m < 0 {
		m += 1 << 32
	}
	return int32(uint32(m))
}
